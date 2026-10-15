package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestExcludedMatchesPathBoundary(t *testing.T) {
	cfg := &config.Config{Exclusions: []string{"/work/private"}}
	if !cfg.Excluded("/work/private/repo/session.jsonl") {
		t.Error("nested path should be excluded")
	}
	if cfg.Excluded("/work/private-other/session.jsonl") {
		t.Error("sibling with shared prefix must not be excluded")
	}
	if cfg.Excluded("/work/public/session.jsonl") {
		t.Error("unrelated path must not be excluded")
	}
}

func TestEffectiveRootsDedupes(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Roots: []config.Root{
		{Provider: model.ProviderCodex, Path: dir},
		{Provider: model.ProviderCodex, Path: dir + string(filepath.Separator)},
	}}
	roots := cfg.EffectiveRoots()
	count := 0
	for _, r := range roots {
		if config.Canonical(r.Path) == config.Canonical(dir) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("canonical root appears %d times, want 1", count)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("TRAILMARK_CONFIG_DIR", filepath.Join(t.TempDir(), "cfg"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("nil config")
	}
}

func TestLoadParsesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	body := `{"roots":[{"provider":"codex","path":"/tmp/extra"}],"exclusions":["/tmp/secret"]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Roots) == 0 || cfg.Roots[0].Path != "/tmp/extra" || cfg.Roots[0].Provider != model.ProviderCodex {
		t.Errorf("configured root must come first, roots = %+v", cfg.Roots)
	}
	if !cfg.Excluded("/tmp/secret/x.jsonl") {
		t.Error("exclusion from file not applied")
	}
}
