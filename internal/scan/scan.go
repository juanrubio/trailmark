// Package scan discovers native session logs under configured roots,
// applies exclusions, dedupes overlapping roots by canonical path,
// and incrementally indexes new or changed logs. A failing root never
// hides the others, and derived data is pruned only for a confirmed
// full walk or a root removed from the configuration.
package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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
	Deferred    int // old, previously unindexed files left for a broader scope
	Discovered  int
	Indexed     int
	Skipped     int // unchanged indexed files or confirmed non-session data
	Partial     int
	Failed      int
	Excluded    int
	Pruned      int      // derived rows removed after confirmed coverage
	FailedRoots []string // roots that could not be walked this run
}

// Progress is a runtime snapshot. Discovered grows during the streaming walk.
type Progress struct {
	Phase    string
	Stats    Stats
	Provider model.Provider
	Path     string
}

// Scanner runs discovery plus indexing over configured roots.
type Scanner struct {
	mu       sync.Mutex // serializes scans: interleaved prune passes are unsafe
	Index    *index.Index
	Adapters map[model.Provider]adapter.Adapter
	Config   *config.Config
	// Progress receives running statistics for discovery and indexing.
	// Observer also receives the phase and current source path.
	Progress func(Stats)
	Observer func(Progress)
	// Now supplies a fixed scan-start clock, primarily for deterministic boundary tests.
	Now    func() time.Time
	cutoff time.Time // protected by mu; fixed for the entire scan
}

// Scan walks every effective root. Roots that fail to walk are named
// in Stats.FailedRoots; their cached rows are preserved and never
// pruned. Pruning covers confirmed deletions under fully walked
// roots and all derived data under roots removed from the config
// since the previous scan.
func (sc *Scanner) Scan(ctx context.Context) (Stats, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	now := time.Now()
	if sc.Now != nil {
		now = sc.Now()
	}
	sc.cutoff = time.Time{}
	if days, _ := sc.Config.IndexingScope(); days > 0 {
		sc.cutoff = now.Add(-time.Duration(days) * 24 * time.Hour)
	}
	var stats Stats
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	sc.observe("discovering", &stats, "", "")
	defer func() { sc.observe("done", &stats, "", "") }()
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
		if err := sc.scanRoot(ctx, canonRoot, ad, root.Provider, seen, &stats); err != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			stats.FailedRoots = append(stats.FailedRoots, root.Path)
			sc.observe("discovering", &stats, root.Provider, root.Path)
			sc.recordRootStatus(ctx, canonRoot, "unavailable")
			continue
		}
		sc.recordRootStatus(ctx, canonRoot, "ok")
		walkedRoots = append(walkedRoots, canonRoot)
	}

	if err := ctx.Err(); err != nil {
		return stats, err
	}
	sc.observe("finishing", &stats, "", "")

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
	if data, err := json.Marshal(stats); err == nil {
		_ = sc.Index.SetMeta(ctx, "last_scan_stats", string(data))
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

const rootStatusKey = "root_status"

// LastScanStats returns the persisted stats of the most recent scan.
func (sc *Scanner) LastScanStats(ctx context.Context) (Stats, bool, error) {
	raw, ok, err := sc.Index.GetMeta(ctx, "last_scan_stats")
	if err != nil || !ok || raw == "" {
		return Stats{}, false, err
	}
	var stats Stats
	if err := json.Unmarshal([]byte(raw), &stats); err != nil {
		return Stats{}, false, fmt.Errorf("decode scan stats: %w", err)
	}
	return stats, true, nil
}

// LastScanAt returns the last scan attempt time, if recorded.
func (sc *Scanner) LastScanAt(ctx context.Context) (string, bool, error) {
	return sc.Index.GetMeta(ctx, "last_scan_at")
}

// RootStatus is one root's last scan outcome.
type RootStatus struct {
	State    string `json:"state"` // ok or unavailable
	LastOKAt string `json:"last_ok_at,omitempty"`
	LastTry  string `json:"last_try,omitempty"`
}

// RootStatuses reads the persisted per-root scan status.
func (sc *Scanner) RootStatuses(ctx context.Context) (map[string]RootStatus, error) {
	raw, ok, err := sc.Index.GetMeta(ctx, rootStatusKey)
	if err != nil || !ok || raw == "" {
		return map[string]RootStatus{}, err
	}
	out := map[string]RootStatus{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode root status: %w", err)
	}
	return out, nil
}

func (sc *Scanner) recordRootStatus(ctx context.Context, root, state string) {
	statuses, err := sc.RootStatuses(ctx)
	if err != nil {
		statuses = map[string]RootStatus{}
	}
	cur := statuses[root]
	now := time.Now().Format(time.RFC3339)
	cur.State = state
	cur.LastTry = now
	if state == "ok" {
		cur.LastOKAt = now
	}
	statuses[root] = cur
	data, err := json.Marshal(statuses)
	if err != nil {
		return
	}
	_ = sc.Index.SetMeta(ctx, rootStatusKey, string(data))
	_ = sc.Index.SetMeta(ctx, "last_scan_at", now)
	if state == "ok" {
		_ = sc.Index.SetMeta(ctx, "last_scan_ok", now)
	}
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
func (sc *Scanner) scanRoot(ctx context.Context, root string, ad adapter.Adapter, provider model.Provider, seen map[string]bool, stats *Stats) error {
	visitedDirs := map[string]bool{}
	return sc.walkDir(ctx, root, ad, provider, seen, stats, visitedDirs)
}

func (sc *Scanner) walkDir(ctx context.Context, dir string, ad adapter.Adapter, provider model.Provider, seen map[string]bool, stats *Stats, visitedDirs map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	canonDir := config.Canonical(dir)
	if visitedDirs[canonDir] {
		return nil
	}
	visitedDirs[canonDir] = true
	sc.observe("discovering", stats, provider, canonDir)
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
			// Still route the file through indexFile so it counts
			// as seen: an unstat-able file must not look deleted.
			if filepath.Ext(entry.Name()) == ".jsonl" {
				if err := sc.indexFile(ctx, path, ad, provider, seen, stats); err != nil {
					return err
				}
			} else {
				stats.Failed++
			}
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
				if err := sc.walkDir(ctx, path, ad, provider, seen, stats, visitedDirs); err != nil {
					return err
				}
				continue
			}
			mode = target.Mode()
		}
		if info.IsDir() || mode.IsDir() {
			if err := sc.walkDir(ctx, path, ad, provider, seen, stats, visitedDirs); err != nil {
				return err
			}
			continue
		}
		if filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		if err := sc.indexFile(ctx, path, ad, provider, seen, stats); err != nil {
			return err
		}
	}
	return nil
}

