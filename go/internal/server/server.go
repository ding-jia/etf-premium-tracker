// Package server 组装 HTTP 服务：行情缓存、后台轮询与全部 API 路由。
//
// 端点与 Python 版 backend/main.py 完全对齐，前端无需改动。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"etf-premium-tracker/internal/config"
	"etf-premium-tracker/internal/dailyfile"
	"etf-premium-tracker/internal/export"
	"etf-premium-tracker/internal/fees"
	"etf-premium-tracker/internal/history"
	"etf-premium-tracker/internal/market"
	"etf-premium-tracker/internal/model"
	"etf-premium-tracker/internal/quote"
	"etf-premium-tracker/internal/store"
	"etf-premium-tracker/internal/watchlist"
)

// errEmptyData 表示上游返回空数据，无法更新缓存。
var errEmptyData = errors.New("上游返回空数据")

// Fetcher 抓取并解析全部 ETF 行情，可注入用于测试。
type Fetcher interface {
	FetchQuotes(ctx context.Context) ([]model.ETF, error)
}

// quoteFetcher 是默认实现：走腾讯财经接口。
type quoteFetcher struct {
	meta    []model.ETF
	fees    map[string]fees.Info
	timeout time.Duration
}

func (f quoteFetcher) FetchQuotes(ctx context.Context) ([]model.ETF, error) {
	raw, err := quote.Fetch(ctx, quote.BuildURL(f.meta), f.timeout)
	if err != nil {
		return nil, err
	}
	data := quote.Parse(raw, f.meta, f.fees)
	if len(data) == 0 {
		return nil, errEmptyData
	}
	// 部分成功按成功处理（否则上游一次抖动就会让整个面板清空），
	// 但必须留下痕迹：这份残缺数据会整体覆盖内存缓存，收盘后还会写进当日快照。
	if len(data) < len(f.meta) {
		log.Printf("警告: 上游仅返回 %d/%d 只 ETF，缓存已被部分数据覆盖", len(data), len(f.meta))
	}
	return data, nil
}

// Options 是 Server 的构造参数。
type Options struct {
	Config  *config.Config
	Meta    []model.ETF
	Fees    map[string]fees.Info
	History *history.Store
	DB      *store.Store
	Fetcher Fetcher          // 可选，默认 quoteFetcher
	Now     func() time.Time // 可选，默认 time.Now（测试注入）
}

// Server 持有行情缓存、后台轮询与全部 HTTP handler。
type Server struct {
	cfg     *config.Config
	meta    []model.ETF
	fees    map[string]fees.Info
	hist    *history.Store
	db      *store.Store
	fetcher Fetcher

	mu            sync.RWMutex // 保护 resp
	resp          model.EtfsResponse
	refreshMu     sync.Mutex // 轮询与 /api/refresh 互斥，避免并发抓取
	lastDailySave string     // 上次保存每日快照的日期，保证每交易日只落盘一次
	now           func() time.Time
}

// New 创建 Server。
func New(opts Options) *Server {
	if opts.Fetcher == nil {
		opts.Fetcher = quoteFetcher{meta: opts.Meta, fees: opts.Fees, timeout: opts.Config.FetchTimeout}
	}
	if opts.Now == nil {
		// 默认基于中国标准时间（UTC+8），不依赖系统时区。
		opts.Now = func() time.Time { return time.Now().In(market.ShanghaiTZ) }
	}
	return &Server{
		cfg:     opts.Config,
		meta:    opts.Meta,
		fees:    opts.Fees,
		hist:    opts.History,
		db:      opts.DB,
		fetcher: opts.Fetcher,
		now:     opts.Now,
		resp: model.EtfsResponse{
			Nasdaq:       []model.ETF{},
			Sp500:        []model.ETF{},
			MarketStatus: "closed",
		},
	}
}

