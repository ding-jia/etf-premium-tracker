package market

import (
	"testing"
	"time"
)

// 2024-01-08 是周一，2024-01-06/07 是周末。
func TestIsTrading(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"周一 09:29 开盘前", time.Date(2024, 1, 8, 9, 29, 0, 0, time.Local), false},
		{"周一 09:30 开盘", time.Date(2024, 1, 8, 9, 30, 0, 0, time.Local), true},
		{"周一 11:29 上午收盘前", time.Date(2024, 1, 8, 11, 29, 0, 0, time.Local), true},
		{"周一 11:30 上午休市", time.Date(2024, 1, 8, 11, 30, 0, 0, time.Local), false},
		{"周一 12:59 午休", time.Date(2024, 1, 8, 12, 59, 0, 0, time.Local), false},
		{"周一 13:00 下午开盘", time.Date(2024, 1, 8, 13, 0, 0, 0, time.Local), true},
		{"周一 14:59 收盘前", time.Date(2024, 1, 8, 14, 59, 0, 0, time.Local), true},
		{"周一 15:00 收盘", time.Date(2024, 1, 8, 15, 0, 0, 0, time.Local), false},
		{"周六 10:00", time.Date(2024, 1, 6, 10, 0, 0, 0, time.Local), false},
		{"周日 10:00", time.Date(2024, 1, 7, 10, 0, 0, 0, time.Local), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTrading(tc.t); got != tc.want {
				t.Fatalf("IsTrading(%v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestIsAfterClose(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"周一 08:00 盘前", time.Date(2024, 1, 8, 8, 0, 0, 0, time.Local), false},
		{"周一 14:59 收盘前", time.Date(2024, 1, 8, 14, 59, 0, 0, time.Local), false},
		{"周一 15:00 整", time.Date(2024, 1, 8, 15, 0, 0, 0, time.Local), true},
		{"周一 16:30 盘后", time.Date(2024, 1, 8, 16, 30, 0, 0, time.Local), true},
		{"周六 16:00", time.Date(2024, 1, 6, 16, 0, 0, 0, time.Local), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAfterClose(tc.t); got != tc.want {
				t.Fatalf("IsAfterClose(%v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}
