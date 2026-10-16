package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
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

func TestQuitModalControlsReadableAcrossThemes(t *testing.T) {
	for _, name := range []string{"green", "blue", "amber", "monokai"} {
		for _, size := range [][2]int{{40, 12}, {80, 24}} {
			for _, light := range []bool{false, true} {
				for _, plain := range []bool{false, true} {
					m := testModel(t)
					m.width, m.height = size[0], size[1]
					m.themeName = name
					m.lightTheme = light
					m.noColor = plain
					m.ascii = plain
					send(m, "q")
					for _, selected := range []bool{false, true} {
						m.quitSelected = selected
						view := m.View().Content
						text := ansi.Strip(view)
						for _, want := range []string{"Quit Trailmark?", "Leave the session browser?", "Cancel]", "Quit]", "Tab Choose", "Enter Confirm", "Esc Cancel"} {
							if !strings.Contains(text, want) {
								t.Fatalf("%s/%v light=%v plain=%v missing %q: %s", name, size, light, plain, want, text)
							}
						}
						focus := "> Cancel"
						if selected {
							focus = "> Quit"
						}
						if !strings.Contains(text, focus) {
							t.Fatalf("missing focus marker %s", focus)
						}
						if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
							t.Fatal("modal overflow")
						}
						if plain && (strings.Contains(view, "38;2;") || strings.Contains(view, "48;2;")) {
							t.Fatal("plain modal added color")
						}
					}
				}
			}
		}
	}
}

func TestQuitRaisedSurfaceStaysInsideCard(t *testing.T) {
	for _, light := range []bool{false, true} {
		m := testModel(t)
		m.width, m.height = 80, 24
		m.themeName = "monokai"
		m.lightTheme = light
		m.noColor = false
		send(m, "q")
		screen := uv.NewRenderBuffer(80, 24)
		uv.NewStyledString(m.View().Content).Draw(terminalTestScreen{screen}, uv.Rect(0, 0, 80, 24))
		want := lipgloss.Color(m.palette().panel)
		wr, wg, wb, _ := want.RGBA()
		// At this width the card ends before column 65. Check every row's
		// surrounding padding, where a leaked raised background is visible.
		for y := 0; y < 24; y++ {
			for _, x := range []int{5, 70} {
				cell := screen.CellAt(x, y)
				if cell == nil || cell.Style.Bg == nil {
					t.Fatalf("missing base background at %d,%d", x, y)
				}
				r, g, b, _ := cell.Style.Bg.RGBA()
				if r != wr || g != wg || b != wb {
					t.Fatalf("card background leaked into padding at %d,%d", x, y)
				}
			}
		}
	}
}
