package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestMemoryRootsLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	cfg := &config.Config{
		MemoryRoots: []config.Root{{Provider: model.ProviderCodex, Path: "/tmp/mem-extra"}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"memory_roots"`) {
		t.Errorf("saved config lacks memory_roots: %s", raw)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.MemoryRoots) != 1 || loaded.MemoryRoots[0].Path != "/tmp/mem-extra" {
		t.Errorf("memory roots after round trip = %+v", loaded.MemoryRoots)
	}
	// Session roots stay separate from memory roots.
	if len(loaded.Roots) != 0 {
		t.Errorf("memory root leaked into session roots: %+v", loaded.Roots)
	}
}

func TestMemoryRootsValidation(t *testing.T) {
	for _, body := range []string{
		`{"memory_roots":[{"provider":"nope","path":"/tmp/x"}]}`,
		`{"memory_roots":[{"provider":"codex","path":""}]}`,
	} {
		dir := t.TempDir()
		t.Setenv("TRAILMARK_CONFIG_DIR", dir)
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := config.Load(); err == nil {
			t.Errorf("Load(%s) must fail validation", body)
		}
	}
}

func TestMemoryRootHelpers(t *testing.T) {
	cfg := &config.Config{}
	r := config.Root{Provider: model.ProviderClaudeCode, Path: "/tmp/mem"}
	if !cfg.AddMemoryRoot(r) || cfg.AddMemoryRoot(r) {
		t.Error("AddMemoryRoot must add once and dedupe")
	}
	if !cfg.IsConfiguredMemoryRoot(model.ProviderClaudeCode, "/tmp/mem") {
		t.Error("IsConfiguredMemoryRoot false after add")
	}
	if cfg.IsConfiguredRoot(model.ProviderClaudeCode, "/tmp/mem") {
		t.Error("memory root must not count as a session root")
	}
	if !cfg.RemoveMemoryRoot(model.ProviderClaudeCode, "/tmp/mem") || cfg.RemoveMemoryRoot(model.ProviderClaudeCode, "/tmp/mem") {
		t.Error("RemoveMemoryRoot must remove once")
	}
	if len(cfg.MemoryRoots) != 0 {
		t.Errorf("memory roots = %+v", cfg.MemoryRoots)
	}
}
