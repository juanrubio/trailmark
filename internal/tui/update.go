package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

// Update handles window, key, and background-scan messages.
func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		wasWide := m.isWide()
		m.width, m.height = msg.Width, msg.Height
		if wasWide != m.isWide() {
			// Crossing the layout boundary must not strand focus in
			// an invisible overview: overview focus becomes the
			// full-width overview when narrow, and the side pane
			// when wide again.
			if m.focus == focusOverview || m.overviewOpen {
				m.focus = focusOverview
				m.overviewOpen = !m.isWide()
			}
		}
		return m, nil
	case scanDoneMsg:
		m.failedRoots = msg.failedRoots
		if msg.err != nil {
			m.notice = "Background refresh failed; showing the cached index."
			return m, nil
		}
		if len(m.sessions) == 0 {
			// First scan on an empty index: nothing to protect yet,
			// so discovered cards appear immediately.
			m.sessions = msg.sessions
			m.partialCount = countPartial(m.sessions)
			m.clampSelection()
			m.refreshDetail()
			return m, nil
		}
		if sessionsSignature(msg.sessions) != sessionsSignature(m.sessions) {
			m.pending = msg.sessions
			m.hasPending = true
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func sessionsSignature(sessions []index.SessionRow) string {
	var b strings.Builder
	for _, s := range sessions {
		b.WriteString(s.SourcePath)
		b.WriteByte('|')
		if s.LastActivity != nil {
			b.WriteString(s.LastActivity.Format("2006-01-02T15:04:05"))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *appModel) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	if msg.Keystroke() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.overlay {
	case overlayHelp:
		if s == "esc" || s == "q" || s == "?" {
			m.overlay = overlayNone
		}
		return m, nil
	case overlaySelector:
		return m.onSelectorKey(s)
	case overlayFilter:
		return m.onFilterKey(msg)
	}
	if m.focus == focusOverview || m.overviewOpen {
		return m.onOverviewKey(s)
	}
	return m.onListKey(s)
}

func (m *appModel) onListKey(s string) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch s {
	case "q":
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "enter":
		if m.isWide() {
			m.focus = focusOverview
			m.overviewScroll = 0
		} else if _, ok := m.selected(); ok {
			m.overviewOpen = true
			m.overviewScroll = 0
		}
	case "w":
		m.openSelector(selectorTime)
	case "p":
		m.openSelector(selectorProject)
	case "h":
		m.openSelector(selectorHarness)
	case "/":
		m.filterPrev = m.filterText
		m.filterSelPrev = m.sel
		m.overlay = overlayFilter
	case "e":
		if c, ok := m.selected(); ok && c.childCount > 0 {
			m.expanded[c.row.SourcePath] = !m.expanded[c.row.SourcePath]
			m.clampSelection()
			m.refreshDetail()
		}
	case "R":
		if m.hasPending {
			m.applyPending()
		}
	case "?":
		m.overlay = overlayHelp
	case "t":
		m.notice = "Transcript reading arrives in a later phase."
	case "r", "f", "n":
		m.notice = "Continuation choices arrive in the next phase."
	case "s":
		m.notice = "Content search arrives in a later phase."
	}
	return m, nil
}

func (m *appModel) onOverviewKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "esc", "q":
		m.overviewOpen = false
		m.focus = focusList
	case "j", "down":
		m.overviewScroll++
	case "k", "up":
		if m.overviewScroll > 0 {
			m.overviewScroll--
		}
	case "?":
		m.overlay = overlayHelp
	}
	return m, nil
}

func (m *appModel) onSelectorKey(s string) (tea.Model, tea.Cmd) {
	options := m.selectorOptions()
	switch s {
	case "esc":
		m.overlay = overlayNone
	case "j", "down":
		if m.selCursor < len(options)-1 {
			m.selCursor++
		}
	case "k", "up":
		if m.selCursor > 0 {
			m.selCursor--
		}
	case "enter":
		if m.selCursor >= 0 && m.selCursor < len(options) {
			m.applySelector(options[m.selCursor])
		}
		m.overlay = overlayNone
	}
	return m, nil
}

func (m *appModel) onFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	changed := false
	switch msg.String() {
	case "esc":
		m.filterText = m.filterPrev
		m.sel = m.filterSelPrev
		m.overlay = overlayNone
		m.clampSelection()
		m.refreshDetail()
		return m, nil
	case "enter":
		m.overlay = overlayNone
		return m, nil
	case "backspace":
		runes := []rune(m.filterText)
		if len(runes) > 0 {
			m.filterText = string(runes[:len(runes)-1])
			changed = true
		}
	default:
		if msg.Text != "" {
			m.filterText += msg.Text
			changed = true
		}
	}
	if changed {
		m.sel = 0
		m.clampSelection()
		m.refreshDetail()
	}
	return m, nil
}

