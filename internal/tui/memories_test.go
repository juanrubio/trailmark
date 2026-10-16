package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/memory"
	"github.com/juanrubio/trailmark/internal/model"
)

// All memory fixtures here are fictional, like the session fixtures.
func memCatalogFixture() *memory.Catalog {
	base := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	atlas := "/home/dev/repos/atlas"
	noteText := "# Preserve migration ordering\n\nAlways apply the schema migration before deploying workers.\nThe migration ordering matters."
	entries := []memory.Entry{
		{
			ID: "e-deploy", DocumentID: "d-deploy", Path: "/fake/projects/atlas/memory/deploy.md",
			Provider: model.ProviderClaudeCode, Category: memory.CategoryNote,
			Title: "Deployment recovery", Description: "Rollback order", Type: "project",
			StartLine: 1, EndLine: 6,
			Text:    "# Deployment recovery\n\nRoll back the worker deployment first, then replay the queue.",
			RawText: "---\nname: Deployment recovery\n---\n# Deployment recovery\n\nRoll back the worker deployment first, then replay the queue.",
			ModTime: base.Add(3 * time.Hour), ProjectCWD: atlas, ProjectEvidence: "session-association",
		},
		{
			ID: "e-note", DocumentID: "d-note", Path: "/fake/projects/atlas/memory/migrations.md",
			Provider: model.ProviderClaudeCode, Category: memory.CategoryNote,
			Title: "Preserve migration ordering", Description: "Schema before workers", Type: "project",
			StartLine: 1, EndLine: 8,
			Text:            noteText,
			RawText:         "---\nname: Preserve migration ordering\noriginSessionId: b1\n---\n" + noteText,
			OriginSessionID: "b1",
			ModTime:         base.Add(2 * time.Hour), ProjectCWD: atlas, ProjectEvidence: "session-association",
			References: []memory.Reference{{Raw: "deploy.md", Kind: "note", EntryID: "e-deploy", Status: "resolved"}},
		},
		{
			ID: "e-cons", DocumentID: "d-cons", Path: "/fake/memories/MEMORY.md",
			Provider: model.ProviderCodex, Category: memory.CategoryConsolidated,
			Title: "Search result caching", HeadingPath: []string{"Search result caching"},
			StartLine: 3, EndLine: 6,
			Text:            "# Search result caching\n\nCache lookup results per project.\nThe cache invalidates on refresh.",
			RawText:         "# Search result caching\n\nCache lookup results per project.\nThe cache invalidates on refresh.",
			ThreadID:        "a1",
			ModTime:         base.Add(time.Hour),
			ProjectEvidence: "unknown",
		},
		{
			ID: "e-esc", DocumentID: "d-esc", Path: "/fake/projects/atlas/memory/escapes.md",
			Provider: model.ProviderClaudeCode, Category: memory.CategoryNote,
			Title: "Escapes \x1b[31mred\x1b[0m sample", StartLine: 1, EndLine: 3,
			Text:            "Body with \x1b[2Jclear and \x1b]8;;https://example.invalid\x07link\x1b]8;;\x07 text.",
			RawText:         "# Escapes\n\nBody with \x1b[31mred\x1b[0m text.",
			ModTime:         base,
			ProjectEvidence: "unknown",
		},
		{
			ID: "e-supp", DocumentID: "d-supp", Path: "/fake/memories/rollout_summaries/fictional.md",
			Provider: model.ProviderCodex, Category: memory.CategorySupporting,
			Title: "Fictional rollout summary", StartLine: 1, EndLine: 4,
			Text: "A supporting summary with no indexed session.", RawText: "thread_id: missing-thread\n\nA supporting summary with no indexed session.",
			ThreadID:        "missing-thread",
			ModTime:         base.Add(-time.Hour),
			ProjectCWD:      "/home/dev/repos/infra",
			ProjectEvidence: "rollout-header",
		},
		{
			ID: "e-index", DocumentID: "d-index", Path: "/fake/projects/atlas/memory/MEMORY.md",
			Provider: model.ProviderClaudeCode, Category: memory.CategoryIndex,
			Title: "Memory index", StartLine: 1, EndLine: 3,
			Text:    "# Memory index\n\n- [Migrations](migrations.md)",
			RawText: "# Memory index\n\n- [Migrations](migrations.md)",
			ModTime: base.Add(-2 * time.Hour), ProjectCWD: atlas, ProjectEvidence: "session-association",
			References: []memory.Reference{{Raw: "migrations.md", Kind: "note", EntryID: "e-note", Status: "resolved"}},
		},
	}
	return &memory.Catalog{
		Entries: entries,
		Documents: []memory.Document{
			{ID: "d-note", Path: entries[1].Path, Provider: model.ProviderClaudeCode, Category: memory.CategoryNote, Status: "ok"},
			{ID: "d-supp", Path: entries[4].Path, Provider: model.ProviderCodex, Category: memory.CategorySupporting, Status: "truncated", StatusDetail: "file exceeds the 2 MiB cap; showing its readable prefix"},
		},
		Sources: []memory.SourceStatus{
			{Provider: model.ProviderClaudeCode, Root: "/fake/projects", State: "ok", Documents: 4, Entries: 4},
			{Provider: model.ProviderCodex, Root: "/fake/memories", State: "ok", Documents: 2, Entries: 2},
		},
	}
}

