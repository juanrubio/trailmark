package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

func cachedClaudeRows(n int) []index.SessionRow {
	rows := make([]index.SessionRow, n)
	for i := range rows {
		rows[i] = index.SessionRow{SourcePath: fmt.Sprintf("/fictional/claude-%02d", i), ID: fmt.Sprint(i), CWD: fmt.Sprintf("/fictional/project-%02d", i), Provider: model.ProviderClaudeCode, LastActivity: &testNow, Purpose: fmt.Sprintf("session %02d", i)}
	}
	return rows
}

func TestBackgroundScanRemovesCachedTelemetryWithoutRefresh(t *testing.T) {
	m := testModel(t)
	rows := cachedClaudeRows(12)
	m.applySessions(rows)
	m.selector = selectorHarness
	m.overlay = overlaySelector
	m.Update(scanDoneMsg{sessions: rows[:6]})
	if len(m.sessions) != 6 || m.hasPending {
		t.Fatal("removal-only scan still waits for R")
	}
	if !strings.Contains(m.selectorOptions()[2].label, "(6)") {
		t.Fatal("open selector retained stale Claude count")
	}
	m.overlay = overlayNone
	if !strings.Contains(ansi.Strip(m.View().Content), "6 sessions") {
		t.Fatal("header retained stale session count")
	}
}

func TestBackgroundScanDefersAdditionsAndReordering(t *testing.T) {
	m := testModel(t)
	old := cachedClaudeRows(3)
	m.applySessions(old)
	fresh := cachedClaudeRows(4)
	fresh = []index.SessionRow{fresh[2], fresh[0], fresh[3]}
	newer := testNow.AddDate(0, 0, 1)
	fresh[0].LastActivity = &newer
	m.Update(scanDoneMsg{sessions: fresh})
	if len(m.sessions) != 2 || m.sessions[0].SourcePath != old[0].SourcePath || m.sessions[1].SourcePath != old[2].SourcePath {
		t.Fatal("surviving rows reordered or new row appeared before R")
	}
	if !m.sessions[1].LastActivity.Equal(testNow) || !m.hasPending || len(m.pending) != 3 {
		t.Fatal("new activity was not deferred")
	}
	send(m, "R")
	if m.hasPending || len(m.sessions) != 3 || m.sessions[0].SourcePath != fresh[0].SourcePath {
		t.Fatal("R did not apply deferred additions/order")
	}
}

func TestBackgroundRemovalRefreshesSelectionDetailAndSelector(t *testing.T) {
	m := testModel(t)
	rows := cachedClaudeRows(3)
	m.applySessions(rows)
	m.move(1)
	removed, _ := m.selected()
	m.overviewFind.query = "old query"
	m.overviewScroll = 10
	m.openSearch()
	m.search.groups = []searchGroup{{row: rows[0], field: "purpose"}, {row: removed.row, field: "purpose"}, {row: rows[2], field: "purpose"}}
	m.search.cursor = 2
	m.selector = selectorProject
	m.overlay = overlaySelector
	m.selCursor = 99
	m.Update(scanDoneMsg{sessions: []index.SessionRow{rows[0], rows[2]}})
	selected, ok := m.selected()
	if !ok || selected.row.SourcePath == removed.row.SourcePath || m.detailFor != selected.row.SourcePath || m.overviewFind.query != "" || m.overviewScroll != 0 {
		t.Fatal("removed selected session left stale detail/find state")
	}
	if len(m.search.groups) != 2 || m.search.cursor >= len(m.search.entries()) {
		t.Fatal("search retained removed session")
	}
	if m.selCursor >= len(m.selectorOptions()) || !strings.Contains(m.notice, "no longer included") {
		t.Fatal("selector cursor or removal notice not refreshed")
	}
}

func TestSuccessfulBackgroundScanSupersedesPending(t *testing.T) {
	m := testModel(t)
	old := cachedClaudeRows(2)
	m.applySessions(old)
	m.Update(scanDoneMsg{sessions: cachedClaudeRows(3)})
	if !m.hasPending {
		t.Fatal("new data should become pending")
	}
	m.Update(scanDoneMsg{sessions: old})
	if m.hasPending || len(m.pending) != 0 {
		t.Fatal("newer identical result retained obsolete pending data")
	}
	m.Update(scanDoneMsg{sessions: nil, err: fmt.Errorf("scan failed")})
	if len(m.sessions) != 2 {
		t.Fatal("failed scan removed cached rows")
	}
}

func TestBackgroundRemovalOfLastSessionClearsOverview(t *testing.T) {
	m := testModel(t)
	m.applySessions(cachedClaudeRows(1))
	m.overviewFind.query = "stale"
	m.overviewScroll = 50
	m.Update(scanDoneMsg{})
	if len(m.sessions) != 0 || m.detail != nil || m.detailFor != "" || m.overviewFind.query != "" || m.overviewScroll != 0 {
		t.Fatal("last-session removal retained stale overview")
	}
	if !strings.Contains(m.notice, "no sessions remain") {
		t.Fatal("last-session notice claimed nearest remaining session")
	}
}
