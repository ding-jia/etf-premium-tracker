// Package market 提供 A 股交易时间判断。
//
// 与 backend/main.py 保持一致：
//
//	上午 09:30-11:30（总分钟 [570, 690)），下午 13:00-15:00（总分钟 [780, 900)）
//	周末（周六、周日）休市。
package market

import "time"

// IsTrading 判断给定时刻是否处于 A 股交易时段。
func IsTrading(t time.Time) bool {
	if isWeekend(t) {
		return false
	}
	total := t.Hour()*60 + t.Minute()
	return (total >= 570 && total < 690) || (total >= 780 && total < 900)
}

// IsAfterClose 判断给定时刻是否已收盘（非周末且总分钟 >= 900，即 15:00 之后）。
//
// 用于每日快照保存时机：只在收盘后才允许落盘当日数据，
// 避免 Python 版在开盘前（如 08:00）就把盘前数据写入当日快照的问题。
func IsAfterClose(t time.Time) bool {
	if isWeekend(t) {
		return false
	}
	return t.Hour()*60+t.Minute() >= 900
}

func isWeekend(t time.Time) bool {
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return true
	}
	return false
}
