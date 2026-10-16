package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/adapter/claudecode"
	"github.com/juanrubio/trailmark/internal/adapter/codex"
	"github.com/juanrubio/trailmark/internal/clipboard"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
	"github.com/juanrubio/trailmark/internal/scan"
)

// Deps wires the TUI to the Phase 1 index and scanner.
type Deps struct {
	Index   *index.Index
	Scanner *scan.Scanner
	Config  *config.Config
}

// Run loads the cached session list, starts the interactive program,
// and refreshes in the background without blocking the first paint.
func Run(ctx context.Context, deps Deps) error {
	load := func(ctx context.Context) ([]index.SessionRow, error) {
		return deps.Index.ListSessions(ctx, 100000)
	}
	sessions, err := load(ctx)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	launchDir, _ := os.Getwd()

	rescan := func(ctx context.Context) ([]index.SessionRow, []string, error) {
		stats, err := deps.Scanner.Scan(ctx)
		if err != nil {
			return nil, nil, err
		}
		fresh, err := load(ctx)
		if err != nil {
			return nil, nil, err
		}
		return fresh, stats.FailedRoots, nil
	}
	scanCmd := func() tea.Msg {
		fresh, failed, err := rescan(ctx)
		if err != nil {
			return scanDoneMsg{err: err}
		}
		return scanDoneMsg{sessions: fresh, failedRoots: failed}
	}
	d := dataDeps{
		overview:  deps.Index.Overview,
		page:      deps.Index.TranscriptPage,
		around:    deps.Index.TranscriptAround,
		inSession: deps.Index.SearchInSession,
		content:   deps.Index.SearchContentScoped,
		caps: map[model.Provider]model.Capability{
			model.ProviderClaudeCode: claudecode.Adapter{}.Capability(),
			model.ProviderCodex:      codex.Adapter{}.Capability(),
		},
		copier:   clipboard.System{},
		rescan:   rescan,
		settings: &settingsBackend{cfg: deps.Config, ix: deps.Index, scanner: deps.Scanner},
	}
	m := newModel(ctx, d, scanCmd, launchDir, home, sessions)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithEnvironment(terminalEnvironment(os.Environ())))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}
	return nil
}

// settingsBackend adapts config, index, and scanner state to the
// settings view. Mutations persist through config.Save.
type settingsBackend struct {
	cfg     *config.Config
	ix      *index.Index
	scanner *scan.Scanner
}

func (b *settingsBackend) Roots(ctx context.Context) ([]RootInfo, error) {
	if b.cfg == nil {
		return nil, nil
	}
	statuses, _ := b.scanner.RootStatuses(ctx)
	paths, _ := b.ix.SourcePaths(ctx)
	var out []RootInfo
	for _, r := range b.cfg.EffectiveRoots() {
		canon := config.Canonical(r.Path)
		info := RootInfo{
			Provider:   r.Provider,
			Path:       r.Path,
			Configured: b.cfg.IsConfiguredRoot(r.Provider, r.Path),
			State:      "unknown",
		}
		if st, ok := statuses[canon]; ok {
			info.State = st.State
			info.LastOK = st.LastOKAt
		}
		for _, p := range paths {
			if p == canon || strings.HasPrefix(p, canon+string(filepath.Separator)) {
				info.Sessions++
			}
		}
		out = append(out, info)
	}
	return out, nil
}

func (b *settingsBackend) Status(ctx context.Context) (SettingsStatus, error) {
	counts, err := b.ix.Stats(ctx)
	if err != nil {
		return SettingsStatus{}, err
	}
	st := SettingsStatus{Indexed: counts.Sessions, Partial: counts.Partial}
	if stats, ok, err := b.scanner.LastScanStats(ctx); err == nil && ok {
		st.Excluded = stats.Excluded
	}
	if raw, ok, err := b.ix.GetMeta(ctx, "last_scan_ok"); err == nil && ok && raw != "" {
		st.LastScan = raw
	} else if at, ok, err := b.scanner.LastScanAt(ctx); err == nil && ok {
		st.LastScan = at
	}
	st.IndexPath = b.ix.Path()
	st.DatabaseBytes, st.DatabaseSizeKnown = databaseFileSize(st.IndexPath)
	return st, nil
}

func (b *settingsBackend) Exclusions() []string {
	if b.cfg == nil {
		return nil
	}
	return append([]string{}, b.cfg.Exclusions...)
}

