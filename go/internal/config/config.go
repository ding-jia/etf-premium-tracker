// Package config 集中管理服务运行所需的全部可调参数。
//
// 所有数据路径的默认值都是相对 CWD 的：仓库根运行、Windows 双击 exe
// （CWD = exe 所在目录）时都能直接生效；需要调整时用对应 flag 覆盖。
package config

import (
	"flag"
	"os"
	"path/filepath"
	"time"
)

// Config 持有服务运行所需的全部参数。
type Config struct {
	Addr          string        // HTTP 监听地址，如 ":8000"
	DataDir       string        // 运行数据目录，存放 history.json 与 premium.db
	WatchlistFile string        // 置顶 ETF 代码文件（每行一个 code）
	FeesFile      string        // ETF 费率 JSON 文件
	FrontendDir   string        // 前端静态文件目录
	PollInterval  time.Duration // 后台轮询上游数据的间隔
	FetchTimeout  time.Duration // 单次上游抓取的超时
}

// Parse 解析命令行 flag 并返回配置。
//
// flag 解析失败（非法值、未知参数）时立即返回描述性错误，由调用方决定退出方式。
func Parse() (*Config, error) {
	cfg := &Config{}
	fs := flag.NewFlagSet("etf-premium-tracker", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", ":8000", "HTTP 监听地址")
	fs.DurationVar(&cfg.PollInterval, "poll", 30*time.Minute, "后台轮询上游数据的间隔")
	fs.DurationVar(&cfg.FetchTimeout, "timeout", 10*time.Second, "单次上游抓取的超时")
	fs.StringVar(&cfg.DataDir, "data-dir", "backend/data", "运行数据目录（相对 CWD）")
	fs.StringVar(&cfg.WatchlistFile, "watchlist", "backend/watchlist.txt", "置顶 ETF 代码文件（相对 CWD）")
	fs.StringVar(&cfg.FeesFile, "fees", "backend/etf_fees.json", "ETF 费率 JSON 文件（相对 CWD）")
	fs.StringVar(&cfg.FrontendDir, "frontend", "frontend", "前端静态文件目录（相对 CWD）")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}
	return cfg, nil
}

// HistoryFile 返回日内历史 JSON 的完整路径。
func (c *Config) HistoryFile() string {
	return filepath.Join(c.DataDir, "history.json")
}

// DBFile 返回 SQLite 数据库的完整路径。
func (c *Config) DBFile() string {
	return filepath.Join(c.DataDir, "premium.db")
}
