package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/memory"
	"github.com/juanrubio/trailmark/internal/model"
)

// memoriesLoadedMsg carries one finished memory catalog load. Loads
// run as commands so opening Memories never blocks on source reads
// (and never waits for the transcript scan).
type memoriesLoadedMsg struct {
	gen     int // load generation; stale results are discarded
	catalog *memory.Catalog
	err     error
}

type memFocus int

const (
	memFocusList memFocus = iota
	memFocusDetail
)

// memCategorySets is the cycle behind the a key. The default set is
// notes plus consolidated memory; indexes, overviews, and raw or
// supporting material stay behind explicit category choices.
var memCategorySets = []struct {
	label string
	cats  []memory.Category
}{
	{"Notes and consolidated memory", memory.DefaultCategories()},
	{"All categories", nil},
	{"Indexes and overviews", []memory.Category{memory.CategoryIndex, memory.CategoryOverview}},
	{"Supporting and raw memory", []memory.Category{memory.CategorySupporting, memory.CategoryRaw}},
}

// memoriesState is the read-only Memories view. It is created lazily
// on first open and preserved across returns to Sessions; open gates
// routing so session state is never disturbed while it exists.
type memoriesState struct {
	open    bool
	catalog *memory.Catalog
	loading bool
	loadGen int // incremented per load; only the latest result applies
	loadErr string

	harness model.Provider
	project string // "" = all, "unknown" = unknown/global, else a cwd
	// projectRelated preserves the session list's Current project
	// matching (nested directories belong) for a carried scope;
	// explicit p cycling chooses exact projects and clears it.
	projectRelated bool
	catIdx         int

	filterText    string
	filterPrev    string
	filterSelPrev int

	editing       string // "", "filter", or "search"
	searchQuery   string
	searchPrev    string
	searchActive  bool
	matches       []memory.Match
	matchIdx      int  // index into the selected entry's match lines
	activeLine    int  // highlighted source line, 0 = none
	revealPending bool // scroll to activeLine once, then leave scrolling alone

	sel           int // index into visibleEntries()
	focus         memFocus
	detailScroll  int
	detailHeight  int // last rendered detail body height
	listHeight    int // last rendered list body height
	raw           bool
	detailOpen    bool // narrow full-width detail
	sourcesOpen   bool
	sourcesScroll int
	notice        string
	rendered      map[string]string // glamour cache: entry|width|theme -> text
}

func (m *appModel) memoriesOpen() bool { return m.mem != nil && m.mem.open }

// openMemories lazily creates the view, carrying the session list's
// harness and project scope (without its date window), and starts
// the first catalog load off the update path.
func (m *appModel) openMemories() tea.Cmd {
	if m.mem == nil {
		st := &memoriesState{open: true, harness: m.harness, rendered: map[string]string{}}
		switch {
		case m.project == "current":
			if m.currentProjectAvailable() {
				st.project = m.currentProjectRoot()
				st.projectRelated = true
			}
		case m.project != "":
			st.project = m.project
		}
		m.mem = st
		return m.loadMemoriesCmd()
	}
	m.mem.open = true
	if m.mem.catalog == nil && !m.mem.loading {
		return m.loadMemoriesCmd()
	}
	return nil
}

// closeMemories returns to Sessions with every piece of session
// state (selection, filters, expansion, scroll) untouched.
func (m *appModel) closeMemories() {
	if m.mem != nil {
		m.mem.open = false
	}
}

func (m *appModel) loadMemoriesCmd() tea.Cmd {
	st := m.mem
	if st == nil {
		return nil
	}
	if m.deps.loadMemories == nil {
		st.loadErr = "Memory sources are unavailable."
		return nil
	}
	st.loading = true
	st.loadGen++
	gen := st.loadGen
	load := m.deps.loadMemories
	ctx := m.ctx
	snapshot := append([]index.SessionRow{}, m.sessions...)
	var cfgSnap *config.Config
	if m.deps.snapshotConfig != nil {
		cfgSnap = m.deps.snapshotConfig()
	}
	return func() tea.Msg {
		catalog, err := load(ctx, snapshot, cfgSnap)
		return memoriesLoadedMsg{gen: gen, catalog: catalog, err: err}
	}
}

