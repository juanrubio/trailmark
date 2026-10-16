package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"strings"
	"testing"
)

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestQuitRequiresConfirmation(t *testing.T) {
	for _, cancel := range []string{"enter", "esc", "n"} {
		m := testModel(t)
		m.filterText, m.overviewScroll = "recent", 7
		sel := m.sel
		_, cmd := m.onKey(key("q"))
		if isQuit(cmd) || m.overlay != overlayQuit || m.quitSelected {
			t.Fatal("q must open with Cancel selected")
		}
		_, cmd = m.onKey(key("q"))
		if isQuit(cmd) || m.overlay != overlayQuit {
			t.Fatal("repeated q bypassed confirmation")
		}
		_, cmd = m.onKey(key(cancel))
		if isQuit(cmd) || m.overlay != overlayNone {
			t.Fatalf("%s did not cancel", cancel)
		}
		if m.filterText != "recent" || m.overviewScroll != 7 || m.sel != sel {
			t.Fatal("cancel changed list state")
		}
	}
	for _, confirm := range [][]string{{"y"}, {"right", "enter"}, {"tab", "enter"}} {
		m := testModel(t)
		send(m, "q")
		var cmd tea.Cmd
		for _, s := range confirm {
			_, cmd = m.onKey(tea.KeyPressMsg{Text: s})
		}
		if !isQuit(cmd) {
			t.Fatalf("%v did not quit", confirm)
		}
	}
}

func TestQuitNestedViewsAndDimensions(t *testing.T) {
	for _, open := range []string{"?", "enter", "t", "s", ","} {
		m := testModel(t)
		send(m, open)
		_, cmd := m.onKey(key("q"))
		if isQuit(cmd) || m.overlay == overlayQuit {
			t.Fatalf("q in %s opened quit", open)
		}
	}
	for _, size := range [][2]int{{40, 12}, {80, 24}, {144, 40}} {
		for _, noColor := range []bool{false, true} {
			for _, light := range []bool{false, true} {
				m := testModel(t)
				m.width, m.height = size[0], size[1]
				m.noColor, m.lightTheme = noColor, light
				send(m, "q")
				view := m.View().Content
				if !strings.Contains(view, "Quit Trailmark?") || lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
					t.Fatalf("bad quit modal %v", size)
				}
			}
		}
	}
}
