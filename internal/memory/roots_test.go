package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestRootsFromConfigDerivation(t *testing.T) {
	base := t.TempDir()
	claudeProjects := filepath.Join(base, "claudehome", "projects")
	codexSessions := filepath.Join(base, "codexhome", "sessions")
	codexMemories := filepath.Join(base, "codexhome", "memories")
	autoDir := filepath.Join(base, "auto-memory")
	extra := filepath.Join(base, "extra-memory")
	for _, d := range []string{claudeProjects, codexSessions, codexMemories, autoDir, extra} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(base, "claudehome"))
	t.Setenv("CODEX_HOME", filepath.Join(base, "codexhome"))
	settings := `{"autoMemoryDirectory": "` + autoDir + `", "someOtherKey": {"nested": true}, "token": "fictional"}`
	if err := os.WriteFile(filepath.Join(base, "claudehome", "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readAutoMemoryDirectory(); got != autoDir {
		t.Errorf("autoMemoryDirectory = %q, want %q", got, autoDir)
	}
	cfg := &config.Config{
		DisableDefaultRoots: true,
		Roots: []config.Root{
			{Provider: model.ProviderClaudeCode, Path: claudeProjects},
			{Provider: model.ProviderCodex, Path: codexSessions},
		},
		MemoryRoots: []config.Root{
			{Provider: model.ProviderCodex, Path: extra},
			{Provider: model.ProviderClaudeCode, Path: filepath.Join(base, "missing")},
		},
	}
	roots := RootsFromConfig(cfg)
	seen := map[string]bool{}
	for _, r := range roots {
		seen[string(r.Provider)+"|"+r.Path] = true
	}
	for _, want := range []Root{
		{Provider: model.ProviderClaudeCode, Path: canonical(claudeProjects)},
		{Provider: model.ProviderCodex, Path: canonical(codexMemories)}, // sibling of sessions + CODEX_HOME dedupe
		{Provider: model.ProviderClaudeCode, Path: canonical(autoDir)},
		{Provider: model.ProviderCodex, Path: canonical(extra)},
	} {
		if !seen[string(want.Provider)+"|"+want.Path] {
			t.Errorf("missing derived root %+v in %+v", want, roots)
		}
	}
	if len(roots) != 4 {
		t.Errorf("roots = %+v, want 4 unique existing", roots)
	}
}

func TestReadAutoMemoryDirectoryAbsent(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if got := readAutoMemoryDirectory(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
