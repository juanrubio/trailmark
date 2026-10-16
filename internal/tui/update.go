package tui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/continuation"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

// Update handles window, key, and background-scan messages.
func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.lightTheme = !msg.IsDark()
		return m, nil
	case tea.WindowSizeMsg:
		wasWide := m.isWide()
		m.width, m.height = msg.Width, msg.Height
		if m.overviewFind.query != "" {
			m.overviewFind.reveal = true
		}
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
		m.View() // normalize scroll offsets for the resized viewport
		return m, nil
	case initialIndexTickMsg:
		if !m.initialIndexing {
			return m, nil
		}
		m.initialFrame++
		if m.deps.scanProgress != nil {
			m.initialProgress = m.deps.scanProgress()
		}
		return m, m.initialTick()
	case scanDoneMsg:
		if msg.id == 0 {
			m.scanFinished = true
		}
		if m.initialIndexing && msg.id == m.initialScanID {
			m.applyScanID = 0
			m.applyNextScan = false
			m.scanFinished = true
			m.initialIndexing = false
			m.failedRoots = msg.failedRoots
			m.applySessions(msg.sessions)
			m.notice = ""
			switch {
			case errors.Is(msg.err, context.Canceled):
				m.notice = "Indexing canceled; showing sessions already indexed."
			case msg.err != nil:
				m.notice = "Initial indexing failed; showing available sessions. Open Settings (,) to check sources."
			case len(msg.failedRoots) > 0:
				m.notice = "Some sources could not be read. Open Settings (,) to check sources."
			}
			return m, nil
		}
		m.failedRoots = msg.failedRoots
		if msg.id != 0 && msg.id == m.applyScanID {
			m.applyScanID = 0
			m.applyNextScan = false
			if msg.err != nil {
				m.notice = "Rescan failed; showing the cached index."
				return m, nil
			}
			m.applySessions(msg.sessions)
			m.pending, m.hasPending = nil, false
			m.reloadSettings()
			return m, nil
		}
		if msg.err != nil {
			m.notice = "Background refresh failed; showing the cached index."
			return m, nil
		}
		if len(m.sessions) == 0 {
			// First scan on an empty index: nothing to protect yet,
			// so discovered cards appear immediately.
			m.sessions = msg.sessions
			m.pending, m.hasPending = nil, false
			m.chooseAutomaticWindow()
			m.partialCount = countPartial(m.sessions)
			m.clampSelection()
			m.refreshDetail()
			return m, nil
		}
		m.removeAbsentSessions(msg.sessions)
		m.refreshScannedDetail()
		if sessionsSignature(msg.sessions) != sessionsSignature(m.sessions) {
			m.pending = msg.sessions
			m.hasPending = true
		} else {
			m.pending, m.hasPending = nil, false
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
	case overlayQuit:
		return m.onQuitKey(s)
	case overlaySheet:
		return m.onSheetKey(s)
	case overlayCopyMenu:
		return m.onCopyMenuKey(s)
	case overlayCopyPreview:
		return m.onCopyPreviewKey(s)
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
	if m.chooseIndexingScope {
		return m.onIndexingScopeKey(s, true)
	}
	if m.initialIndexing {
		switch s {
		case "q":
			m.overlay, m.quitSelected = overlayQuit, false
		case "esc":
			cancel := m.deps.cancelInitialScan
			if m.initialScanID != 0 {
				cancel = m.deps.cancelRescan
			}
			if !m.initialCanceling && cancel != nil {
				m.initialCanceling = true
				cancel()
			}
		}
		return m, nil
	}
	if m.settings != nil {
		return m.onSettingsKey(msg)
	}
	if m.transcript != nil {
		return m.onTranscriptKey(msg)
	}
	if m.search != nil {
		return m.onSearchKey(msg)
	}
	if m.focus == focusOverview || m.overviewOpen {
		if m.overviewFind.editing {
			return m.onOverviewFindInput(msg)
		}
		return m.onOverviewKey(s)
	}
	return m.onListKey(s)
}

func (m *appModel) onListKey(s string) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch s {
	case "q":
		m.overlay, m.quitSelected = overlayQuit, false
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
	case ",":
		m.openSettings()
	case "t":
		if c, ok := m.selected(); ok {
			m.openTranscript(c.row, 0, false)
		}
	case "r":
		m.openSheet(continuation.Resume)
	case "f":
		m.openSheet(continuation.Fork)
	case "n":
		m.openSheet(continuation.Clean)
	case "c":
		m.openCopyMenu()
	case "s":
		m.openSearch()
	}
	return m, nil
}

