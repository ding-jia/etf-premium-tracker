// Package fees 加载并缓存 ETF 费率数据（backend/etf_fees.json）。
package fees

import (
	"encoding/json"
	"os"
)

// Info 是 etf_fees.json 中单只 ETF 的费率条目。
//
// JSON 字段名带 _fee 后缀，与 /api/etfs 返回的 {mgmt, custodian, total} 形状不同，
// 解析时不可混用。
type Info struct {
	MgmtFee      float64 `json:"mgmt_fee"`
	CustodianFee float64 `json:"custodian_fee"`
	TotalFee     float64 `json:"total_fee"`
}

// Load 读取费率 JSON 文件，返回 code → Info 映射。
// 文件不存在或解析失败时返回非 nil 错误。
func Load(path string) (map[string]Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := make(map[string]Info)
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
