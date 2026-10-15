// Package scan discovers native session logs under configured roots,
// applies exclusions, dedupes overlapping roots by canonical path,
// and incrementally indexes new or changed logs. A failing root never
// hides the others, and derived data is pruned only for a confirmed
// full walk or a root removed from the configuration.
package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/adapter/claudecode"
	"github.com/juanrubio/trailmark/internal/adapter/codex"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

const walkedRootsKey = "walked_roots"

// Stats reports one scan's outcome.
type Stats struct {
	Discovered  int
	Indexed     int
	Skipped     int // unchanged since the last successful index
	Partial     int
	Failed      int
	Excluded    int
	Pruned      int      // derived rows removed after confirmed coverage
	FailedRoots []string // roots that could not be walked this run
}

// Scanner runs discovery plus indexing over configured roots.
type Scanner struct {
	Index    *index.Index
	Adapters map[model.Provider]adapter.Adapter
	Config   *config.Config
	// Progress, when set, is called after each file with running stats.
	Progress func(Stats)
}

// Scan walks every effective root. Roots that fail to walk are named
// in Stats.FailedRoots; their cached rows are preserved and never
// pruned. Pruning covers confirmed deletions under fully walked
// roots and all derived data under roots removed from the config
// since the previous scan.
func (sc *Scanner) Scan(ctx context.Context) (Stats, error) {
	var stats Stats
	seen := map[string]bool{}
	currentRoots := map[string]bool{}
	var walkedRoots []string

	prevRoots, err := sc.storedRoots(ctx)
	if err != nil {
		return stats, err
	}
	for _, root := range sc.Config.EffectiveRoots() {
		ad, ok := sc.Adapters[root.Provider]
		if !ok {
			continue
		}
		canonRoot := config.Canonical(root.Path)
		currentRoots[canonRoot] = true
		if err := sc.scanRoot(ctx, canonRoot, ad, seen, &stats); err != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			stats.FailedRoots = append(stats.FailedRoots, root.Path)
			continue
		}
		walkedRoots = append(walkedRoots, canonRoot)
	}

	// Prune confirmed deletions and removed roots' derived data.
	pruneRoots := append([]string{}, walkedRoots...)
	for _, prev := range prevRoots {
		if !currentRoots[prev] {
			pruneRoots = append(pruneRoots, prev) // root removed from config
		}
	}
	indexed, err := sc.Index.SourcePaths(ctx)
	if err != nil {
		return stats, err
	}
	for _, path := range indexed {
		if seen[path] || !underAny(path, pruneRoots) {
			continue
		}
		if err := sc.Index.Remove(ctx, path); err != nil {
			return stats, err
		}
		stats.Pruned++
	}

	// Persist roots whose data must survive the next scan: roots
	// walked now, plus previously stored roots still configured but
	// failed this run (their coverage is unconfirmed, not deleted).
	keep := map[string]bool{}
	for _, r := range walkedRoots {
		keep[r] = true
	}
	for _, prev := range prevRoots {
		if currentRoots[prev] {
			keep[prev] = true
		}
	}
	stored := make([]string, 0, len(keep))
	for r := range keep {
		stored = append(stored, r)
	}
	sort.Strings(stored)
	if err := sc.storeRoots(ctx, stored); err != nil {
		return stats, err
	}
	return stats, nil
}

func (sc *Scanner) storedRoots(ctx context.Context) ([]string, error) {
	raw, ok, err := sc.Index.GetMeta(ctx, walkedRootsKey)
	if err != nil || !ok || raw == "" {
		return nil, err
	}
	var roots []string
	if err := json.Unmarshal([]byte(raw), &roots); err != nil {
		return nil, fmt.Errorf("decode stored roots: %w", err)
	}
	return roots, nil
}

func (sc *Scanner) storeRoots(ctx context.Context, roots []string) error {
	data, err := json.Marshal(roots)
	if err != nil {
		return fmt.Errorf("encode stored roots: %w", err)
	}
	return sc.Index.SetMeta(ctx, walkedRootsKey, string(data))
}

