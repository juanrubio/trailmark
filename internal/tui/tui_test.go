package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/juanrubio/trailmark/internal/continuation"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

var testNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)

func at(day, hour int) *time.Time {
	t := time.Date(2026, 10, day, hour, 0, 0, 0, time.Local)
	return &t
}

func fixtureSessions() []index.SessionRow {
	return []index.SessionRow{
		{SourcePath: "/logs/a.jsonl", Provider: model.ProviderCodex, ID: "a1", Purpose: "Improve the recent-session UX", PurposeOrigin: model.OriginNativeTitle, CWD: "/home/dev/repos/trailmark", LastActivity: at(15, 10)},
		{SourcePath: "/logs/b.jsonl", Provider: model.ProviderClaudeCode, ID: "b1", Purpose: "Repair the deployment flow", PurposeOrigin: model.OriginRequestExcerpt, CWD: "/home/dev/repos/music-lab", LastActivity: at(15, 8)},
		{SourcePath: "/logs/c.jsonl", Provider: model.ProviderCodex, ID: "c1", Purpose: "Investigate refresh failures", CWD: "/home/dev/worktrees/dashboard-fix", LastActivity: at(14, 17)},
		{SourcePath: "/logs/old.jsonl", Provider: model.ProviderCodex, ID: "old1", Purpose: "Ancient cleanup task", CWD: "/home/dev/repos/infra", LastActivity: at(1, 9)},
		{SourcePath: "/logs/parent.jsonl", Provider: model.ProviderCodex, ID: "p1", Purpose: "Parent investigation", CWD: "/home/dev/repos/infra", LastActivity: at(15, 9)},
		{SourcePath: "/logs/child.jsonl", Provider: model.ProviderCodex, ID: "ch1", ParentID: "p1", Purpose: "Child subtask", CWD: "/home/dev/repos/infra", LastActivity: at(15, 9)},
		{SourcePath: "/logs/orphan.jsonl", Provider: model.ProviderCodex, ID: "or1", ParentID: "missing", Purpose: "Orphaned child work", CWD: "/home/dev/repos/infra", LastActivity: at(15, 7)},
	}
}

func fixtureOverview(ctx context.Context, path string) (*index.Overview, error) {
	return &index.Overview{
		InitialRequest:  "Make recent work easy to recognize",
		LatestUser:      "Continue with this work",
		LatestAssistant: "Updated the overview design",
		Initial:         index.Excerpt{Text: "Make recent work easy to recognize", Line: 2, RecordID: "u-1"},
		LatestUserEx:    index.Excerpt{Text: "Continue with this work", Line: 9},
		LatestAsstEx:    index.Excerpt{Text: "Updated the overview design", Line: 10},
	}, nil
}

func testModel(t *testing.T) *appModel {
	t.Helper()
	continuation.DirExists = func(string) bool { return true }
	m := newModel(context.Background(), fixtureOverview, nil, "/home/dev/repos/trailmark", "/home/dev", fixtureSessions(), testCaps(), &fakeClipboard{})
	m.now = func() time.Time { return testNow }
	m.width, m.height = 120, 40
	return m
}

func key(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return tea.KeyPressMsg{Text: s}
}

func send(m *appModel, s string) {
	m.Update(key(s))
}

func TestDefaultWindowIsSevenDaysGrouped(t *testing.T) {
	m := testModel(t)
	cards := m.cards()
	// 7-day window excludes the Oct 1 session; child collapses under parent.
	var purposes []string
	for _, c := range cards {
		purposes = append(purposes, c.row.Purpose)
	}
	joined := strings.Join(purposes, " | ")
	if strings.Contains(joined, "Ancient cleanup") {
		t.Error("old session must be outside the 7-day window")
	}
	if strings.Contains(joined, "Child subtask") {
		t.Error("child must be collapsed under its parent by default")
	}
	if !strings.Contains(joined, "Orphaned child work") {
		t.Error("orphan child must stay visible as its own entry")
	}
	view := m.View().Content
	for _, want := range []string{"Today", "Yesterday", "Improve the recent-session UX", "trailmark", "Last 7 days"} {
		if !strings.Contains(view, want) {
			t.Errorf("wide view missing %q", want)
		}
	}
}

