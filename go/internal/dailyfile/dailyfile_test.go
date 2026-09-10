package dailyfile

import (
	"os"
	"path/filepath"
	"testing"
)

// Upsert 必须幂等：同一天反复写入只替换值，不重复追加。
func TestUpsertReplacesSameDate(t *testing.T) {
	pts := Upsert(nil, "2026-09-10", 2.5)
	if len(pts) != 1 || pts[0].Date != "2026-09-10" || *pts[0].Premium != 2.5 {
		t.Fatalf("首次写入 = %+v", pts)
	}
	pts = Upsert(pts, "2026-09-10", 3.75)
	if len(pts) != 1 || *pts[0].Premium != 3.75 {
		t.Fatalf("同日替换后 = %+v", pts)
	}
	pts = Upsert(pts, "2026-09-11", 1.25)
	if len(pts) != 2 || pts[1].Date != "2026-09-11" {
		t.Fatalf("追加新日期后 = %+v", pts)
	}
}

// Load 对不存在的文件返回空 map（首次运行场景）。
func TestLoadMissingFileIsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

// WriteFile 必须创建父目录、可重复覆盖，且不残留临时文件。
func TestWriteFileCreatesDirAndDoesNotLeaveTemp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "daily.json")

	if err := WriteFile(path, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != `{"a":1}` {
		t.Fatalf("content = %s, err = %v", got, err)
	}
	if err := WriteFile(path, []byte(`{"b":2}`)); err != nil {
		t.Fatalf("WriteFile 覆盖: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != `{"b":2}` {
		t.Fatalf("覆盖后 content = %s", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("目录里残留了临时文件: %v", entries)
	}
}

// Load → Upsert → Marshal → WriteFile → Load 的完整往返，并确认是紧凑格式。
func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daily.json")
	daily, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	daily["513500"] = Upsert(daily["513500"], "2026-09-10", 8.96)
	data, err := Marshal(daily)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"513500":[["2026-09-10",8.96]]}`; string(data) != want {
		t.Fatalf("data = %s, want %s", data, want)
	}
	if err := WriteFile(path, data); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back["513500"]) != 1 || *back["513500"][0].Premium != 8.96 {
		t.Fatalf("往返后 = %+v", back)
	}
}