// onMemoriesLoaded applies a finished load, retaining the selected
// entry by ID where it survives and otherwise landing on the
// nearest remaining entry, mirroring session refresh behavior.
func (m *appModel) onMemoriesLoaded(msg memoriesLoadedMsg) {
	st := m.mem
	if st == nil {
		return
	}
	if msg.gen != st.loadGen {
		return // a newer load superseded this result
	}
	st.loading = false
	if msg.err != nil {
		// Keep any usable catalog rather than replacing it with an
		// empty result after a failed refresh.
		st.loadErr = "Memories could not be loaded."
		return
	}
	st.loadErr = ""
	keepID := ""
	oldSel := st.sel
	if e, ok := st.selectedEntry(); ok {
		keepID = e.ID
	}
	st.catalog = msg.catalog
	st.rendered = map[string]string{}
	st.recomputeMatches()
	entries := st.visibleEntries()
	found := false
	if keepID != "" {
		for i, e := range entries {
			if e.ID == keepID {
				st.sel = i
				found = true
				break
			}
		}
	}
	if !found {
		st.sel = oldSel
		if keepID != "" && len(entries) > 0 {
			st.notice = "The selected memory is no longer included; showing the nearest remaining entry."
		}
	}
	st.clampSel()
}

func (st *memoriesState) filter() memory.Filter {
	return memory.Filter{
		Provider:       st.harness,
		Project:        st.project,
		Categories:     memCategorySets[st.catIdx].cats,
		Text:           st.filterText,
		ProjectRelated: st.projectRelated,
	}
}

// visibleEntries is the selectable list: search matches replace the
// filtered list while a content search is active.
func (st *memoriesState) visibleEntries() []memory.Entry {
	if st.catalog == nil {
		return nil
	}
	if st.searchActive {
		out := make([]memory.Entry, 0, len(st.matches))
		for _, mt := range st.matches {
			out = append(out, mt.Entry)
		}
		return out
	}
	return memory.FilterEntries(st.catalog, st.filter())
}

func (st *memoriesState) selectedEntry() (memory.Entry, bool) {
	entries := st.visibleEntries()
	if st.sel < 0 || st.sel >= len(entries) {
		return memory.Entry{}, false
	}
	return entries[st.sel], true
}

func (st *memoriesState) clampSel() {
	n := len(st.visibleEntries())
	if n == 0 {
		st.sel = 0
		return
	}
	if st.sel >= n {
		st.sel = n - 1
	}
	if st.sel < 0 {
		st.sel = 0
	}
}

func (st *memoriesState) recomputeMatches() {
	if !st.searchActive || st.catalog == nil {
		return
	}
	st.matches = memory.Search(st.catalog, strings.TrimSpace(st.searchQuery), st.filter())
}

// afterScopeChange resets selection after harness, project, or
// category changes, keeping an active search scoped to the new set.
func (st *memoriesState) afterScopeChange() {
	st.sel = 0
	st.detailScroll = 0
	st.activeLine = 0
	st.matchIdx = 0
	st.recomputeMatches()
	st.clampSel()
}

func (st *memoriesState) moveSel(delta int) {
	st.sel += delta
	st.clampSel()
	st.detailScroll = 0
	st.activeLine = 0
	st.matchIdx = 0
}

func (st *memoriesState) matchForSelected() *memory.Match {
	e, ok := st.selectedEntry()
	if !ok {
		return nil
	}
	for i := range st.matches {
		if st.matches[i].Entry.ID == e.ID {
			return &st.matches[i]
		}
	}
	return nil
}