type memLoader struct {
	cat      *memory.Catalog
	calls    int
	snapshot []index.SessionRow
	cfgSnap  *config.Config
}

func (l *memLoader) load(_ context.Context, sessions []index.SessionRow, cfg *config.Config) (*memory.Catalog, error) {
	l.calls++
	l.snapshot = sessions
	l.cfgSnap = cfg
	return l.cat, nil
}

func memTestModel(t *testing.T) (*appModel, *memLoader) {
	t.Helper()
	m := testModel(t)
	loader := &memLoader{cat: memCatalogFixture()}
	m.deps.snapshotConfig = func() *config.Config { return &config.Config{} }
	m.deps.loadMemories = loader.load
	return m, loader
}

func openMemoriesForTest(t *testing.T, m *appModel) {
	t.Helper()
	_, cmd := m.Update(key("m"))
	if cmd == nil {
		t.Fatal("opening Memories must start a catalog load command")
	}
	m.Update(cmd())
	if !m.memoriesOpen() {
		t.Fatal("Memories must be open after m")
	}
	if m.mem.catalog == nil {
		t.Fatal("catalog must be applied after the load message")
	}
}

func selectMemoryForTest(t *testing.T, m *appModel, id string) {
	t.Helper()
	for i, e := range m.mem.visibleEntries() {
		if e.ID == id {
			m.mem.sel = i
			return
		}
	}
	t.Fatalf("entry %s not visible in %v", id, m.mem.visibleEntries())
}

func typeText(m *appModel, s string) {
	for _, r := range s {
		m.Update(key(string(r)))
	}
}

func TestMemoriesOpenListsDefaultCategories(t *testing.T) {
	m, loader := memTestModel(t)
	openMemoriesForTest(t, m)
	if loader.calls != 1 {
		t.Fatalf("loader calls = %d, want 1", loader.calls)
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Memories", "Notes and consolidated memory", "Deployment recovery", "Preserve migration ordering", "Search result caching", "atlas", "Modified:"} {
		if !strings.Contains(view, want) {
			t.Errorf("memories view missing %q", want)
		}
	}
	// Index and supporting entries stay out of the default set.
	for _, e := range m.mem.visibleEntries() {
		if e.Category == memory.CategoryIndex || e.Category == memory.CategorySupporting {
			t.Errorf("default categories must exclude %s entry %s", e.Category, e.ID)
		}
	}
	if len(m.mem.visibleEntries()) != 4 {
		t.Fatalf("visible = %d, want 4", len(m.mem.visibleEntries()))
	}
}

func TestMemoriesCarrySessionScope(t *testing.T) {
	m, _ := memTestModel(t)
	m.project = "/home/dev/repos/atlas"
	openMemoriesForTest(t, m)
	if m.mem.project != "/home/dev/repos/atlas" {
		t.Fatalf("project = %q, want carried atlas cwd", m.mem.project)
	}
	for _, e := range m.mem.visibleEntries() {
		if e.ProjectCWD != "/home/dev/repos/atlas" {
			t.Errorf("entry %s outside carried project: %q", e.ID, e.ProjectCWD)
		}
	}

	m2, _ := memTestModel(t)
	m2.project = "current"
	openMemoriesForTest(t, m2)
	if m2.mem.project != "/home/dev/repos/trailmark" {
		t.Fatalf("current project resolved to %q", m2.mem.project)
	}
}

