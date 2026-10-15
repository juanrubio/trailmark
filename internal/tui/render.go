package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/juanrubio/trailmark/internal/model"
)

var (
	styleAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleHeading = lipgloss.NewStyle().Bold(true)
	stylePurpose = lipgloss.NewStyle().Bold(true)
)

// View renders the current frame.
func (m *appModel) View() tea.View {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}
	var content string
	switch {
	case w < 40 || h < 12:
		content = "Terminal too small for Trailmark.\nResize to at least 40x12 · q quit"
	case m.overlay == overlayHelp:
		content = m.viewHelp(w, h)
	case m.overlay == overlaySelector:
		content = m.viewSelector(w, h)
	case m.overlay == overlaySheet:
		content = m.viewSheet(w, h)
	case m.overlay == overlayCopyMenu:
		content = m.viewCopyMenu(w, h)
	case m.overlay == overlayCopyPreview:
		content = m.viewCopyPreview(w, h)
	case m.overviewOpen:
		content = m.viewOverviewPage(w, h)
	default:
		content = m.viewMain(w, h)
	}
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
		list := m.renderList(listW, bodyH)
		overview := m.renderOverview(overviewW, bodyH)
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, " │ ", overview)
	} else {
		body = m.renderList(w, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *appModel) renderHeader(w int) string {
	scope := m.windowLabel() + " · " + m.projectLabel() + " · " + m.harnessLabel()
	if m.filterText != "" {
		scope += " · Filter: " + m.filterText
	}
	left := styleHeading.Render("Trailmark") + "  " + scope
	right := itoa(len(m.visibleSessions())) + " sessions"
	line := left
	if lipgloss.Width(left)+lipgloss.Width(right)+2 <= w {
		line = left + strings.Repeat(" ", w-lipgloss.Width(left)-lipgloss.Width(right)) + right
	} else {
		line = truncate(left, w)
	}
	return line + "\n" + strings.Repeat("─", min(w, 120))
}

func (m *appModel) renderFooter(w int) string {
	var lines []string
	if m.hasPending {
		lines = append(lines, styleAccent.Render("New activity · R to refresh"))
	}
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	if m.partialCount > 0 {
		lines = append(lines, "Some logs could not be fully parsed; showing partial coverage.")
	}
	for _, failed := range m.failedRoots {
		lines = append(lines, "Could not scan "+failed+"; its cached sessions were kept.")
	}
	if m.overlay == overlayFilter {
		lines = append(lines, "Filter: "+m.filterText+"_  (Enter keep · Esc cancel)")
		return strings.Join(lines, "\n")
	}
	if m.focus == focusOverview || m.overviewOpen {
		lines = append(lines, "↑↓/jk Scroll  Esc Back  ? Help")
		return strings.Join(lines, "\n")
	}
	if w >= 110 {
		hints := "↑↓/jk Select  Enter Overview  p Project  w When  h Harness  / Filter  t Transcript  ? Help  q Quit"
		lines = append(lines, truncate(hints, w))
	} else {
		lines = append(lines,
			truncate("↑↓/jk Select  Enter Overview  p Project  w When  h Harness", w),
			truncate("/ Filter  t Transcript  ? Help  q Quit", w))
	}
	if m.windowDays != 0 && len(m.visibleSessions()) == 0 {
		lines = append(lines, "Show all time: w then All time")
	}
	return strings.Join(lines, "\n")
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
			lines = append(lines, styleHeading.Render(dayHeading(c.row.LastActivity, now)))
			prevDay = day
		}
		if i == m.sel {
			selectedLine = len(lines)
		}
		lines = append(lines, m.cardLines(c, i == m.sel, w)...)
	}
	return windowLines(lines, selectedLine, h)
}

func (m *appModel) cardLines(c card, selected bool, w int) []string {
	marker := "  "
	if selected {
		marker = styleAccent.Render("▌ ")
	}
	indent := ""
	if c.child {
		indent = "  "
	}
	purpose := c.row.Purpose
	if purpose == "" {
		purpose = "Purpose unavailable"
	}
	line1 := marker + indent + stylePurpose.Render(truncate(purpose, w-4-len(indent)))
	path := shortenHome(c.row.CWD, m.home)
	line2 := "  " + indent + truncate(projectName(c.row.CWD)+"  "+path, w-2)
	meta := providerLabel(c.row.Provider) + " · " + relativeTime(c.row.LastActivity, m.now())
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
	line3 := "  " + indent + styleDim.Render(truncate(meta, w-2))
	return []string{line1, line2, line3}
}