func (m *appModel) onMemoriesKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := m.mem
	s := msg.String()
	if st.sourcesOpen {
		switch s {
		case "esc", "q", "i":
			st.sourcesOpen = false
		case "j", "down":
			st.sourcesScroll++
		case "k", "up":
			if st.sourcesScroll > 0 {
				st.sourcesScroll--
			}
		case "pgdown":
			st.sourcesScroll += max(1, st.detailHeight)
		case "pgup":
			st.sourcesScroll = max(0, st.sourcesScroll-max(1, st.detailHeight))
		case "?":
			m.openHelp()
		case ",":
			m.openSettings()
		}
		return m, nil
	}
	if st.editing != "" {
		return m.onMemoriesEditKey(msg)
	}
	detailFocused := st.focus == memFocusDetail || st.detailOpen
	switch s {
	case "?":
		m.openHelp()
	case ",":
		m.openSettings()
	case "m":
		m.closeMemories()
	case "esc", "q":
		switch {
		case st.detailOpen:
			st.detailOpen = false
			st.focus = memFocusList
		case st.focus == memFocusDetail:
			st.focus = memFocusList
		default:
			m.closeMemories()
		}
	case "j", "down":
		if detailFocused {
			st.detailScroll++
		} else {
			st.moveSel(1)
		}
	case "k", "up":
		if detailFocused {
			if st.detailScroll > 0 {
				st.detailScroll--
			}
		} else {
			st.moveSel(-1)
		}
	case "pgdown":
		if detailFocused {
			st.detailScroll += max(1, st.detailHeight)
		} else {
			st.moveSel(max(1, st.listHeight/3))
		}
	case "pgup":
		if detailFocused {
			st.detailScroll = max(0, st.detailScroll-max(1, st.detailHeight))
		} else {
			st.moveSel(-max(1, st.listHeight/3))
		}
	case "tab", "shift+tab":
		if m.isWide() {
			if st.focus == memFocusList {
				st.focus = memFocusDetail
			} else {
				st.focus = memFocusList
			}
		}
	case "enter":
		if m.isWide() {
			st.focus = memFocusDetail
		} else if _, ok := st.selectedEntry(); ok {
			st.detailOpen = true
			st.focus = memFocusDetail
		}
	case "/":
		st.filterPrev = st.filterText
		st.filterSelPrev = st.sel
		st.editing = "filter"
	case "s":
		st.searchPrev = st.searchQuery
		st.editing = "search"
	case "n":
		m.memMatchStep(1)
	case "N":
		m.memMatchStep(-1)
	case "h":
		switch st.harness {
		case "":
			st.harness = model.ProviderClaudeCode
		case model.ProviderClaudeCode:
			st.harness = model.ProviderCodex
		default:
			st.harness = ""
		}
		st.afterScopeChange()
	case "p":
		options := append([]string{""}, memory.Projects(st.catalog)...)
		options = append(options, "unknown")
		idx := 0
		for i, opt := range options {
			if opt == st.project {
				idx = i
				break
			}
		}
		st.project = options[(idx+1)%len(options)]
		st.projectRelated = false
		st.afterScopeChange()
	case "a":
		st.catIdx = (st.catIdx + 1) % len(memCategorySets)
		st.afterScopeChange()
	case "v":
		st.raw = !st.raw
	case "R":
		st.notice = ""
		return m, m.loadMemoriesCmd()
	case "c":
		m.copySelectedMemory()
	case "o":
		m.openMemoryReference()
	case "i":
		st.sourcesOpen = true
		st.sourcesScroll = 0
	}
	return m, nil
}

// onMemoriesEditKey follows onFilterKey conventions: the list filter
// applies live and Esc restores the previous text and selection;
// content search runs on Enter, like cross-session search.
func (m *appModel) onMemoriesEditKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := m.mem
	switch msg.String() {
	case "esc":
		if st.editing == "filter" {
			st.filterText = st.filterPrev
			st.sel = st.filterSelPrev
			st.recomputeMatches()
			st.clampSel()
		} else {
			st.searchQuery = st.searchPrev
			// A refresh landing mid-edit recomputed matches with
			// the draft query; restoring the committed query must
			// recompute so results match their label again.
			st.recomputeMatches()
			st.clampSel()
		}
		st.editing = ""
	case "enter":
		if st.editing == "search" {
			m.applyMemorySearch()
		}
		st.editing = ""
	case "backspace":
		if st.editing == "filter" {
			if trimLastRune(&st.filterText) {
				st.sel = 0
				st.detailScroll = 0
				st.recomputeMatches()
				st.clampSel()
			}
		} else {
			trimLastRune(&st.searchQuery)
		}
	default:
		if msg.Text == "" {
			return m, nil
		}
		if st.editing == "filter" {
			st.filterText += msg.Text
			st.sel = 0
			st.detailScroll = 0
			st.recomputeMatches()
			st.clampSel()
		} else {
			st.searchQuery += msg.Text
		}
	}
	return m, nil
}

