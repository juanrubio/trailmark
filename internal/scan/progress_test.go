package scan_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/adapter/claudecode"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
	"github.com/juanrubio/trailmark/internal/scan"
)

func TestProgressReportsBeforeAndAfterExcludedFile(t *testing.T) {
	root, _, path := setupRoots(t)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	sc := newScanner(ix, &config.Config{DisableDefaultRoots: true, Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: root}}, Exclusions: []string{path}})
	var events []scan.Progress
	sc.Observer = func(p scan.Progress) { events = append(events, p) }
	stats, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Excluded != 1 {
		t.Fatalf("stats: %+v", stats)
	}
	if events[0].Phase != "discovering" || events[len(events)-1].Phase != "done" {
		t.Fatalf("phases: %+v", events)
	}
	sawBefore, sawAfter := false, false
	for _, p := range events {
		if p.Phase == "indexing" {
			if p.Stats.Discovered != 1 {
				t.Fatalf("indexing without discovered file: %+v", p)
			}
			sawBefore = sawBefore || p.Path == path && p.Stats.Excluded == 0
			sawAfter = sawAfter || p.Path == path && p.Stats.Excluded == 1
		}
	}
	if !sawBefore || !sawAfter {
		t.Fatalf("missing before/after exclusion progress: %+v", events)
	}
}

func TestCancellationKeepsCommittedSessionsAndSkipsPrune(t *testing.T) {
	root, _, _ := setupRoots(t)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	sc := newScanner(ix, &config.Config{DisableDefaultRoots: true, Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: root}}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sc.Observer = func(p scan.Progress) {
		if p.Phase == "indexing" && p.Stats.Indexed == 1 {
			cancel()
		}
	}
	stats, err := sc.Scan(ctx)
	if !errors.Is(err, context.Canceled) || stats.Indexed != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	rows, err := ix.ListSessions(context.Background(), 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if _, ok, err := sc.LastScanStats(context.Background()); err != nil || ok {
		t.Fatalf("canceled scan persisted completion: ok=%v err=%v", ok, err)
	}
}

type cancelParsingAdapter struct {
	claudecode.Adapter
	beforeParse func()
}

func (a cancelParsingAdapter) ParseFile(ctx context.Context, _ string) (*model.Session, error) {
	a.beforeParse()
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestProgressPrecedesParsingAndCancellationIsNotFailure(t *testing.T) {
	root, _, path := setupRoots(t)
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var latest scan.Progress
	sc := newScanner(ix, &config.Config{DisableDefaultRoots: true, Roots: []config.Root{{Provider: model.ProviderClaudeCode, Path: root}}})
	sc.Observer = func(p scan.Progress) { latest = p }
	sc.Adapters = map[model.Provider]adapter.Adapter{model.ProviderClaudeCode: cancelParsingAdapter{beforeParse: func() {
		if latest.Phase != "indexing" || latest.Path != path || latest.Stats.Discovered != 1 {
			t.Errorf("missing early parsing progress: %+v", latest)
		}
		cancel()
	}}}
	stats, err := sc.Scan(ctx)
	if !errors.Is(err, context.Canceled) || stats.Failed != 0 || stats.Indexed != 0 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	if latest.Phase != "done" {
		t.Fatalf("terminal progress: %+v", latest)
	}
}
