package tui

// Regression tests for reviewer findings: stale refresh results and
// one-shot search-match reveal.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/juanrubio/trailmark/internal/memory"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestMemoriesStaleRefreshResultDiscarded(t *testing.T) {
	m, loader := memTestModel(t)
	openMemoriesForTest(t, m)
	original := m.mem.catalog

	// A result from an older generation must not replace the catalog.
	m.Update(memoriesLoadedMsg{gen: m.mem.loadGen - 1, catalog: &memory.Catalog{}})
	if m.mem.catalog != original {
		t.Fatal("stale load result replaced the current catalog")
	}

	// The latest generation still applies.
	_, cmd := m.Update(key("R"))
	if cmd == nil {
		t.Fatal("R must start a refresh load")
	}
	fresh := &memory.Catalog{}
	loader.cat = fresh
	m.Update(cmd())
	if m.mem.catalog != fresh {
		t.Fatal("latest load result was not applied")
	}
}

func TestMemoriesProjectPathSanitizedEverywhere(t *testing.T) {
	m, loader := memTestModel(t)
	evil := "/fake/\x1b[31mevil\x1b]8;;https://example.invalid\x07proj"
	loader.cat = &memory.Catalog{Entries: []memory.Entry{{
		ID: "e-evil", DocumentID: "d-evil", Path: "/fake/memory/evil.md",
		Provider: model.ProviderCodex, Category: memory.CategoryNote,
		Title: "Fictional evil-path note", StartLine: 1, EndLine: 2,
		Text: "Body.", RawText: "Body.", ModTime: time.Now(),
		ProjectCWD: evil, ProjectEvidence: "rollout-header",
	}}}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	openMemoriesForTest(t, m)
	content := m.View().Content
	if strings.Contains(content, "\x1b]8") || strings.Contains(content, "[31mevil") {
		t.Error("recorded project path reached the terminal unsanitized (list or detail)")
	}
	// Scope header after cycling the project filter to the evil cwd.
	send(m, "p")
	content = m.View().Content
	if strings.Contains(content, "\x1b]8") || strings.Contains(content, "[31mevil") {
		t.Error("recorded project path reached the terminal unsanitized (scope header)")
	}
}

func TestMemoriesRenderedEntitiesCannotInjectControls(t *testing.T) {
	m, loader := memTestModel(t)
	text := "Before &#x1b;[8mhiddenword&#x1b;[0m after, plus &#27; decimal and &#27[8mnosemi&#27[0m semicolon-less and &#x10000001b;[8moverflow&#x10000001b;[0m wrapped and ~~&#x**1b**;[8msplitentity~~ split."
	loader.cat = &memory.Catalog{Entries: []memory.Entry{{
		ID: "e-ent", DocumentID: "d-ent", Path: "/fake/memory/ent.md",
		Provider: model.ProviderClaudeCode, Category: memory.CategoryNote,
		Title: "Entity sample", StartLine: 1, EndLine: 1,
		Text: text, RawText: text, ModTime: time.Now(), ProjectEvidence: "unknown",
	}}}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	openMemoriesForTest(t, m)
	content := m.View().Content
	if strings.Contains(content, "\x1b[8m") {
		t.Error("entity-decoded conceal sequence reached the terminal")
	}
	// The word remains readable as ordinary text in rendered output.
	lines := m.memRenderedLines(loader.cat.Entries[0], 100)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "\x1b[8m") {
		t.Error("glamour output contains a source-injected conceal sequence")
	}
}

