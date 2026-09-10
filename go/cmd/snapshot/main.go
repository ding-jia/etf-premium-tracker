// Command snapshot 抓一次腾讯行情，把当日溢价率写进在线版使用的静态日线文件。
//
// 它是给 GitHub Actions 定时任务用的：静态站自己写不回仓库，所以在 runner 上
// 跑这个命令，提交后由 Actions 部署，在线版的数据就能无人值守地更新。
//
// 用法：
//
//	cd go && go run ./cmd/snapshot            # 收盘后写入当日溢价率
//	go run ./cmd/snapshot -dry-run            # 只抓取并打印，不写文件（验证连通性）
//	go run ./cmd/snapshot -force              # 未到收盘也写入（会写入盘中值）
//
// 反复运行是幂等的：同一天只会替换当天的值，不会产生重复点。
package main

import (
	"context"
	"flag"
	"log"
	"path/filepath"
	"time"

	"etf-premium-tracker/internal/config"
	"etf-premium-tracker/internal/dailyfile"
	"etf-premium-tracker/internal/etfs"
	"etf-premium-tracker/internal/market"
	"etf-premium-tracker/internal/quote"
)

func main() {
	dataFile := flag.String("data", "pages/data/daily.json", "日线数据文件路径（默认相对仓库根）")
	timeout := flag.Duration("timeout", 10*time.Second, "单次上游抓取超时")
	dryRun := flag.Bool("dry-run", false, "只抓取并打印，不写入文件")
	force := flag.Bool("force", false, "未到收盘时间也写入（写入的是盘中值）")
	flag.Parse()

	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if root := config.RepoRoot(""); root != "" && !explicit["data"] {
		*dataFile = filepath.Join(root, *dataFile)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	raw, err := quote.Fetch(ctx, quote.BuildURL(etfs.All), *timeout)
	if err != nil {
		log.Fatalf("抓取行情失败: %v", err)
	}
	quotes := quote.Parse(raw, etfs.All, nil)
	if len(quotes) == 0 {
		log.Fatalf("上游没有返回任何可用行情")
	}
	log.Printf("已获取 %d/%d 只 ETF 行情", len(quotes), len(etfs.All))

	now := time.Now().In(market.ShanghaiTZ)
	today := now.Format("2006-01-02")
	fresh := 0
	for _, item := range quotes {
		if item.Premium != nil {
			fresh++
		}
	}
	log.Printf("其中 %d 只有溢价率，快照日期 %s（%s，%s）",
		fresh, today, now.Format("15:04:05"), statusText(now))

	if *dryRun {
		log.Printf("dry-run：不写入 %s", *dataFile)
		return
	}
	if !*force && !market.IsAfterClose(now) {
		log.Printf("未到收盘时间（15:00 之后才写收盘值），跳过写入；如需强制写入盘中值请加 -force")
		return
	}

	daily, err := dailyfile.Load(*dataFile)
	if err != nil {
		log.Fatalf("读取 %s 失败: %v", *dataFile, err)
	}
	written := 0
	for _, item := range quotes {
		if item.Premium == nil {
			continue // IOPV 缺失，宁可不写也不写 null
		}
		daily[item.Code] = dailyfile.Upsert(daily[item.Code], today, *item.Premium)
		written++
	}
	if written == 0 {
		log.Fatalf("没有任何可写入的溢价率，保持文件不变")
	}
	data, err := dailyfile.Marshal(daily)
	if err != nil {
		log.Fatalf("序列化失败: %v", err)
	}
	if err := dailyfile.WriteFile(*dataFile, data); err != nil {
		log.Fatalf("写入 %s 失败: %v", *dataFile, err)
	}
	total := 0
	for _, points := range daily {
		total += len(points)
	}
	log.Printf("已更新 %s：本次写入 %d 只，累计 %d 只 / %d 点 / %d 字节",
		*dataFile, written, len(daily), total, len(data))
}

func statusText(now time.Time) string {
	if market.IsTrading(now) {
		return "交易中"
	}
	if market.IsAfterClose(now) {
		return "已收盘"
	}
	return "非交易时段"
}
