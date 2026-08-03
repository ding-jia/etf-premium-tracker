package watchlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToggleAddAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchlist.txt")

	codes, in, err := Toggle(path, "513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 1 || codes[0] != "513100" || !in {
		t.Fatalf("add: codes=%v in=%v", codes, in)
	}

	codes, in, err = Toggle(path, "159941")
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 2 || codes[1] != "159941" || !in {
		t.Fatalf("add second: codes=%v in=%v", codes, in)
	}

	codes, in, err = Toggle(path, "513100")
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 1 || in {
		t.Fatalf("remove: codes=%v in=%v", codes, in)
	}
	if codes[0] != "159941" {
		t.Fatalf("order should preserve: %v", codes)
	}
}

func TestReadSkipsEmptyLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchlist.txt")
	if err := os.WriteFile(path, []byte("513100\n\n159941\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	codes, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 2 || codes[0] != "513100" || codes[1] != "159941" {
		t.Fatalf("codes = %v", codes)
	}
}

func TestReadMissingFile(t *testing.T) {
	codes, err := Read(filepath.Join(t.TempDir(), "nope.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if codes != nil {
		t.Fatalf("want nil, got %v", codes)
	}
}
