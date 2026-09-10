// Package etfs 提供全部跟踪 ETF 的静态元数据表。
//
// 数据逐条转录自 backend/main.py 的 ETFS 列表（共 17 只：
// 13 只 nasdaq + 4 只 sp500），不增删、不"纠错"，与 Python 版保持一致。
package etfs

import "etf-premium-tracker/internal/model"

// All 是全部 ETF 的静态元数据，行情字段保持零值（序列化为 null）。
var All = []model.ETF{
	{Code: "513100", Name: "纳指ETF国泰", Category: "nasdaq", Manager: "国泰基金", Exchange: "SH"},
	{Code: "159941", Name: "纳指ETF广发", Category: "nasdaq", Manager: "广发基金", Exchange: "SZ"},
	{Code: "513300", Name: "纳斯达克ETF华夏", Category: "nasdaq", Manager: "华夏基金", Exchange: "SH"},
	{Code: "159632", Name: "纳斯达克ETF华安", Category: "nasdaq", Manager: "华安基金", Exchange: "SZ"},
	{Code: "513110", Name: "纳指ETF华泰柏瑞", Category: "nasdaq", Manager: "华泰柏瑞基金", Exchange: "SH"},
	{Code: "159696", Name: "纳指ETF易方达", Category: "nasdaq", Manager: "易方达基金", Exchange: "SZ"},
	{Code: "159501", Name: "纳指ETF嘉实", Category: "nasdaq", Manager: "嘉实基金", Exchange: "SZ"},
	{Code: "159513", Name: "纳斯达克100ETF大成", Category: "nasdaq", Manager: "大成基金", Exchange: "SZ"},
	{Code: "159659", Name: "纳斯达克100ETF招商", Category: "nasdaq", Manager: "招商基金", Exchange: "SZ"},
	{Code: "159660", Name: "纳指ETF汇添富", Category: "nasdaq", Manager: "汇添富基金", Exchange: "SZ"},
	{Code: "513390", Name: "纳指100ETF博时", Category: "nasdaq", Manager: "博时基金", Exchange: "SH"},
	{Code: "513870", Name: "纳指ETF富国", Category: "nasdaq", Manager: "富国基金", Exchange: "SH"},
	{Code: "159509", Name: "纳指科技ETF景顺", Category: "nasdaq", Manager: "景顺长城基金", Exchange: "SZ"},
	{Code: "513500", Name: "标普500ETF博时", Category: "sp500", Manager: "博时基金", Exchange: "SH"},
	{Code: "159655", Name: "标普500ETF华夏", Category: "sp500", Manager: "华夏基金", Exchange: "SZ"},
	{Code: "159612", Name: "标普500ETF国泰", Category: "sp500", Manager: "国泰基金", Exchange: "SZ"},
	{Code: "513650", Name: "标普500ETF南方", Category: "sp500", Manager: "南方基金", Exchange: "SH"},
}