func TestMemoriesFilterEditApplyAndCancel(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "/")
	typeText(m, "deploy")
	if got := m.mem.visibleEntries(); len(got) != 1 || got[0].ID != "e-deploy" {
		t.Fatalf("live filter visible = %v", got)
	}
	send(m, "esc") // cancel restores the previous (empty) filter
	if m.mem.filterText != "" || len(m.mem.visibleEntries()) != 4 {
		t.Fatalf("cancel must restore filter, got %q / %d entries", m.mem.filterText, len(m.mem.visibleEntries()))
	}
	send(m, "/")
	typeText(m, "caching")
	send(m, "enter")
	if m.mem.filterText != "caching" {
		t.Fatalf("filterText = %q", m.mem.filterText)
	}
	if got := m.mem.visibleEntries(); len(got) != 1 || got[0].ID != "e-cons" {
		t.Fatalf("applied filter visible = %v", got)
	}
}

func TestMemoriesSearchAndMatchNavigation(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "s")
	typeText(m, "migration")
	send(m, "enter")
	if !m.mem.searchActive || len(m.mem.matches) != 1 {
		t.Fatalf("searchActive=%v matches=%d", m.mem.searchActive, len(m.mem.matches))
	}
	mt := m.mem.matches[0]
	if mt.Entry.ID != "e-note" || mt.Count == 0 || len(mt.Lines) == 0 || len(mt.Snippets) == 0 {
		t.Fatalf("match = %+v", mt)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "matches") {
		t.Error("search list must show match counts/snippets")
	}
	send(m, "n")
	if !m.mem.raw {
		t.Error("match navigation must switch to the raw view")
	}
	if m.mem.activeLine != mt.Lines[0] {
		t.Fatalf("activeLine = %d, want %d", m.mem.activeLine, mt.Lines[0])
	}
	if len(mt.Lines) > 1 {
		send(m, "n")
		if m.mem.activeLine != mt.Lines[1] {
			t.Fatalf("second n activeLine = %d, want %d", m.mem.activeLine, mt.Lines[1])
		}
		send(m, "N")
		if m.mem.activeLine != mt.Lines[0] {
			t.Fatalf("N activeLine = %d, want %d", m.mem.activeLine, mt.Lines[0])
		}
	}
	// No-results state is explicit.
	send(m, "s")
	for range m.mem.searchQuery {
		send(m, "backspace")
	}
	typeText(m, "zzzznope")
	send(m, "enter")
	if len(m.mem.visibleEntries()) != 0 {
		t.Fatal("no-result search must list nothing")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "No matches for") {
		t.Error("no-results state must be explained")
	}
}

func TestMemoriesRawToggle(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	selectMemoryForTest(t, m, "e-note")
	if m.mem.raw {
		t.Fatal("default must be rendered, not raw")
	}
	send(m, "v")
	if !m.mem.raw {
		t.Fatal("v must switch to raw")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "name: Preserve migration ordering") {
		t.Error("raw view must show the exact source slice including frontmatter")
	}
	if !strings.Contains(view, "1 ---") {
		t.Error("raw view must carry source line numbers")
	}
	send(m, "v")
	if m.mem.raw {
		t.Fatal("v must switch back to rendered")
	}
	view = ansi.Strip(m.View().Content)
	if strings.Contains(view, "name: Preserve") {
		t.Error("rendered view must not show frontmatter body text")
	}
	if !strings.Contains(view, "Always apply the schema") {
		t.Error("rendered view must show the memory body")
	}
}

func TestMemoriesCopyExplicitOnly(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	fake := m.deps.copier.(*fakeClipboard)
	selectMemoryForTest(t, m, "e-note")
	m.View()
	if fake.text != "" {
		t.Fatal("selecting a memory must never copy")
	}
	e, _ := m.mem.selectedEntry()
	send(m, "c")
	want := e.Text + "\n\nSource: " + e.Path + ":" + itoa(e.StartLine)
	if fake.text != want {
		t.Fatalf("copied %q, want %q", fake.text, want)
	}
	if !strings.Contains(m.mem.notice, "Copied") {
		t.Fatalf("notice = %q", m.mem.notice)
	}
}