func (m *appModel) onOverviewKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "e":
		m.workExpanded = !m.workExpanded
		m.workReveal = m.overviewFind.query == ""
		m.overviewFind.current = 0
		m.overviewFind.reveal = m.overviewFind.query != ""
		m.normalizeOverviewScroll()
	case "/":
		m.normalizeOverviewScroll()
		m.overviewFind.editing = true
		m.overviewFind.draft = m.overviewFind.query
	case "[", "]":
		m.stepOverviewFind(s == "]")
	case "T":
		if c, ok := m.selected(); ok {
			atSeq := 0
			if m.detail != nil {
				atSeq = m.detail.Initial.Seq
			}
			m.openTranscript(c.row, atSeq, false)
		}
		return m, nil
	case "t":
		if c, ok := m.selected(); ok {
			atSeq := 0
			if m.detail != nil && m.detail.LatestUserEx.Seq > 0 {
				atSeq = m.detail.LatestUserEx.Seq
			} else if m.detail != nil && m.detail.Initial.Seq > 0 {
				atSeq = m.detail.Initial.Seq
			}
			m.openTranscript(c.row, atSeq, false)
		}
		return m, nil
	case "s":
		m.openSearch()
		return m, nil
	case "r":
		m.openSheet(continuation.Resume)
		return m, nil
	case "f":
		m.openSheet(continuation.Fork)
		return m, nil
	case "n":
		m.openSheet(continuation.Clean)
		return m, nil
	case "c":
		m.openCopyMenu()
		return m, nil
	case "esc", "q":
		m.overviewOpen = false
		m.focus = focusList
	case "j", "down", "k", "up", "pgdown", "pgup":
		m.normalizeOverviewScroll() // normalize before applying a relative move
		step := 1
		if s == "pgdown" || s == "pgup" {
			step = max(1, m.overviewViewportHeight)
		}
		if s == "k" || s == "up" || s == "pgup" {
			step = -step
		}
		m.overviewScroll += step
		m.normalizeOverviewScroll()
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
	m.applySessions(m.pending)
	m.pending = nil
	m.hasPending = false
}

// applySessions replaces the visible list, preserving the selected
// session by source path where it still exists.
func (m *appModel) applySessions(sessions []index.SessionRow) {
	keep := ""
	oldSel := m.sel
	if c, ok := m.selected(); ok {
		keep = c.row.SourcePath
	}
	m.sessions = sessions
	m.chooseAutomaticWindow()
	m.detailFor = "" // force the overview to reload after a refresh
	m.partialCount = countPartial(m.sessions)
	found := false
	for i, c := range m.cards() {
		if c.row.SourcePath == keep && keep != "" {
			m.sel = i
			found = true
			break
		}
	}
	if !found && keep != "" && len(m.cards()) > 0 {
		// UX: land on the nearest remaining entry and say why.
		m.sel = oldSel
		m.clampSelection()
		m.notice = "The selected session is no longer included; showing the nearest remaining session."
		m.refreshDetail()
		return
	}
	if !found && keep == "" {
		m.sel = 0
	}
	m.clampSelection()
	m.refreshDetail()
	if found {
		m.notice = "Showing the latest session ordering."
	}
}

// removeAbsentSessions applies confirmed removals immediately while retaining
// the old row values/order for survivors. New activity still waits for R.
func (m *appModel) removeAbsentSessions(fresh []index.SessionRow) {
	present := make(map[string]bool, len(fresh))
	for _, row := range fresh {
		present[row.SourcePath] = true
	}
	keep := ""
	oldSel := m.sel
	if c, ok := m.selected(); ok {
		keep = c.row.SourcePath
	}
	var savedSource string
	var savedScope *appModel
	if m.search != nil {
		snapshot := *m
		savedScope = &snapshot
		savedScope.windowDays = m.search.savedWindow
		savedScope.project = m.search.savedProject
		savedScope.harness = m.search.savedHarness
		savedScope.sel = m.search.savedSel
		if c, ok := savedScope.selected(); ok {
			savedSource = c.row.SourcePath
		}
	}
	survivors := make([]index.SessionRow, 0, len(m.sessions))
	for _, row := range m.sessions {
		if present[row.SourcePath] {
			survivors = append(survivors, row)
		}
	}
	if len(survivors) == len(m.sessions) {
		return
	}
	m.sessions = survivors
	m.partialCount = countPartial(survivors)
	m.chooseAutomaticWindow()
	found := false
	for i, c := range m.cards() {
		if c.row.SourcePath == keep {
			m.sel = i
			found = true
			break
		}
	}
	if !found {
		m.sel = oldSel
	}
	m.clampSelection()
	if keep != "" && !present[keep] {
		m.overviewScroll = 0
		m.detailFor = ""
		m.notice = "The selected session is no longer included; showing the nearest remaining session."
		if len(m.cards()) == 0 {
			m.notice = "The selected session is no longer included; no sessions remain in this scope."
		}
	}
	m.refreshDetail()
	if m.search != nil {
		groups := m.search.groups[:0]
		for _, group := range m.search.groups {
			if present[group.row.SourcePath] {
				groups = append(groups, group)
			}
		}
		m.search.groups = groups
		m.search.cursor = max(0, min(m.search.cursor, len(m.search.entries())-1))
		savedScope.sessions = survivors
		savedCards := savedScope.cards()
		m.search.savedSel = max(0, min(m.search.savedSel, len(savedCards)-1))
		for i, c := range savedCards {
			if c.row.SourcePath == savedSource {
				m.search.savedSel = i
				break
			}
		}
	}
	if m.transcript != nil && !present[m.transcript.row.SourcePath] {
		m.transcript = nil
	}
	if m.overlay == overlaySelector {
		m.selCursor = max(0, min(m.selCursor, len(m.selectorOptions())-1))
	}
}

