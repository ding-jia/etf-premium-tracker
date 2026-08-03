package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"etf-premium-tracker/internal/config"
	"etf-premium-tracker/internal/etfs"
	"etf-premium-tracker/internal/fees"
	"etf-premium-tracker/internal/history"
	"etf-premium-tracker/internal/model"
	"etf-premium-tracker/internal/store"
)

// fakeFetcher 返回固定的两条行情，模拟腾讯接口。
type fakeFetcher struct{}

func (fakeFetcher) FetchQuotes(context.Context) ([]model.ETF, error) {
	price := 1.234
	iopv := 1.2
	premium := 2.8
	return []model.ETF{
		{Code: "513100", Name: "纳指ETF国泰", Category: "nasdaq", Manager: "国泰基金", Exchange: "SH",
			Price: &price, IOPV: &iopv, NAV: &price, Premium: &premium,
			ChangePct: &price, Volume: &price, Amount: &price, PrevClose: &price, FundScale: &price},
		{Code: "513500", Name: "标普500ETF博时", Category: "sp500", Manager: "博时基金", Exchange: "SH",
			Price: &price, IOPV: &iopv, NAV: &price, Premium: &premium,
			ChangePct: &price, Volume: &price, Amount: &price, PrevClose: &price, FundScale: &price},
	}, nil
}

// failFetcher 模拟上游不可达。
type failFetcher struct{}

func (failFetcher) FetchQuotes(context.Context) ([]model.ETF, error) {
	return nil, errEmptyData
}

func newTestServer(t *testing.T, fetcher Fetcher) *Server {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Addr:          ":0",
		DataDir:       dir,
		WatchlistFile: filepath.Join(dir, "watchlist.txt"),
		FeesFile:      filepath.Join(dir, "etf_fees.json"),
		FrontendDir:   filepath.Join(t.TempDir(), "frontend"),
		PollInterval:  time.Hour, // 测试不触发后台轮询
		FetchTimeout:  time.Second,
	}
	hist := history.New(480)
	db, err := store.Open(filepath.Join(dir, "premium.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Init(); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	return New(Options{
		Config:  cfg,
		Meta:    etfs.All,
		Fees:    map[string]fees.Info{},
		History: hist,
		DB:      db,
		Fetcher: fetcher,
	})
}

func doRequest(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestETFsEndpoint(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	if err := srv.RefreshOnce(context.Background()); err != nil {
		t.Fatalf("RefreshOnce: %v", err)
	}
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/etfs")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp model.EtfsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Nasdaq) != 1 || resp.Nasdaq[0].Code != "513100" {
		t.Fatalf("nasdaq = %+v", resp.Nasdaq)
	}
	if len(resp.Sp500) != 1 || resp.Sp500[0].Code != "513500" {
		t.Fatalf("sp500 = %+v", resp.Sp500)
	}
	if resp.TotalCount != 2 {
		t.Fatalf("total_count = %d, want 2", resp.TotalCount)
	}
	if resp.MarketStatus != "open" && resp.MarketStatus != "closed" {
		t.Fatalf("market_status = %q", resp.MarketStatus)
	}
	if resp.UpdateTime == "" {
		t.Fatal("update_time should not be empty")
	}
}

func TestRefreshSuccess(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	rec := doRequest(t, srv.Handler(), http.MethodPost, "/api/refresh")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp model.EtfsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.TotalCount != 2 {
		t.Fatalf("total_count = %d, want 2", resp.TotalCount)
	}
}

func TestRefreshBadGateway(t *testing.T) {
	srv := newTestServer(t, failFetcher{})
	rec := doRequest(t, srv.Handler(), http.MethodPost, "/api/refresh")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var resp model.ErrorDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Detail != "数据获取失败，上游不可达" {
		t.Fatalf("detail = %q", resp.Detail)
	}
}

func TestWatchlistToggle(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})

	rec := doRequest(t, srv.Handler(), http.MethodPost, "/api/watchlist/toggle/513100")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp model.ToggleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.InWatchlist || len(resp.Codes) != 1 || resp.Codes[0] != "513100" {
		t.Fatalf("toggle add: %+v", resp)
	}

	rec = doRequest(t, srv.Handler(), http.MethodPost, "/api/watchlist/toggle/513100")
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.InWatchlist || len(resp.Codes) != 0 {
		t.Fatalf("toggle remove: %+v", resp)
	}
}

