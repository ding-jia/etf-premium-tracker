// Command server 是 ETF 溢价率监控的 Go 版服务入口。
//
// 用法：
//
//	go run ./cmd/server            # 默认配置，监听 :8000
//	go run ./cmd/server -poll 30m  # 调整轮询间隔
//
// 所有数据/文件路径默认相对 CWD，可用 -data-dir / -watchlist / -fees / -frontend 覆盖。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"etf-premium-tracker/internal/config"
	"etf-premium-tracker/internal/etfs"
	"etf-premium-tracker/internal/fees"
	"etf-premium-tracker/internal/history"
	"etf-premium-tracker/internal/server"
	"etf-premium-tracker/internal/store"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("配置解析失败: %v", err)
	}

	feesMap, err := fees.Load(cfg.FeesFile)
	if err != nil {
		log.Printf("警告: 费率文件加载失败（%v），费率列显示为 --", err)
		feesMap = map[string]fees.Info{}
	}

	hist := history.New(480)
	if err := hist.Load(cfg.HistoryFile()); err != nil {
		log.Printf("警告: 历史文件加载失败（%v），以空历史启动", err)
	}

	db, err := store.Open(cfg.DBFile())
	if err != nil {
		log.Fatalf("数据库打开失败: %v", err)
	}
	defer db.Close()
	if err := db.Init(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	srv := server.New(server.Options{
		Config:  cfg,
		Meta:    etfs.All,
		Fees:    feesMap,
		History: hist,
		DB:      db,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.RefreshOnce(ctx); err != nil {
		log.Printf("警告: 启动首次拉取失败: %v", err)
	} else {
		log.Printf("启动数据已就绪")
	}
	srv.Start(ctx)

	httpServer := &http.Server{
		Addr:    cfg.Addr,
		Handler: srv.Handler(),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP 关闭异常: %v", err)
		}
	}()

	log.Printf("ETF 溢价率监控已启动: http://%s", cfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP 服务异常退出: %v", err)
	}
	log.Printf("服务已退出")
}