func underAny(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// scanRoot walks one canonical root, following symlinked directories
// with a visited set so cycles cannot loop. File and directory
// symlinks resolve to canonical paths, which dedupes overlapping
// roots and symlinked stores.
func (sc *Scanner) scanRoot(ctx context.Context, root string, ad adapter.Adapter, seen map[string]bool, stats *Stats) error {
	visitedDirs := map[string]bool{}
	return sc.walkDir(ctx, root, ad, seen, stats, visitedDirs)
}

func (sc *Scanner) walkDir(ctx context.Context, dir string, ad adapter.Adapter, seen map[string]bool, stats *Stats, visitedDirs map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	canonDir := config.Canonical(dir)
	if visitedDirs[canonDir] {
		return nil
	}
	visitedDirs[canonDir] = true
	entries, err := os.ReadDir(canonDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(canonDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			stats.Failed++
			continue
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			target, err := os.Stat(path)
			if err != nil {
				stats.Failed++
				continue
			}
			if target.IsDir() {
				if err := sc.walkDir(ctx, path, ad, seen, stats, visitedDirs); err != nil {
					return err
				}
				continue
			}
			mode = target.Mode()
		}
		if info.IsDir() || mode.IsDir() {
			if err := sc.walkDir(ctx, path, ad, seen, stats, visitedDirs); err != nil {
				return err
			}
			continue
		}
		if filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		if err := sc.indexFile(ctx, path, ad, seen, stats); err != nil {
			return err
		}
	}
	return nil
}

func (sc *Scanner) indexFile(ctx context.Context, path string, ad adapter.Adapter, seen map[string]bool, stats *Stats) error {
	canon := config.Canonical(path)
	if seen[canon] {
		return nil // overlapping root: first walk wins
	}
	seen[canon] = true
	stats.Discovered++
	if sc.Config.Excluded(canon) {
		stats.Excluded++
		return sc.Index.Remove(ctx, canon)
	}
	info, err := os.Stat(canon)
	if err != nil {
		stats.Failed++
		return nil
	}
	if size, mtimeNanos, cwd, version, ok, err := sc.Index.FileState(ctx, canon); err != nil {
		return err
	} else if ok && size == info.Size() && mtimeNanos == info.ModTime().UnixNano() && version == index.ParserVersion {
		if cwd != "" && sc.Config.Excluded(cwd) {
			stats.Excluded++
			return sc.Index.Remove(ctx, canon)
		}
		stats.Skipped++
		sc.report(stats)
		return nil
	}
	session, err := ad.ParseFile(ctx, canon)
	if err != nil {
		stats.Failed++
		sc.report(stats)
		return nil // one bad log must not hide the others
	}
	// Project exclusions match the recorded cwd. The transcript was
	// read only to decide inclusion; nothing of it is indexed.
	if session.CWD != "" && sc.Config.Excluded(session.CWD) {
		stats.Excluded++
		return sc.Index.Remove(ctx, canon)
	}
	session.SourcePath = canon
	for i := range session.Messages {
		session.Messages[i].Locator.SourcePath = canon
	}
	if err := sc.Index.Upsert(ctx, session, info.Size(), info.ModTime()); err != nil {
		return err
	}
	stats.Indexed++
	if session.Partial {
		stats.Partial++
	}
	sc.report(stats)
	return nil
}

func (sc *Scanner) report(stats *Stats) {
	if sc.Progress != nil {
		sc.Progress(*stats)
	}
}

// DefaultAdapters builds the MVP adapter set, loading the Codex
// thread-name index when present.
func DefaultAdapters(home string) map[model.Provider]adapter.Adapter {
	names, _ := codex.LoadThreadNames(filepath.Join(home, ".codex", "session_index.jsonl"))
	return map[model.Provider]adapter.Adapter{
		model.ProviderClaudeCode: claudecode.Adapter{},
		model.ProviderCodex:      codex.Adapter{ThreadNames: names},
	}
}