// refreshScannedDetail reloads overview-only metadata after indexing even when
// card rows did not change. Keep cached detail and interaction state on failure.
func (m *appModel) refreshScannedDetail() {
	c, ok := m.selected()
	if !ok || m.deps.overview == nil {
		return
	}
	ov, err := m.deps.overview(m.ctx, c.row.SourcePath)
	if err != nil || ov == nil {
		return
	}
	m.detail = ov
	m.detailFor = c.row.SourcePath
}

// refreshDetail loads the selected session's overview synchronously;
// index reads are local and bounded to one session's messages.
func (m *appModel) refreshDetail() {
	c, ok := m.selected()
	if !ok {
		m.overviewFind = overviewFindState{}
		m.workExpanded = false
		m.detail = nil
		m.detailFor = ""
		return
	}
	if m.overviewFind.source != c.row.SourcePath {
		m.overviewFind = overviewFindState{source: c.row.SourcePath}
		m.workExpanded = false
	}
	if m.detailFor == c.row.SourcePath && m.detail != nil {
		return
	}
	if m.deps.overview == nil {
		m.detail = nil
		return
	}
	ov, err := m.deps.overview(m.ctx, c.row.SourcePath)
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
				m.manualWindow = true
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
	m.chooseAutomaticWindow()
	m.sel = 0
	m.clampSelection()
	m.overviewScroll = 0
	m.refreshDetail()
}

func (m *appModel) isWide() bool { w, _, _, _ := m.viewport(); return w >= 110 }

// --- continuation sheet and copying ---

func (m *appModel) openSheet(action continuation.Action) {
	c, ok := m.selected()
	if !ok {
		return
	}
	capability := m.deps.caps[c.row.Provider]
	prepared := continuation.Prepare(action, capability, c.row.Provider, c.row.ID, c.row.CWD, c.row.Purpose)
	m.sheet = &prepared
	m.sheetState = ""
	m.sheetScroll = 0
	m.overlay = overlaySheet
}

func (m *appModel) onSheetKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "j", "down":
		m.sheetScroll++
	case "k", "up":
		if m.sheetScroll > 0 {
			m.sheetScroll--
		}
	case "esc", "q":
		m.overlay = overlayNone
		m.sheet = nil
	case "enter", "c":
		if m.sheet != nil && m.sheet.Available && m.deps.copier != nil {
			if err := m.deps.copier.Copy(m.sheet.Command); err != nil {
				m.sheetState = "failed"
			} else {
				m.sheetState = "copied"
			}
		}
	}
	return m, nil
}

func (m *appModel) openCopyMenu() {
	c, ok := m.selected()
	if !ok {
		return
	}
	m.refreshDetail()
	m.copyItems = m.buildCopyItems(c.row, m.detail)
	m.copyCursor = 0
	m.copyState = ""
	m.overlay = overlayCopyMenu
}

func (m *appModel) onCopyMenuKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "esc", "q":
		m.overlay = overlayNone
	case "j", "down":
		if m.copyCursor < len(m.copyItems)-1 {
			m.copyCursor++
		}
	case "k", "up":
		if m.copyCursor > 0 {
			m.copyCursor--
		}
	case "enter":
		if m.copyCursor >= 0 && m.copyCursor < len(m.copyItems) {
			item := m.copyItems[m.copyCursor]
			if !item.available {
				m.notice = "That content is unavailable for this session."
				m.overlay = overlayNone
				return m, nil
			}
			m.copyPreview = &item
			m.copyState = ""
			m.copyScroll = 0
			m.copyReturn = "menu"
			m.overlay = overlayCopyPreview
		}
	}
	return m, nil
}

func (m *appModel) onCopyPreviewKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "j", "down":
		m.copyScroll++
	case "k", "up":
		if m.copyScroll > 0 {
			m.copyScroll--
		}
	case "esc":
		if m.copyReturn == "transcript" {
			m.overlay = overlayNone
		} else {
			m.overlay = overlayCopyMenu
		}
	case "q":
		m.overlay = overlayNone
	case "enter", "c":
		if m.copyPreview != nil && m.deps.copier != nil {
			if err := m.deps.copier.Copy(m.copyPreview.text); err != nil {
				m.copyState = "failed"
			} else {
				m.copyState = "copied"
			}
		}
	}
	return m, nil
}

func (m *appModel) onQuitKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "esc", "n":
		m.overlay = overlayNone
	case "left", "right", "up", "down", "tab", "shift+tab", "h", "l":
		m.quitSelected = !m.quitSelected
	case "y":
		return m, tea.Quit
	case "enter":
		if m.quitSelected {
			return m, tea.Quit
		}
		m.overlay = overlayNone
	}
	return m, nil
}