func trimLastRune(s *string) bool {
	runes := []rune(*s)
	if len(runes) == 0 {
		return false
	}
	*s = string(runes[:len(runes)-1])
	return true
}

func (m *appModel) applyMemorySearch() {
	st := m.mem
	st.notice = ""
	query := strings.TrimSpace(st.searchQuery)
	st.searchQuery = query // one normalized query for every recomputation
	if query == "" {
		st.searchActive = false
		st.matches = nil
	} else {
		st.searchActive = true
		st.matches = memory.Search(st.catalog, query, st.filter())
		if len(st.matches) == 0 {
			st.notice = fmt.Sprintf("No matches for %q in the current scope.", query)
		}
	}
	st.sel = 0
	st.detailScroll = 0
	st.activeLine = 0
	st.matchIdx = 0
	st.clampSel()
}

// memMatchStep moves between matching source lines inside the
// selected entry, switching to the raw view so the highlighted line
// carries its exact source line number.
func (m *appModel) memMatchStep(delta int) {
	st := m.mem
	if !st.searchActive {
		st.notice = "No active content search. Press s to search memory content."
		return
	}
	mt := st.matchForSelected()
	if mt == nil {
		return
	}
	if len(mt.Lines) == 0 {
		st.notice = "The match for this entry is in its title or description."
		return
	}
	if st.activeLine == 0 {
		if delta < 0 {
			st.matchIdx = len(mt.Lines) - 1
		} else {
			st.matchIdx = 0
		}
	} else {
		st.matchIdx += delta
		if st.matchIdx >= len(mt.Lines) {
			st.matchIdx = 0
		}
		if st.matchIdx < 0 {
			st.matchIdx = len(mt.Lines) - 1
		}
	}
	st.activeLine = mt.Lines[st.matchIdx]
	st.revealPending = true
	st.raw = true
	st.focus = memFocusDetail
	if !m.isWide() {
		st.detailOpen = true
	}
	st.notice = fmt.Sprintf("Match %d of %d in this entry · source line %d", st.matchIdx+1, len(mt.Lines), st.activeLine)
}

// copySelectedMemory copies only on the explicit c key, never on
// selection, and always appends the source provenance.
func (m *appModel) copySelectedMemory() {
	st := m.mem
	e, ok := st.selectedEntry()
	if !ok {
		return
	}
	if m.deps.copier == nil {
		st.notice = "Copy failed: the clipboard is unavailable."
		return
	}
	payload := e.Text + "\n\nSource: " + e.Path + ":" + itoa(e.StartLine)
	if err := m.deps.copier.Copy(payload); err != nil {
		st.notice = "Copy failed: the clipboard is unavailable."
		return
	}
	st.notice = "Copied memory text with its source reference."
}

// memRefAction is one openable reference of the selected entry: a
// resolved note link or a recorded session identifier.
type memRefAction struct {
	label     string
	entryID   string
	sessionID string
}

// memRefActions lists every openable reference in display order:
// resolved notes first, then distinct session identifiers.
func memRefActions(e memory.Entry) []memRefAction {
	var out []memRefAction
	for _, ref := range e.References {
		if ref.Kind == "note" && ref.Status == "resolved" && ref.EntryID != "" {
			out = append(out, memRefAction{label: "Note: " + sanitizeText(ref.Raw), entryID: ref.EntryID})
		}
	}
	seen := map[string]bool{}
	var ids []string
	if e.OriginSessionID != "" {
		ids = append(ids, e.OriginSessionID)
	}
	if e.ThreadID != "" {
		ids = append(ids, e.ThreadID)
	}
	for _, ref := range e.References {
		if ref.Kind == "session" && ref.SessionID != "" {
			ids = append(ids, ref.SessionID)
		}
	}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, memRefAction{label: "Session: " + sanitizeText(id), sessionID: id})
		}
	}
	return out
}

// openMemoryReference follows the entry's only openable reference
// directly; with several, it opens the reference chooser so every
// resolved note and session reference stays reachable.
func (m *appModel) openMemoryReference() {
	st := m.mem
	e, ok := st.selectedEntry()
	if !ok {
		return
	}
	actions := memRefActions(e)
	switch {
	case len(actions) == 0:
		st.notice = "This memory has no resolved note or session reference to open."
	case len(actions) == 1:
		m.followMemRef(e, actions[0])
	default:
		m.openSelector(selectorReference)
	}
}

