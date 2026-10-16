package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/index"
)

type scopeTestStore struct {
	fakeSettings
	days int
	set  bool
	fail bool
}

func (s *scopeTestStore) IndexingDays() (int, bool) { return s.days, s.set }
func (s *scopeTestStore) SetIndexingDays(days int) error {
	if s.fail {
		return errors.New("disk full")
	}
	s.days, s.set = days, true
	return nil
}

func runScopeCommand(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, child := range batch {
			runScopeCommand(child)
		}
	}
}

func TestFirstRunScopeDefersScanUntilSaved(t *testing.T) {
	store := &scopeTestStore{fail: true}
	scans := 0
	m := newModel(context.Background(), dataDeps{settings: store}, func() tea.Msg { scans++; return nil }, "", "", nil)
	if !m.chooseIndexingScope || m.initialIndexing || m.scopeCursor != 1 {
		t.Fatal("empty unset cache must choose scope, defaulting to 30 days")
	}
	runScopeCommand(m.Init())
	if scans != 0 {
		t.Fatal("scan started before scope selection")
	}
	_, cmd := m.onKey(key("enter"))
	runScopeCommand(cmd)
	if scans != 0 || !m.chooseIndexingScope || m.scopeError == "" || store.set {
		t.Fatal("save error must retain chooser without scanning")
	}
	store.fail = false
	_, cmd = m.onKey(key("enter"))
	runScopeCommand(cmd)
	if scans != 1 || m.chooseIndexingScope || !m.initialIndexing || store.days != 30 || !store.set {
		t.Fatal("successful save must persist scope before starting scan")
	}
}

func TestFirstRunScopeSkipQuitAndLegacy(t *testing.T) {
	store := &scopeTestStore{}
	m := newModel(context.Background(), dataDeps{settings: store}, func() tea.Msg { t.Fatal("unexpected scan"); return nil }, "", "", nil)
	m.onKey(key("q"))
	if m.overlay != overlayQuit || m.quitSelected {
		t.Fatal("q must open Cancel-default confirmation")
	}
	m.onKey(key("esc"))
	if !m.chooseIndexingScope {
		t.Fatal("canceling quit must preserve scope chooser")
	}
	m.onKey(key("esc"))
	if m.chooseIndexingScope || m.initialIndexing || store.set {
		t.Fatal("Esc must skip without choosing or scanning")
	}
	m.onKey(key(","))
	if m.settings == nil {
		t.Fatal("skipped first run must allow settings")
	}
	for _, rows := range [][]index.SessionRow{fixtureSessions(), nil} {
		legacy := newModel(context.Background(), dataDeps{settings: &fakeSettings{}}, func() tea.Msg { return nil }, "", "", rows)
		if legacy.chooseIndexingScope || legacy.initialIndexing != (len(rows) == 0) {
			t.Fatal("legacy store startup changed")
		}
	}
	warm := newModel(context.Background(), dataDeps{settings: store}, func() tea.Msg { return nil }, "", "", fixtureSessions())
	if warm.chooseIndexingScope || warm.initialIndexing {
		t.Fatal("warm unset scope must browse immediately")
	}
	explicit := newModel(context.Background(), dataDeps{settings: &scopeTestStore{days: 7, set: true}}, func() tea.Msg { return nil }, "", "", nil)
	if explicit.chooseIndexingScope || !explicit.initialIndexing {
		t.Fatal("persisted scope must start directly")
	}
}

