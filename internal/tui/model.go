// Package tui implements Trailmark's recent-work TUI (Phase 2):
// a seven-day, day-grouped session list with a selected-session
// overview, keyboard-first throughout.
package tui

import (
	"context"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/clipboard"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/continuation"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/memory"
	"github.com/juanrubio/trailmark/internal/model"
	"github.com/juanrubio/trailmark/internal/scan"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlaySelector
	overlayFilter
	overlaySheet
	overlayCopyMenu
	overlayCopyPreview
	overlayQuit
)

type selectorKind int

const (
	selectorTime selectorKind = iota
	selectorProject
	selectorHarness
	selectorReference
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
	id          int // 0 for the startup scan; settings rescans carry a token
	sessions    []index.SessionRow
	failedRoots []string
	err         error
}

// overviewFunc loads detail for one session by source path.
type overviewFunc func(ctx context.Context, sourcePath string) (*index.Overview, error)

// dataDeps bundles the index reads the TUI needs.
type dataDeps struct {
	scanProgress      func() scan.Progress
	cancelInitialScan func()
	cancelRescan      func()
	overview          overviewFunc
	page              func(ctx context.Context, sourcePath string, offset, limit int) ([]index.TranscriptMessage, int, error)
	around            func(ctx context.Context, sourcePath string, seq, before, after int) ([]index.TranscriptMessage, int, int, error)
	inSession         func(ctx context.Context, sourcePath, query string) ([]int, error)
	content           func(ctx context.Context, query string, paths []string, limit int) ([]index.ContentGroup, error)
	caps              map[model.Provider]model.Capability
	// snapshotConfig copies live configuration on the update
	// goroutine (where Settings mutations also happen); the
	// asynchronous loader derives everything filesystem-bound
	// from that snapshot plus the in-memory session snapshot, so
	// it never races Settings and never waits on the transcript
	// database either.
	snapshotConfig func() *config.Config
	loadMemories   func(ctx context.Context, sessions []index.SessionRow, cfg *config.Config) (*memory.Catalog, error)
	copier         clipboard.Copier
	settings       settingsStore
	rescan         func(ctx context.Context) ([]index.SessionRow, []string, error)
}

type appModel struct {
	chooseIndexingScope bool
	scopeCursor         int
	scopeError          string
	initialIndexing     bool
	initialScanID       int
	initialCanceling    bool
	scanFinished        bool
	initialStarted      time.Time
	initialFrame        int
	initialProgress     scan.Progress
	ctx                 context.Context
	deps                dataDeps
	transcript          *transcriptState
	search              *searchState
	settings            *settingsState
	mem                 *memoriesState

	applyNextScan bool
	scanSeq       int
	applyScanID   int
	launchDir     string
	home          string
	now           func() time.Time
	scanCmd       tea.Cmd // background refresh command, may be nil

	theme                      *themeStyles
	themeName, themeCachedName string
	themeLight, themeNoColor   bool
	ascii                      bool
	lightTheme                 bool
	noColor                    bool
	width, height              int
	sessions                   []index.SessionRow
	pending                    []index.SessionRow
	hasPending                 bool
	partialCount               int

	manualWindow           bool // explicit time selections stay fixed across scope changes
	windowDays             int  // 0 means All time; default 7
	harness                model.Provider
	project                string // "" = all, "current" = current project, else a cwd
	filterText             string
	filterPrev             string
	filterSelPrev          int
	projectRoot            string
	projectRootSet         bool
	sel                    int // index into cards()
	expanded               map[string]bool
	detail                 *index.Overview
	detailFor              string
	focus                  focusPane
	workReveal             bool
	workExpanded           bool
	overviewOpen           bool // narrow full-width overview
	quitSelected           bool // Cancel is the default choice
	helpScroll             int
	overlay                overlayKind
	selector               selectorKind
	selCursor              int
	overviewFind           overviewFindState
	overviewViewportHeight int // current scrollable area, excluding pinned actions
	overviewScroll         int
	notice                 string
	failedRoots            []string

	sheet       *continuation.Prepared
	sheetState  string // "", "copied", "failed"
	copyItems   []copyItem
	copyCursor  int
	copyPreview *copyItem
	copyState   string // "", "copied", "failed"
	copyScroll  int
	copyReturn  string // "menu" or "transcript"
	sheetScroll int
}

func newModel(ctx context.Context, deps dataDeps, scanCmd tea.Cmd, launchDir, home string, sessions []index.SessionRow) *appModel {
	m := &appModel{
		ctx:        ctx,
		noColor:    colorDisabled(),
		ascii:      os.Getenv("TERM") == "dumb",
		deps:       deps,
		launchDir:  launchDir,
		home:       home,
		now:        time.Now,
		scanCmd:    scanCmd,
		sessions:   sessions,
		windowDays: 7,
		expanded:   map[string]bool{},
	}
	if store, ok := deps.settings.(themeStore); ok {
		m.themeName = store.Theme()
	}
	m.initialIndexing = len(sessions) == 0 && scanCmd != nil
	if store, ok := deps.settings.(indexingScopeStore); ok && m.initialIndexing {
		if _, set := store.IndexingDays(); !set {
			m.chooseIndexingScope = true
			m.initialIndexing = false
			m.scopeCursor = 1
		}
	}
	m.initialStarted = m.now()
	m.partialCount = countPartial(sessions)
	m.applyLaunchProjectDefault()
	m.chooseAutomaticWindow()
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

func (m *appModel) Init() tea.Cmd {
	if m.chooseIndexingScope {
		return tea.RequestBackgroundColor
	}
	return tea.Batch(tea.RequestBackgroundColor, m.scanCmd, m.initialTick())
}

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
