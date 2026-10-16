package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/scan"
)

func initialModel() *appModel {
	m := newModel(context.Background(), dataDeps{}, func() tea.Msg { return nil }, "/project", "/home/dev", nil)
	m.width, m.height = 80, 24
	m.noColor = true
	return m
}

func TestInitialIndexingProgressAndNavigation(t *testing.T) {
	m := initialModel()
	m.deps.scanProgress = func() scan.Progress {
		return scan.Progress{Phase: "indexing", Stats: scan.Stats{Discovered: 53, Indexed: 4, Skipped: 2, Excluded: 3, Deferred: 5}, Path: "/home/dev/logs/large.jsonl"}
	}
	_, cmd := m.Update(initialIndexTickMsg(time.Now()))
	if cmd == nil || m.initialFrame != 1 {
		t.Fatal("active scan must animate independently")
	}
	view := ansi.Strip(m.View().Content)
	for _, s := range []string{"Initial indexing", "53 discovered", "9 checked", "4 indexed", "~/logs/large.jsonl", "local cache", "5 older files deferred"} {
		if !strings.Contains(view, s) {
			t.Errorf("missing %q in %s", s, view)
		}
	}
	for _, s := range []string{",", "s", "/", "p", "enter", "r"} {
		m.onKey(key(s))
	}
	if m.settings != nil || m.search != nil || m.overlay != overlayNone || m.focus != focusList {
		t.Fatal("navigation escaped initial screen")
	}
	m.onKey(key("q"))
	if m.overlay != overlayQuit || m.quitSelected {
		t.Fatal("quit must default to Cancel")
	}
	_, cmd = m.Update(initialIndexTickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("quit modal stopped progress")
	}
	m.onKey(key("esc"))
	if !m.initialIndexing {
		t.Fatal("closing quit must preserve scan")
	}
}

func TestInitialIndexingCancelAndCompletion(t *testing.T) {
	for _, failure := range []error{nil, context.Canceled, errors.New("unreadable")} {
		m := initialModel()
		canceled := 0
		m.deps.cancelInitialScan = func() { canceled++ }
		m.onKey(key("esc"))
		m.onKey(key("esc"))
		if canceled != 1 || !m.initialCanceling || !m.initialIndexing {
			t.Fatal("cancel must request stop once and wait for committed data")
		}
		rows := fixtureSessions()[:1]
		m.Update(scanDoneMsg{sessions: rows, err: failure})
		if m.initialIndexing || len(m.sessions) != 1 {
			t.Fatal("must show committed sessions on every outcome")
		}
		_, cmd := m.Update(initialIndexTickMsg(time.Now()))
		if cmd != nil {
			t.Fatal("completed scan scheduled another tick")
		}
		if failure != nil && m.notice == "" {
			t.Fatal("completion outcome missing")
		}
	}
}

func TestInitialIndexingSuccessClearsPreviousCancellationNotice(t *testing.T) {
	m := initialModel()
	m.notice = "Indexing skipped. Open Settings (,) to choose coverage and rescan."
	m.Update(scanDoneMsg{sessions: fixtureSessions()[:1]})
	if m.notice != "" || m.initialIndexing {
		t.Fatal("successful indexing retained an obsolete cancellation notice")
	}
}

func TestInitialIndexingEmptyAndWarmStart(t *testing.T) {
	m := initialModel()
	m.Update(scanDoneMsg{})
	view := ansi.Strip(m.View().Content)
	if m.initialIndexing || strings.Contains(view, "first scan completes") || !strings.Contains(view, "Settings") {
		t.Fatal(view)
	}
	m = newModel(context.Background(), dataDeps{}, func() tea.Msg { return nil }, "", "", []index.SessionRow{{SourcePath: "/a"}})
	if m.initialIndexing {
		t.Fatal("warm cache must open browser")
	}
	m.Update(scanDoneMsg{sessions: []index.SessionRow{{SourcePath: "/a"}, {SourcePath: "/b"}}})
	if !m.hasPending || len(m.sessions) != 1 {
		t.Fatal("warm refresh semantics changed")
	}
	m = newModel(context.Background(), dataDeps{}, nil, "", "", nil)
	if m.initialIndexing {
		t.Fatal("nil scan must retain normal empty browser")
	}
}

func TestInitialIndexingGeometryAndSanitization(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {80, 24}, {160, 42}} {
		for _, theme := range []string{"green", "amber", "blue", "monokai"} {
			for _, plain := range []bool{false, true} {
				for _, ascii := range []bool{false, true} {
					for _, light := range []bool{false, true} {
						m := initialModel()
						m.width, m.height = size[0], size[1]
						m.themeName = theme
						m.noColor = plain
						m.ascii = ascii
						m.lightTheme = light
						m.initialProgress = scan.Progress{Path: "/home/dev/\x1b[31mdirty\npath\x1b[0m/" + strings.Repeat("verylong", 100), Stats: scan.Stats{Failed: 2}}
						view := m.View().Content
						if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
							t.Fatalf("%v %s plain=%v ascii=%v light=%v got %dx%d", size, theme, plain, ascii, light, lipgloss.Width(view), lipgloss.Height(view))
						}
						if plain && (strings.Contains(view, "[38;") || strings.Contains(view, "[48;")) {
							t.Fatal("NO_COLOR contains color")
						}
						if strings.Contains(view, "\x1b[31m") {
							t.Fatal("path injected terminal style")
						}
					}
				}
			}
		}
	}
}
