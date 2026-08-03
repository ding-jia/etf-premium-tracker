// Package history 维护日内溢价率历史（内存 + history.json 持久化）。
//
// 文件格式与 Python 版 backend/data/history.json 兼容：
//
//	{"code": [[unix_ts, premium], ...]}
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"etf-premium-tracker/internal/model"
)

// Store 是带锁的日内历史容器。每只 ETF 最多保留 maxLen 条采样。
type Store struct {
	mu     sync.Mutex
	points map[string][]model.HistoryPoint
	maxLen int
}

// New 创建 Store，maxLen 为每只 ETF 最多保留的采样条数（对齐 Python 的 480）。
func New(maxLen int) *Store {
	return &Store{points: make(map[string][]model.HistoryPoint), maxLen: maxLen}
}

// Load 从 JSON 文件加载历史；文件不存在时视为空历史（返回 nil 错误）。
func (s *Store) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m := make(map[string][]model.HistoryPoint)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points = m
	return nil
}

// Save 将历史写入 JSON 文件（自动创建父目录）。
func (s *Store) Save(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.points)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// Append 追加一条采样，超出 maxLen 时丢弃最早的记录。
func (s *Store) Append(code string, ts int64, premium *float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.points[code]
	list = append(list, model.HistoryPoint{TS: ts, Premium: premium})
	if s.maxLen > 0 && len(list) > s.maxLen {
		list = list[len(list)-s.maxLen:]
	}
	s.points[code] = list
}

// Get 返回某只 ETF 的完整历史（调用方不得修改）。
func (s *Store) Get(code string) []model.HistoryPoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.points[code]
}
