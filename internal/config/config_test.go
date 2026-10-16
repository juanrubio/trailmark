package config_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSaveRoundTripKeepsDefaultsOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	cfg := &config.Config{
		Roots:      []config.Root{{Provider: model.ProviderCodex, Path: "/tmp/extra"}},
		Exclusions: []string{"/tmp/secret"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Roots) != 1 || loaded.Roots[0].Path != "/tmp/extra" {
		t.Errorf("configured roots after round trip = %+v", loaded.Roots)
	}
	if !loaded.Excluded("/tmp/secret/x.jsonl") {
		t.Error("exclusion lost in round trip")
	}
	found := false
	for _, r := range loaded.EffectiveRoots() {
		if r.Path == "/tmp/extra" {
			found = true
		}
	}
	if !found {
		t.Error("effective roots must include the configured root")
	}
}

func TestDisableDefaultRoots(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	body := `{"disable_default_roots":true,"roots":[{"provider":"codex","path":"/tmp/only"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	roots := cfg.EffectiveRoots()
	if len(roots) != 1 || roots[0].Path != "/tmp/only" {
		t.Errorf("effective roots = %+v, want only the configured root", roots)
	}
}

func TestEffectiveRootsHonorDisableFlagInSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	before := len(cfg.EffectiveRoots())
	cfg.DisableDefaultRoots = true
	if got := len(cfg.EffectiveRoots()); got != 0 {
		t.Errorf("effective roots after disabling defaults = %d (was %d), want 0 without reload", got, before)
	}
}

func TestThemeConfigPersistenceAndFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"unknown-future","disable_default_roots":true,"exclusions":["/fictional/private"],"custom":{"keep":42}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "green" {
		t.Fatal("unknown theme did not fall back to Green")
	}
	for _, theme := range []string{"blue", "amber", "monokai", "green"} {
		cfg.Theme = theme
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		reloaded, err := config.Load()
		if err != nil || reloaded.Theme != theme || len(reloaded.Exclusions) != 1 || !reloaded.DisableDefaultRoots {
			t.Fatalf("theme save lost configuration: %+v %v", reloaded, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(raw), `"keep": 42`) {
			t.Fatal("theme save lost unknown fields")
		}
	}
}
