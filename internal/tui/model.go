// Package tui implements Trailmark's recent-work TUI (Phase 2):
// a seven-day, day-grouped session list with a selected-session
// overview, keyboard-first throughout.
package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlaySelector
	overlayFilter
)

type selectorKind int

const (
	selectorTime selectorKind = iota
	selectorProject
	selectorHarness
)

// focusPane distinguishes list and overview focus in the wide layout.
type focusPane int

const (
	focusList focusPane = iota
	focusOverview
)

// scanDoneMsg carries the result of a background scan plus the
// reloaded session list.
type scanDoneMsg struct {
	sessions    []index.SessionRow
	failedRoots []string
	err         error
}

// overviewFunc loads detail for one session by source path.
type overviewFunc func(ctx context.Context, sourcePath string) (*index.Overview, error)

type appModel struct {
	ctx       context.Context
	overview  overviewFunc
	launchDir string
	home      string
	now       func() time.Time
	scanCmd   tea.Cmd // background refresh command, may be nil

	width, height int
	sessions      []index.SessionRow
	pending       []index.SessionRow
	hasPending    bool
	partialCount  int

	windowDays     int // 0 means All time; default 7
	harness        model.Provider
	project        string // "" = all, "current" = current project, else a cwd
	filterText     string
	filterPrev     string
	filterSelPrev  int
	projectRoot    string
	projectRootSet bool
	sel            int // index into cards()
	expanded       map[string]bool
	detail         *index.Overview
	detailFor      string
	focus          focusPane
	overviewOpen   bool // narrow full-width overview
	overlay        overlayKind
	selector       selectorKind
	selCursor      int
	overviewScroll int
	notice         string
	failedRoots    []string
}

func newModel(ctx context.Context, overview overviewFunc, scanCmd tea.Cmd, launchDir, home string, sessions []index.SessionRow) *appModel {
	m := &appModel{
		ctx:        ctx,
		overview:   overview,
		launchDir:  launchDir,
		home:       home,
		now:        time.Now,
		scanCmd:    scanCmd,
		sessions:   sessions,
		windowDays: 7,
		expanded:   map[string]bool{},
	}
	m.partialCount = countPartial(sessions)
	m.refreshDetail()
	return m
}

func countPartial(sessions []index.SessionRow) int {
	n := 0
	for _, s := range sessions {
		if s.Partial {
			n++
		}
	}
	return n
}

func (m *appModel) Init() tea.Cmd { return m.scanCmd }

// providerLabel renders the supporting provider name.
func providerLabel(p model.Provider) string {
	switch p {
	case model.ProviderCodex:
		return "Codex"
	case model.ProviderClaudeCode:
		return "Claude Code"
	default:
		return string(p)
	}
}

func (m *appModel) windowLabel() string {
	switch m.windowDays {
	case 0:
		return "All time"
	case 1:
		return "Today"
	default:
		return "Last " + itoa(m.windowDays) + " days"
	}
}

func (m *appModel) harnessLabel() string {
	if m.harness == "" {
		return "All harnesses"
	}
	return providerLabel(m.harness)
}

func (m *appModel) projectLabel() string {
	switch m.project {
	case "":
		return "All projects"
	case "current":
		return "Current project"
	default:
		return projectName(m.project)
	}
}

// projectName derives a short project label from a cwd.
func projectName(cwd string) string {
	if cwd == "" {
		return "Unknown directory"
	}
	base := cwd[strings.LastIndex(cwd, "/")+1:]
	if base == "" {
		return cwd
	}
	return base
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