// renderOverview renders the selected-session overview into h lines.
func (m *appModel) renderOverview(w, h int) string {
	c, ok := m.selected()
	if !ok {
		return ""
	}
	ov := m.detail
	row := c.row
	var lines []string
	purpose := row.Purpose
	if purpose == "" {
		purpose = "Purpose unavailable"
	}
	for _, l := range wrap(purpose, w) {
		lines = append(lines, stylePurpose.Render(l))
	}
	meta := projectName(row.CWD) + " · " + providerLabel(row.Provider)
	if row.LastActivity != nil {
		meta += " · " + row.LastActivity.Format("Mon 02 Jan 15:04")
	} else {
		meta += " · date unknown"
	}
	for _, l := range wrap(meta, w) {
		lines = append(lines, styleDim.Render(l))
	}
	lines = append(lines, "")
	lines = append(lines, styleHeading.Render("Directory"))
	lines = append(lines, wrap(row.CWD, w)...)
	if row.CWD == "" {
		lines[len(lines)-1] = "Directory unavailable"
	}
	lines = append(lines, "", styleHeading.Render("Purpose")+"  "+styleDim.Render(originLabel(row.PurposeOrigin)))
	lines = append(lines, wrap(purpose, w)...)
	lines = append(lines, "", styleHeading.Render("Initial request"))
	if ov != nil && ov.InitialRequest != "" {
		lines = append(lines, wrap(ov.InitialRequest, w)...)
	} else {
		lines = append(lines, "Initial request unavailable")
	}
	lines = append(lines, "", styleHeading.Render("Latest exchange"))
	if ov != nil && (ov.LatestUser != "" || ov.LatestAssistant != "") {
		if ov.LatestUser != "" {
			lines = append(lines, wrap("User: "+ov.LatestUser, w)...)
		}
		if ov.LatestAssistant != "" {
			lines = append(lines, wrap("Agent reported: "+ov.LatestAssistant, w)...)
		}
	} else {
		lines = append(lines, "Latest exchange unavailable")
	}
	if row.ParentID != "" {
		lines = append(lines, "", "Child session of "+row.ParentID)
	}
	if row.ForkedFrom != "" {
		lines = append(lines, "Forked from "+row.ForkedFrom)
	}
	lines = append(lines, "", "Continue with this work")
	lines = append(lines, "[r] Resume   [f] Fork   [n] Clean session")
	for _, l := range wrap("Opens command preparation; no agent is launched.", w) {
		lines = append(lines, styleDim.Render(l))
	}
	if m.overviewScroll > 0 {
		maxScroll := len(lines) - 1
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := m.overviewScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}
		lines = lines[scroll:]
	}
	return windowLines(lines, 0, h)
}

func (m *appModel) viewOverviewPage(w, h int) string {
	header := m.renderHeader(w)
	footer := m.renderFooter(w)
	bodyH := h - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	body := m.renderOverview(w, bodyH)
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
	lines := []string{styleHeading.Render(title), ""}
	for i, opt := range m.selectorOptions() {
		marker := "  "
		if i == m.selCursor {
			marker = styleAccent.Render("▌ ")
		}
		label := opt.label
		if opt.note == "current" {
			label += " (current)"
		}
		lines = append(lines, marker+label)
	}
	lines = append(lines, "", "↑↓/jk Select  Enter Choose  Esc Cancel")
	return windowLines(lines, m.selCursor+2, h)
}

func (m *appModel) viewHelp(w, h int) string {
	lines := []string{
		styleHeading.Render("Keys"), "",
		"↑↓/jk        Move selection",
		"Enter        Open or focus the session overview",
		"Esc          Back",
		"p            Project selector",
		"w            Time-window selector",
		"h            Harness selector",
		"/            Filter purpose, project, path, provider",
		"e            Expand or collapse child sessions",
		"R            Apply refreshed session ordering",
		"t            Transcript (later phase)",
		"r / f / n    Resume, fork, clean session",
		"c            Copy directory or session context",
		"s            Content search (later phase)",
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
	var out []rune
	width := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > w-1 {
			break
		}
		out = append(out, r)
		width += rw
	}
	return string(out) + "…"
}

// wrap word-wraps plain text to width w.
func wrap(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if lipgloss.Width(line)+1+lipgloss.Width(word) > w {
				out = append(out, line)
				line = word
			} else {
				line += " " + word
			}
		}
		out = append(out, line)
	}
	return out
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
		return "No session sources found yet.\n\nSessions appear after the first scan completes."
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
	lines := []string{styleHeading.Render(p.Title), ""}
	if p.Purpose != "" {
		lines = append(lines, wrap(p.Purpose, w)...)
	}
	lines = append(lines, "Provider: "+providerLabel(p.Provider), "")
	lines = append(lines, styleHeading.Render("What happens"))
	lines = append(lines, wrap(p.History, w)...)
	lines = append(lines, "", styleHeading.Render("Target directory"))
	target := p.TargetDir
	if target == "" {
		target = "unavailable"
	}
	lines = append(lines, wrap(target, w)...)
	lines = append(lines, "", styleHeading.Render("Command"))
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
		pinned = append(pinned, styleAccent.Render("Command copied to the clipboard."))
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
	return windowLines(lines, m.sheetScroll, bodyH) + "\n" + strings.Join(pinned, "\n")
}

func (m *appModel) viewCopyMenu(w, h int) string {
	lines := []string{styleHeading.Render("Copy"), ""}
	for i, item := range m.copyItems {
		marker := "  "
		if i == m.copyCursor {
			marker = styleAccent.Render("▌ ")
		}
		lines = append(lines, marker+item.label)
	}
	lines = append(lines, "", "↑↓/jk Select  Enter Preview  Esc Back")
	return strings.Join(lines, "\n")
}

func (m *appModel) viewCopyPreview(w, h int) string {
	if m.copyPreview == nil {
		return ""
	}
	header := []string{styleHeading.Render(m.copyPreview.label), "", "This is exactly what will be copied:", ""}
	var pinned []string
	switch m.copyState {
	case "copied":
		pinned = append(pinned, styleAccent.Render("Copied to the clipboard."))
	case "failed":
		pinned = append(pinned, "Copy failed: the clipboard is unavailable. Select the text above and copy it manually.")
	}
	pinned = append(pinned, "Enter/c Copy  ↑↓/jk Scroll  Esc Back")
	bodyH := h - len(header) - len(pinned)
	if bodyH < 1 {
		bodyH = 1
	}
	body := windowLines(wrapPreserve(m.copyPreview.text, w), m.copyScroll, bodyH)
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