func TestMemoriesSourcesPathsSanitized(t *testing.T) {
	m, loader := memTestModel(t)
	evil := "/fake/\x1b[31mred\x1b]8;;https://example.invalid\x07path"
	loader.cat.Documents = append(loader.cat.Documents, memory.Document{
		ID: "d-evil", Path: evil + "/bad.md", Provider: model.ProviderCodex,
		Category: memory.CategoryNote, Status: "unreadable", StatusDetail: "fictional",
	})
	loader.cat.Sources = append(loader.cat.Sources, memory.SourceStatus{
		Provider: model.ProviderCodex, Root: evil, State: "unreadable",
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	openMemoriesForTest(t, m)
	send(m, "i")
	content := m.View().Content
	if strings.Contains(content, "\x1b]8") || strings.Contains(content, "[31mred") {
		t.Error("source or document path reached the sources view unsanitized")
	}
}

func TestMemoriesCurrentProjectIncludesNested(t *testing.T) {
	m, loader := memTestModel(t)
	// Session scope: Current project rooted at the fixture repo.
	m.project = "current"
	m.projectRoot = "/home/dev/repos/atlas"
	m.projectRootSet = true
	loader.cat.Entries = append(loader.cat.Entries, memory.Entry{
		ID: "e-nested", DocumentID: "d-nested", Path: "/fake/projects/atlas/memory/nested.md",
		Provider: model.ProviderClaudeCode, Category: memory.CategoryNote,
		Title: "Nested dir note", StartLine: 1, EndLine: 1, Text: "x", RawText: "x",
		ModTime: time.Now(), ProjectCWD: "/home/dev/repos/atlas/sub/dir", ProjectEvidence: "session-association",
	})
	openMemoriesForTest(t, m)
	if !m.mem.projectRelated {
		t.Fatal("carried Current project scope must keep related matching")
	}
	found := false
	for _, e := range m.mem.visibleEntries() {
		if e.ID == "e-nested" {
			found = true
		}
	}
	if !found {
		t.Error("memory recorded in a repo subdirectory vanished from Current project scope")
	}
	if got := m.mem.projectScopeLabel(); got != "Current project" {
		t.Errorf("scope label = %q, want Current project", got)
	}
}

func TestMemoriesReferenceChooserReachesLaterRefs(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	selectMemoryForTest(t, m, "e-note") // note ref + origin session b1
	send(m, "o")
	if m.overlay != overlaySelector {
		t.Fatal("chooser must open")
	}
	// Second option is the origin session; choosing it must be
	// reachable despite the resolved note listed first.
	send(m, "j")
	send(m, "enter")
	if m.memoriesOpen() {
		t.Fatal("choosing the session reference must close Memories")
	}
	c, ok := m.selected()
	if !ok || c.row.ID != "b1" {
		t.Fatalf("selected session = %v, want b1", c.row.ID)
	}
}

func TestMemoriesSearchQueryNormalizedAcrossRefresh(t *testing.T) {
	m, loader := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "s")
	typeText(m, "migration ")
	send(m, "enter")
	before := len(m.mem.matches)
	if before == 0 {
		t.Fatal("trailing-space query must match initially")
	}
	_, cmd := m.Update(key("R"))
	if cmd == nil {
		t.Fatal("R must refresh")
	}
	m.Update(cmd())
	if len(m.mem.matches) != before {
		t.Fatalf("matches changed across refresh: before=%d after=%d (loader calls %d)", before, len(m.mem.matches), loader.calls)
	}
	// Category changes recompute with the same normalized query.
	send(m, "a")
	if len(m.mem.matches) == 0 {
		t.Error("matches vanished after category change")
	}
}

func TestMemoriesReferenceOpensHiddenTarget(t *testing.T) {
	m, loader := memTestModel(t)
	now := time.Now()
	loader.cat = &memory.Catalog{Entries: []memory.Entry{
		{
			ID: "e-src", DocumentID: "d-src", Path: "/fake/memories/MEMORY.md",
			Provider: model.ProviderCodex, Category: memory.CategoryConsolidated,
			Title: "Consolidated source", StartLine: 1, EndLine: 2, Text: "x", RawText: "x",
			ModTime: now, ProjectEvidence: "unknown",
			References: []memory.Reference{{Raw: "rollout_summaries/hidden.md", Kind: "note", EntryID: "e-hidden", Status: "resolved"}},
		},
		{
			ID: "e-hidden", DocumentID: "d-hidden", Path: "/fake/memories/rollout_summaries/hidden.md",
			Provider: model.ProviderCodex, Category: memory.CategorySupporting,
			Title: "Hidden supporting target", StartLine: 1, EndLine: 2, Text: "y", RawText: "y",
			ModTime: now, ProjectEvidence: "rollout-header",
		},
	}}
	openMemoriesForTest(t, m)
	// Default categories hide the supporting target.
	for _, e := range m.mem.visibleEntries() {
		if e.ID == "e-hidden" {
			t.Fatal("precondition: supporting target must start hidden")
		}
	}
	send(m, "o")
	e, ok := m.mem.selectedEntry()
	if !ok || e.ID != "e-hidden" {
		t.Fatalf("selected = %v, want hidden target opened via broadened scope", e.ID)
	}
	if m.mem.catIdx != 1 {
		t.Errorf("categories must broaden to All, catIdx=%d", m.mem.catIdx)
	}
}

func TestMemoriesOpenDuringInitialIndexing(t *testing.T) {
	m, _ := memTestModel(t)
	m.initialIndexing = true
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, cmd := m.Update(key("m"))
	if cmd == nil {
		t.Fatal("m during initial indexing must start the memory load")
	}
	m.Update(cmd())
	if !m.memoriesOpen() || m.mem.catalog == nil {
		t.Fatal("Memories must open without waiting for indexing")
	}
	if view := m.View().Content; !strings.Contains(view, "Memories") {
		t.Error("Memories must render over the indexing screen")
	}
	if !m.initialIndexing {
		t.Error("opening Memories must not disturb the running indexing")
	}
	// Esc from the memories list returns to the indexing screen.
	send(m, "esc")
	if m.memoriesOpen() {
		t.Fatal("Esc must leave Memories")
	}
	if view := m.View().Content; strings.Contains(view, "Notes and consolidated") {
		t.Error("indexing screen must render again after leaving Memories")
	}
}

func TestHelpScrollableAtShortHeight(t *testing.T) {
	m, _ := memTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	send(m, "?")
	first := m.View().Content
	if strings.Contains(first, "Memory sources and coverage") {
		t.Skip("help fits without scrolling at this size")
	}
	for range 30 {
		send(m, "j")
	}
	scrolled := m.View().Content
	if !strings.Contains(scrolled, "Memory sources and coverage") {
		t.Error("Memories help section must be reachable by scrolling at 80x24")
	}
	for range 30 {
		send(m, "k")
	}
	if back := m.View().Content; !strings.Contains(back, "Move selection") {
		t.Error("scrolling back up must return to the top of help")
	}
}

func TestMemoriesLoadUsesSessionSnapshotNotDatabase(t *testing.T) {
	m, loader := memTestModel(t)
	openMemoriesForTest(t, m)
	if len(loader.snapshot) != len(m.sessions) || len(loader.snapshot) == 0 {
		t.Fatalf("loader must receive the in-memory session snapshot (%d rows), got %d", len(m.sessions), len(loader.snapshot))
	}
	for i := range loader.snapshot {
		if loader.snapshot[i].SourcePath != m.sessions[i].SourcePath {
			t.Fatalf("snapshot row %d mismatch", i)
		}
	}
}

func TestEscapeForGlamourPreservesLiteralContent(t *testing.T) {
	in := "a && b\n\n```sh\nrun --a && run --b &#x1b;\n```\n\n    indented && code\n\n[link](https://example.invalid/?a=1&b=2) and fish & chips\n"
	out := escapeForGlamour(in)
	if !strings.Contains(out, "run --a && run --b &#x1b;") {
		t.Error("fenced code (parser-identified) keeps literal ampersands")
	}
	if !strings.Contains(escapeForGlamour("para\n```\nrun && x\n```"), "run && x") {
		t.Error("paragraph-interrupting fenced code keeps literal ampersands")
	}
	if !strings.Contains(escapeForGlamour("\n```\nrun && never closed"), "run && never") {
		t.Error("unclosed fenced code keeps literal ampersands")
	}
	if strings.Contains(escapeForGlamour("<pre>\n\n```\n&#x1b;[8mhidden\n```\n</pre>"), "&#x1b;") {
		t.Error("entities inside HTML blocks must stay escaped (no fence exemption there)")
	}
	if strings.Contains(escapeForGlamour("    ~~~\n\n&#x1b;[8mhidden"), "&#x1b;") {
		t.Error("indented pseudo-fence text must stay entity-escaped")
	}
	if !strings.Contains(out, "indented && code") {
		t.Error("parser-identified indented code keeps literal ampersands")
	}
	if strings.Contains(escapeForGlamour("- item\n\n    &#x1b;[8mhidden"), "&#x1b;") {
		t.Error("indented list paragraph must stay entity-escaped")
	}
	if !strings.Contains(out, "?a=1&b=2") {
		t.Error("parser-validated link destinations keep literal ampersands")
	}
	if strings.Contains(escapeForGlamour("[x](a&#x1b;[8mhidden trailing"), "&#x1b;") {
		t.Error("incomplete link text must stay entity-escaped")
	}
	continuation := escapeForGlamour("Paragraph\n    &#x1b;[8mhidden&#x1b;[0m")
	if strings.Contains(continuation, "&#x1b;") {
		t.Error("indented paragraph continuation is prose and must stay escaped")
	}
	if strings.Contains(escapeForGlamour("[ok][r]\n\n](x&#27;[8mhidden&#27;[0m)\n\n[r]: https://example.invalid"), "&#27;") {
		t.Error("reference-style links must not exempt later text")
	}
	if strings.Contains(escapeForGlamour("~~<https://example.invalid/&#27;[8mhidden&#27;[0m>~~"), "&#27;") {
		t.Error("autolinks inside strikethrough must stay escaped (renderer flattens and unescapes)")
	}
	if strings.Contains(escapeForGlamour("~~[x](https://example.invalid/?a=&#27;)~~"), "&#27;") {
		t.Error("link destinations inside strikethrough must stay escaped")
	}
	// Entity-bearing destinations are safe either way: neither
	// goldmark nor glamour decodes Destination bytes, so they can
	// never become control characters in the href.
	if out := escapeForGlamour("~~<https://example.invalid/&#27;[8mhidden&#27;[0m>~~<https://example.invalid/&#27;[8mhidden&#27;[0m>"); strings.Count(out, "&#27;") != 2 {
		t.Errorf("only the real autolink's entities may stay literal: %q", out)
	}
	if out := escapeForGlamour("~~<https://example.invalid/&#27;[8mhidden>~~ then <https://example.invalid/?a=1&b=2>"); strings.Contains(out, "&#27;") || !strings.Contains(out, "a=1&b=2") {
		t.Errorf("strikethrough autolink must consume its occurrence without exempting it: %q", out)
	}
	if out := escapeForGlamour("https://example.invalid/&#27;[8mhidden&#27;[0m\n\n~~<https://example.invalid/&#27;[8mhidden&#27;[0m>~~"); strings.Count(out, "&#27;") != 2 {
		t.Errorf("bare autolink must bind to its own (earliest) occurrence, not the strikethrough copy: %q", out)
	}
	if out := escapeForGlamour("<div>\nhttps://example.invalid/&#27;[8mhidden\n</div>\n\n<https://example.invalid/?a=1&b=2>"); strings.Contains(out, "&#27;") || !strings.Contains(out, "a=1&b=2") {
		t.Errorf("HTML block content must not be mistaken for the later autolink: %q", out)
	}
	if out := escapeForGlamour("[docs][r]\n\n[r]: https://example.invalid/?a=1&b=2"); !strings.Contains(out, "?a=1&b=2") {
		t.Errorf("reference-definition destinations keep literal ampersands: %q", out)
	}
	dup := escapeForGlamour("`<https://example.invalid/&#27;[8mhidden>` then <https://example.invalid/&#27;[8mhidden>")
	if strings.Count(dup, "&#27;") != 1 {
		t.Errorf("code-span copy must stay escaped while the real autolink is exempt: %q", dup)
	}
	inCode := escapeForGlamour("`[x](&#x1b;[8mhidden&#x1b;[0m)` inline code")
	if strings.Contains(inCode, "&#x1b;") {
		t.Error("entities inside inline code spans must stay escaped")
	}
	malformed := escapeForGlamour("]( &#x1b;[8mhidden&#x1b;[0m and ](no-close &#x1b;")
	if strings.Contains(malformed, "&#x1b;") {
		t.Error("malformed link text must stay entity-escaped")
	}
	if !strings.Contains(out, "a &amp;&amp; b") || !strings.Contains(out, "fish &amp; chips") {
		t.Error("prose ampersands must be escaped for glamour")
	}
	// Plain (no-color) rendering shows the original text.
	m, loader := memTestModel(t)
	m.noColor = true
	loader.cat = &memory.Catalog{Entries: []memory.Entry{{
		ID: "e-amp", DocumentID: "d-amp", Path: "/fake/amp.md",
		Provider: model.ProviderClaudeCode, Category: memory.CategoryNote,
		Title: "Amp note", StartLine: 1, EndLine: 1,
		Text: "a && b", RawText: "a && b", ModTime: time.Now(), ProjectEvidence: "unknown",
	}}}
	lines := m.memRenderedLines(loader.cat.Entries[0], 80)
	if joined := strings.Join(lines, "\n"); !strings.Contains(joined, "a && b") || strings.Contains(joined, "&amp;") {
		t.Errorf("plain rendering must show literal ampersands: %q", joined)
	}
	// The final rendered hyperlink carries the literal query.
	m.noColor = false
	m.mem = &memoriesState{rendered: map[string]string{}}
	if rendered := strings.Join(m.memRenderedLines(memory.Entry{
		ID: "e-link", Text: "[docs](https://example.invalid/?a=1&b=2)", RawText: "[docs](https://example.invalid/?a=1&b=2)",
	}, 120), "\n"); !strings.Contains(rendered, "a=1&b=2") || strings.Contains(rendered, "a=1&amp;b=2") {
		t.Errorf("final rendered hyperlink must carry the literal query: %q", rendered)
	}
	if rendered := strings.Join(m.memRenderedLines(memory.Entry{
		ID: "e-auto", Text: "See <https://example.invalid/?a=1&b=2> now", RawText: "See <https://example.invalid/?a=1&b=2> now",
	}, 120), "\n"); !strings.Contains(rendered, "a=1&b=2") || strings.Contains(rendered, "a=1&amp;b=2") {
		t.Errorf("final rendered autolink must carry the literal query: %q", rendered)
	}
	if rendered := strings.Join(m.memRenderedLines(memory.Entry{
		ID: "e-strike", Text: "~~<https://example.invalid/&#27;[8mhidden&#27;[0m>~~", RawText: "~~<https://example.invalid/&#27;[8mhidden&#27;[0m>~~",
	}, 120), "\n"); strings.Contains(rendered, "\x1b[8m") {
		t.Errorf("rendered strikethrough must not contain injected conceal sequence: %q", rendered)
	}
}

func TestMemoriesSearchCancelAfterRefreshRestoresResults(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "s")
	typeText(m, "migration")
	send(m, "enter")
	committed := len(m.mem.matches)
	if committed == 0 {
		t.Fatal("precondition: committed search must have matches")
	}
	// Start a refresh, then begin editing a different query.
	_, cmd := m.Update(key("R"))
	if cmd == nil {
		t.Fatal("R must refresh")
	}
	send(m, "s")
	typeText(m, "zzzznope")
	// Refresh lands mid-edit and recomputes with the draft.
	m.Update(cmd())
	// Cancel restores the committed query AND its results.
	send(m, "esc")
	if m.mem.searchQuery != "migration" {
		t.Fatalf("query = %q, want restored migration", m.mem.searchQuery)
	}
	if len(m.mem.matches) != committed {
		t.Fatalf("matches = %d, want restored %d", len(m.mem.matches), committed)
	}
}

