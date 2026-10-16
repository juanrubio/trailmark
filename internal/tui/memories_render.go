package tui

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/juanrubio/trailmark/internal/memory"
)

// viewMemories mirrors viewMain: a list/detail split on wide
// terminals (same breakpoint and proportions as the session
// overview layout) and list-then-detail on narrow ones.
func (m *appModel) viewMemories(w, h int) string {
	st := m.mem
	if st.sourcesOpen {
		return m.viewMemorySources(w, h)
	}
	header := m.memHeader(w)
	footer := m.memFooter(w)
	bodyH := h - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	var body string
	switch {
	case m.isWide():
		listW := w * 45 / 100
		if listW < 30 {
			listW = 30
		}
		detailW := w - listW - 3
		list := fitPane(m.memList(listW-2, bodyH), listW-2, bodyH)
		detail := fitPane(m.memDetail(detailW-2, bodyH), detailW-2, bodyH)
		divider := m.styles().rule.Render(strings.TrimSuffix(strings.Repeat(" "+m.verticalRule()+" \n", bodyH), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Padding(0, 1).Render(list), divider, lipgloss.NewStyle().Padding(0, 1).Render(detail))
	case st.detailOpen:
		body = fitPane(m.memDetail(w, bodyH), w, bodyH)
	default:
		body = fitPane(m.memList(w, bodyH), w, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *appModel) memHeader(w int) string {
	st := m.mem
	scope := st.harnessScopeLabel() + " · " + st.projectScopeLabel() + " · " + memCategorySets[st.catIdx].label
	if st.filterText != "" {
		scope += " · Filter: " + st.filterText
	}
	if st.searchActive {
		scope += " · Search: " + strings.TrimSpace(st.searchQuery)
	}
	if m.isWide() && st.focus == memFocusDetail {
		scope += " · Detail focused"
	}
	// Distinct mode styling: an inverted accent badge instead of
	// the Sessions accent wordmark, over an accent-colored rule,
	// so the active mode is recognizable at a glance.
	left := m.styles().modeBadge.Render(" Memories ") + " " + scope
	right := itoa(len(st.visibleEntries())) + " memories"
	line := left
	if lipgloss.Width(left)+lipgloss.Width(right)+2 <= w {
		line = left + strings.Repeat(" ", w-lipgloss.Width(left)-lipgloss.Width(right)) + right
	} else {
		line = truncate(left, w)
	}
	raised := m.styles().raised.Width(w).Render(line)
	if !m.noColor {
		raised = m.restoreSurface(raised, m.palette().raised)
	}
	return raised + "\n" + m.styles().accent.Render(strings.Repeat(m.horizontalRule(), max(0, w)))
}

func (m *appModel) memFooter(w int) string {
	st := m.mem
	var lines []string
	if st.loading {
		lines = append(lines, m.styles().path.Render("Loading memories…"))
	}
	if st.notice != "" {
		lines = append(lines, m.styles().path.Render(st.notice))
	}
	switch st.editing {
	case "filter":
		lines = append(lines, m.styles().path.Render("Filter: "+st.filterText+"_  (")+m.shortcutHints(" · ", "Enter", "keep", "Esc", "cancel")+m.styles().path.Render(")"))
		return m.rule(w) + "\n" + m.footerLines(lines, w)
	case "search":
		lines = append(lines, m.styles().path.Render("Search content: "+st.searchQuery+"_  (")+m.shortcutHints(" · ", "Enter", "search", "Esc", "cancel")+m.styles().path.Render(")"))
		return m.rule(w) + "\n" + m.footerLines(lines, w)
	}
	if w >= 110 {
		lines = append(lines,
			truncate(m.shortcutHints("  ", "↑↓/jk", "Select", "Enter", "Open", "Tab", "Focus", "/", "Filter", "s", "Search", "n/N", "Matches"), w),
			truncate(m.shortcutHints("  ", "h", "Harness", "p", "Project", "a", "Categories", "v", "Raw/rendered", "o", "Open reference", "c", "Copy", "i", "Sources", "R", "Refresh", "?", "Help", "m", "Sessions"), w))
	} else {
		lines = append(lines,
			truncate(m.shortcutHints("  ", "↑↓/jk", "Select", "Enter", "Open", "/", "Filter", "s", "Search"), w),
			truncate(m.shortcutHints("  ", "h", "Harness", "p", "Project", "a", "Categories", "v", "Raw", "m", "Sessions", "Esc", "Back"), w))
	}
	return m.rule(w) + "\n" + m.footerLines(lines, w)
}

// memList renders the entry list into h lines, with search snippets
// under each entry while a content search is active.
func (m *appModel) memList(w, h int) string {
	st := m.mem
	st.listHeight = h
	if st.catalog == nil {
		if st.loadErr != "" {
			return st.loadErr + "\n\nPress R to retry, or i to see source status."
		}
		return "Loading memories…"
	}
	if len(st.catalog.Sources) == 0 {
		return strings.Join(wrap("No memory sources were found on this machine.", w), "\n") + "\n\n" +
			strings.Join(wrap("Trailmark looks beside your Claude Code and Codex session stores. To add a memory location, use Add memory root in Settings (, from the Sessions view) or add it under memory_roots in Trailmark's configuration, then press R here. Press i for source status.", w), "\n")
	}
	entries := st.visibleEntries()
	if len(entries) == 0 {
		switch {
		case st.searchActive:
			return strings.Join(wrap(fmt.Sprintf("No matches for %q in the current scope.", strings.TrimSpace(st.searchQuery)), w), "\n") + "\n\nPress s to edit the search, or broaden with h (harness), p (project), or a (categories)."
		case len(st.catalog.Entries) == 0:
			return "No memories found in the configured sources.\n\nPress i to review source coverage, or R to refresh."
		default:
			return "No memories match the current filters.\n\nActive scope: " + st.harnessScopeLabel() + " · " + st.projectScopeLabel() + " · " + memCategorySets[st.catIdx].label + "\nPress h, p, or a to adjust the scope, or / to edit the filter."
		}
	}
	var lines []string
	selectedLine := 0
	for i, e := range entries {
		if i == st.sel {
			selectedLine = len(lines)
		}
		lines = append(lines, m.memCardLines(e, i == st.sel, w)...)
		if st.searchActive && i < len(st.matches) {
			mt := st.matches[i]
			if mt.Count > 0 {
				label := itoa(mt.Count) + " matches"
				if mt.Count == 1 {
					label = "1 match"
				}
				lines = append(lines, "  "+m.styles().muted.Render(truncate(label, max(1, w-2))))
			}
			for _, sn := range mt.Snippets[:min(2, len(mt.Snippets))] {
				snippet := strings.ReplaceAll(sanitizeText(sn), "\n", " ")
				hl := highlightWithStyle(snippet, strings.TrimSpace(st.searchQuery), m.styles().match)
				lines = append(lines, "  "+truncate(hl, max(1, w-2)))
			}
		}
		lines = append(lines, "")
	}
	return windowLines(lines, selectedLine, h)
}

func (m *appModel) memCardLines(e memory.Entry, selected bool, w int) []string {
	st := m.styles()
	innerW := max(1, w-3)
	title := sanitizeText(e.Title)
	if title == "" {
		title = "(untitled)"
	}
	project := memEntryProjectShort(e)
	if e.ProjectCWD != "" {
		project += "  " + sanitizeText(shortenHome(e.ProjectCWD, m.home))
	}
	meta := providerLabel(e.Provider) + " · " + memCategoryLabel(e.Category)
	if !e.ModTime.IsZero() {
		mt := e.ModTime
		age := relativeTime(&mt, m.now())
		if lipgloss.Width(meta)+lipgloss.Width(age)+2 <= innerW {
			meta += strings.Repeat(" ", innerW-lipgloss.Width(meta)-lipgloss.Width(age)) + age
		} else {
			meta += " · " + age
		}
	}
	rows := []string{st.purpose.Render(truncate(title, innerW)), st.path.Render(truncate(project, innerW)), st.muted.Render(truncate(meta, innerW))}
	for i, row := range rows {
		marker := "  "
		if selected {
			marker = st.accent.Render(m.selectionMarker())
		}
		row = marker + row
		if selected {
			row = m.paintBackground(st.selected.Width(w).Render(row), true)
		} else {
			row = st.base.Width(w).Render(row)
		}
		rows[i] = row
	}
	return rows
}

// memLine is one rendered detail line; src is its 1-based source
// line in raw mode (0 for headers and rendered output).
type memLine struct {
	text string
	src  int
}

// memDetail renders the selected entry: header metadata first, then
// the rendered Markdown or raw source, then recorded references.
func (m *appModel) memDetail(w, h int) string {
	st := m.mem
	st.detailHeight = h
	e, ok := st.selectedEntry()
	if !ok {
		if st.catalog == nil && st.loadErr == "" {
			return "Loading memories…"
		}
		return ""
	}
	lines := m.memDetailLines(e, w)
	// Reveal the active search-match line once per navigation, then
	// clamp scrolling the same way the overview does. Manual
	// scrolling after the reveal must not snap back to the match.
	if st.revealPending && st.activeLine > 0 {
		for i, l := range lines {
			if l.src == st.activeLine {
				if i < st.detailScroll || i >= st.detailScroll+h {
					st.detailScroll = max(0, i-h/2)
				}
				break
			}
		}
		st.revealPending = false
	}
	st.detailScroll = scrollStart(len(lines), st.detailScroll, h)
	texts := make([]string, 0, len(lines))
	for _, l := range lines {
		texts = append(texts, l.text)
	}
	end := min(len(texts), st.detailScroll+h)
	return strings.Join(texts[st.detailScroll:end], "\n")
}

func (m *appModel) memDetailLines(e memory.Entry, w int) []memLine {
	st := m.mem
	var lines []memLine
	add := func(text string) { lines = append(lines, memLine{text: text}) }
	addWrapped := func(text string) {
		for _, l := range wrap(sanitizeText(text), w) {
			add(l)
		}
	}
	title := sanitizeText(e.Title)
	if title == "" {
		title = "(untitled)"
	}
	for _, l := range strings.Split(m.styles().purpose.Render(strings.Join(wrap(title, w), "\n")), "\n") {
		add(l)
	}
	addWrapped(providerLabel(e.Provider) + " · " + memCategoryLabel(e.Category) + " · " + memEntryProjectShort(e))
	add("")
	project := "Project: "
	switch {
	case e.ProjectCWD != "":
		project += shortenHome(e.ProjectCWD, m.home)
		if label := memEvidenceLabel(e.ProjectEvidence); label != "" {
			project += " (" + label + ")"
		}
	case e.ProjectEvidence == "global":
		project += "Global (recorded as global)"
	default:
		project += "Unknown (no recorded association)"
	}
	addWrapped(project)
	source := "Source: " + shortenHome(e.Path, m.home)
	if e.StartLine > 0 {
		source += ":" + itoa(e.StartLine)
		if e.EndLine > e.StartLine {
			source += "-" + itoa(e.EndLine)
		}
	}
	addWrapped(source)
	if e.ModTime.IsZero() {
		add("Modified: unknown")
	} else {
		add("Modified: " + e.ModTime.Format("Mon 02 Jan 2006 15:04"))
	}
	if e.RecordedAt != nil {
		add("Recorded: " + e.RecordedAt.Format("Mon 02 Jan 2006 15:04"))
	}
	if e.Type != "" {
		addWrapped("Type: " + e.Type + " (recorded)")
	}
	if e.Description != "" {
		addWrapped("Description: " + e.Description)
	}
	if e.OriginSessionID != "" {
		addWrapped("Origin session: " + e.OriginSessionID)
	}
	if e.ThreadID != "" {
		addWrapped("Thread: " + e.ThreadID)
	}
	if len(e.HeadingPath) > 0 {
		addWrapped("Section: " + strings.Join(e.HeadingPath, " > "))
	}
	if doc, found := st.document(e.DocumentID); found && doc.Status != "" && doc.Status != "ok" {
		status := "Source status: " + doc.Status
		if doc.Status == "truncated" {
			status += " — showing the readable prefix of this file."
		}
		if doc.StatusDetail != "" {
			status += " " + doc.StatusDetail
		}
		for _, l := range wrap(m.styles().gold.Render(sanitizeText(status)), w) {
			add(l)
		}
	}
	add("")
	add(m.rule(w))
	mode := "Rendered"
	if st.raw {
		mode = "Raw source"
	}
	add(m.styles().muted.Render(mode + " · v toggles"))
	if st.raw {
		lines = append(lines, m.memRawLines(e, w)...)
	} else {
		for _, l := range m.memRenderedLines(e, w) {
			add(l)
		}
	}
	add("")
	add(m.styles().muted.Render("References"))
	refs := memReferenceLines(e)
	if len(refs) == 0 {
		add("No references recorded.")
	}
	for _, ref := range refs {
		addWrapped(ref)
	}
	return lines
}

// memRawLines shows the exact source slice with source line
// numbers. All source text is sanitized of terminal escape and
// control sequences before display.
func (m *appModel) memRawLines(e memory.Entry, w int) []memLine {
	st := m.mem
	raw := e.RawText
	if raw == "" {
		raw = e.Text
	}
	srcLines := strings.Split(strings.TrimSuffix(sanitizeText(raw), "\n"), "\n")
	numW := len(itoa(max(1, e.EndLine)))
	bodyW := max(1, w-numW-1)
	var out []memLine
	for i, l := range srcLines {
		src := e.StartLine + i
		l = strings.ReplaceAll(l, "\t", "    ")
		parts := wrapPreserve(l, bodyW)
		for j, part := range parts {
			num := strings.Repeat(" ", numW)
			if j == 0 {
				num = fmt.Sprintf("%*d", numW, src)
			}
			if src == st.activeLine && strings.TrimSpace(st.searchQuery) != "" {
				part = highlightWithStyle(part, strings.TrimSpace(st.searchQuery), m.styles().match)
			}
			out = append(out, memLine{text: m.styles().muted.Render(num) + " " + part, src: src})
		}
	}
	return out
}

// escapeForGlamour escapes ampersands everywhere glamour would
// entity-decode text, using glamour's own Markdown parser
// (goldmark, same extensions) to identify the two regions it
// does not decode: code-block contents and link/image
// destinations (plus autolink URLs). Saved commands and URLs
// keep their literal ampersands there; everything else — prose,
// inline spans, HTML blocks, malformed links — is escaped, so
// decoded control characters can never reach the terminal.
func escapeForGlamour(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	ranges := glamourLiteralRanges(s)
	var b strings.Builder
	b.Grow(len(s))
	ri := 0
	for i := 0; i < len(s); i++ {
		for ri < len(ranges) && i >= ranges[ri][1] {
			ri++
		}
		if s[i] != '&' {
			b.WriteByte(s[i])
			continue
		}
		if (ri < len(ranges) && i >= ranges[ri][0] && i < ranges[ri][1]) || safeNamedEntity(s[i+1:]) {
			b.WriteByte('&')
		} else {
			b.WriteString("&amp;")
		}
	}
	return b.String()
}

func safeNamedEntity(rest string) bool {
	for _, ent := range []string{"amp;", "lt;", "gt;", "quot;", "apos;"} {
		if strings.HasPrefix(rest, ent) {
			return true
		}
	}
	return false
}

// glamourLiteralRanges returns sorted, merged byte ranges of s
// that glamour renders without entity decoding: fenced/indented
// code block contents, link and image destinations, and
// autolink URL text.
func glamourLiteralRanges(s string) [][2]int {
	src := []byte(s)
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	pctx := parser.NewContext()
	doc := md.Parser().Parse(text.NewReader(src), parser.WithContext(pctx))
	var ranges [][2]int
	for _, ref := range pctx.References() {
		if r, ok := referenceDestinationRange(src, ref.Label(), ref.Destination()); ok {
			ranges = append(ranges, r)
		}
	}
	// Autolink labels are located sequentially in document
	// order with an advancing cursor, so an identical string
	// inside a code span (a text segment that advances the
	// cursor) or an earlier autolink can never be mistaken for
	// the parsed autolink.
	type spanEvent struct {
		start, stop int
		label       []byte // non-nil for autolinks
		exempt      bool   // false for autolinks inside strikethrough
	}
	var events []spanEvent
	// Glamour's strikethrough renderer flattens descendants and
	// entity-decodes the whole text, so nothing inside one may
	// be exempted — but its text and autolinks still consume
	// their source occurrences during sequential location.
	strikeDepth := 0
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if n.Kind() == extast.KindStrikethrough {
			if entering {
				strikeDepth++
			} else {
				strikeDepth--
			}
			return ast.WalkContinue, nil
		}
		if !entering {
			return ast.WalkContinue, nil
		}
		// HTML blocks produce no text segments, so record their
		// extent explicitly: the autolink cursor must advance
		// past raw HTML, or a URL string inside it could be
		// mistaken for a later autolink. (Blocks with inline
		// children advance the cursor via their text segments.)
		if n.Kind() == ast.KindHTMLBlock {
			if segs := n.Lines(); segs.Len() > 0 {
				events = append(events, spanEvent{start: segs.At(0).Start, stop: segs.At(segs.Len() - 1).Stop})
			}
		}
		if strikeDepth > 0 {
			switch n.Kind() {
			case ast.KindAutoLink:
				if al, ok := n.(*ast.AutoLink); ok {
					if label := al.Label(src); len(label) > 0 {
						events = append(events, spanEvent{label: label})
					}
				}
				return ast.WalkSkipChildren, nil
			case ast.KindText:
				if t, ok := n.(*ast.Text); ok {
					events = append(events, spanEvent{start: t.Segment.Start, stop: t.Segment.Stop})
				}
			}
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindFencedCodeBlock, ast.KindCodeBlock:
			segs := n.Lines()
			for i := 0; i < segs.Len(); i++ {
				seg := segs.At(i)
				ranges = append(ranges, [2]int{seg.Start, seg.Stop})
			}
			return ast.WalkSkipChildren, nil
		case ast.KindLink, ast.KindImage:
			var dest []byte
			switch ln := n.(type) {
			case *ast.Link:
				dest = ln.Destination
			case *ast.Image:
				dest = ln.Destination
			}
			if r, ok := linkDestinationRange(n, src, dest); ok {
				ranges = append(ranges, r)
			}
		case ast.KindAutoLink:
			if al, ok := n.(*ast.AutoLink); ok {
				if label := al.Label(src); len(label) > 0 {
					events = append(events, spanEvent{label: label, exempt: true})
				}
			}
			return ast.WalkSkipChildren, nil
		case ast.KindText:
			if t, ok := n.(*ast.Text); ok {
				events = append(events, spanEvent{start: t.Segment.Start, stop: t.Segment.Stop})
			}
		}
		return ast.WalkContinue, nil
	})
	cursor := 0
	for _, ev := range events {
		if ev.label == nil {
			if ev.stop > cursor {
				cursor = ev.stop
			}
			continue
		}
		// Whichever form (angle-bracketed or bare) occurs first
		// at or after the cursor is this autolink's span;
		// preferring one form could bind to a later copy.
		angle := append(append([]byte("<"), ev.label...), '>')
		start := -1
		if k := bytes.Index(src[cursor:], angle); k >= 0 {
			start = cursor + k + 1
		}
		if k := bytes.Index(src[cursor:], ev.label); k >= 0 {
			if s := cursor + k; start < 0 || s < start {
				start = s
			}
		}
		if start < 0 {
			continue
		}
		// Every autolink consumes its occurrence, exempt or not,
		// so a later identical autolink binds to its own span.
		cursor = start + len(ev.label)
		if ev.exempt {
			ranges = append(ranges, [2]int{start, cursor})
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	merged := ranges[:0]
	for _, r := range ranges {
		if n := len(merged); n > 0 && r[0] <= merged[n-1][1] {
			if r[1] > merged[n-1][1] {
				merged[n-1][1] = r[1]
			}
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

// referenceDestinationRange locates a link reference
// definition's destination token in the source and exempts it
// only when the raw token equals the parsed destination.
// Labels match the parser's normalization loosely (case-folded,
// whitespace-collapsed); anything unexpected yields no
// exemption.
func referenceDestinationRange(src, label, dest []byte) ([2]int, bool) {
	norm := func(b []byte) string {
		return strings.Join(strings.Fields(strings.ToLower(string(b))), " ")
	}
	for i := 0; i < len(src); i++ {
		if src[i] != '[' {
			continue
		}
		j := bytes.Index(src[i:], []byte("]:"))
		if j < 0 {
			break
		}
		if norm(src[i+1:i+j]) != norm(label) {
			continue
		}
		k := i + j + 2
		for k < len(src) && (src[k] == ' ' || src[k] == '\t' || src[k] == '\n') {
			k++
		}
		if k >= len(src) {
			break
		}
		if src[k] == '<' {
			if e := bytes.IndexByte(src[k:], '>'); e > 1 && bytes.Equal(src[k+1:k+e], dest) {
				return [2]int{k + 1, k + e}, true
			}
			continue
		}
		end := k
		for end < len(src) && src[end] != ' ' && src[end] != '\t' && src[end] != '\n' {
			end++
		}
		if bytes.Equal(src[k:end], dest) {
			return [2]int{k, end}, true
		}
	}
	return [2]int{}, false
}

// linkDestinationRange locates a parsed link/image destination
// in the source: the token right after the "](" following the
// link text. The parser has already validated the link, so a
// balanced scan of that token is exact; anything unexpected
// yields no exemption.
func linkDestinationRange(n ast.Node, src, dest []byte) ([2]int, bool) {
	lastStop := -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := c.(*ast.Text); ok && t.Segment.Stop > lastStop {
				lastStop = t.Segment.Stop
			}
		}
		return ast.WalkContinue, nil
	})
	if lastStop < 0 {
		return [2]int{}, false
	}
	// The destination must belong to this link: "](" immediately
	// after the link text (allowing only closing emphasis
	// markers between). Reference-style links have no inline
	// destination here and must never exempt later text.
	j := lastStop
	for j < len(src) && (src[j] == '*' || src[j] == '_' || src[j] == '~') {
		j++
	}
	if j+1 >= len(src) || src[j] != ']' || src[j+1] != '(' {
		return [2]int{}, false
	}
	start := j + 2
	if start >= len(src) {
		return [2]int{}, false
	}
	// Only exempt when the raw token is exactly the parsed
	// destination: any difference means the parser decoded
	// entities or escapes, and the raw bytes stay escaped.
	okRange := func(a, b int) ([2]int, bool) {
		if bytes.Equal(src[a:b], dest) {
			return [2]int{a, b}, true
		}
		return [2]int{}, false
	}
	if src[start] == '<' {
		if j := bytes.IndexByte(src[start:], '>'); j > 1 {
			return okRange(start+1, start+j)
		}
		return [2]int{}, false
	}
	depth := 0
	for j := start; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '(':
			depth++
		case ')':
			if depth == 0 {
				if j == start {
					return [2]int{}, false
				}
				return okRange(start, j)
			}
			depth--
		case ' ', '\t', '\n':
			if j == start {
				return [2]int{}, false
			}
			return okRange(start, j)
		}
	}
	return [2]int{}, false
}

// memRenderedLines renders Markdown through glamour, cached per
// entry, width, and theme. In no-color or ASCII mode (and if
// glamour fails) it falls back to plain wrapped sanitized text.
// Source text is sanitized before glamour ever sees it, including
// entity-encoded controls glamour would otherwise decode.
func (m *appModel) memRenderedLines(e memory.Entry, w int) []string {
	st := m.mem
	sanitized := sanitizeText(e.Text)
	if strings.TrimSpace(sanitized) == "" {
		return []string{"This entry has no body text. Press v for the raw source."}
	}
	if m.noColor || m.ascii {
		return wrapTranscript(sanitized, w)
	}
	text := escapeForGlamour(sanitized)
	themeKey := "dark"
	if m.lightTheme {
		themeKey = "light"
	}
	key := e.ID + "|" + itoa(w) + "|" + themeKey
	if cached, ok := st.rendered[key]; ok {
		return strings.Split(cached, "\n")
	}
	rendered := ""
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(themeKey),
		glamour.WithWordWrap(max(1, w)),
	)
	if err == nil {
		rendered, err = renderer.Render(text)
	}
	if err != nil || rendered == "" {
		return wrapTranscript(sanitized, w)
	}
	lines := strings.Split(strings.Trim(rendered, "\n"), "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	joined := strings.Join(lines, "\n")
	if st.rendered == nil {
		st.rendered = map[string]string{}
	}
	st.rendered[key] = joined
	return lines
}

// memReferenceLines lists recorded references with their resolution
// status. External links are shown as text and never opened.
func memReferenceLines(e memory.Entry) []string {
	var out []string
	for _, ref := range e.References {
		raw := sanitizeText(ref.Raw)
		label := raw
		switch ref.Kind {
		case "note":
			label = "Note: " + raw
		case "session":
			label = "Session: " + raw
			if ref.SessionID != "" {
				label = "Session: " + ref.SessionID
			}
		case "url":
			label = "Link: " + raw
		default:
			label = "Path: " + raw
		}
		status := "unavailable"
		switch ref.Status {
		case "resolved":
			status = "available · o opens"
		case "external":
			status = "external (not opened)"
		}
		out = append(out, label+" · "+status)
	}
	if len(out) == 0 {
		if e.OriginSessionID != "" {
			out = append(out, "Session: "+e.OriginSessionID+" · o opens if indexed")
		}
		if e.ThreadID != "" {
			out = append(out, "Session: "+e.ThreadID+" · o opens if indexed")
		}
	}
	return out
}

func (st *memoriesState) document(id string) (memory.Document, bool) {
	if st.catalog == nil || id == "" {
		return memory.Document{}, false
	}
	for _, doc := range st.catalog.Documents {
		if doc.ID == id {
			return doc, true
		}
	}
	return memory.Document{}, false
}

// viewMemorySources is the coverage overlay: every source root with
// its state and counts, plus any documents needing attention.
func (m *appModel) viewMemorySources(w, h int) string {
	st := m.mem
	lines := []string{m.styles().heading.Render("Memory sources"), ""}
	if st.catalog == nil {
		if st.loadErr != "" {
			lines = append(lines, st.loadErr)
		} else {
			lines = append(lines, "Loading memories…")
		}
	} else {
		if len(st.catalog.Sources) == 0 {
			lines = append(lines, wrap("No memory sources were found. Add a memory location with Add memory root in Settings (, from the Sessions view), then press R to refresh.", w)...)
		}
		for _, src := range st.catalog.Sources {
			lines = append(lines, m.styles().heading.Render(truncate(sanitizeText(providerLabel(src.Provider)+"  "+shortenHome(src.Root, m.home)), w)))
			state := "State: " + src.State
			if src.Detail != "" {
				state += " · " + sanitizeText(src.Detail)
			}
			lines = append(lines, wrap(state, w)...)
			lines = append(lines, fmt.Sprintf("Documents: %d · Entries: %d", src.Documents, src.Entries), "")
		}
		var problems []memory.Document
		for _, doc := range st.catalog.Documents {
			if doc.Status != "" && doc.Status != "ok" {
				problems = append(problems, doc)
			}
		}
		if len(problems) > 0 {
			lines = append(lines, m.styles().heading.Render("Documents needing attention"))
			for _, doc := range problems {
				line := sanitizeText(shortenHome(doc.Path, m.home)) + " · " + sanitizeText(doc.Status)
				if doc.StatusDetail != "" {
					line += " · " + sanitizeText(doc.StatusDetail)
				}
				lines = append(lines, wrap(line, w)...)
			}
		}
		if st.loadErr != "" {
			lines = append(lines, "", m.styles().gold.Render(st.loadErr))
		}
	}
	footer := m.rule(w) + "\n" + truncate(m.shortcutHints("  ", "↑↓/jk", "Scroll", "PgUp/PgDown", "Page", "Esc", "Back"), w)
	bodyH := max(1, h-lipgloss.Height(footer))
	st.detailHeight = bodyH
	st.sourcesScroll = scrollStart(len(lines), st.sourcesScroll, bodyH)
	end := min(len(lines), st.sourcesScroll+bodyH)
	body := fitPane(strings.Join(lines[st.sourcesScroll:end], "\n"), w, bodyH)
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}
