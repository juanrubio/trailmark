package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexingScopePersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"future_setting":"kept"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, configured := cfg.IndexingScope(); configured {
		t.Fatal("unset scope became configured")
	}
	for _, days := range []int{7, 30, 0} {
		if err := cfg.SetIndexingScope(days); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		got, ok := loaded.IndexingScope()
		if !ok || got != days {
			t.Fatalf("scope=%d configured=%v", got, ok)
		}
		raw, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(raw), `"future_setting": "kept"`) {
			t.Fatalf("unknown field lost: %s %v", raw, err)
		}
	}
	if err := cfg.SetIndexingScope(14); err == nil {
		t.Fatal("accepted invalid scope")
	}
	if err := os.WriteFile(path, []byte(`{"indexing_days":14}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("loaded invalid scope")
	}
}
func TestIndexingScopeSaveFailureRollsBack(t *testing.T) {
	cfg := &Config{}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRAILMARK_CONFIG_DIR", blocker)
	if err := cfg.SetIndexingScope(30); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if _, ok := cfg.IndexingScope(); ok {
		t.Fatal("failed save changed unset scope")
	}
	previous := 7
	cfg.IndexingDays = &previous
	if err := cfg.SetIndexingScope(30); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if got, ok := cfg.IndexingScope(); !ok || got != 7 {
		t.Fatalf("failed save changed scope: %d %v", got, ok)
	}
}
