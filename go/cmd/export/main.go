// Command export 把 SQLite 每日快照导出成在线版（pages/）使用的静态 JSON。
//
// 用法：
//
//	cd go && go run ./cmd/export               # backend/data/premium.db → pages/data/daily.json
//	go run ./cmd/export -out /tmp/daily.json   # 指定输出路径
//
// 在线版是纯静态站、读不到后端数据库；把导出结果提交到仓库（会自动部署），
// 在线版图表就能画出与本地版同源的真实溢价率曲线。
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"etf-premium-tracker/internal/config"
	"etf-premium-tracker/internal/dailyfile"
	"etf-premium-tracker/internal/etfs"
	"etf-premium-tracker/internal/export"
	"etf-premium-tracker/internal/store"
)

func main() {
	dataDir := flag.String("data-dir", "backend/data", "运行数据目录（默认相对仓库根）")
	out := flag.String("out", "pages/data/daily.json", "输出文件路径（默认相对仓库根）")
	flag.Parse()

	// 仅对未显式指定的默认路径按仓库根解析，与 config.Parse 的行为保持一致。
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if root := config.RepoRoot(""); root != "" {
		if !explicit["data-dir"] {
			*dataDir = filepath.Join(root, *dataDir)
		}
		if !explicit["out"] {
			*out = filepath.Join(root, *out)
		}
	}

	dbPath := filepath.Join(*dataDir, "premium.db")
	if _, err := os.Stat(dbPath); err != nil {
		log.Fatalf("找不到数据库 %s（用 -data-dir 指定运行数据目录）", dbPath)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	codes := make([]string, 0, len(etfs.All))
	for _, item := range etfs.All {
		codes = append(codes, item.Code)
	}

	daily, sum, err := export.Daily(db, codes)
	if err != nil {
		log.Fatalf("导出失败: %v", err)
	}
	if sum.Points == 0 {
		log.Fatalf("%s 里没有每日快照，未写出文件（避免把在线版数据清空）", dbPath)
	}
	data, err := dailyfile.Marshal(daily)
	if err != nil {
		log.Fatalf("序列化失败: %v", err)
	}
	if err := dailyfile.WriteFile(*out, data); err != nil {
		log.Fatalf("写入 %s 失败: %v", *out, err)
	}
	log.Printf("已导出 %s：%d 只 / %d 点 / %s ~ %s / %d 字节",
		*out, sum.Codes, sum.Points, sum.From, sum.To, len(data))
}
