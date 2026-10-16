package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/index"
)

func scrollText() string {
	var lines []string
	for i := 0; i < 120; i++ {
		lines = append(lines, fmt.Sprintf("line %03d", i))
	}
	return strings.Join(lines, "\n")
}

func TestTranscriptScrollBoundariesAndResize(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 80, 24
	c, _ := m.selected()
	m.transcript = &transcriptState{row: c.row, msgs: []index.TranscriptMessage{{Seq: 1, Kind: "message", Role: "user", Text: scrollText()}}, expanded: map[int]bool{}}
	view := func() string { return ansi.Strip(m.View().Content) }
	start := view()
	send(m, "J")
	if view() == start {
		t.Fatal("first J at first message did not scroll")
	}
	for i := 0; i < 100; i++ {
		send(m, "J")
	}
	bottom := view()
	send(m, "K")
	if view() == bottom {
		t.Fatal("K after overscrolling bottom did not immediately move")
	}
	for i := 0; i < 100; i++ {
		send(m, "K")
	}
	top := view()
	send(m, "J")
	if view() == top {
		t.Fatal("J after overscrolling top did not immediately move")
	}
	m.onKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	page := view()
	m.onKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if view() == page {
		t.Fatal("page up did not move")
	}
	for i := 0; i < 100; i++ {
		send(m, "J")
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 50})
	bottom = view()
	send(m, "K")
	if view() == bottom {
		t.Fatal("resize retained hidden overscroll")
	}
	m.transcript.matches = []int{1}
	m.transcript.matchPos = 0
	m.transcript.query = "line 000"
	m.transcriptRevealMatch(m.transcript)
	if m.transcript.lineScroll != 0 || !strings.Contains(view(), "line 000") {
		t.Fatal("match jump retained scroll offset")
	}
}

func TestOverviewScrollBoundariesAndResize(t *testing.T) {
	for _, width := range []int{80, 144} {
		m := testModel(t)
		m.width, m.height = width, 24
		send(m, "enter")
		m.detail = &index.Overview{InitialRequest: scrollText()}
		view := func() string { return ansi.Strip(m.View().Content) }
		for i := 0; i < 200; i++ {
			send(m, "down")
		}
		bottom := view()
		send(m, "up")
		if view() == bottom {
			t.Fatalf("%d: up after bottom overscroll did not move", width)
		}
		for i := 0; i < 200; i++ {
			send(m, "up")
		}
		top := view()
		send(m, "down")
		if view() == top {
			t.Fatalf("%d: down after top overscroll did not move", width)
		}
		m.onKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
		page := view()
		m.onKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
		if view() == page {
			t.Fatal("overview page up did not move")
		}
		for i := 0; i < 200; i++ {
			send(m, "down")
		}
		m.Update(tea.WindowSizeMsg{Width: width, Height: 50})
		bottom = view()
		send(m, "up")
		if view() == bottom {
			t.Fatal("overview resize retained hidden overscroll")
		}
	}
}

func TestTranscriptPreviousPageMovesOnFirstKey(t *testing.T) {
	m := testModel(t)
	c, _ := m.selected()
	m.transcript = &transcriptState{row: c.row, msgs: append([]index.TranscriptMessage(nil), fixtureMsgs[3:]...), offset: 3, total: len(fixtureMsgs), expanded: map[int]bool{}}
	before, _ := m.transcript.current()
	send(m, "k")
	after, ok := m.transcript.current()
	if !ok || after.Seq >= before.Seq {
		t.Fatal("first k at page boundary did not select preceding message")
	}
}

func TestTranscriptPageKeysCrossLoadedWindows(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 80, 24
	msgs := make([]index.TranscriptMessage, 850)
	for i := range msgs {
		msgs[i] = index.TranscriptMessage{Seq: i + 1, Kind: "message", Role: "user", Text: fmt.Sprintf("message %04d", i+1)}
	}
	m.deps.page = func(_ context.Context, _ string, offset, limit int) ([]index.TranscriptMessage, int, error) {
		end := min(len(msgs), offset+limit)
		return append([]index.TranscriptMessage(nil), msgs[offset:end]...), len(msgs), nil
	}
	c, _ := m.selected()
	m.openTranscript(c.row, 0, false)
	page := func(code rune) {
		m.onKey(tea.KeyPressMsg{Code: code})
		if len(m.transcript.msgs) > maxLoadedMessages {
			t.Fatal("paging exceeded memory bound")
		}
	}
	for i := 0; i < 250; i++ {
		page(tea.KeyPgDown)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "message 0850") || m.transcript.offset == 0 {
		t.Fatal("page down stopped at loaded boundary")
	}
	page(tea.KeyPgUp)
	if ansi.Strip(m.View().Content) == view {
		t.Fatal("reverse page stuck after final message")
	}
	for i := 0; i < 250; i++ {
		page(tea.KeyPgUp)
	}
	if m.transcript.offset != 0 || !strings.Contains(ansi.Strip(m.View().Content), "message 0001") {
		t.Fatal("page up did not reload evicted history")
	}
}
