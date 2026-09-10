package export

import (
	"path/filepath"
	"testing"

	"etf-premium-tracker/internal/dailyfile"
	"etf-premium-tracker/internal/store"
)

func newTestDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "premium.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Init(); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	return db
}

// 输出格式必须与 /api/daily/{code} 一致：{"code": [[date, premium], ...]}，
// 且没有数据的代码不写出空数组。
func TestDailySkipsCodesWithoutData(t *testing.T) {
	db := newTestDB(t)
	p1, p2 := 1.5, -2.25
	if err := db.UpsertDaily([]store.DailyRow{
		{Code: "513500", Date: "2026-05-19", Premium: &p1, Price: 1, IOPV: 1},
		{Code: "513500", Date: "2026-05-20", Premium: &p2, Price: 1, IOPV: 1},
	}); err != nil {
		t.Fatal(err)
	}

	daily, sum, err := Daily(db, []string{"513500", "513100"})
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if sum.Codes != 1 || sum.Points != 2 || sum.From != "2026-05-19" || sum.To != "2026-05-20" {
		t.Fatalf("summary = %+v", sum)
	}
	data, err := dailyfile.Marshal(daily)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"513500":[["2026-05-19",1.5],["2026-05-20",-2.25]]}`
	if string(data) != want {
		t.Fatalf("data = %s, want %s", data, want)
	}
}

// 溢价率为 NULL（IOPV 缺失）时导出为 null，与后端 API 语义一致。
func TestDailyNullPremium(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertDaily([]store.DailyRow{
		{Code: "513100", Date: "2026-05-19", Premium: nil, Price: 1, IOPV: 0},
	}); err != nil {
		t.Fatal(err)
	}
	daily, _, err := Daily(db, []string{"513100"})
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	data, err := dailyfile.Marshal(daily)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"513100":[["2026-05-19",null]]}`; string(data) != want {
		t.Fatalf("data = %s, want %s", data, want)
	}
}

// 空库导出空 map（服务器的 ExportDaily 据此跳过写入，避免清空线上数据）。
func TestDailyEmptyStore(t *testing.T) {
	daily, sum, err := Daily(newTestDB(t), []string{"513500"})
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(daily) != 0 || sum.Points != 0 {
		t.Fatalf("daily = %v, summary = %+v", daily, sum)
	}
}
