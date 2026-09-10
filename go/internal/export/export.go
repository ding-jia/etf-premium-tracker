// Package export 把 SQLite 每日快照转换成在线版（GitHub Pages）可用的数据。
//
// 在线版是纯静态站、拿不到后端的 SQLite，导出后它就能画出与本地版同源的
// 真实溢价率曲线，而不是回落到腾讯 K 线的收盘价。
//
// 本包只负责"从数据库取数据"；文件格式、原子写入与幂等合并见 internal/dailyfile。
package export

import (
	"etf-premium-tracker/internal/model"
	"etf-premium-tracker/internal/store"
)

// Summary 描述一次导出的规模，用于日志与命令行输出。
type Summary struct {
	Codes  int    // 有数据的代码数
	Points int    // 数据点总数
	From   string // 最早交易日
	To     string // 最晚交易日
}

// Daily 按 codes 给出的顺序读取每日快照，返回 code → 序列 的映射。
//
// 没有任何数据的代码会被跳过（不写空数组），保证结果只包含有效历史。
// 调用方负责用 dailyfile.Marshal / dailyfile.WriteFile 落盘。
func Daily(db *store.Store, codes []string) (map[string][]model.DailyPoint, Summary, error) {
	out := make(map[string][]model.DailyPoint, len(codes))
	var sum Summary
	for _, code := range codes {
		points, err := db.QueryDaily(code)
		if err != nil {
			return nil, Summary{}, err
		}
		if len(points) == 0 {
			continue
		}
		out[code] = points
		sum.Codes++
		sum.Points += len(points)
		if first := points[0].Date; sum.From == "" || first < sum.From {
			sum.From = first
		}
		if last := points[len(points)-1].Date; last > sum.To {
			sum.To = last
		}
	}
	return out, sum, nil
}
