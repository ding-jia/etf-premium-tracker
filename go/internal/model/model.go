// Package model 定义与前端契约严格一致的 DTO。
package model

import (
	"encoding/json"
	"fmt"
)

// Fee 是 ETF 的费率结构，对应 JSON 对象 {"mgmt","custodian","total"}。
//
// etf_fees.json 缺表时三个字段为 null（nil 指针）。
type Fee struct {
	Mgmt      *float64 `json:"mgmt"`
	Custodian *float64 `json:"custodian"`
	Total     *float64 `json:"total"`
}

// ETF 是单只 ETF 的全部展示字段，对应 /api/etfs 返回的每一项。
//
// 行情字段用 *float64 表达 null 语义（上游缺失时为 null）；
// 静态元数据字段（code/name/category/manager/exchange）恒非空。
type ETF struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Category  string   `json:"category"`
	Manager   string   `json:"manager"`
	Exchange  string   `json:"exchange"`
	Price     *float64 `json:"price"`
	IOPV      *float64 `json:"iopv"`
	NAV       *float64 `json:"nav"`
	Premium   *float64 `json:"premium"`
	ChangePct *float64 `json:"change_pct"`
	Volume    *float64 `json:"volume"`
	Amount    *float64 `json:"amount"`
	PrevClose *float64 `json:"prev_close"`
	Fee       Fee      `json:"fee"`
	FundScale *float64 `json:"fund_scale"`
}

// HistoryPoint 是日内历史的一个采样点，JSON 序列化为二元数组 [ts, premium]。
//
// 现有 backend/data/history.json 的格式是 {"code": [[ts, premium], ...]}，
// 因此必须自定义 MarshalJSON / UnmarshalJSON 才能读写该文件。
type HistoryPoint struct {
	TS      int64    // 采样 Unix 时间戳（秒），保持整数
	Premium *float64 // 溢价率，IOPV 缺失时为 null
}

// MarshalJSON 将 HistoryPoint 序列化为 [ts, premium]。
func (p HistoryPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{p.TS, p.Premium})
}

// UnmarshalJSON 从 [ts, premium] 还原 HistoryPoint。
func (p *HistoryPoint) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("HistoryPoint 必须是 [ts, premium] 二元组，实际长度 %d", len(raw))
	}
	if err := json.Unmarshal(raw[0], &p.TS); err != nil {
		return fmt.Errorf("HistoryPoint ts 必须是整数: %w", err)
	}
	if err := json.Unmarshal(raw[1], &p.Premium); err != nil {
		return fmt.Errorf("HistoryPoint premium 必须是数字或 null: %w", err)
	}
	return nil
}

// DailyPoint 是每日快照的一个点，JSON 序列化为二元数组 [date, premium]。
//
// date 是 "YYYY-MM-DD" 字符串（与 /api/daily/{code} 的响应一致）。
type DailyPoint struct {
	Date    string   // 交易日，格式 "YYYY-MM-DD"
	Premium *float64 // 收盘溢价率，缺失时为 null
}

// MarshalJSON 将 DailyPoint 序列化为 [date, premium]。
func (p DailyPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{p.Date, p.Premium})
}

// UnmarshalJSON 从 [date, premium] 还原 DailyPoint。
func (p *DailyPoint) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("DailyPoint 必须是 [date, premium] 二元组，实际长度 %d", len(raw))
	}
	if err := json.Unmarshal(raw[0], &p.Date); err != nil {
		return fmt.Errorf("DailyPoint date 必须是字符串: %w", err)
	}
	if err := json.Unmarshal(raw[1], &p.Premium); err != nil {
		return fmt.Errorf("DailyPoint premium 必须是数字或 null: %w", err)
	}
	return nil
}

// EtfsResponse 是 /api/etfs 的完整响应。
type EtfsResponse struct {
	Nasdaq       []ETF  `json:"nasdaq"`
	Sp500        []ETF  `json:"sp500"`
	MarketStatus string `json:"market_status"` // "open" | "closed"
	UpdateTime   string `json:"update_time"`   // "YYYY-MM-DD HH:MM:SS"（本地时区）
	TotalCount   int    `json:"total_count"`
}

// WatchlistResponse 是 /api/watchlist 的响应。
type WatchlistResponse struct {
	Codes []string `json:"codes"`
}

// ToggleResponse 是 /api/watchlist/toggle/{code} 的响应。
type ToggleResponse struct {
	Codes       []string `json:"codes"`
	InWatchlist bool     `json:"in_watchlist"`
}

// HistoryResponse 是 /api/history/{code} 的响应。
type HistoryResponse struct {
	Code    string         `json:"code"`
	History []HistoryPoint `json:"history"`
}

// DailyResponse 是 /api/daily/{code} 的响应。
type DailyResponse struct {
	Code  string       `json:"code"`
	Daily []DailyPoint `json:"daily"`
}

// ErrorDetail 是错误响应体，如 {"detail": "..."}。
type ErrorDetail struct {
	Detail string `json:"detail"`
}
