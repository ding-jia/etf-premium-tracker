package model

import (
	"encoding/json"
	"testing"
)

// TestHistoryPointRoundTrip 验证 HistoryPoint 与 JSON [ts, premium] 的双向转换。
func TestHistoryPointRoundTrip(t *testing.T) {
	premium := 10.55
	cases := []struct {
		name  string
		point HistoryPoint
		want  string
	}{
		{name: "with premium", point: HistoryPoint{TS: 1782361510, Premium: &premium}, want: "[1782361510,10.55]"},
		{name: "nil premium", point: HistoryPoint{TS: 1782361510, Premium: nil}, want: "[1782361510,null]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.point)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("Marshal = %s, want %s", got, tc.want)
			}

			var back HistoryPoint
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if back.TS != tc.point.TS {
				t.Fatalf("TS = %d, want %d", back.TS, tc.point.TS)
			}
			switch {
			case tc.point.Premium == nil && back.Premium != nil:
				t.Fatalf("Premium = %v, want nil", *back.Premium)
			case tc.point.Premium != nil && (back.Premium == nil || *back.Premium != *tc.point.Premium):
				t.Fatalf("Premium = %v, want %v", back.Premium, *tc.point.Premium)
			}
		})
	}
}

// TestDailyPointRoundTrip 验证 DailyPoint 与 JSON [date, premium] 的双向转换。
func TestDailyPointRoundTrip(t *testing.T) {
	premium := -1.25
	cases := []struct {
		name  string
		point DailyPoint
		want  string
	}{
		{name: "with premium", point: DailyPoint{Date: "2024-01-08", Premium: &premium}, want: `["2024-01-08",-1.25]`},
		{name: "nil premium", point: DailyPoint{Date: "2024-01-08", Premium: nil}, want: `["2024-01-08",null]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.point)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("Marshal = %s, want %s", got, tc.want)
			}

			var back DailyPoint
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if back.Date != tc.point.Date {
				t.Fatalf("Date = %q, want %q", back.Date, tc.point.Date)
			}
			switch {
			case tc.point.Premium == nil && back.Premium != nil:
				t.Fatalf("Premium = %v, want nil", *back.Premium)
			case tc.point.Premium != nil && (back.Premium == nil || *back.Premium != *tc.point.Premium):
				t.Fatalf("Premium = %v, want %v", back.Premium, *tc.point.Premium)
			}
		})
	}
}