// absolutizePath expands ~ and requires an absolute result, so a
// stored path can never silently re-anchor to a future launch dir.
func absolutizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute: %q", path)
	}
	return filepath.Clean(path), nil
}

func (b *settingsBackend) AddRoot(provider model.Provider, path string) error {
	if b.cfg == nil {
		return fmt.Errorf("configuration unavailable")
	}
	if provider != model.ProviderClaudeCode && provider != model.ProviderCodex {
		return fmt.Errorf("unknown provider %q", provider)
	}
	abs, err := absolutizePath(path)
	if err != nil {
		return err
	}
	b.cfg.AddRoot(config.Root{Provider: provider, Path: abs})
	return b.cfg.Save()
}

func (b *settingsBackend) RemoveRoot(_ context.Context, provider model.Provider, path string) error {
	if b.cfg == nil {
		return fmt.Errorf("configuration unavailable")
	}
	if b.cfg.RemoveConfiguredRoot(provider, path) {
		return b.cfg.Save()
	}
	for _, d := range config.DefaultRoots() {
		if d.Provider == provider && config.Canonical(d.Path) == config.Canonical(path) {
			b.cfg.DisableDefaultRoots = true
			return b.cfg.Save()
		}
	}
	return fmt.Errorf("root not found")
}

func (b *settingsBackend) AddExclusion(path string) error {
	if b.cfg == nil {
		return fmt.Errorf("configuration unavailable")
	}
	abs, err := absolutizePath(path)
	if err != nil {
		return err
	}
	b.cfg.AddExclusion(abs)
	if err := b.cfg.Save(); err != nil {
		return err
	}
	// Applying an exclusion removes already-derived data now; it
	// must not wait for a successful walk that may never come.
	return b.removeExcludedDerived(context.Background())
}

// removeExcludedDerived deletes index rows whose source path or
// recorded cwd falls under any configured exclusion.
func (b *settingsBackend) removeExcludedDerived(ctx context.Context) error {
	paths, err := b.ix.SourcePaths(ctx)
	if err != nil {
		return err
	}
	for _, p := range paths {
		excluded := b.cfg.Excluded(p)
		if !excluded {
			if _, _, cwd, _, ok, err := b.ix.FileState(ctx, p); err == nil && ok && cwd != "" {
				excluded = b.cfg.Excluded(cwd)
			}
		}
		if excluded {
			if err := b.ix.Remove(ctx, p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *settingsBackend) RemoveExclusion(path string) error {
	if b.cfg == nil {
		return fmt.Errorf("configuration unavailable")
	}
	if !b.cfg.RemoveExclusion(path) {
		return fmt.Errorf("exclusion not found")
	}
	return b.cfg.Save()
}

func (b *settingsBackend) ClearCache(ctx context.Context) error {
	return b.ix.Clear(ctx)
}

// terminalEnvironment uses conservative xterm rendering for Kitty. Older Kitty
// releases (including 0.16) do not implement REP, but the renderer assumes every
// xterm-kitty terminal does. Ignored REP sequences collapse padding and move
// subsequent panes left. Keep this override local to Bubble Tea; COLORTERM and
// all other environment entries remain available for color/capability detection.
func terminalEnvironment(env []string) []string {
	result := append([]string(nil), env...)
	for i, entry := range result {
		if entry == "TERM=xterm-kitty" {
			result[i] = "TERM=xterm-256color"
		}
	}
	return result
}

func (b *settingsBackend) Theme() string {
	if b.cfg == nil {
		return "green"
	}
	return config.NormalizeTheme(b.cfg.Theme)
}
func (b *settingsBackend) SetTheme(name string) error {
	if b.cfg == nil {
		return fmt.Errorf("configuration unavailable")
	}
	previous := b.cfg.Theme
	b.cfg.Theme = config.NormalizeTheme(name)
	if err := b.cfg.Save(); err != nil {
		b.cfg.Theme = previous
		return err
	}
	return nil
}

// databaseFileSize includes SQLite's active WAL/shared-memory sidecars without
// checkpointing or otherwise modifying the database. Optional missing files
// contribute zero; any other stat failure leaves the measurement unavailable.
func databaseFileSize(path string) (int64, bool) {
	main, err := os.Stat(path)
	if err != nil || !main.Mode().IsRegular() {
		return 0, false
	}
	total := main.Size()
	for _, suffix := range []string{"-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return 0, false
		}
		total += info.Size()
	}
	return total, true
}
