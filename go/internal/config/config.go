// Package config 集中管理服务运行所需的全部可调参数。
//
// 相对路径默认值（data-dir / watchlist / fees / frontend）按仓库根解析：
// 从 CWD 向上探测含 go/go.mod 的目录，保证 start.sh 与 `cd go && go run ./cmd/server`
// 行为一致，不再依赖 CWD 所在层级（修复旧版从 go/ 目录运行会写入 go/backend/data
// 与 backend/data 分叉的问题）。探测失败时回退为相对 CWD。
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config 持有服务运行所需的全部参数。
type Config struct {
	Addr           string        // HTTP 监听地址，如 ":8000"
	DataDir        string        // 运行数据目录，存放 history.json 与 premium.db
	WatchlistFile  string        // 置顶 ETF 代码文件（每行一个 code）
	FeesFile       string        // ETF 费率 JSON 文件
	FrontendDir    string        // 前端静态文件目录
	PagesDailyFile string        // 在线版静态日线数据输出路径（空字符串表示不导出）
	PollInterval   time.Duration // 后台轮询上游数据的间隔
	FetchTimeout   time.Duration // 单次上游抓取的超时
}

// Parse 解析命令行 flag 并返回配置。
//
// flag 解析失败（非法值、未知参数）时立即返回描述性错误，由调用方决定退出方式。
// 未显式指定的相对路径默认值按仓库根解析（见 RepoRoot），显式传入的路径保持原样
// （相对 CWD，遵循 Go 惯例）。
func Parse() (*Config, error) {
	cfg := &Config{}
	fs := flag.NewFlagSet("etf-premium-tracker", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", ":8000", "HTTP 监听地址")
	fs.DurationVar(&cfg.PollInterval, "poll", 30*time.Minute, "后台轮询上游数据的间隔")
	fs.DurationVar(&cfg.FetchTimeout, "timeout", 10*time.Second, "单次上游抓取的超时")
	fs.StringVar(&cfg.DataDir, "data-dir", "backend/data", "运行数据目录（默认相对仓库根）")
	fs.StringVar(&cfg.WatchlistFile, "watchlist", "backend/watchlist.txt", "置顶 ETF 代码文件（默认相对仓库根）")
	fs.StringVar(&cfg.FeesFile, "fees", "backend/etf_fees.json", "ETF 费率 JSON 文件（默认相对仓库根）")
	fs.StringVar(&cfg.FrontendDir, "frontend", "frontend", "前端静态文件目录（默认相对仓库根）")
	fs.StringVar(&cfg.PagesDailyFile, "pages-daily", "pages/data/daily.json", "在线版静态日线数据输出路径（空字符串表示不导出）")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	// 时长参数必须为正：PollInterval <= 0 会让 time.NewTicker 在
	// server.Start 的后台 goroutine 里 panic（无 recover，进程直接退出）；
	// FetchTimeout <= 0 在 http.Client 里表示"永不超时"，上游卡死会长期占住刷新锁。
	if cfg.PollInterval <= 0 {
		return nil, fmt.Errorf("-poll 必须大于 0，实际为 %s", cfg.PollInterval)
	}
	if cfg.FetchTimeout <= 0 {
		return nil, fmt.Errorf("-timeout 必须大于 0，实际为 %s", cfg.FetchTimeout)
	}

	// 仅对"用户未显式覆盖"的路径默认值按仓库根解析。
	if root := RepoRoot(""); root != "" {
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if !set["data-dir"] {
			cfg.DataDir = filepath.Join(root, cfg.DataDir)
		}
		if !set["watchlist"] {
			cfg.WatchlistFile = filepath.Join(root, cfg.WatchlistFile)
		}
		if !set["fees"] {
			cfg.FeesFile = filepath.Join(root, cfg.FeesFile)
		}
		if !set["frontend"] {
			cfg.FrontendDir = filepath.Join(root, cfg.FrontendDir)
		}
		if !set["pages-daily"] && cfg.PagesDailyFile != "" {
			cfg.PagesDailyFile = filepath.Join(root, cfg.PagesDailyFile)
		}
	}
	return cfg, nil
}

// RepoRoot 从 startDir（"" 表示 CWD）向上探测仓库根：第一个含 go/go.mod 的目录。
// 找不到时返回 ""，调用方回退为相对 CWD。
func RepoRoot(startDir string) string {
	if startDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		startDir = wd
	}
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// HistoryFile 返回日内历史 JSON 的完整路径。
func (c *Config) HistoryFile() string {
	return filepath.Join(c.DataDir, "history.json")
}

// DBFile 返回 SQLite 数据库的完整路径。
func (c *Config) DBFile() string {
	return filepath.Join(c.DataDir, "premium.db")
}
