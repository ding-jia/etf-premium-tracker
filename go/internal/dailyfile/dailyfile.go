// Package dailyfile 负责每日溢价率数据文件（pages/data/daily.json）的读写。
//
// 文件格式与 /api/daily/{code} 的响应体一致，每个点是 [date, premium] 二元组：
//
//	{"513500": [["2026-05-19",1.78], ...], "513100": [...]}
//
// 三处共用这份格式：internal/export（从 SQLite 生成）、服务器的定时重写、
// 以及 GitHub Actions 里的 cmd/snapshot（直接抓行情追加）。
// 单独成包是为了让 cmd/snapshot 不必拖入 SQLite 依赖（纯 Go SQLite 编译很重）。
package dailyfile

import (
	"encoding/json"
	"os"
	"path/filepath"

	"etf-premium-tracker/internal/model"
)

// Load 读取已导出的日线数据；文件不存在时返回空 map 而不是错误
// （首次运行时仓库里还没有该文件）。
func Load(path string) (map[string][]model.DailyPoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]model.DailyPoint{}, nil
		}
		return nil, err
	}
	out := map[string][]model.DailyPoint{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Marshal 序列化为紧凑 JSON。encoding/json 对 map 按键排序，
// 因此相同数据永远产出相同字节，便于 git diff。
func Marshal(daily map[string][]model.DailyPoint) ([]byte, error) {
	return json.Marshal(daily)
}

// Upsert 写入某个交易日的溢价率：同日已有则替换，否则追加到末尾。
//
// 每日序列天然按日期升序，而任何一次抓取只可能写"今天"这一个日期，
// 因此"已存在就替换、否则追加"即可保持有序，反复运行也是幂等的。
func Upsert(points []model.DailyPoint, date string, premium float64) []model.DailyPoint {
	p := premium
	for i := range points {
		if points[i].Date == date {
			points[i].Premium = &p
			return points
		}
	}
	return append(points, model.DailyPoint{Date: date, Premium: &p})
}

// WriteFile 以"临时文件 + rename"原子写入，自动创建父目录。
//
// 原子替换是必需的：数据文件会被 git 提交并部署到线上，
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
	// CreateTemp 默认 0600；数据文件是要提交并对外提供的静态资源，放宽到 0644。
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