func TestMemoriesCycleHarnessProjectCategories(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "h")
	if m.mem.harness != model.ProviderClaudeCode {
		t.Fatalf("harness = %q", m.mem.harness)
	}
	for _, e := range m.mem.visibleEntries() {
		if e.Provider != model.ProviderClaudeCode {
			t.Errorf("non-Claude entry under Claude scope: %s", e.ID)
		}
	}
	send(m, "h")
	if m.mem.harness != model.ProviderCodex {
		t.Fatalf("harness = %q", m.mem.harness)
	}
	send(m, "h")
	if m.mem.harness != "" {
		t.Fatalf("harness must cycle back to all, got %q", m.mem.harness)
	}
	// Project cycle reaches Unknown/global last.
	for range 6 {
		if m.mem.project == "unknown" {
			break
		}
		send(m, "p")
	}
	if m.mem.project != "unknown" {
		t.Fatalf("project cycle never reached unknown: %q", m.mem.project)
	}
	for _, e := range m.mem.visibleEntries() {
		if e.ProjectCWD != "" {
			t.Errorf("unknown scope must exclude associated entry %s", e.ID)
		}
	}
	send(m, "p")
	if m.mem.project != "" {
		t.Fatalf("project must cycle back to all, got %q", m.mem.project)
	}
	// Category cycle: All -> Indexes+overviews -> Supporting+raw -> default.
	send(m, "a")
	if len(m.mem.visibleEntries()) != 6 {
		t.Fatalf("All categories visible = %d, want 6", len(m.mem.visibleEntries()))
	}
	var indexEntry *memory.Entry
	for i, e := range m.mem.visibleEntries() {
		if e.Category == memory.CategoryIndex {
			indexEntry = &m.mem.visibleEntries()[i]
		}
	}
	if indexEntry == nil || memCategoryLabel(indexEntry.Category) != "Index" {
		t.Fatal("MEMORY.md entry must be labelled Index")
	}
	send(m, "a")
	if got := m.mem.visibleEntries(); len(got) != 1 || got[0].ID != "e-index" {
		t.Fatalf("index/overview set = %v", got)
	}
	send(m, "a")
	if got := m.mem.visibleEntries(); len(got) != 1 || got[0].ID != "e-supp" {
		t.Fatalf("supporting/raw set = %v", got)
	}
	send(m, "a")
	if m.mem.catIdx != 0 || len(m.mem.visibleEntries()) != 4 {
		t.Fatalf("category cycle must return to default, idx=%d visible=%d", m.mem.catIdx, len(m.mem.visibleEntries()))
	}
}

func TestMemoriesRefreshRetainsSelectionByID(t *testing.T) {
	m, loader := memTestModel(t)
	openMemoriesForTest(t, m)
	selectMemoryForTest(t, m, "e-note")
	extra := memory.Entry{
		ID: "e-newer", DocumentID: "d-newer", Path: "/fake/projects/atlas/memory/newer.md",
		Provider: model.ProviderClaudeCode, Category: memory.CategoryNote, Title: "A newer note",
		StartLine: 1, EndLine: 2, Text: "Newer fictional note.", RawText: "Newer fictional note.",
		ModTime: time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local), ProjectCWD: "/home/dev/repos/atlas",
	}
	fresh := memCatalogFixture()
	fresh.Entries = append([]memory.Entry{extra}, fresh.Entries...)
	loader.cat = fresh
	_, cmd := m.Update(key("R"))
	if cmd == nil {
		t.Fatal("R must start an explicit refresh load")
	}
	m.Update(cmd())
	e, ok := m.mem.selectedEntry()
	if !ok || e.ID != "e-note" {
		t.Fatalf("selection after refresh = %v, want e-note retained", e.ID)
	}
}