func TestSettingsOpensFromMemoriesAndReturns(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, ",")
	if m.settings == nil {
		t.Fatal(", in Memories must open Settings")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Sources and settings") {
		t.Fatal("Settings must render above Memories")
	}
	// Keys go to Settings while it is open (j moves its cursor,
	// not the memory selection).
	selBefore := m.mem.sel
	send(m, "j")
	if m.mem.sel != selBefore {
		t.Fatal("keys leaked to Memories while Settings was open")
	}
	send(m, "esc")
	if m.settings != nil {
		t.Fatal("Esc must close Settings")
	}
	if !m.memoriesOpen() {
		t.Fatal("closing Settings must return to Memories")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Memories") {
		t.Fatal("Memories must render again after Settings closes")
	}
	// Also from the Sources screen.
	send(m, "i")
	send(m, ",")
	if m.settings == nil {
		t.Fatal(", in Memory Sources must open Settings")
	}
	send(m, "esc")
	if m.settings != nil || !m.memoriesOpen() {
		t.Fatal("closing Settings from Sources must return to Memories")
	}
}

func TestCommaTypesInMemoriesFilter(t *testing.T) {
	m, _ := memTestModel(t)
	openMemoriesForTest(t, m)
	send(m, "/")
	send(m, ",")
	if m.settings != nil {
		t.Fatal(", while editing the filter must not open Settings")
	}
	if m.mem.filterText != "," {
		t.Fatalf("filter text = %q, want the typed comma", m.mem.filterText)
	}
}

