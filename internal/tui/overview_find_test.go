package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/index"
)

func typeOverviewFind(m *appModel, text string) {
	send(m, "/")
	m.onKey(tea.KeyPressMsg{Text: text})
	send(m, "enter")
}

func TestOverviewFindInputNavigationAndReset(t *testing.T) {
	for _, w := range []int{80, 144} {
		m := testModel(t)
		m.width, m.height = w, 24
		send(m, "enter")
		m.detail = &index.Overview{InitialRequest: "needle first\n" + scrollText() + "\nneedle last needle"}
		m.View()
		typeOverviewFind(m, "needle")
		plain := ansi.Strip(m.View().Content)
		if len(m.overviewFind.matches) != 3 || !strings.Contains(plain, "Overview match 1 of 3") {
			t.Fatalf("%d: find not applied: %s", w, plain)
		}
		send(m, "]")
		plain = ansi.Strip(m.View().Content)
		if !strings.Contains(plain, "needle last needle") || m.overviewScroll == 0 {
			t.Fatal("offscreen next hit not revealed")
		}
		send(m, "]")
		if m.overviewFind.current != 2 {
			t.Fatal("repeated hit on same line not navigable")
		}
		send(m, "]")
		if m.overviewFind.current != 0 || !strings.Contains(ansi.Strip(m.View().Content), "needle first") {
			t.Fatal("find did not wrap to first hit")
		}
		send(m, "[")
		if m.overviewFind.current != 2 {
			t.Fatal("backward find did not wrap")
		}
		// Input owns command letters; cancel retains the applied query and match.
		send(m, "/")
		m.onKey(tea.KeyPressMsg{Text: "qrn"})
		send(m, "esc")
		if m.overlay != overlayNone || m.overviewFind.query != "needle" || m.overviewFind.current != 2 {
			t.Fatal("cancel or command-letter input changed navigation")
		}
		send(m, "/")
		for range len("needle") {
			send(m, "backspace")
		}
		send(m, "enter")
		if m.overviewFind.query != "" || len(m.overviewFind.matches) != 0 {
			t.Fatal("empty query did not clear find")
		}
		typeOverviewFind(m, "nothing-like-this")
		if !strings.Contains(ansi.Strip(m.View().Content), "No matches in overview") {
			t.Fatal("missing no-results feedback")
		}
		send(m, "esc")
		send(m, "j")
		send(m, "enter")
		m.View()
		if m.overviewFind.query != "" {
			t.Fatal("query leaked to another session")
		}
	}
}

func TestOverviewFindStyledTextAndResize(t *testing.T) {
	m := testModel(t)
	m.noColor = false
	m.width, m.height = 80, 24
	send(m, "enter")
	m.detail = &index.Overview{InitialRequest: scrollText() + "\nUniqueNeedle"}
	typeOverviewFind(m, "uniqueneedle")
	view := m.View().Content
	if len(m.overviewFind.matches) != 1 || !strings.Contains(ansi.Strip(view), "UniqueNeedle") || !strings.Contains(view, "4;") && !strings.Contains(view, ";4m") {
		t.Fatal("active case-insensitive match not visibly highlighted")
	}
	m.Update(tea.WindowSizeMsg{Width: 144, Height: 30})
	send(m, "]")
	if !strings.Contains(ansi.Strip(m.View().Content), "UniqueNeedle") {
		t.Fatal("resize lost find target")
	}
	typeOverviewFind(m, "38;2")
	if len(m.overviewFind.matches) != 0 {
		t.Fatal("ANSI style sequences were searchable")
	}
}

func TestOverviewFindAcrossWraps(t *testing.T) {
	for _, path := range []bool{false, true} {
		m := testModel(t)
		m.width, m.height = 40, 24
		if path {
			m.sessions[0].CWD = "/fictional/" + strings.Repeat("directory", 12) + "/final"
		}
		send(m, "enter")
		query := "silver bridge"
		m.detail = &index.Overview{InitialRequest: strings.Repeat("x", 30) + " silver bridge beyond"}
		if path {
			query = m.sessions[0].CWD[30:47]
		}
		typeOverviewFind(m, query)
		if len(m.overviewFind.matches) == 0 {
			t.Fatalf("path=%v: wrapped substring not found", path)
		}
		count := len(m.overviewFind.matches)
		multi := false
		for _, hit := range m.overviewFind.matches {
			if len(hit.parts) > 1 {
				multi = true
			}
		}
		if !multi {
			t.Fatalf("path=%v: fixture does not cross wrapping boundary", path)
		}
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		m.View()
		if len(m.overviewFind.matches) != count {
			t.Fatalf("path=%v: resize changed logical match count", path)
		}
	}
}