func (m *appModel) followMemRef(e memory.Entry, action memRefAction) {
	st := m.mem
	if action.entryID != "" {
		if st.selectEntryByID(action.entryID) {
			st.notice = "Opened the referenced memory."
			return
		}
		// The target is loaded but hidden by the current scope
		// (for example a supporting rollout referenced from a
		// consolidated entry under the default categories).
		// Following an explicit reference broadens the scope to
		// show it; exclusions already applied at load still hold.
		found := false
		if st.catalog != nil {
			for _, cand := range st.catalog.Entries {
				if cand.ID == action.entryID {
					found = true
					break
				}
			}
		}
		if found {
			st.searchActive = false
			st.matches = nil
			st.filterText = ""
			st.harness = ""
			st.project = ""
			st.projectRelated = false
			st.catIdx = 1 // All categories
			st.sel = 0
			st.detailScroll = 0
			if st.selectEntryByID(action.entryID) {
				st.notice = "Opened the referenced memory (scope broadened to show it)."
				return
			}
		}
		st.notice = "The referenced memory is not in the loaded catalog."
		return
	}
	ids := []string{action.sessionID}
	var rows []index.SessionRow
	seen := map[string]bool{}
	for _, row := range m.sessions {
		if row.Provider != e.Provider || row.ID == "" || seen[row.SourcePath] {
			continue
		}
		for _, id := range ids {
			if row.ID == id {
				seen[row.SourcePath] = true
				rows = append(rows, row)
				break
			}
		}
	}
	switch {
	case len(rows) == 0:
		st.notice = "The referenced session is not in the indexed sessions."
	case len(rows) > 1:
		st.notice = fmt.Sprintf("The session reference is ambiguous: %d indexed sessions share this identifier.", len(rows))
	default:
		target := rows[0]
		m.closeMemories()
		// Clear session filters to All so the referenced session is
		// visible wherever it sits; this is an explicit navigation.
		m.manualWindow = true
		m.windowDays = 0
		m.harness = ""
		m.project = ""
		m.filterText = ""
		m.focus = focusList
		m.overviewOpen = false
		if m.selectSession(target.SourcePath) {
			m.notice = "Opened the session referenced by this memory."
		} else {
			m.clampSelection()
			m.refreshDetail()
			m.notice = "The referenced session could not be shown."
		}
	}
}

func (st *memoriesState) selectEntryByID(id string) bool {
	for i, e := range st.visibleEntries() {
		if e.ID == id {
			st.sel = i
			st.detailScroll = 0
			st.activeLine = 0
			st.matchIdx = 0
			return true
		}
	}
	return false
}

func memCategoryLabel(c memory.Category) string {
	switch c {
	case memory.CategoryNote:
		return "Note"
	case memory.CategoryIndex:
		return "Index"
	case memory.CategoryConsolidated:
		return "Consolidated"
	case memory.CategoryOverview:
		return "Overview"
	case memory.CategorySupporting:
		return "Supporting"
	case memory.CategoryRaw:
		return "Raw"
	default:
		return string(c)
	}
}

func (st *memoriesState) harnessScopeLabel() string {
	if st.harness == "" {
		return "All harnesses"
	}
	return providerLabel(st.harness)
}

func (st *memoriesState) projectScopeLabel() string {
	if st.projectRelated {
		return "Current project"
	}
	switch st.project {
	case "":
		return "All projects"
	case "unknown":
		return "Unknown/global"
	default:
		// Recorded paths are untrusted display content.
		return sanitizeText(projectName(st.project))
	}
}

// memEntryProjectShort labels an entry's project without ever
// guessing: an unassociated entry is Unknown, and only entries the
// source records as global are labelled Global.
func memEntryProjectShort(e memory.Entry) string {
	if e.ProjectCWD != "" {
		return sanitizeText(projectName(e.ProjectCWD))
	}
	if e.ProjectEvidence == "global" {
		return "Global"
	}
	return "Unknown"
}

func memEvidenceLabel(evidence string) string {
	switch evidence {
	case "session-association":
		return "session association"
	case "rollout-header":
		return "recorded rollout directory"
	case "global":
		return "recorded as global"
	default:
		return ""
	}
}