// RefreshOnce 抓取一次行情并更新缓存；失败时缓存保持不变。
func (s *Server) RefreshOnce(ctx context.Context) error {
	data, err := s.fetcher.FetchQuotes(ctx)
	if err != nil {
		return err
	}

	now := s.now()
	nowTS := now.Unix()
	for _, item := range data {
		s.hist.Append(item.Code, nowTS, item.Premium)
	}
	if err := s.hist.Save(s.cfg.HistoryFile()); err != nil {
		log.Printf("警告: history 保存失败: %v", err)
	}

	isTrading := market.IsTrading(now)
	today := now.Format("2006-01-02")
	// 收盘后（15:00 之后、非周末）且当日尚未保存 → 落盘每日快照。
	// 与 Python 版差异：Python 在"非交易时段"（含开盘前）都会保存，这里收紧为收盘后。
	if !isTrading && market.IsAfterClose(now) && s.lastDailySave != today {
		if s.saveDailySnapshot(data, today) {
			s.lastDailySave = today
			// 快照落盘后同步刷新在线版使用的静态日线数据
			if err := s.ExportDaily(); err != nil {
				log.Printf("警告: 导出在线版日线数据失败: %v", err)
			}
		}
	}

	status := "closed"
	if isTrading {
		status = "open"
	}
	nasdaq, sp500 := splitByCategory(data)

	s.mu.Lock()
	s.resp = model.EtfsResponse{
		Nasdaq:       nasdaq,
		Sp500:        sp500,
		MarketStatus: status,
		UpdateTime:   now.Format("2006-01-02 15:04:05"),
		TotalCount:   len(data),
	}
	s.mu.Unlock()
	log.Printf("行情已更新: %d 只 ETF（%s）", len(data), status)
	return nil
}

// refresh 带互斥，供后台轮询与 /api/refresh 共用。
func (s *Server) refresh(ctx context.Context) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	return s.RefreshOnce(ctx)
}

// saveDailySnapshot 收盘后保存当日快照，返回是否成功。
func (s *Server) saveDailySnapshot(data []model.ETF, today string) bool {
	hasIOPV := false
	for _, item := range data {
		if item.IOPV != nil {
			hasIOPV = true
			break
		}
	}
	if !hasIOPV {
		return false
	}
	rows := make([]store.DailyRow, 0, len(data))
	for _, item := range data {
		price, iopv := 0.0, 0.0
		if item.Price != nil {
			price = *item.Price
		}
		if item.IOPV != nil {
			iopv = *item.IOPV
		}
		rows = append(rows, store.DailyRow{
			Code:    item.Code,
			Date:    today,
			Premium: item.Premium,
			Price:   price,
			IOPV:    iopv,
		})
	}
	if err := s.db.UpsertDaily(rows); err != nil {
		log.Printf("警告: 每日快照保存失败: %v", err)
		return false
	}
	log.Printf("已保存 %s 每日快照（%d 只）", today, len(rows))
	return true
}

// ExportDaily 重新生成在线版（GitHub Pages）使用的静态日线数据。
//
// 在线版是纯静态站、读不到 SQLite；把导出文件提交到仓库并部署后，
// 它就能画出与本地版同源的真实溢价率曲线，而不是回落到收盘价。
// 配置为空字符串时不做任何事；数据库还没有快照时跳过写入，避免把已提交的数据清空。
func (s *Server) ExportDaily() error {
	if s.cfg.PagesDailyFile == "" {
		return nil
	}
	codes := make([]string, 0, len(s.meta))
	for _, item := range s.meta {
		codes = append(codes, item.Code)
	}
	daily, sum, err := export.Daily(s.db, codes)
	if err != nil {
		return err
	}
	if sum.Points == 0 {
		log.Printf("跳过导出 %s：数据库里还没有每日快照", s.cfg.PagesDailyFile)
		return nil
	}
	data, err := dailyfile.Marshal(daily)
	if err != nil {
		return err
	}
	if err := dailyfile.WriteFile(s.cfg.PagesDailyFile, data); err != nil {
		return err
	}
	log.Printf("已导出在线版日线数据 %s（%d 只 / %d 点 / %s ~ %s / %d 字节）",
		s.cfg.PagesDailyFile, sum.Codes, sum.Points, sum.From, sum.To, len(data))
	return nil
}

