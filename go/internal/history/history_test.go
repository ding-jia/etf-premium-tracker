package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendTruncates(t *testing.T) {
	s := New(3)
	for i := int64(1); i <= 5; i++ {
		p := float64(i)
		s.Append("513100", i, &p)
	}
	got := s.Get("513100")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].TS != 3 || got[1].TS != 4 || got[2].TS != 5 {
		t.Fatalf("TS = [%d %d %d], want [3 4 5]", got[0].TS, got[1].TS, got[2].TS)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "history.json")

	s := New(480)
	p := 2.5
	s.Append("159941", 100, &p)
	s.Append("159941", 101, nil)
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2 := New(480)
	if err := s2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s2.Get("159941")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].TS != 100 || got[0].Premium == nil || *got[0].Premium != 2.5 {
		t.Fatalf("got[0] = %+v", got[0])
	}
	if got[1].TS != 101 || got[1].Premium != nil {
		t.Fatalf("got[1] = %+v", got[1])
	}

	// 文件格式必须与 Python 版一致：{"code": [[ts, premium], ...]}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string][]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	points, ok := raw["159941"]
	if !ok || len(points) != 2 {
		t.Fatalf("raw shape wrong: %s", data)
	}
	if string(points[0]) != "[100,2.5]" {
		t.Fatalf("first point = %s", points[0])
	}
	if string(points[1]) != "[101,null]" {
		t.Fatalf("second point = %s", points[1])
	}
}

func TestLoadMissingFile(t *testing.T) {
	s := New(480)
	if err := s.Load(filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if got := s.Get("513100"); len(got) != 0 {
		t.Fatalf("want empty history, got %d points", len(got))
	}
}
