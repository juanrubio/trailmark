package scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/adapter/claudecode"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
	"github.com/juanrubio/trailmark/internal/scan"
)

type countingAdapter struct {
	claudecode.Adapter
	calls *int
}

func (a countingAdapter) ParseFile(ctx context.Context, path string) (*model.Session, error) {
	*a.calls++
	return a.Adapter.ParseFile(ctx, path)
}

func TestIndexingScopeUsesMtimeBoundaryAndExpands(t *testing.T) {
	now := time.Date(2026, 12, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		age      time.Duration
		deferred bool
	}{
		{"old", 31 * 24 * time.Hour, true}, {"boundary", 30 * 24 * time.Hour, false}, {"recent modification with old events", time.Hour, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _, path := setupRoots(t)
			stamp := now.Add(-tc.age)
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer ix.Close()
			days := 30
			cfg := &config.Config{IndexingDays: &days, DisableDefaultRoots: true, Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: root}}}
			sc := newScanner(ix, cfg)
			sc.Now = func() time.Time { return now }
			calls := 0
			sc.Adapters = map[model.Provider]adapter.Adapter{model.ProviderClaudeCode: countingAdapter{calls: &calls}}
			stats, err := sc.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if tc.deferred {
				if stats.Deferred != 1 || calls != 0 {
					t.Fatalf("stats=%+v calls=%d", stats, calls)
				}
			} else if stats.Indexed != 1 || calls != 1 {
				t.Fatalf("stats=%+v calls=%d", stats, calls)
			}
			days = 0
			stats, err = sc.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			rows, err := ix.ListSessions(context.Background(), 100)
			if err != nil || len(rows) != 1 || stats.Deferred != 0 {
				t.Fatalf("rows=%v stats=%+v err=%v", rows, stats, err)
			}
		})
	}
}

func TestNarrowScopeKeepsCachedFilesFreshAndHonorsDeletionExclusion(t *testing.T) {
	root, _, path := setupRoots(t)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	cfg := &config.Config{DisableDefaultRoots: true, Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: root}}}
	sc := newScanner(ix, cfg)
	ctx := context.Background()
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	days := 7
	cfg.IndexingDays = &days
	old := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	stats, err := sc.Scan(ctx)
	if err != nil || stats.Indexed != 1 || stats.Deferred != 0 {
		t.Fatalf("cached refresh stats=%+v err=%v", stats, err)
	}
	cfg.Exclusions = []string{"/work/p"}
	stats, err = sc.Scan(ctx)
	if err != nil || stats.Excluded != 1 {
		t.Fatalf("exclusion stats=%+v err=%v", stats, err)
	}
	cfg.Exclusions = nil
	days = 0
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	days = 7
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	stats, err = sc.Scan(ctx)
	if err != nil || stats.Pruned != 1 {
		t.Fatalf("deletion stats=%+v err=%v", stats, err)
	}
}

func TestScopeIsSnapshottedBeforeTraversal(t *testing.T) {
	t.Setenv("TRAILMARK_CONFIG_DIR", t.TempDir())
	root, _, path := setupRoots(t)
	now := time.Now()
	old := now.Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	days := 7
	cfg := &config.Config{IndexingDays: &days, DisableDefaultRoots: true, Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: root}}}
	sc := newScanner(ix, cfg)
	sc.Now = func() time.Time { return now }
	changed := false
	sc.Observer = func(p scan.Progress) {
		if !changed && p.Phase == "indexing" {
			changed = true
			if err := cfg.SetIndexingScope(0); err != nil {
				t.Error(err)
			}
		}
	}
	stats, err := sc.Scan(context.Background())
	if err != nil || stats.Deferred != 1 {
		t.Fatalf("scope changed in-flight: %+v %v", stats, err)
	}
	stats, err = sc.Scan(context.Background())
	if err != nil || stats.Indexed != 1 {
		t.Fatalf("next scan ignored new scope: %+v %v", stats, err)
	}
}