func (m *appModel) move(delta int) {
	m.sel += delta
	m.clampSelection()
	m.overviewScroll = 0
	m.refreshDetail()
}

func (m *appModel) applyPending() {
	keep := ""
	if c, ok := m.selected(); ok {
		keep = c.row.SourcePath
	}
	m.sessions = m.pending
	m.detailFor = "" // force the overview to reload after a refresh
	m.partialCount = countPartial(m.sessions)
	m.pending = nil
	m.hasPending = false
	m.sel = 0
	for i, c := range m.cards() {
		if c.row.SourcePath == keep {
			m.sel = i
			break
		}
	}
	m.clampSelection()
	m.refreshDetail()
	m.notice = "Showing the latest session ordering."
}

// refreshDetail loads the selected session's overview synchronously;
// index reads are local and bounded to one session's messages.
func (m *appModel) refreshDetail() {
	c, ok := m.selected()
	if !ok {
		m.detail = nil
		m.detailFor = ""
		return
	}
	if m.detailFor == c.row.SourcePath && m.detail != nil {
		return
	}
	if m.overview == nil {
		m.detail = nil
		return
	}
	ov, err := m.overview(m.ctx, c.row.SourcePath)
	if err != nil {
		m.detail = nil
		m.detailFor = ""
		return
	}
	m.detail = ov
	m.detailFor = c.row.SourcePath
}

// --- selectors ---

type selectorOption struct {
	label string
	apply func(m *appModel)
	note  string
}

func (m *appModel) openSelector(kind selectorKind) {
	m.selector = kind
	m.overlay = overlaySelector
	m.selCursor = 0
	options := m.selectorOptions()
	for i, opt := range options {
		if opt.note == "current" {
			m.selCursor = i
			break
		}
	}
}

func (m *appModel) selectorOptions() []selectorOption {
	switch m.selector {
	case selectorTime:
		var out []selectorOption
		for _, opt := range []struct {
			label string
			days  int
		}{{"Today", 1}, {"3 days", 3}, {"7 days", 7}, {"30 days", 30}, {"All time", 0}} {
			opt := opt
			o := selectorOption{label: opt.label, apply: func(m *appModel) {
				m.windowDays = opt.days
				m.afterFilterChange()
			}}
			if m.windowDays == opt.days {
				o.note = "current"
			}
			out = append(out, o)
		}
		return out
	case selectorHarness:
		counts := map[model.Provider]int{}
		for _, s := range m.sessions {
			if m.inWindow(s) && m.inProject(s) {
				counts[s.Provider]++
			}
		}
		var out []selectorOption
		for _, opt := range []struct {
			label string
			p     model.Provider
		}{{"All harnesses", ""}, {"Codex", model.ProviderCodex}, {"Claude Code", model.ProviderClaudeCode}} {
			opt := opt
			label := opt.label
			if opt.p != "" {
				label += " (" + itoa(counts[opt.p]) + ")"
			}
			o := selectorOption{label: label, apply: func(m *appModel) {
				m.harness = opt.p
				m.afterFilterChange()
			}}
			if m.harness == opt.p {
				o.note = "current"
			}
			out = append(out, o)
		}
		return out
	default: // selectorProject
		out := []selectorOption{{label: "All projects", apply: func(m *appModel) {
			m.project = ""
			m.afterFilterChange()
		}}}
		if m.project == "" {
			out[0].note = "current"
		}
		current := selectorOption{label: "Current project", apply: func(m *appModel) {
			if !m.currentProjectAvailable() {
				m.project = ""
				m.afterFilterChange()
				m.notice = "Current project is unavailable here; showing all projects."
				return
			}
			m.project = "current"
			m.afterFilterChange()
		}}
		if !m.currentProjectAvailable() {
			current.label += " (unavailable)"
		}
		if m.project == "current" {
			current.note = "current"
		}
		out = append(out, current)
		for _, p := range m.projects() {
			p := p
			o := selectorOption{label: p, apply: func(m *appModel) {
				m.project = p
				m.afterFilterChange()
			}}
			if m.project == p {
				o.note = "current"
			}
			out = append(out, o)
		}
		return out
	}
}

func (m *appModel) applySelector(opt selectorOption) {
	if opt.apply != nil {
		opt.apply(m)
	}
}

func (m *appModel) afterFilterChange() {
	m.sel = 0
	m.clampSelection()
	m.overviewScroll = 0
	m.refreshDetail()
}

func (m *appModel) isWide() bool { return m.width >= 110 }
