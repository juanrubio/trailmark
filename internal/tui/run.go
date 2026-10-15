package tui

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/scan"
)

// Deps wires the TUI to the Phase 1 index and scanner.
type Deps struct {
	Index   *index.Index
	Scanner *scan.Scanner
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

	scanCmd := func() tea.Msg {
		stats, err := deps.Scanner.Scan(ctx)
		if err != nil {
			return scanDoneMsg{err: err}
		}
		fresh, err := load(ctx)
		if err != nil {
			return scanDoneMsg{err: err}
		}
		return scanDoneMsg{sessions: fresh, failedRoots: stats.FailedRoots}
	}
	m := newModel(ctx, deps.Index.Overview, scanCmd, launchDir, home, sessions)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}
	return nil
}