func TestModeStylingDistinctAndShortcutInStatusBars(t *testing.T) {
	m, _ := memTestModel(t)
	m.noColor = false
	m.theme = nil
	// Sessions status bar advertises m.
	footer := ansi.Strip(m.renderFooter(140))
	if !strings.Contains(footer, "m Memories") {
		t.Errorf("session status bar must show the m shortcut: %q", footer)
	}
	sessionHeader := m.renderHeader(140)
	openMemoriesForTest(t, m)
	memHeader := m.memHeader(140)
	if sessionHeader == memHeader {
		t.Fatal("mode headers must differ")
	}
	// Memories header uses the inverted accent badge style (accent
	// background), unlike the Sessions accent wordmark.
	if got := m.styles().modeBadge.GetBackground(); got != lipgloss.Color(m.palette().accent) {
		t.Errorf("mode badge background = %v, want accent %v", got, m.palette().accent)
	}
	if m.styles().modeBadge.GetBackground() == m.styles().heading.GetBackground() {
		t.Error("mode badge must differ from the heading style")
	}
	if !strings.Contains(ansi.Strip(memHeader), "Memories") {
		t.Error("memories header must name the mode")
	}
	// Memories status bar advertises m back to Sessions.
	memFooter := ansi.Strip(m.memFooter(140))
	if !strings.Contains(memFooter, "m Sessions") {
		t.Errorf("memories status bar must show the m shortcut: %q", memFooter)
	}
}

