package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"image/color"
	"strings"
	"testing"
)

func TestVisualFrameDimensions(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {180, 50}, {80, 24}, {40, 12}} {
		m := testModel(t)
		m.width, m.height = size[0], size[1]
		view := m.View().Content
		if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
			t.Errorf("%v: got %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		plain := strings.Split(ansi.Strip(view), "\n")
		iw, ih, x, y := m.viewport()
		if plain[y+1] != strings.Repeat(" ", x)+strings.Repeat("─", iw)+strings.Repeat(" ", x) {
			t.Error("header rule width")
		}
		if iw >= 110 && !strings.Contains(plain[y+ih-2], strings.Repeat("─", iw)) {
			t.Error("footer not pinned")
		}
		if m.isWide() {
			for _, line := range plain[y+2 : y+ih-2] {
				if []rune(line)[x+iw*45/100+1] != '│' {
					t.Errorf("missing divider: %q", line)
					break
				}
			}
		}
		assertFrameGutters(t, m, view)
	}
}
func TestVisualThemesAndSelection(t *testing.T) {
	m := testModel(t)
	m.noColor = false
	c, _ := m.selected()
	for _, light := range []bool{false, true} {
		m.lightTheme = light
		for _, line := range m.cardLines(c, true, 50) {
			if lipgloss.Width(line) != 50 || !strings.HasPrefix(ansi.Strip(line), "▌ ") {
				t.Errorf("selected card: %q", line)
			}
		}
	}
	m.Update(tea.BackgroundColorMsg{Color: color.White})
	if !m.lightTheme {
		t.Error("light background ignored")
	}
	m.Update(tea.BackgroundColorMsg{Color: color.Black})
	if m.lightTheme {
		t.Error("dark background ignored")
	}
	m.noColor = true
	if got := m.styles().accent.GetForeground(); got != (lipgloss.NoColor{}) {
		t.Errorf("no-color accent has foreground %v", got)
	}
}

func TestVisualSecondaryDimensions(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {160, 45}, {80, 24}, {40, 12}} {
		for _, keys := range [][]string{{"w"}, {"r"}, {"t"}, {"s"}, {","}, {"enter"}} {
			m := testModel(t)
			m.width, m.height = size[0], size[1]
			for _, key := range keys {
				send(m, key)
			}
			view := m.View().Content
			assertFrameGutters(t, m, view)
			if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
				t.Errorf("%v %v: got %dx%d", size, keys, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestVisualLongCommandFallback(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 80, 24
	m.deps.copier = &fakeClipboard{err: errFakeClipboard}
	send(m, "r")
	command := "cd '/" + strings.Repeat("long-directory/", 20) + "' && codex resume session"
	m.sheet.Command = command
	send(m, "enter")
	if !strings.Contains(ansi.Strip(m.View().Content), command) {
		t.Fatal("manual clipboard fallback changed or clipped command")
	}
}
func TestVisualLongProseWrap(t *testing.T) {
	text := "/" + strings.Repeat("long-directory/", 20) + "reachable-suffix"
	lines := wrap(text, 36)
	if strings.Join(lines, "") != text {
		t.Fatal("long path lost content")
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 36 {
			t.Fatal("long path exceeds pane")
		}
	}
	m := testModel(t)
	send(m, "r")
	m.sheet.TargetDir = text
	found := false
	for i := 0; i < 60; i++ {
		m.sheetScroll = i
		if strings.Contains(ansi.Strip(m.View().Content), "reachable-suffix") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("path suffix unreachable by scrolling")
	}
}

func TestFooterShortcutColorAndPlainLabels(t *testing.T) {
	m := testModel(t)
	for _, light := range []bool{false, true} {
		m.lightTheme = light
		m.noColor = false
		styled := m.shortcutHints("  ", "Enter", "Overview", "q", "Quit")
		if ansi.Strip(styled) != "Enter Overview  q Quit" || !strings.Contains(styled, "\x1b[") {
			t.Fatal("shortcut labels or styling missing")
		}
	}
	m.noColor = true
	m.ascii = true
	m.hasPending = true
	m.partialCount = 1
	m.notice = "Information"
	m.failedRoots = []string{"/fictional/source"}
	footer := m.renderFooter(120)
	if strings.Contains(footer, "\x1b") || !strings.Contains(footer, "R to refresh") || !strings.Contains(footer, "Some logs could not be fully parsed") {
		t.Fatal("plain terminal footer lost text or contains ANSI")
	}
}

func assertFrameGutters(t *testing.T, m *appModel, view string) {
	t.Helper()
	_, _, x, y := m.viewport()
	tw, th := m.terminalSize()
	lines := strings.Split(ansi.Strip(view), "\n")
	for i, line := range lines {
		if i < y || i >= th-y {
			if strings.TrimSpace(line) != "" {
				t.Fatalf("nonempty outer row: %q", line)
			}
			continue
		}
		if !strings.HasPrefix(line, strings.Repeat(" ", x)) || !strings.HasSuffix(line, strings.Repeat(" ", x)) || lipgloss.Width(line) != tw {
			t.Fatalf("missing frame gutter: %q", line)
		}
	}
}

func TestTranscriptWordWrapPreservesParagraphsAndLongTokens(t *testing.T) {
	text := "    a model would remain readable across word boundaries\n\n    second paragraph"
	lines := wrapTranscript(text, 26)
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, "    ") {
			t.Fatal("indentation lost")
		}
		if lipgloss.Width(line) > 26 {
			t.Fatal("transcript exceeds viewport")
		}
	}
	if strings.Join(strings.Fields(strings.Join(lines, " ")), " ") != strings.Join(strings.Fields(text), " ") {
		t.Fatal("word wrapping lost text or split a word")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "\n\n") {
		t.Fatal("paragraph break lost")
	}
	token := "/" + strings.Repeat("directory/", 40)
	if strings.Join(wrapTranscript(token, 24), "") != token {
		t.Fatal("long token fallback lost content")
	}
	m := testModel(t)
	send(m, "t")
	m.transcript.msgs[0].Text = text + "\n" + token
	assertFrameGutters(t, m, m.View().Content)
}