func TestSettingsIndexingScopePersistsAndRescans(t *testing.T) {
	store := &scopeTestStore{days: 7, set: true}
	rescans := 0
	m := newModel(context.Background(), dataDeps{settings: store, rescan: func(context.Context) ([]index.SessionRow, []string, error) { rescans++; return nil, nil, nil }}, nil, "", "", fixtureSessions())
	m.openSettings()
	for i, item := range m.settings.items {
		if item.action == "indexing-scope" {
			m.settings.cursor = i
		}
	}
	m.onKey(key("enter"))
	if m.settings.mode != "indexing-scope" || m.settings.scopeCursor != 0 {
		t.Fatal("settings should show saved scope")
	}
	m.onKey(key("down"))
	m.onKey(key("down"))
	store.fail = true
	_, cmd := m.onKey(key("enter"))
	if cmd != nil || store.days != 7 || m.settings.mode != "indexing-scope" {
		t.Fatal("failed scope save triggered rescan")
	}
	store.fail = false
	_, cmd = m.onKey(key("enter"))
	runScopeCommand(cmd)
	if store.days != 0 || rescans != 1 || m.settings.mode != "list" {
		t.Fatal("All history must save and rescan")
	}
	m.openSettings()
	for i, item := range m.settings.items {
		if item.action == "indexing-scope" {
			m.settings.cursor = i
		}
	}
	m.onKey(key("enter"))
	if m.settings.scopeCursor != 2 {
		t.Fatal("settings did not reload All history")
	}
	m.settings = nil
	if m.indexingCoverageHint() != "" {
		t.Fatal("unlimited index has limited hint")
	}
	store.days = 30
	for _, rows := range [][]index.SessionRow{nil, fixtureSessions()} {
		m.sessions = rows
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "30d") || !strings.Contains(view, "Index") {
			t.Fatal(view)
		}
	}
}

func TestScopeChooserNarrowGeometryAndControls(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {80, 24}, {160, 42}} {
		for _, ascii := range []bool{false, true} {
			m := newModel(context.Background(), dataDeps{settings: &scopeTestStore{}}, func() tea.Msg { return nil }, "", "", nil)
			m.width, m.height = size[0], size[1]
			m.noColor = true
			m.ascii = ascii
			for _, failure := range []string{"", "Could not save indexing scope: disk full"} {
				m.scopeError = failure
				view := m.View().Content
				if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
					t.Fatal("chooser geometry", size)
				}
				plain := ansi.Strip(view)
				for _, control := range []string{"Choose initial indexing scope", "Enter", "Start", "Esc", "Skip", "q", "Quit", "7 days", "30 days", "All history"} {
					if !strings.Contains(plain, control) {
						t.Errorf("%v missing %q in %s", size, control, plain)
					}
				}
				if failure != "" && !strings.Contains(plain, "Could not save") {
					t.Fatal("save error hidden")
				}
			}
		}
	}
}

func TestEmptySettingsScopeRescanShowsProgressAndCancels(t *testing.T) {
	store := &scopeTestStore{days: 7, set: true}
	canceled := 0
	m := newModel(context.Background(), dataDeps{settings: store, cancelRescan: func() { canceled++ }, rescan: func(context.Context) ([]index.SessionRow, []string, error) {
		return fixtureSessions()[:1], nil, context.Canceled
	}}, nil, "", "", nil)
	m.openSettings()
	for i, item := range m.settings.items {
		if item.action == "indexing-scope" {
			m.settings.cursor = i
		}
	}
	m.onKey(key("enter"))
	m.onKey(key("down"))
	m.onKey(key("down"))
	_, cmd := m.onKey(key("enter"))
	if cmd == nil || !m.initialIndexing || m.initialScanID == 0 || m.settings != nil {
		t.Fatal("empty scope expansion must show live progress")
	}
	m.onKey(key("esc"))
	if canceled != 1 || !m.initialCanceling {
		t.Fatal("empty rescan must cancel its own context")
	}
	// Deliver the foreground scan's result; startup id 0 must not stop it.
	m.Update(scanDoneMsg{})
	if !m.initialIndexing {
		t.Fatal("unrelated startup completion stopped foreground rescan")
	}
	m.Update(scanDoneMsg{id: m.initialScanID, sessions: fixtureSessions()[:1], err: context.Canceled})
	if m.initialIndexing || len(m.sessions) != 1 || !strings.Contains(m.notice, "canceled") {
		t.Fatal("canceled foreground rescan must browse committed sessions")
	}
	_, tick := m.Update(initialIndexTickMsg{})
	if tick != nil {
		t.Fatal("foreground completion kept animation running")
	}
}