func TestMemoriesNoteReferenceNavigation(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	selectMemoryForTest(t, m, "e-note")
	// e-note has two openable references (the deploy note and its
	// origin session), so o opens the reference chooser; the note
	// is listed first.
	send(m, "o")
	if m.overlay != overlaySelector || m.selector != selectorReference {
		t.Fatal("multiple references must open the reference chooser")
	}
	send(m, "enter")
	if !m.memoriesOpen() {
		t.Fatal("note reference must stay inside Memories")
	}
	e, _ := m.mem.selectedEntry()
	if e.ID != "e-deploy" {
		t.Fatalf("selected = %s, want referenced e-deploy", e.ID)
	}
}

func TestMemoriesSessionReferenceUnique(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	// Hide the target (Codex a1) behind session filters; the memory
	// scope was already carried at open, so this only affects the
	// session list the reference navigation must clear.
	m.harness = model.ProviderClaudeCode
	m.filterText = "zzzz"
	m.windowDays = 1
	selectMemoryForTest(t, m, "e-cons") // ThreadID a1, Codex
	send(m, "o")
	if m.memoriesOpen() {
		t.Fatal("unique session reference must close Memories")
	}
	if m.harness != "" || m.project != "" || m.filterText != "" || m.windowDays != 0 {
		t.Fatalf("session filters must clear to All: harness=%q project=%q filter=%q days=%d", m.harness, m.project, m.filterText, m.windowDays)
	}
	c, ok := m.selected()
	if !ok || c.row.ID != "a1" {
		t.Fatalf("selected session = %v, want a1", c.row.ID)
	}
	if !strings.Contains(m.notice, "Opened the session") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestMemoriesSessionReferenceAmbiguousAndMissing(t *testing.T) {
	m, _ := memTestModel(t)
	m.sessions = append(m.sessions, index.SessionRow{SourcePath: "/logs/a2.jsonl", Provider: model.ProviderCodex, ID: "a1", Purpose: "Duplicate identifier session", CWD: "/home/dev/repos/infra", LastActivity: at(15, 11)})
	openMemoriesForTest(t, m)
	selectMemoryForTest(t, m, "e-cons")
	send(m, "o")
	if !m.memoriesOpen() {
		t.Fatal("ambiguous reference must not navigate")
	}
	if !strings.Contains(m.mem.notice, "ambiguous") {
		t.Fatalf("notice = %q", m.mem.notice)
	}
	// Missing: supporting entry whose thread is not indexed.
	send(m, "a") // All categories
	selectMemoryForTest(t, m, "e-supp")
	send(m, "o")
	if !m.memoriesOpen() || !strings.Contains(m.mem.notice, "not in the indexed sessions") {
		t.Fatalf("missing reference notice = %q", m.mem.notice)
	}
}

func TestMemoriesEscRestoresSessionState(t *testing.T) {
	m, _ := memTestModel(t)
	m.sel = 2
	m.refreshDetail()
	before, _ := m.selected()
	openMemoriesForTest(t, m)
	send(m, "j")
	send(m, "j")
	send(m, "h") // memory-only scope change
	send(m, "esc")
	if m.memoriesOpen() {
		t.Fatal("esc from the list must return to Sessions")
	}
	after, _ := m.selected()
	if after.row.SourcePath != before.row.SourcePath || m.sel != 2 {
		t.Fatalf("session selection changed: %s -> %s (sel %d)", before.row.SourcePath, after.row.SourcePath, m.sel)
	}
	if m.harness != "" {
		t.Fatal("memory harness cycling must not touch the session harness filter")
	}
	// Reopening preserves memories state without reloading.
	_, cmd := m.Update(key("m"))
	if cmd != nil {
		t.Fatal("reopening with a loaded catalog must not reload")
	}
	if m.mem.harness != model.ProviderClaudeCode {
		t.Fatalf("memories state not preserved: harness %q", m.mem.harness)
	}
	// m toggles back to Sessions too.
	send(m, "m")
	if m.memoriesOpen() {
		t.Fatal("m inside Memories must return to Sessions")
	}
}

func TestMemoriesNarrowOpenAndBack(t *testing.T) {
	m, _ := memTestModel(t)
	m.width, m.height = 80, 24
	openMemoriesForTest(t, m)
	if m.mem.detailOpen {
		t.Fatal("narrow view must start on the list")
	}
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "References") {
		t.Fatal("narrow list must not render the detail pane")
	}
	send(m, "enter")
	if !m.mem.detailOpen {
		t.Fatal("enter must open the narrow detail")
	}
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Source:") || !strings.Contains(view, "Modified:") {
		t.Error("narrow detail must show source metadata")
	}
	send(m, "esc")
	if m.mem.detailOpen || !m.memoriesOpen() {
		t.Fatal("first esc must return to the memory list")
	}
	send(m, "esc")
	if m.memoriesOpen() {
		t.Fatal("second esc must return to Sessions")
	}
}

