package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

// searchState is the explicit cross-session content search mode.
type searchState struct {
	query   string
	editing bool
	ran     bool
	groups  []searchGroup
	cursor  int // index into entries()
	status  string
	// Snapshot of home scope, restored when search closes so
	// broadening never leaks into the overview.
	savedWindow  int
	savedProject string
	savedHarness model.Provider
	savedSel     int
}

type searchGroup struct {
	row   index.SessionRow
	field string // metadata field name, or "" for content matches
	hits  []index.SearchHit
	count int
}

type searchEntry struct {
	group int
	hit   int // -1 for a metadata entry
}

// scopedSessions applies time, harness, and project filters only:
// content search scope never includes the metadata text filter.
func (m *appModel) scopedSessions() []index.SessionRow {
	var out []index.SessionRow
	for _, s := range m.sessions {
		if !m.inWindow(s) {
			continue
		}
		if m.harness != "" && s.Provider != m.harness {
			continue
		}
		if !m.inProject(s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (m *appModel) openSearch() {
	m.search = &searchState{
		editing:      true,
		savedWindow:  m.windowDays,
		savedProject: m.project,
		savedHarness: m.harness,
		savedSel:     m.sel,
	}
}

// closeSearch leaves search mode and restores the home scope that
// was active when search opened, undoing any broadening.
func (m *appModel) closeSearch() {
	if m.search == nil {
		return
	}
	m.windowDays = m.search.savedWindow
	m.project = m.search.savedProject
	m.harness = m.search.savedHarness
	m.sel = m.search.savedSel
	m.search = nil
	m.clampSelection()
	m.refreshDetail()
}

func (m *appModel) onSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := m.search
	s := msg.String()
	if st.editing {
		switch s {
		case "esc":
			m.closeSearch()
		case "enter":
			st.editing = false
			m.runContentSearch()
		case "backspace":
			runes := []rune(st.query)
			if len(runes) > 0 {
				st.query = string(runes[:len(runes)-1])
			}
		default:
			if msg.Text != "" {
				st.query += msg.Text
			}
		}
		return m, nil
	}
	switch s {
	case "esc", "q":
		m.closeSearch()
		return m, nil
	case "j", "down":
		if st.cursor < len(st.entries())-1 {
			st.cursor++
		}
	case "k", "up":
		if st.cursor > 0 {
			st.cursor--
		}
	case "enter":
		m.openSearchEntry()
	case "/":
		st.editing = true
	case "a": // broaden: All time
		m.windowDays = 0
		m.runContentSearch()
	case "P": // broaden: all projects
		m.project = ""
		m.runContentSearch()
	case "H": // broaden: all harnesses
		m.harness = ""
		m.runContentSearch()
	}
	return m, nil
}

func (st *searchState) entries() []searchEntry {
	var out []searchEntry
	for gi, g := range st.groups {
		if g.field != "" {
			out = append(out, searchEntry{group: gi, hit: -1})
		}
		for hi := range g.hits {
			out = append(out, searchEntry{group: gi, hit: hi})
		}
	}
	return out
}

func (m *appModel) runContentSearch() {
	st := m.search
	st.ran = true
	st.cursor = 0
	st.groups = nil
	st.status = ""
	query := strings.TrimSpace(st.query)
	if query == "" || m.deps.content == nil {
		return
	}
	scoped := m.scopedSessions()
	paths := make([]string, 0, len(scoped))
	byPath := map[string]index.SessionRow{}
	for _, s := range scoped {
		paths = append(paths, s.SourcePath)
		byPath[s.SourcePath] = s
	}
	contentGroups, err := m.deps.content(m.ctx, query, paths, 50)
	if err != nil {
		st.status = "Search unavailable; some sources may be only partially indexed."
		return
	}
	groupsByPath := map[string]int{}
	var contentOnly []searchGroup
	for _, cg := range contentGroups {
		groupsByPath[cg.Row.SourcePath] = len(contentOnly)
		contentOnly = append(contentOnly, searchGroup{row: cg.Row, hits: cg.Hits, count: cg.Count})
	}
	terms := queryTerms(query)
	containsAll := func(text string) bool {
		lower := strings.ToLower(text)
		for _, term := range terms {
			if !strings.Contains(lower, strings.ToLower(term)) {
				return false
			}
		}
		return len(terms) > 0
	}
	var purposeMeta, directoryMeta []searchGroup
	for _, s := range scoped {
		field := ""
		switch {
		case containsAll(s.Purpose) || containsAll(s.Title):
			field = "Purpose"
		case containsAll(s.CWD):
			field = "Directory"
		}
		if field == "" {
			continue
		}
		if idx, ok := groupsByPath[s.SourcePath]; ok {
			contentOnly[idx].field = field
		} else if field == "Purpose" {
			purposeMeta = append(purposeMeta, searchGroup{row: s, field: field})
		} else {
			directoryMeta = append(directoryMeta, searchGroup{row: s, field: field})
		}
	}
	rest := contentOnly
	// Purpose metadata first, then relevance-ranked content, then
	// directory-only metadata.
	st.groups = append(append(purposeMeta, rest...), directoryMeta...)
	if len(st.groups) == 0 {
		st.status = "No matches in the current scope."
	}
}

func (m *appModel) openSearchEntry() {
	st := m.search
	entries := st.entries()
	if st.cursor < 0 || st.cursor >= len(entries) {
		return
	}
	e := entries[st.cursor]
	g := st.groups[e.group]
	if e.hit < 0 {
		// Metadata match: restore the home scope, open the session
		// overview, and name the matching field.
		m.closeSearch()
		if !m.selectSession(g.row.SourcePath) {
			m.notice = "Matched in " + g.field + ", but the session is hidden by the active text filter."
			return
		}
		m.notice = "Matched in " + g.field + ": " + purposeOrUnavailable(g.row)
		if m.isWide() {
			m.focus = focusOverview
		} else {
			m.overviewOpen = true
		}
		return
	}
	hit := g.hits[e.hit]
	m.openTranscript(g.row, hit.Seq, true)
}

// selectSession moves home selection to a session, expanding its
// parent first when the session is a collapsed child.
func (m *appModel) selectSession(sourcePath string) bool {
	target, found := func() (index.SessionRow, bool) {
		for _, s := range m.sessions {
			if s.SourcePath == sourcePath {
				return s, true
			}
		}
		return index.SessionRow{}, false
	}()
	if found && target.ParentID != "" {
		for _, s := range m.sessions {
			if s.Provider == target.Provider && s.Store == target.Store && s.ID == target.ParentID {
				m.expanded[s.SourcePath] = true
			}
		}
	}
	for i, c := range m.cards() {
		if c.row.SourcePath == sourcePath {
			m.sel = i
			m.refreshDetail()
			return true
		}
	}
	return false
}

// viewSearch renders the cross-session search mode.
func (m *appModel) viewSearch(w, h int) string {
	st := m.search
	header := styleHeading.Render("Search session content") + "\n" +
		styleDim.Render(truncate(m.windowLabel()+" · "+m.projectLabel()+" · "+m.harnessLabel(), w)) + "\n" +
		"Query: " + st.query + searchCursor(st)
	footer := m.searchFooter(st)
	bodyH := h - 4 - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	var lines []string
	cursorLine := 0
	entryIdx := 0
	for gi, g := range st.groups {
		title := purposeOrUnavailable(g.row)
		meta := projectName(g.row.CWD) + " · " + providerLabel(g.row.Provider)
		if g.field != "" {
			meta += " · matched in " + g.field
		}
		if g.count > 0 {
			meta += fmt.Sprintf(" · %d matches", g.count)
		}
		lines = append(lines, styleHeading.Render(truncate(title, w)), styleDim.Render(truncate(meta, w)))
		_ = gi
		if g.field != "" {
			if entryIdx == st.cursor {
				cursorLine = len(lines)
			}
			lines = append(lines, "  "+markerFor(entryIdx == st.cursor)+"Metadata match in "+g.field)
			entryIdx++
		}
		for _, hit := range g.hits {
			if entryIdx == st.cursor {
				cursorLine = len(lines)
			}
			kind := "conversation"
			switch hit.Kind {
			case "tool_output":
				kind = "tool output"
			case "tool_call":
				kind = "tool call"
			}
			snippet := strings.ReplaceAll(sanitizeText(hit.Snippet), "\n", " ")
			lines = append(lines, "  "+markerFor(entryIdx == st.cursor)+styleDim.Render(kind+"  ")+truncate(snippet, w-14))
			entryIdx++
		}
		lines = append(lines, "")
	}
	if st.ran && len(st.groups) == 0 && !st.editing {
		lines = append(lines, "No matches in the current scope.", "", "Broaden: a All time · P All projects · H All harnesses · / Edit query")
	}
	body := windowLines(lines, cursorLine, bodyH)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func searchCursor(st *searchState) string {
	if st.editing {
		return "_"
	}
	return ""
}

func markerFor(selected bool) string {
	if selected {
		return "▌ "
	}
	return "  "
}

func (m *appModel) searchFooter(st *searchState) string {
	if st.editing {
		return "Type to edit · Enter Search · Esc Back"
	}
	lines := []string{"↑↓/jk Select  Enter Open  / Edit query  a All time  P All projects  H All harnesses  Esc Back"}
	if st.status != "" {
		lines = append([]string{st.status}, lines...)
	}
	return strings.Join(lines, "\n")
}
