// Package export 把 SQLite 每日快照导出成在线版（GitHub Pages）直接可用的静态 JSON。
//
// 在线版是纯静态站、拿不到后端的 SQLite，导出后它就能画出与本地版同源的
// 真实溢价率曲线，不必再回落到腾讯 K 线的收盘价。
//
// 文件格式与 /api/daily/{code} 的响应体一致（每个点都是 [date, premium] 二元组），
// 两端可以共用同一套解析逻辑：
//
//	{"513500": [["2026-05-19",1.78], ...], "513100": [...]}
package export

import (
	"encoding/json"
	"os"
	"path/filepath"

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

// Daily 按 codes 给出的顺序读取每日快照并序列化为紧凑 JSON。
//
// 没有任何数据的代码会被跳过（不写空数组），保证文件只包含有效历史。
// encoding/json 对 map 按键排序，因此相同数据永远产出相同字节，便于 git diff。
func Daily(db *store.Store, codes []string) ([]byte, Summary, error) {
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
	data, err := json.Marshal(out)
	if err != nil {
		return nil, Summary{}, err
	}
	return data, sum, nil
}

// WriteFile 以"临时文件 + rename"原子写入，自动创建父目录。
//
// 原子替换是必需的：导出文件会被 git 提交并部署到线上，
// 半截文件会让在线版图表读到损坏的 JSON。
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".daily-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后此调用无害
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp 默认 0600；导出文件是要提交并对外提供的静态资源，放宽到 0644。
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