func TestMemoriesMatchRevealOnlyOnce(t *testing.T) {
	m, loader := memTestModel(t)
	var body strings.Builder
	body.WriteString("# Long fictional note\n")
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&body, "Filler line %d about fictional systems.\n", i)
		if i == 45 {
			body.WriteString("The needleword appears here.\n")
		}
	}
	text := body.String()
	loader.cat = &memory.Catalog{Entries: []memory.Entry{{
		ID: "e-long", DocumentID: "d-long", Path: "/fake/memory/long.md",
		Provider: model.ProviderClaudeCode, Category: memory.CategoryNote,
		Title: "Long fictional note", StartLine: 1, EndLine: 62,
		Text: text, RawText: text, ModTime: time.Now(), ProjectEvidence: "unknown",
	}}}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	openMemoriesForTest(t, m)
	send(m, "s")
	typeText(m, "needleword")
	send(m, "enter")
	send(m, "n")
	if m.mem.activeLine == 0 {
		t.Fatal("match navigation must set the active line")
	}
	_ = m.View()
	if m.mem.detailScroll == 0 {
		t.Fatal("first render must reveal the match below the fold")
	}
	if m.mem.revealPending {
		t.Fatal("reveal must be consumed after the first render")
	}
	// Manual scrolling back to the top must stick.
	for range 10 {
		send(m, "pgup")
	}
	_ = m.View()
	if m.mem.detailScroll != 0 {
		t.Fatalf("manual scroll snapped back to the match: detailScroll=%d", m.mem.detailScroll)
	}
}