func (sc *Scanner) indexFile(ctx context.Context, path string, ad adapter.Adapter, provider model.Provider, seen map[string]bool, stats *Stats) error {
	canon := config.Canonical(path)
	if seen[canon] {
		return nil
	}
	seen[canon] = true
	stats.Discovered++
	sc.observe("indexing", stats, provider, canon)
	defer func() { sc.observe("indexing", stats, provider, canon) }()
	if err := ctx.Err(); err != nil {
		return err
	}

	if sc.Config.Excluded(canon) {
		stats.Excluded++
		return sc.Index.Remove(ctx, canon)
	}
	info, err := os.Stat(canon)
	if err != nil {
		stats.Failed++
		return nil
	}
	size, mtimeNanos, cwd, version, wasIndexed, err := sc.Index.FileState(ctx, canon)
	if err != nil {
		return err
	} else if wasIndexed && size == info.Size() && mtimeNanos == info.ModTime().UnixNano() && version == index.ParserVersion {
		if cwd != "" && sc.Config.Excluded(cwd) {
			stats.Excluded++
			return sc.Index.Remove(ctx, canon)
		}
		stats.Skipped++
		return nil
	}
	if !wasIndexed && !sc.cutoff.IsZero() && info.ModTime().Before(sc.cutoff) {
		stats.Deferred++
		return nil
	}
	session, err := ad.ParseFile(ctx, canon)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, adapter.ErrNotSession) {
		if err := sc.Index.Remove(ctx, canon); err != nil {
			return err
		}
		if wasIndexed {
			stats.Pruned++
		}
		stats.Skipped++
		return nil
	}
	if err != nil {
		stats.Failed++
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
	return nil
}

func (sc *Scanner) observe(phase string, stats *Stats, provider model.Provider, path string) {
	snapshot := *stats
	snapshot.FailedRoots = append([]string(nil), stats.FailedRoots...)
	if sc.Observer != nil {
		sc.Observer(Progress{Phase: phase, Stats: snapshot, Provider: provider, Path: path})
	}
	sc.report(&snapshot)
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
