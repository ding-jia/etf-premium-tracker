package config

import (
	"os"
	"path/filepath"
	"testing"
)

// makeFakeRepo 构造 root/go/go.mod 的假仓库结构并切换 CWD 到 go/ 下。
func makeFakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go", "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(filepath.Join(root, "go")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	return root
}

func TestRepoRoot(t *testing.T) {
	root := t.TempDir()
	goDir := filepath.Join(root, "go")
	if err := os.MkdirAll(goDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		start string
		want  string
	}{
		{"从 go/ 目录", goDir, root},
		{"从仓库根", root, root},
		{"从深层目录", filepath.Join(goDir, "internal", "server"), root},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RepoRoot(tc.start); got != root {
				t.Fatalf("RepoRoot(%q) = %q, want %q", tc.start, got, root)
			}
		})
	}
}

func TestRepoRootNotFound(t *testing.T) {
	dir := t.TempDir()
	if got := RepoRoot(dir); got != "" {
		t.Fatalf("RepoRoot = %q, want empty", got)
	}
}

func TestParseDefaultsResolveToRepoRoot(t *testing.T) {
	root := makeFakeRepo(t)
	os.Args = []string{"server"}
	t.Cleanup(func() { os.Args = nil })

	cfg, err := Parse()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(root, "backend", "data") {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, filepath.Join(root, "backend", "data"))
	}
	if cfg.WatchlistFile != filepath.Join(root, "backend", "watchlist.txt") {
		t.Fatalf("WatchlistFile = %q", cfg.WatchlistFile)
	}
	if cfg.FeesFile != filepath.Join(root, "backend", "etf_fees.json") {
		t.Fatalf("FeesFile = %q", cfg.FeesFile)
	}
	if cfg.FrontendDir != filepath.Join(root, "frontend") {
		t.Fatalf("FrontendDir = %q", cfg.FrontendDir)
	}
}

func TestParseExplicitFlagKeepsValue(t *testing.T) {
	root := makeFakeRepo(t)
	os.Args = []string{"server", "-data-dir", "./mydata"}
	t.Cleanup(func() { os.Args = nil })

	cfg, err := Parse()
	if err != nil {
		t.Fatal(err)
	}
	// 显式传入的路径保持原样（相对 CWD）
	if cfg.DataDir != "./mydata" {
		t.Fatalf("DataDir = %q, want ./mydata", cfg.DataDir)
	}
	// 未覆盖的默认值仍按仓库根解析
	if cfg.FrontendDir != filepath.Join(root, "frontend") {
		t.Fatalf("FrontendDir = %q, want %q", cfg.FrontendDir, filepath.Join(root, "frontend"))
	}
}