func TestWatchlistGet(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	rec := doRequest(t, srv.Handler(), http.MethodPost, "/api/watchlist/toggle/159941")
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle status = %d", rec.Code)
	}
	rec = doRequest(t, srv.Handler(), http.MethodGet, "/api/watchlist")
	var resp model.WatchlistResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Codes) != 1 || resp.Codes[0] != "159941" {
		t.Fatalf("codes = %v", resp.Codes)
	}
}

func TestDailyEndpoint(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	p := 2.5
	if err := srv.db.UpsertDaily([]store.DailyRow{
		{Code: "513100", Date: "2024-01-08", Premium: &p, Price: 1, IOPV: 1},
	}); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/daily/513100")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp model.DailyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Daily) != 1 || resp.Daily[0].Date != "2024-01-08" ||
		resp.Daily[0].Premium == nil || *resp.Daily[0].Premium != 2.5 {
		t.Fatalf("daily = %+v", resp.Daily)
	}
	// 前端依赖的 JSON 形状 [date, premium]
	if !strings.Contains(rec.Body.String(), `["2024-01-08",2.5]`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHistoryEndpoint(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	p := 1.5
	srv.hist.Append("513100", 100, &p)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/history/513100")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp model.HistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.History) != 1 || resp.History[0].TS != 100 ||
		resp.History[0].Premium == nil || *resp.History[0].Premium != 1.5 {
		t.Fatalf("history = %+v", resp.History)
	}
	// 未知代码 → 空数组而非 null
	rec = doRequest(t, srv.Handler(), http.MethodGet, "/api/history/999999")
	if !strings.Contains(rec.Body.String(), `"history":[]`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestFeesEndpoint(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/fees")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStaticFallback(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	cfg := srv.cfg
	if err := os.MkdirAll(cfg.FrontendDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FrontendDir, "index.html"), []byte("<html>hi</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FrontendDir, "style.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, srv.Handler(), http.MethodGet, "/style.css")
	if rec.Code != http.StatusOK || rec.Body.String() != "body{}" {
		t.Fatalf("style.css: code=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, srv.Handler(), http.MethodGet, "/some/unknown/path")
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>hi</html>" {
		t.Fatalf("fallback: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCORSPreflight(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})
	rec := doRequest(t, srv.Handler(), http.MethodOptions, "/api/etfs")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("CORS header = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestDailySnapshotOnlyAfterClose 验证每日快照只在收盘后保存、每交易日一次：
// 开盘前不保存；收盘后保存；同日重复刷新不重复保存；次日收盘再保存新一天。
func TestDailySnapshotOnlyAfterClose(t *testing.T) {
	srv := newTestServer(t, fakeFetcher{})

	// 周一 08:00 开盘前 → 不保存
	srv.now = func() time.Time { return time.Date(2024, 1, 8, 8, 0, 0, 0, time.Local) }
	if err := srv.RefreshOnce(context.Background()); err != nil {
		t.Fatalf("RefreshOnce morning: %v", err)
	}
	got, err := srv.db.QueryDaily("513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("snapshot must not be saved before close, got %+v", got)
	}

	// 周一 15:30 收盘后 → 保存
	srv.now = func() time.Time { return time.Date(2024, 1, 8, 15, 30, 0, 0, time.Local) }
	if err := srv.RefreshOnce(context.Background()); err != nil {
		t.Fatalf("RefreshOnce close: %v", err)
	}
	got, err = srv.db.QueryDaily("513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Date != "2024-01-08" {
		t.Fatalf("snapshot = %+v, want one row for 2024-01-08", got)
	}

	// 同日再次刷新 → 不重复保存
	if err := srv.RefreshOnce(context.Background()); err != nil {
		t.Fatalf("RefreshOnce same day: %v", err)
	}
	got, err = srv.db.QueryDaily("513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("snapshot duplicated: %+v", got)
	}

	// 周二 15:30 → 保存新一天
	srv.now = func() time.Time { return time.Date(2024, 1, 9, 15, 30, 0, 0, time.Local) }
	if err := srv.RefreshOnce(context.Background()); err != nil {
		t.Fatalf("RefreshOnce next day: %v", err)
	}
	got, err = srv.db.QueryDaily("513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Date != "2024-01-09" {
		t.Fatalf("second day snapshot missing: %+v", got)
	}
}