func splitByCategory(data []model.ETF) (nasdaq, sp500 []model.ETF) {
	for _, item := range data {
		switch item.Category {
		case "nasdaq":
			nasdaq = append(nasdaq, item)
		case "sp500":
			sp500 = append(sp500, item)
		}
	}
	if nasdaq == nil {
		nasdaq = []model.ETF{}
	}
	if sp500 == nil {
		sp500 = []model.ETF{}
	}
	return nasdaq, sp500
}

// Start 启动后台轮询 goroutine，直到 ctx 取消。
func (s *Server) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.cfg.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("后台轮询已停止")
				return
			case <-ticker.C:
				if err := s.refresh(ctx); err != nil {
					log.Printf("警告: 轮询失败: %v", err)
				}
			}
		}
	}()
}

// Handler 返回带 CORS 的完整路由（Go 1.22+ 方法 + 路径模式）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/etfs", s.handleETFs)
	mux.HandleFunc("GET /api/watchlist", s.handleWatchlist)
	mux.HandleFunc("POST /api/watchlist/toggle/{code}", s.handleToggleWatchlist)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("GET /api/fees", s.handleFees)
	mux.HandleFunc("GET /api/history/{code}", s.handleHistory)
	mux.HandleFunc("GET /api/daily/{code}", s.handleDaily)
	mux.HandleFunc("/", s.serveStatic)
	return corsMiddleware(mux)
}

func (s *Server) handleETFs(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	resp := s.resp
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWatchlist(w http.ResponseWriter, _ *http.Request) {
	codes, err := watchlist.Read(s.cfg.WatchlistFile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorDetail{Detail: "读取 watchlist 失败"})
		return
	}
	if codes == nil {
		codes = []string{} // 对齐 Python：空列表而非 null
	}
	writeJSON(w, http.StatusOK, model.WatchlistResponse{Codes: codes})
}

func (s *Server) handleToggleWatchlist(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !validCode(code) {
		writeJSON(w, http.StatusBadRequest, model.ErrorDetail{Detail: "无效的 ETF 代码"})
		return
	}
	codes, in, err := watchlist.Toggle(s.cfg.WatchlistFile, code)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorDetail{Detail: "写入 watchlist 失败"})
		return
	}
	writeJSON(w, http.StatusOK, model.ToggleResponse{Codes: codes, InWatchlist: in})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if err := s.refresh(r.Context()); err != nil {
		writeJSON(w, http.StatusBadGateway, model.ErrorDetail{Detail: "数据获取失败，上游不可达"})
		return
	}
	s.mu.RLock()
	resp := s.resp
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleFees(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.fees)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	history := s.hist.Get(code)
	if history == nil {
		history = []model.HistoryPoint{}
	}
	writeJSON(w, http.StatusOK, model.HistoryResponse{Code: code, History: history})
}

func (s *Server) handleDaily(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	daily, err := s.db.QueryDaily(code)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorDetail{Detail: "查询每日数据失败"})
		return
	}
	if daily == nil {
		daily = []model.DailyPoint{}
	}
	writeJSON(w, http.StatusOK, model.DailyResponse{Code: code, Daily: daily})
}

// serveStatic 提供前端静态文件，缺失时回退 index.html（对齐 Python 行为）。
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	frontendDir := filepath.Clean(s.cfg.FrontendDir)
	target := filepath.Join(frontendDir, filepath.Clean(r.URL.Path))
	if target != frontendDir && !strings.HasPrefix(target, frontendDir+string(os.PathSeparator)) {
		writeJSON(w, http.StatusNotFound, model.ErrorDetail{Detail: "not found"})
		return
	}
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}
	index := filepath.Join(frontendDir, "index.html")
	if _, err := os.Stat(index); err == nil {
		http.ServeFile(w, r, index)
		return
	}
	writeJSON(w, http.StatusNotFound, model.ErrorDetail{Detail: "frontend not found"})
}

// validCode 校验 6 位数字 ETF 代码（对齐全部静态元数据格式）。
func validCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("警告: 响应编码失败: %v", err)
	}
}