func TestMemoriesSourcesOverlay(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "i")
	if !m.mem.sourcesOpen {
		t.Fatal("i must open the sources overlay")
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Memory sources", "/fake/projects", "/fake/memories", "State: ok", "truncated"} {
		if !strings.Contains(view, want) {
			t.Errorf("sources overlay missing %q", want)
		}
	}
	send(m, "esc")
	if m.mem.sourcesOpen || !m.memoriesOpen() {
		t.Fatal("esc must close only the sources overlay")
	}
}

func TestMemoriesSanitizesSourceEscapes(t *testing.T) {
	m, _ := memTestModel(t)
	m.noColor = true // plain fallback path, still must be sanitized
	openMemoriesForTest(t, m)
	selectMemoryForTest(t, m, "e-esc")
	for _, raw := range []bool{false, true} {
		m.mem.raw = raw
		view := m.View().Content
		for _, bad := range []string{"\x1b[31m", "\x1b[2J", "\x1b]8", "https://example.invalid\x07"} {
			if strings.Contains(view, bad) {
				t.Errorf("raw=%v: source escape %q leaked into the view", raw, bad)
			}
		}
		if !strings.Contains(ansi.Strip(view), "red") {
			t.Errorf("raw=%v: sanitized text must remain readable", raw)
		}
	}
}

func TestMemoriesSizesDoNotOverflow(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 12}, {120, 40}} {
		m, _ := memTestModel(t)
		m.width, m.height = size[0], size[1]
		openMemoriesForTest(t, m)
		for _, stage := range []string{"list", "detail", "sources"} {
			switch stage {
			case "detail":
				send(m, "enter")
				send(m, "v")
			case "sources":
				send(m, "esc")
				if m.mem.focus == memFocusDetail {
					send(m, "esc")
				}
				if !m.memoriesOpen() {
					m.Update(key("m"))
				}
				send(m, "i")
			}
			view := m.View().Content
			if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
				t.Errorf("%v %s: got %dx%d", size, stage, lipgloss.Width(view), lipgloss.Height(view))
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size[0] {
					t.Errorf("%v %s: line width %d exceeds %d: %q", size, stage, lipgloss.Width(line), size[0], ansi.Strip(line))
					break
				}
			}
		}
	}
}

func TestMemoriesFocusTabAndDetailScroll(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	m.View() // populate detailHeight
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.mem.focus != memFocusDetail {
		t.Fatal("tab must focus the detail pane on wide terminals")
	}
	selBefore := m.mem.sel
	send(m, "j")
	if m.mem.sel != selBefore {
		t.Fatal("j with detail focus must scroll, not move selection")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.mem.focus != memFocusList {
		t.Fatal("tab must return focus to the list")
	}
	send(m, "j")
	if m.mem.sel != selBefore+1 {
		t.Fatal("j with list focus must move selection")
	}
}

func TestMemoriesWithoutLoaderIsExplained(t *testing.T) {
	m := testModel(t) // no loadMemories dependency
	_, cmd := m.Update(key("m"))
	if cmd != nil {
		t.Fatal("missing loader must not start a command")
	}
	if !m.memoriesOpen() {
		t.Fatal("Memories must still open and explain itself")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Memory sources are unavailable") {
		t.Error("missing loader must be explained in the view")
	}
}

func TestMemoriesHelpReachable(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "?")
	if m.overlay != overlayHelp {
		t.Fatal("? must open help from Memories")
	}
	// Help body itself (not the height-truncated frame) lists the keys.
	if help := m.viewHelp(80, 40); !strings.Contains(help, "Memories:") || !strings.Contains(help, "Refresh memories") {
		t.Error("help must include the Memories section")
	}
	send(m, "esc")
	if m.overlay != overlayNone || !m.memoriesOpen() {
		t.Fatal("closing help must return to Memories")
	}
}