func TestExpandChildrenWithE(t *testing.T) {
	m := testModel(t)
	// Move selection to the parent card.
	for i, c := range m.cards() {
		if c.row.ID == "p1" {
			m.sel = i
		}
	}
	send(m, "e")
	found := false
	for _, c := range m.cards() {
		if c.row.ID == "ch1" && c.child {
			found = true
		}
	}
	if !found {
		t.Error("child must appear after e expands the parent")
	}
}

func TestHarnessSelectorFilters(t *testing.T) {
	m := testModel(t)
	send(m, "h")
	if m.overlay != overlaySelector {
		t.Fatal("h must open the harness selector")
	}
	send(m, "j") // Codex
	send(m, "enter")
	if m.harness != model.ProviderCodex {
		t.Fatalf("harness = %q", m.harness)
	}
	for _, c := range m.cards() {
		if c.row.Provider != model.ProviderCodex {
			t.Errorf("non-Codex card visible: %s", c.row.Purpose)
		}
	}
	if !strings.Contains(m.View().Content, "Codex") {
		t.Error("header must show the active harness")
	}
}

func TestTimeSelectorAllTimeShowsOld(t *testing.T) {
	m := testModel(t)
	send(m, "w")
	// Options: Today, 3, 7, 30, All time -> cursor starts at current (7 days, index 2)
	send(m, "j")
	send(m, "j")
	send(m, "enter")
	if m.windowDays != 0 {
		t.Fatalf("windowDays = %d, want All time", m.windowDays)
	}
	found := false
	for _, c := range m.cards() {
		if c.row.ID == "old1" {
			found = true
		}
	}
	if !found {
		t.Error("All time must include the October 1 session")
	}
}

func TestMetadataFilterPreservesOrder(t *testing.T) {
	m := testModel(t)
	send(m, "/")
	for _, r := range "deployment" {
		send(m, string(r))
	}
	send(m, "enter")
	cards := m.cards()
	if len(cards) != 1 || cards[0].row.ID != "b1" {
		t.Fatalf("cards = %+v, want only the deployment session", cards)
	}
}

func TestPendingRefreshKeepsSelectionUntilR(t *testing.T) {
	m := testModel(t)
	m.sel = 1
	before, _ := m.selected()
	fresh := fixtureSessions()
	fresh[0], fresh[1] = fresh[1], fresh[0] // reorder
	m.Update(scanDoneMsg{sessions: fresh})
	if !m.hasPending {
		t.Fatal("changed ordering must set the pending flag")
	}
	still, _ := m.selected()
	if still.row.SourcePath != before.row.SourcePath {
		t.Error("selection must not move before R applies the refresh")
	}
	if !strings.Contains(m.View().Content, "New activity") {
		t.Error("pending refresh must be announced")
	}
	send(m, "R")
	if m.hasPending {
		t.Error("R must clear the pending flag")
	}
	after, _ := m.selected()
	if after.row.SourcePath != before.row.SourcePath {
		t.Error("R must restore selection to the same session")
	}
}

func TestNarrowOverviewOpenAndBack(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 80, 24
	send(m, "enter")
	if !m.overviewOpen {
		t.Fatal("Enter at 80 columns must open the full overview")
	}
	view := m.View().Content
	for _, want := range []string{"Directory", "/home/dev/repos/trailmark", "Initial request", "Latest exchange"} {
		if !strings.Contains(view, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	send(m, "esc")
	if m.overviewOpen {
		t.Error("Esc must return to the list")
	}
}

func TestResizePreservesSelection(t *testing.T) {
	m := testModel(t)
	m.sel = 2
	before, _ := m.selected()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 50})
	after, _ := m.selected()
	if after.row.SourcePath != before.row.SourcePath {
		t.Error("resize must preserve the selected session")
	}
}

