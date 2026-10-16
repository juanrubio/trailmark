package tui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/juanrubio/trailmark/internal/model"
)

// View renders the current frame.
func (m *appModel) View() tea.View {
	tw, th := m.terminalSize()
	w, h, x, y := m.viewport()
	var content string
	switch {
	case tw < 40 || th < 12:
		content = "Terminal too small for Trailmark.\nResize to at least 40x12 · q quit"
	case m.overlay == overlayQuit:
		pw, ph := min(w, 50), min(h, 11)
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, m.viewQuitModal(pw, ph))
	case m.chooseIndexingScope:
		content = m.viewIndexingScope(w, h, true)
	case m.initialIndexing:
		content = m.viewInitialIndexing(w, h)
	case m.overlay == overlayHelp:
		content = m.viewHelp(w, h)
	case m.overlay == overlaySelector:
		pw, ph := min(w, 64), min(h, len(m.selectorOptions())+6)
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, m.panel(m.viewSelector(pw-4, ph-2), pw, ph))
	case m.overlay == overlaySheet:
		if m.sheetState == "failed" {
			content = m.viewSheet(w, h)
		} else {
			content = m.panel(m.viewSheet(w-4, h-2), w, h)
		}
	case m.overlay == overlayCopyMenu:
		content = m.panel(m.viewCopyMenu(w-4, h-2), w, h)
	case m.overlay == overlayCopyPreview:
		content = m.viewCopyPreview(w, h)
	case m.settings != nil:
		content = m.viewSettings(w, h)
	case m.transcript != nil:
		content = m.viewTranscript(w, h)
	case m.search != nil:
		content = m.viewSearch(w, h)
	case m.overviewOpen:
		content = m.viewOverviewPage(w, h)
	default:
		content = m.viewMain(w, h)
	}
	if !(m.overlay == overlaySheet && m.sheetState == "failed") {
		content = fitPane(content, w, h)
	}
	if x > 0 || y > 0 {
		rows := strings.Split(content, "\n")
		for i, line := range rows {
			rows[i] = strings.Repeat(" ", x) + line + strings.Repeat(" ", x)
		}
		blank := strings.Repeat(" ", tw)
		for range y {
			rows = append([]string{blank}, rows...)
			rows = append(rows, blank)
		}
		content = strings.Join(rows, "\n")
	}
	content = m.frame(content, tw, th)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *appModel) viewMain(w, h int) string {
	header := m.renderHeader(w)
	footer := m.renderFooter(w)
	bodyH := h - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	var body string
	if m.isWide() {
		listW := w * 45 / 100
		if listW < 30 {
			listW = 30
		}
		overviewW := w - listW - 3
		list := fitPane(m.renderList(listW-2, bodyH), listW-2, bodyH)
		overview := fitPane(m.renderOverview(overviewW-2, bodyH), overviewW-2, bodyH)
		divider := m.styles().rule.Render(strings.TrimSuffix(strings.Repeat(" "+m.verticalRule()+" \n", bodyH), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Padding(0, 1).Render(list), divider, lipgloss.NewStyle().Padding(0, 1).Render(overview))
	} else {
		body = fitPane(m.renderList(w, bodyH), w, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *appModel) renderHeader(w int) string {
	scope := m.windowLabel() + " · " + m.projectLabel() + " · " + m.harnessLabel()
	if m.filterText != "" {
		scope += " · Filter: " + m.filterText
	}
	left := m.styles().heading.Render("Trailmark") + "  " + scope
	right := itoa(len(m.visibleSessions())) + " sessions"
	line := left
	if lipgloss.Width(left)+lipgloss.Width(right)+2 <= w {
		line = left + strings.Repeat(" ", w-lipgloss.Width(left)-lipgloss.Width(right)) + right
	} else {
		line = truncate(left, w)
	}
	raised := m.styles().raised.Width(w).Render(line)
	if !m.noColor {
		bg := m.palette().raised
		raised = m.restoreSurface(raised, bg)
	}
	if hint := m.indexingCoverageHint(); hint != "" {
		if w < 110 {
			hint = "Index new files: " + itoa(m.indexingDays()) + "d | , Settings"
		}
		return raised + "\n" + m.styles().muted.Render(truncate(hint, w)) + "\n" + m.rule(w)
	}
	return raised + "\n" + m.rule(w)
}

func (m *appModel) renderFooter(w int) string {
	var lines []string
	if m.hasPending {
		lines = append(lines, m.styles().accent.Render("New activity · ")+m.shortcutHints("", "R", "to refresh"))
	}
	if m.notice != "" {
		lines = append(lines, m.styles().path.Render(m.notice))
	}
	if m.partialCount > 0 {
		lines = append(lines, m.styles().gold.Render("Some logs could not be fully parsed; showing partial coverage."))
	}
	for _, failed := range m.failedRoots {
		lines = append(lines, m.styles().gold.Render("Could not scan "+failed+"; its cached sessions were kept."))
	}
	if m.overlay == overlayFilter {
		lines = append(lines, m.styles().path.Render("Filter: "+m.filterText+"_  (")+m.shortcutHints(" · ", "Enter", "keep", "Esc", "cancel")+m.styles().path.Render(")"))
		return m.rule(w) + "\n" + m.footerLines(lines, w)
	}
	if m.focus == focusOverview || m.overviewOpen {
		if find := m.overviewFindFooter(); find != "" {
			lines = append(lines, m.styles().path.Render(find))
		}
		lines = append(lines, m.shortcutHints("  ", "↑↓/jk", "Scroll", "PgUp/PgDown", "Page", "/", "Find", "[ ]", "Matches", "e", "Work", "Esc", "Back", "?", "Help"))
		return m.rule(w) + "\n" + m.footerLines(lines, w)
	}
	if w >= 110 {
		hints := m.shortcutHints("  ", "↑↓/jk", "Select", "Enter", "Overview", "p", "Project", "w", "When", "h", "Harness", "/", "Filter", "t", "Transcript", "s", "Search", "?", "Help", "q", "Quit")
		lines = append(lines, truncate(hints, w))
	} else {
		lines = append(lines,
			truncate(m.shortcutHints("  ", "↑↓/jk", "Select", "Enter", "Overview", "p", "Project", "w", "When", "h", "Harness"), w),
			truncate(m.shortcutHints("  ", "/", "Filter", "t", "Transcript", "?", "Help", "q", "Quit"), w))
	}
	if m.windowDays != 0 && len(m.visibleSessions()) == 0 {
		lines = append(lines, m.styles().path.Render("Show all time: ")+m.shortcutHints("", "w", "then All time"))
	}
	return m.rule(w) + "\n" + m.footerLines(lines, w)
}

// footerLines bounds notices so the list and navigation remain reachable.
func (m *appModel) footerLines(lines []string, w int) string {
	_, height, _, _ := m.viewport()
	limit := max(1, height/3-1)
	if len(lines) > limit {
		lines = append(lines[:max(0, limit-1)], lines[len(lines)-1])
	}
	for i, line := range lines {
		lines[i] = truncate(line, w)
	}
	return strings.Join(lines, "\n")
}

// panel reserves its border and padding before rendering selectable content.
func (m *appModel) panel(content string, w, h int) string {
	border := lipgloss.RoundedBorder()
	if m.ascii {
		border = lipgloss.ASCIIBorder()
	}
	style := lipgloss.NewStyle().Border(border).Padding(0, 1)
	if !m.noColor {
		p := m.palette()
		style = style.Foreground(lipgloss.Color(p.text)).BorderForeground(lipgloss.Color(p.separator))
	}
	return style.Render(fitPane(content, max(1, w-4), max(1, h-2)))
}

// renderList renders the day-grouped card list into h lines.
func (m *appModel) renderList(w, h int) string {
	cards := m.cards()
	if len(cards) == 0 {
		return m.emptyState(w)
	}
	now := m.now()
	var lines []string
	selectedLine := 0
	prevDay := "\x00"
	for i, c := range cards {
		day := "unknown"
		if c.row.LastActivity != nil {
			day = c.row.LastActivity.Format("2006-01-02")
		}
		if !c.child && day != prevDay {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			label := dayHeading(c.row.LastActivity, now)
			lines = append(lines, m.styles().muted.Render(label)+" "+m.rule(max(0, w-lipgloss.Width(label)-1)))
			prevDay = day
		}
		if i == m.sel {
			selectedLine = len(lines)
		}
		lines = append(lines, m.cardLines(c, i == m.sel, w)...)
		lines = append(lines, "")
	}
	return windowLines(lines, selectedLine, h)
}

func (m *appModel) cardLines(c card, selected bool, w int) []string {
	st := m.styles()
	indent := ""
	if c.child {
		indent = "  "
	}
	innerW := max(1, w-3-len(indent))
	purpose := c.row.Purpose
	if purpose == "" {
		purpose = "Purpose unavailable"
	}
	path := projectName(c.row.CWD) + "  " + shortenHome(c.row.CWD, m.home)
	meta := providerLabel(c.row.Provider)
	if c.row.Partial {
		meta += " · partial"
	}
	if c.child {
		meta += " · child session"
	}
	if c.childCount > 0 {
		meta += " · " + itoa(c.childCount) + " child session"
		if c.childCount > 1 {
			meta += "s"
		}
		if c.expanded {
			meta += " (e collapse)"
		} else {
			meta += " (e expand)"
		}
	}
	age := relativeTime(c.row.LastActivity, m.now())
	if lipgloss.Width(meta)+lipgloss.Width(age)+2 <= innerW {
		meta += strings.Repeat(" ", innerW-lipgloss.Width(meta)-lipgloss.Width(age)) + age
	} else {
		meta += " · " + age
	}
	rows := []string{st.purpose.Render(truncate(purpose, innerW)), st.path.Render(truncate(path, innerW)), st.muted.Render(truncate(meta, innerW))}
	for i, row := range rows {
		marker := "  "
		if selected {
			marker = st.accent.Render(m.selectionMarker())
		}
		row = marker + indent + row
		if selected {
			row = m.paintBackground(st.selected.Width(w).Render(row), true)
		} else {
			row = st.base.Width(w).Render(row)
		}
		rows[i] = row
	}
	return rows
}

// renderOverview renders the selected-session overview into h lines.
func (m *appModel) renderOverview(w, h int) string {
	c, ok := m.selected()
	if !ok {
		return ""
	}
	if m.overviewFind.source != c.row.SourcePath {
		m.overviewFind = overviewFindState{source: c.row.SourcePath}
	}
	ov := m.detail
	row := c.row
	var lines []string
	var blocks []overviewFindBlock
	appendBlock := func(source string, rendered []string, prefix int) {
		start := len(lines)
		lines = append(lines, rendered...)
		blocks = append(blocks, overviewFindBlock{source: ansi.Strip(source), start: start, end: len(lines), prefix: prefix})
	}
	purpose := row.Purpose
	if purpose == "" {
		purpose = "Purpose unavailable"
	}
	appendBlock(purpose, strings.Split(m.styles().purpose.Render(strings.Join(wrap(purpose, w), "\n")), "\n"), 0)
	meta := projectName(row.CWD) + " · " + providerLabel(row.Provider)
	if row.LastActivity != nil {
		meta += " · " + row.LastActivity.Format("Mon 02 Jan 15:04")
	} else {
		meta += " · date unknown"
	}
	appendBlock(meta, strings.Split(m.styles().muted.Render(strings.Join(wrap(meta, w), "\n")), "\n"), 0)
	lines = append(lines, "")
	lines = append(lines, m.styles().muted.Render("Directory"))
	var directory []string
	for _, line := range wrapPreserve(row.CWD, max(1, w-2)) {
		directory = append(directory, m.styles().rule.Render(m.verticalRule()+" ")+m.styles().path.Render(line))
	}
	appendBlock(row.CWD, directory, 2)
	if row.CWD == "" {
		lines[len(lines)-1] = "Directory unavailable"
	}
	lines = append(lines, "", m.styles().muted.Render("Purpose")+"  "+m.styles().muted.Render(originLabel(row.PurposeOrigin)))
	appendBlock(purpose, wrap(purpose, w), 0)
	lines = append(lines, "", m.styles().muted.Render("Initial request"))
	if ov != nil && ov.InitialRequest != "" {
		appendBlock(ov.InitialRequest, wrap(ov.InitialRequest, w), 0)
	} else {
		lines = append(lines, "Initial request unavailable")
	}
	lines = append(lines, "", m.styles().muted.Render("Latest exchange"))
	if ov != nil && (ov.LatestUser != "" || ov.LatestAssistant != "") {
		if ov.LatestUser != "" {
			appendBlock("User: "+ov.LatestUser, wrap("User: "+ov.LatestUser, w), 0)
		}
		if ov.LatestAssistant != "" {
			appendBlock("Agent reported: "+ov.LatestAssistant, wrap(m.styles().gold.Render("Agent reported:")+" "+ov.LatestAssistant, w), 0)
		}
	} else {
		lines = append(lines, "Latest exchange unavailable")
	}
	metadata := model.SessionMetadata{}
	if ov != nil {
		metadata = ov.Metadata
	}
	lines = append(lines, "", m.styles().muted.Render("Recorded session details"))
	for _, text := range overviewMetadataLines(metadata, row.Provider) {
		appendBlock(text, wrap(m.styleRecordedLine(text, false), w), 0)
	}
	workTitle := "[e] Recorded work · collapsed"
	if m.workExpanded {
		workTitle = "[e] Recorded work · expanded"
	}
	workHeading := len(lines) + 1
	lines = append(lines, "", m.styles().muted.Render(workTitle))
	if m.workExpanded {
		for _, text := range overviewWorkLines(metadata.Work) {
			appendBlock(text, wrap(m.styleRecordedLine(text, true), w), 0)
		}
	}
	if row.ParentID != "" {
		lines = append(lines, "", "Child session of "+row.ParentID)
	}
	if row.ForkedFrom != "" {
		lines = append(lines, "Forked from "+row.ForkedFrom)
	}
	actions := []string{m.rule(w), m.styles().muted.Render("Continue with this work"), m.paintBackground(m.styles().selected.Render(m.styles().accent.Render("[r] Resume")), true) + "   " + m.styles().accent.Render("[f] Fork") + "   " + m.styles().accent.Render("[n] Clean session")}
	// Keep the actions at the bottom while the session detail scrolls above.
	for _, line := range wrap("Opens command preparation; no agent is launched.", w) {
		actions = append(actions, m.styles().muted.Render(line))
	}
	if w < 58 {
		actions[2] = m.styles().accent.Render("[r] Resume  [f] Fork  [n] Clean session")
	}
	if h < 8 {
		actions = []string{m.rule(w), m.styles().accent.Render("[r] Resume  [f] Fork  [n] Clean")}
	}
	bodyH := max(1, h-len(actions))
	m.overviewViewportHeight = bodyH
	lines = m.applyOverviewFind(lines, blocks, bodyH)
	if m.workReveal {
		m.overviewScroll = workHeading
		m.workReveal = false
	}
	m.overviewScroll = scrollStart(len(lines), m.overviewScroll, bodyH)
	lines = lines[m.overviewScroll:]
	return fitPane(windowLines(lines, 0, bodyH), w, bodyH) + "\n" + strings.Join(actions, "\n")
}

// normalizeOverviewScroll uses only the overview layout, avoiding a complete
// session-list render for each scroll key.
func (m *appModel) normalizeOverviewScroll() {
	w, h, _, _ := m.viewport()
	bodyH := max(1, h-lipgloss.Height(m.renderHeader(w))-lipgloss.Height(m.renderFooter(w)))
	if m.isWide() && !m.overviewOpen {
		listW := max(30, w*45/100)
		w = w - listW - 5
	}
	m.renderOverview(w, bodyH)
}

func (m *appModel) viewOverviewPage(w, h int) string {
	header := m.renderHeader(w)
	footer := m.renderFooter(w)
	bodyH := h - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	body := fitPane(m.renderOverview(w, bodyH), w, bodyH)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *appModel) viewSelector(w, h int) string {
	var title string
	switch m.selector {
	case selectorTime:
		title = "When"
	case selectorProject:
		title = "Project"
	case selectorHarness:
		title = "Harness"
	}
	lines := []string{m.styles().heading.Render(title), ""}
	for i, opt := range m.selectorOptions() {
		marker := "  "
		if i == m.selCursor {
			marker = m.styles().accent.Render(m.selectionMarker())
		}
		label := opt.label
		if opt.note == "current" {
			label += " (current)"
		}
		row := marker + label
		if i == m.selCursor {
			row = m.paintBackground(m.styles().selected.Width(w).Render(row), true)
		}
		lines = append(lines, row)
	}
	return fitPane(windowLines(lines, m.selCursor+2, max(1, h-2)), w, max(1, h-2)) + "\n" + m.rule(w) + "\n" + truncate("↑↓/jk Select  Enter Choose  Esc Cancel", w)
}

func (m *appModel) viewHelp(w, h int) string {
	lines := []string{
		m.styles().heading.Render("Keys"), "",
		"↑↓/jk        Move selection",
		"Enter        Open or focus the session overview",
		"Esc          Back",
		"p            Project selector",
		"w            Time-window selector",
		"h            Harness selector",
		"/            Filter list or find in overview/transcript",
		"[ / ]        Previous / next find match",
		"e            Expand children / overview work / transcript tools",
		"R            Apply refreshed session ordering",
		"t            Transcript at the latest exchange",
		"T            Transcript at the initial request",
		"J / K        Scroll transcript one rendered line down / up",
		"PgUp/PgDown  Page transcript or overview",
		"s            Search session content",
		"r / f / n    Resume, fork, clean session",
		"c            Copy directory or session context",
		",            Sources and settings",
		"?            This help",
		"q            Quit",
		"", "Press Esc or q to close.",
	}
	return strings.Join(lines, "\n")
}

func originLabel(o model.PurposeOrigin) string {
	switch o {
	case model.OriginNativeTitle:
		return "Native title"
	case model.OriginRequestExcerpt:
		return "Request excerpt"
	default:
		return "Purpose unavailable"
	}
}

// shortenHome abbreviates the home directory prefix to ~.
func shortenHome(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// wrap word-wraps plain text to width w.
func wrap(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	return strings.Split(ansi.Wrap(s, w, ""), "\n")
}

// windowLines returns at most h lines keeping keepVisible in view.
func windowLines(lines []string, keepVisible, h int) string {
	if h <= 0 || len(lines) <= h {
		return strings.Join(lines, "\n")
	}
	start := keepVisible - h/2
	if start < 0 {
		start = 0
	}
	if start+h > len(lines) {
		start = len(lines) - h
	}
	return strings.Join(lines[start:start+h], "\n")
}

func (m *appModel) emptyState(w int) string {
	if len(m.sessions) == 0 {
		if m.scanFinished {
			return "No sessions indexed.\n\nPress , to check sources in Settings or rescan."
		}
		return "No session sources found yet.\n\nPress , to check sources in Settings."
	}
	if m.filterText != "" {
		return "No sessions match the active filter.\n\nFilter: " + m.filterText + "\nPress / to edit it, or clear the text to see every session in scope."
	}
	if m.windowDays == 7 {
		return "No sessions in the last 7 days.\n\nPress w and choose All time to see older work."
	}
	return "No sessions in the current scope.\n\nActive scope: " + m.windowLabel() + " · " + m.projectLabel() + " · " + m.harnessLabel() + "\nPress w, p, or h to adjust the scope."
}

func (m *appModel) viewSheet(w, h int) string {
	if m.sheet == nil {
		return ""
	}
	p := m.sheet
	lines := []string{m.styles().heading.Render(p.Title), ""}
	if p.Purpose != "" {
		lines = append(lines, wrap(p.Purpose, w)...)
	}
	lines = append(lines, "Provider: "+providerLabel(p.Provider), "")
	lines = append(lines, m.styles().heading.Render("What happens"))
	lines = append(lines, wrap(p.History, w)...)
	lines = append(lines, "", m.styles().heading.Render("Target directory"))
	target := p.TargetDir
	if target == "" {
		target = "unavailable"
	}
	lines = append(lines, wrap(target, w)...)
	lines = append(lines, "", m.styles().heading.Render("Command"))
	var pinned []string
	if p.Available {
		if m.sheetState == "failed" {
			// One unbroken line: the terminal soft-wraps it without
			// inserting characters, so manual selection stays exact.
			lines = append(lines, p.Command)
		} else {
			lines = append(lines, wrap(p.Command, w)...)
		}
		pinned = append(pinned, "This command is prepared for copying only. Trailmark does not run it.", "It targets a POSIX shell on Linux.")
	} else {
		lines = append(lines, wrap("Unavailable: "+p.Reason, w)...)
	}
	switch m.sheetState {
	case "copied":
		pinned = append(pinned, m.styles().accent.Render("Command copied to the clipboard."))
	case "failed":
		pinned = append(pinned, "Copy failed: the clipboard is unavailable. Select the command text above and copy it manually.")
	}
	if p.Available {
		pinned = append(pinned, "Enter/c Copy command  ↑↓/jk Scroll  Esc Back")
	} else {
		pinned = append(pinned, "↑↓/jk Scroll  Esc Back")
	}
	bodyH := h - len(pinned)
	if bodyH < 1 {
		bodyH = 1
	}
	for i, line := range pinned {
		pinned[i] = truncate(line, w)
	}
	body := windowLines(lines, m.sheetScroll, bodyH)
	if m.sheetState == "failed" {
		body = m.frame(body, w, bodyH)
	} else {
		body = fitPane(body, w, bodyH)
	}
	return body + "\n" + strings.Join(pinned, "\n")
}

func (m *appModel) viewCopyMenu(w, h int) string {
	lines := []string{m.styles().heading.Render("Copy"), ""}
	for i, item := range m.copyItems {
		marker := "  "
		if i == m.copyCursor {
			marker = m.styles().accent.Render(m.selectionMarker())
		}
		row := marker + item.label
		if i == m.copyCursor {
			row = m.paintBackground(m.styles().selected.Width(w).Render(row), true)
		}
		lines = append(lines, row)
	}
	return fitPane(windowLines(lines, m.copyCursor+2, max(1, h-2)), w, max(1, h-2)) + "\n" + m.rule(w) + "\n" + truncate("↑↓/jk Select  Enter Preview  Esc Back", w)
}

func (m *appModel) viewCopyPreview(w, h int) string {
	if m.copyPreview == nil {
		return ""
	}
	header := []string{m.styles().heading.Render(m.copyPreview.label), "", "This is exactly what will be copied:", ""}
	var pinned []string
	switch m.copyState {
	case "copied":
		pinned = append(pinned, m.styles().accent.Render("Copied to the clipboard."))
	case "failed":
		pinned = append(pinned, "Copy failed: the clipboard is unavailable. Select the text above and copy it manually.")
	}
	pinned = append(pinned, "Enter/c Copy  ↑↓/jk Scroll  Esc Back")
	bodyH := h - len(header) - len(pinned)
	if bodyH < 1 {
		bodyH = 1
	}
	for i, line := range pinned {
		pinned[i] = truncate(line, w)
	}
	body := fitPane(windowLines(wrapPreserve(m.copyPreview.text, w), m.copyScroll, bodyH), w, bodyH)
	return strings.Join(header, "\n") + "\n" + body + "\n" + strings.Join(pinned, "\n")
}

// wrapPreserve hard-wraps text at display width without collapsing
// or altering any whitespace inside a line. Only line breaks are
// added, so previews of copy payloads stay faithful.
func wrapPreserve(s string, w int) []string {
	if w <= 0 {
		return strings.Split(s, "\n")
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		line := ""
		width := 0
		for _, r := range para {
			rw := lipgloss.Width(string(r))
			if rw == 0 {
				rw = 1 // tabs and controls still occupy a cell here
			}
			if width+rw > w && line != "" {
				out = append(out, line)
				line, width = "", 0
			}
			line += string(r)
			width += rw
		}
		out = append(out, line)
	}
	return out
}

// viewQuitModal has its own raised surface and accent border so confirmation
// remains distinct from the surrounding session browser.
func (m *appModel) viewQuitModal(w, h int) string {
	p, st := m.palette(), m.styles()
	button := func(label string, focused, warning bool) string {
		marker := "  "
		if focused {
			marker = "> "
		}
		style := lipgloss.NewStyle().Padding(0, 1)
		if focused {
			style = style.Bold(true)
		}
		if !m.noColor {
			style = style.Foreground(lipgloss.Color(p.text)).Background(lipgloss.Color(p.selection))
			if focused {
				fill := p.accent
				if warning {
					fill = p.warning
				}
				ink := "#000000"
				if m.lightTheme {
					ink = "#ffffff"
				}
				style = style.Foreground(lipgloss.Color(ink)).Background(lipgloss.Color(fill))
			}
		}
		return style.Render("[" + marker + label + "]")
	}
	innerW := max(1, w-4)
	title := st.accent.Bold(true).Render("Quit Trailmark?")
	rule := st.accent.Render(strings.Repeat(m.horizontalRule(), innerW))
	choose := "←/→ or Tab Choose"
	if m.ascii {
		choose = "Left/Right or Tab Choose"
	}
	lines := []string{
		title,
		"Leave the session browser?",
		"",
		button("Cancel", !m.quitSelected, false) + "   " + button("Quit", m.quitSelected, true),
		"",
		rule,
		st.muted.Render(choose),
		st.muted.Render("Enter Confirm · Esc Cancel"),
	}
	if m.ascii {
		lines[len(lines)-1] = st.muted.Render("Enter Confirm | Esc Cancel")
	}
	border := lipgloss.RoundedBorder()
	if m.ascii {
		border = lipgloss.ASCIIBorder()
	}
	card := lipgloss.NewStyle().Border(border).Padding(0, 1)
	if !m.noColor {
		card = card.Foreground(lipgloss.Color(p.text)).Background(lipgloss.Color(p.raised)).BorderForeground(lipgloss.Color(p.accent)).BorderBackground(lipgloss.Color(p.raised))
	}
	content := card.Render(fitPane(strings.Join(lines, "\n"), innerW, max(1, h-2)))
	if !m.noColor {
		content = m.restoreSurface(content, p.raised)
		// End each card row before Place adds surrounding padding. The outer
		// frame restores the base surface after these resets.
		rows := strings.Split(content, "\n")
		for i := range rows {
			rows[i] += "\x1b[0m"
		}
		content = strings.Join(rows, "\n")
	}
	return content
}

func (m *appModel) terminalSize() (int, int) {
	if m.width == 0 {
		return 80, 24
	}
	return m.width, m.height
}

// viewport reserves one outer gutter around every mode. Small supported
// terminals keep their rows and use narrower horizontal margins.
func (m *appModel) viewport() (w, h, x, y int) {
	w, h = m.terminalSize()
	if w >= 44 {
		x = 2
	} else if w >= 40 {
		x = 1
	}
	if h >= 14 && w >= 40 {
		y = 1
	}
	return max(1, w-2*x), max(1, h-2*y), x, y
}

// wrapTranscript preserves explicit paragraphs and their indentation while
// preferring word boundaries. Oversized individual tokens still hard-wrap.
func wrapTranscript(s string, w int) []string {
	if w <= 0 {
		return strings.Split(s, "\n")
	}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\t", "    "), "\n") {
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		if len(indent) >= w {
			out = append(out, wrapPreserve(line, w)...)
			continue
		}
		for _, part := range strings.Split(ansi.Wrap(strings.TrimPrefix(line, indent), w-len(indent), ""), "\n") {
			out = append(out, indent+part)
		}
	}
	return out
}
