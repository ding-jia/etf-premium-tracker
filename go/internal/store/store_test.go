package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"etf-premium-tracker/internal/model"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "premium.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestUpsertAndQuery(t *testing.T) {
	s := openTestStore(t)

	p1 := 3.5
	p2 := -0.2
	rows := []DailyRow{
		{Code: "513100", Date: "2024-01-08", Premium: &p1, Price: 1.2, IOPV: 1.15},
		{Code: "513100", Date: "2024-01-09", Premium: &p2, Price: 1.3, IOPV: 1.31},
		{Code: "159941", Date: "2024-01-08", Premium: nil, Price: 0, IOPV: 0},
	}
	if err := s.UpsertDaily(rows); err != nil {
		t.Fatalf("UpsertDaily: %v", err)
	}

	got, err := s.QueryDaily("513100")
	if err != nil {
		t.Fatalf("QueryDaily: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Date != "2024-01-08" || got[0].Premium == nil || *got[0].Premium != 3.5 {
		t.Fatalf("got[0] = %+v", got[0])
	}
	if got[1].Date != "2024-01-09" || got[1].Premium == nil || *got[1].Premium != -0.2 {
		t.Fatalf("got[1] = %+v", got[1])
	}

	got2, err := s.QueryDaily("159941")
	if err != nil {
		t.Fatalf("QueryDaily 159941: %v", err)
	}
	if len(got2) != 1 || got2[0].Premium != nil {
		t.Fatalf("null premium row = %+v", got2)
	}
}

func TestUpsertIdempotent(t *testing.T) {
	s := openTestStore(t)

	p := 1.0
	rows := []DailyRow{{Code: "513100", Date: "2024-01-08", Premium: &p, Price: 1, IOPV: 1}}
	if err := s.UpsertDaily(rows); err != nil {
		t.Fatal(err)
	}
	p2 := 2.0
	rows[0].Premium = &p2
	if err := s.UpsertDaily(rows); err != nil {
		t.Fatal(err)
	}

	got, err := s.QueryDaily("513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || *got[0].Premium != 2.0 {
		t.Fatalf("upsert should replace, got %+v", got)
	}
}

func TestQueryEmpty(t *testing.T) {
	s := openTestStore(t)
	got, err := s.QueryDaily("513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestJSONShape(t *testing.T) {
	// DailyPoint 必须序列化为 [date, premium]（前端 renderChart 依赖）。
	p := 1.5
	point := model.DailyPoint{Date: "2024-01-08", Premium: &p}
	data, err := json.Marshal(point)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `["2024-01-08",1.5]` {
		t.Fatalf("json = %s", data)
	}
}