func TestEmptyWindowState(t *testing.T) {
	m := testModel(t)
	send(m, "w")
	send(m, "k") // 3 days
	send(m, "k") // Today
	send(m, "enter")
	// Today still has sessions; use the filter to force an empty state.
	m.filterText = "no-such-text"
	if !strings.Contains(m.View().Content, "No sessions match") {
		t.Error("empty filter state must be explicit")
	}
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func TestOrphanChildLabeledInList(t *testing.T) {
	m := testModel(t)
	view := m.View().Content
	for _, c := range m.cards() {
		if c.row.ID == "or1" && !c.child {
			t.Error("orphan child card must be flagged as a child entry")
		}
	}
	if !strings.Contains(view, "child session") {
		t.Error("list must label the orphan child entry")
	}
}

func TestResizeAcrossBoundaryKeepsOverviewReachable(t *testing.T) {
	m := testModel(t)
	send(m, "enter") // wide: focus overview
	if m.focus != focusOverview {
		t.Fatal("Enter in wide layout must focus the overview")
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !m.overviewOpen {
		t.Error("narrowing with overview focus must open the full overview, not strand focus")
	}
	send(m, "j") // scrolls the visible overview, not the hidden list
	if m.sel != 0 {
		t.Error("j in the narrow overview must not move list selection")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.overviewOpen || m.focus != focusOverview {
		t.Error("widening must restore the side-pane overview focus")
	}
}

func TestFirstScanAppliesImmediately(t *testing.T) {
	m := newModel(context.Background(), fixtureOverview, nil, "/home/dev/repos/trailmark", "/home/dev", nil, testCaps(), &fakeClipboard{})
	m.now = func() time.Time { return testNow }
	m.Update(scanDoneMsg{sessions: fixtureSessions()})
	if m.hasPending {
		t.Error("first scan on an empty index must not hide behind R")
	}
	if len(m.cards()) == 0 {
		t.Error("first scan results must be visible immediately")
	}
}

func TestWideFrameFitsWidth(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 110, 40
	for _, line := range strings.Split(m.View().Content, "\n") {
		if w := lipglossWidth(line); w > 110 {
			t.Errorf("line exceeds 110 cols (%d): %q", w, line)
		}
	}
	m.width, m.height = 80, 24
	view := m.View().Content
	if !strings.Contains(view, "? Help") || !strings.Contains(view, "q Quit") {
		t.Error("80x24 footer must show Help and Quit actions")
	}
	for _, line := range strings.Split(view, "\n") {
		if w := lipglossWidth(line); w > 80 {
			t.Errorf("line exceeds 80 cols (%d): %q", w, line)
		}
	}
}

func TestHelpOverlay(t *testing.T) {
	m := testModel(t)
	send(m, "?")
	if !strings.Contains(m.View().Content, "Keys") {
		t.Error("help overlay must render")
	}
	send(m, "esc")
	if m.overlay != overlayNone {
		t.Error("Esc must close help")
	}
}

type fakeClipboard struct {
	text string
	err  error
}

func (f *fakeClipboard) Copy(text string) error {
	if f.err != nil {
		return f.err
	}
	f.text = text
	return nil
}

func testCaps() map[model.Provider]model.Capability {
	return map[model.Provider]model.Capability{
		model.ProviderClaudeCode: {Provider: model.ProviderClaudeCode, ResumeTemplate: "claude --resume {id}", ForkTemplate: "claude --resume {id} --fork-session", ForkSupported: true, CleanTemplate: "claude"},
		model.ProviderCodex:      {Provider: model.ProviderCodex, ResumeTemplate: "codex resume {id}", ForkTemplate: "codex fork {id}", ForkSupported: true, CleanTemplate: "codex"},
	}
}

func TestResumeSheetShowsCommandAndCopies(t *testing.T) {
	m := testModel(t)
	send(m, "r")
	if m.overlay != overlaySheet || m.sheet == nil {
		t.Fatal("r must open the resume sheet")
	}
	if !m.sheet.Available {
		t.Fatalf("sheet unavailable: %s", m.sheet.Reason)
	}
	view := m.View().Content
	for _, want := range []string{"Resume session", "/home/dev/repos/trailmark", "Copy command"} {
		if !strings.Contains(view, want) {
			t.Errorf("sheet missing %q", want)
		}
	}
	fake := m.copier.(*fakeClipboard)
	send(m, "enter")
	if m.sheetState != "copied" || !strings.Contains(fake.text, "codex resume") {
		t.Errorf("copied = %q state=%q", fake.text, m.sheetState)
	}
	send(m, "esc")
	if m.overlay != overlayNone {
		t.Error("Esc must close the sheet")
	}
}

func TestSheetClipboardFailureShowsSelectableText(t *testing.T) {
	m := testModel(t)
	m.copier = &fakeClipboard{err: errFakeClipboard}
	send(m, "r")
	send(m, "enter")
	if m.sheetState != "failed" {
		t.Fatalf("state = %q, want failed", m.sheetState)
	}
	view := m.View().Content
	if !strings.Contains(view, "Copy failed") || !strings.Contains(view, m.sheet.Command) {
		t.Error("failure state must show the command as selectable text")
	}
}

func TestCopyMenuPreviewHasProvenance(t *testing.T) {
	m := testModel(t)
	send(m, "c")
	if m.overlay != overlayCopyMenu {
		t.Fatal("c must open the copy menu")
	}
	send(m, "j") // initial request
	send(m, "enter")
	if m.overlay != overlayCopyPreview || m.copyPreview == nil {
		t.Fatal("Enter must show the exact copy preview")
	}
	view := m.View().Content
	for _, want := range []string{"Provider:", "Session:", "Source:", "exactly what will be copied"} {
		if !strings.Contains(view, want) {
			t.Errorf("preview missing provenance %q", want)
		}
	}
	send(m, "enter")
	fake := m.copier.(*fakeClipboard)
	if !strings.Contains(fake.text, "Make recent work easy to recognize") {
		t.Errorf("copied text = %q", fake.text)
	}
	send(m, "esc")
	if m.overlay != overlayCopyMenu {
		t.Error("Esc from preview must return to the copy menu")
	}
}

var errFakeClipboard = errClipboard("unavailable")

type errClipboard string

func (e errClipboard) Error() string { return string(e) }

func TestFailedSheetShowsExactCommand(t *testing.T) {
	m := testModel(t)
	m.copier = &fakeClipboard{err: errFakeClipboard}
	send(m, "r")
	send(m, "enter")
	if m.sheetState != "failed" {
		t.Fatalf("state = %q", m.sheetState)
	}
	if !strings.Contains(m.View().Content, m.sheet.Command) {
		t.Error("failed sheet must contain the exact unbroken command for manual selection")
	}
}

func TestForkUnavailableSheetInTUI(t *testing.T) {
	m := testModel(t)
	caps := testCaps()
	c := caps[model.ProviderCodex]
	c.ForkSupported = false
	c.ForkTemplate = ""
	caps[model.ProviderCodex] = c
	m.caps = caps
	send(m, "f")
	if m.sheet == nil || m.sheet.Available {
		t.Fatal("fork sheet must be unavailable")
	}
	view := m.View().Content
	if !strings.Contains(view, "Fork unavailable") || strings.Contains(view, "Copy command") {
		t.Error("unavailable sheet must explain and not advertise copying")
	}
}

func TestEscFromSheetPreservesFilterAndSelection(t *testing.T) {
	m := testModel(t)
	m.filterText = "deployment"
	m.afterFilterChange()
	m.sel = 0
	send(m, "r")
	send(m, "esc")
	if m.filterText != "deployment" {
		t.Errorf("filter lost: %q", m.filterText)
	}
	if c, ok := m.selected(); !ok || c.row.ID != "b1" {
		t.Error("selection must be restored after Esc")
	}
}

func TestFilterInputSwallowsActionKeys(t *testing.T) {
	m := testModel(t)
	send(m, "/")
	send(m, "r")
	send(m, "c")
	if m.overlay != overlayFilter || m.sheet != nil {
		t.Error("action keys typed in the filter must not open sheets")
	}
	if m.filterText != "rc" {
		t.Errorf("filter text = %q, want typed runes", m.filterText)
	}
	send(m, "esc")
}

func TestCopyPayloadBytesExactWithWhitespace(t *testing.T) {
	m := testModel(t)
	send(m, "c")
	send(m, "j")
	send(m, "enter")
	if m.copyPreview == nil {
		t.Fatal("preview missing")
	}
	send(m, "enter")
	fake := m.copier.(*fakeClipboard)
	if fake.text != m.copyPreview.text {
		t.Error("copied bytes must equal the previewed payload exactly")
	}
	if !strings.Contains(m.View().Content, "exactly what will be copied") {
		t.Error("preview heading missing")
	}
}
