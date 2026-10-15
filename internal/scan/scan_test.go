package scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/adapter/claudecode"
	"github.com/juanrubio/trailmark/internal/adapter/codex"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
	"github.com/juanrubio/trailmark/internal/scan"
)

const claudeLog = `{"type":"user","sessionId":"s-1","timestamp":"2026-10-14T09:00:00Z","cwd":"/work/p","uuid":"u-1","message":{"role":"user","content":"Synthetic scan task one"}}
{"type":"assistant","sessionId":"s-1","timestamp":"2026-10-14T09:01:00Z","cwd":"/work/p","uuid":"a-1","message":{"role":"assistant","content":"Done with scan task one"}}
`

func setupRoots(t *testing.T) (claudeRoot, codexRoot, logPath string) {
	t.Helper()
	base := t.TempDir()
	claudeRoot = filepath.Join(base, "claude-projects", "proj-a")
	codexRoot = filepath.Join(base, "codex-sessions", "2026", "10", "14")
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(codexRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath = filepath.Join(claudeRoot, "s-1.jsonl")
	if err := os.WriteFile(logPath, []byte(claudeLog), 0o600); err != nil {
		t.Fatal(err)
	}
	codexBody, err := os.ReadFile(filepath.Join("..", "adapter", "codex", "testdata", "rollout-basic.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexRoot, "rollout-1.jsonl"), codexBody, 0o600); err != nil {
		t.Fatal(err)
	}
	return claudeRoot, filepath.Dir(filepath.Dir(filepath.Dir(codexRoot))), logPath
}

func newScanner(ix *index.Index, cfg *config.Config) *scan.Scanner {
	return &scan.Scanner{
		Index: ix,
		Adapters: map[model.Provider]adapter.Adapter{
			model.ProviderClaudeCode: claudecode.Adapter{},
			model.ProviderCodex:      codex.Adapter{},
		},
		Config: cfg,
	}
}

func TestScanIndexesBothProviders(t *testing.T) {
	ctx := context.Background()
	claudeRoot, codexRoot, _ := setupRoots(t)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{
		{Provider: model.ProviderClaudeCode, Path: filepath.Dir(claudeRoot)},
		{Provider: model.ProviderCodex, Path: codexRoot},
	}}
	stats, err := newScanner(ix, cfg).Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Indexed != 2 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want 2 indexed", stats)
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.ByProvider[model.ProviderClaudeCode] != 1 || counts.ByProvider[model.ProviderCodex] != 1 {
		t.Errorf("byProvider = %v", counts.ByProvider)
	}
}

func TestScanIsIncrementalAndDedupesOverlappingRoots(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, _ := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{
		{Provider: model.ProviderClaudeCode, Path: parent},
		{Provider: model.ProviderClaudeCode, Path: claudeRoot}, // overlaps parent
		{Provider: model.ProviderClaudeCode, Path: parent},     // exact duplicate
	}}
	sc := newScanner(ix, cfg)
	first, err := sc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Indexed != 1 || first.Discovered != 1 {
		t.Errorf("first = %+v, want 1 discovered and indexed once", first)
	}
	second, err := sc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Indexed != 0 || second.Skipped != 1 {
		t.Errorf("second = %+v, want unchanged file skipped", second)
	}
}

func TestScanExclusionRemovesDerivedData(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, logPath := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: parent}}}
	sc := newScanner(ix, cfg)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.Exclusions = []string{claudeRoot}
	stats, err := sc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Excluded != 1 {
		t.Errorf("stats = %+v, want 1 excluded", stats)
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Sessions != 0 {
		t.Errorf("sessions = %d after exclusion, want 0", counts.Sessions)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("source log must remain on disk: %v", err)
	}
}

func TestScanExcludesByRecordedCWD(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, _ := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	// Fixture session records cwd /work/p; excluding that project
	// must keep its transcript out of the index entirely.
	cfg := &config.Config{
		Roots:      []config.Root{{Provider: model.ProviderClaudeCode, Path: parent}},
		Exclusions: []string{"/work/p"},
	}
	stats, err := newScanner(ix, cfg).Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Excluded != 1 {
		t.Errorf("stats = %+v, want 1 excluded by cwd", stats)
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Sessions != 0 {
		t.Errorf("sessions = %d, want 0 for cwd-excluded project", counts.Sessions)
	}
	hits, err := ix.SearchContent(ctx, "Synthetic scan task", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("FTS hits = %d, want 0 for cwd-excluded project", len(hits))
	}
}

func TestScanCWDExclusionRemovesPreviouslyIndexed(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, _ := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: parent}}}
	sc := newScanner(ix, cfg)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.Exclusions = []string{"/work/p"}
	stats, err := sc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Excluded != 1 {
		t.Errorf("stats = %+v, want 1 excluded after config change", stats)
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Sessions != 0 {
		t.Errorf("sessions = %d, want previously indexed rows removed", counts.Sessions)
	}
}

func TestScanFollowsSymlinkedRoot(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, _ := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	linkParent := filepath.Join(t.TempDir(), "linked-projects")
	if err := os.Symlink(parent, linkParent); err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: linkParent}}}
	stats, err := newScanner(ix, cfg).Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Indexed != 1 {
		t.Errorf("stats = %+v, want 1 indexed via symlinked root", stats)
	}
}

func TestScanMissingRootDoesNotHideOthers(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, _ := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{
		{Provider: model.ProviderClaudeCode, Path: filepath.Join(t.TempDir(), "does-not-exist")},
		{Provider: model.ProviderClaudeCode, Path: parent},
	}}
	stats, err := newScanner(ix, cfg).Scan(ctx)
	if err != nil {
		t.Fatalf("missing root must not fail the whole scan: %v", err)
	}
	if stats.Indexed != 1 {
		t.Errorf("stats = %+v, want the valid root indexed", stats)
	}
	if len(stats.FailedRoots) != 1 {
		t.Errorf("FailedRoots = %v, want the missing root reported", stats.FailedRoots)
	}
}

func TestScanRootRemovalPrunesDerivedData(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, _ := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: parent}}}
	sc := newScanner(ix, cfg)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.Roots = nil
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Sessions != 0 {
		t.Errorf("sessions = %d after root removal, want 0", counts.Sessions)
	}
}

func TestScanPrunesConfirmedDeletion(t *testing.T) {
	ctx := context.Background()
	claudeRoot, _, logPath := setupRoots(t)
	parent := filepath.Dir(claudeRoot)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: parent}}}
	sc := newScanner(ix, cfg)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	stats, err := sc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Pruned != 1 {
		t.Errorf("stats = %+v, want 1 pruned", stats)
	}
}
