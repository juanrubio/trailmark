package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/juanrubio/trailmark/internal/index"
)

// transcriptState is the open transcript view. Messages load in
// bounded pages; a jump loads a window around the target message.
type transcriptState struct {
	row        index.SessionRow
	msgs       []index.TranscriptMessage
	offset     int // global index of msgs[0]
	total      int
	cursor     int // index into msgs
	expanded   map[int]bool
	expandAll  bool
	query      string
	searching  bool
	searchPrev string
	matches    []int // matching seq numbers, ascending
	matchPos   int   // index into matches, -1 when none active
	status     string
	lineScroll int
}

const transcriptPageSize = 200

func (m *appModel) openTranscript(row index.SessionRow, atSeq int, fromSearch bool) {
	_ = fromSearch // Esc returns to results because m.search stays set
	t := &transcriptState{row: row, expanded: map[int]bool{}, matchPos: -1}
	if m.deps.page == nil {
		t.status = "Transcript unavailable."
		m.transcript = t
		return
	}
	if atSeq > 0 && m.deps.around != nil {
		msgs, offset, total, err := m.deps.around(m.ctx, row.SourcePath, atSeq, 60, 140)
		if err == nil {
			t.msgs, t.offset, t.total = msgs, offset, total
			for i, msg := range msgs {
				if msg.Seq >= atSeq {
					t.cursor = i
					break
				}
				t.cursor = i
			}
		}
	}
	if t.msgs == nil {
		msgs, total, err := m.deps.page(m.ctx, row.SourcePath, 0, transcriptPageSize)
		if err != nil {
			t.status = "Transcript unavailable."
		} else {
			t.msgs, t.total = msgs, total
		}
	}
	m.transcript = t
}

func (t *transcriptState) current() (index.TranscriptMessage, bool) {
	if t.cursor < 0 || t.cursor >= len(t.msgs) {
		return index.TranscriptMessage{}, false
	}
	return t.msgs[t.cursor], true
}

// ensureLoaded makes seq the cursor, reloading a window around it
// when it falls outside the loaded page.
func (m *appModel) transcriptGotoSeq(t *transcriptState, seq int) {
	for i, msg := range t.msgs {
		if msg.Seq == seq {
			t.cursor = i
			return
		}
	}
	if m.deps.around == nil {
		return
	}
	msgs, offset, total, err := m.deps.around(m.ctx, t.row.SourcePath, seq, 60, 140)
	if err != nil {
		return
	}
	t.msgs, t.offset, t.total = msgs, offset, total
	for i, msg := range msgs {
		if msg.Seq >= seq {
			t.cursor = i
			return
		}
		t.cursor = i
	}
}

// maxLoadedMessages bounds the transcript window in memory.
const maxLoadedMessages = 600

func (m *appModel) transcriptLoadMore(t *transcriptState) {
	if m.deps.page == nil || t.offset+len(t.msgs) >= t.total {
		return
	}
	msgs, _, err := m.deps.page(m.ctx, t.row.SourcePath, t.offset+len(t.msgs), transcriptPageSize)
	if err == nil && len(msgs) > 0 {
		t.msgs = append(t.msgs, msgs...)
		trimTranscriptWindow(t, true)
	}
}

func (m *appModel) transcriptLoadPrev(t *transcriptState) {
	if m.deps.page == nil || t.offset <= 0 {
		return
	}
	start := t.offset - transcriptPageSize
	if start < 0 {
		start = 0
	}
	msgs, _, err := m.deps.page(m.ctx, t.row.SourcePath, start, t.offset-start)
	if err != nil || len(msgs) == 0 {
		return
	}
	t.msgs = append(msgs, t.msgs...)
	t.offset = start
	t.cursor += len(msgs)
	trimTranscriptWindow(t, false)
}

// trimTranscriptWindow evicts from the side opposite to travel so
// the loaded window never exceeds maxLoadedMessages.
func trimTranscriptWindow(t *transcriptState, forward bool) {
	for len(t.msgs) > maxLoadedMessages {
		if forward {
			t.msgs = t.msgs[1:]
			t.offset++
			if t.cursor > 0 {
				t.cursor--
			}
		} else {
			t.msgs = t.msgs[:len(t.msgs)-1]
		}
	}
}

func (m *appModel) onTranscriptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := m.transcript
	s := msg.String()
	if t.searching {
		switch s {
		case "esc":
			t.query = t.searchPrev
			t.searching = false
		case "enter":
			t.searching = false
			m.runTranscriptSearch(t)
		case "backspace":
			runes := []rune(t.query)
			if len(runes) > 0 {
				t.query = string(runes[:len(runes)-1])
			}
		default:
			if msg.Text != "" {
				t.query += msg.Text
			}
		}
		return m, nil
	}
	switch s {
	case "esc", "q":
		m.transcript = nil
	case "j", "down":
		t.lineScroll = 0
		if t.cursor < len(t.msgs)-1 {
			t.cursor++
		} else {
			m.transcriptLoadMore(t)
			if t.cursor < len(t.msgs)-1 {
				t.cursor++
			}
		}
	case "k", "up":
		t.lineScroll = 0
		if t.cursor > 0 {
			t.cursor--
		} else {
			m.transcriptLoadPrev(t)
		}
	case "J":
		t.lineScroll += 10
	case "K":
		t.lineScroll -= 10
	case "/":
		t.searchPrev = t.query
		t.searching = true
	case "]":
		m.transcriptMatchStep(t, 1)
	case "[":
		m.transcriptMatchStep(t, -1)
	case "e":
		t.expandAll = !t.expandAll
	case "enter":
		if msg, ok := t.current(); ok {
			t.expanded[msg.Seq] = !t.expanded[msg.Seq]
		}
	case "c":
		if msg, ok := t.current(); ok {
			item := copyItem{
				label:     "Copy message",
				text:      provenanceHeader(t.row) + "\n\n" + excerptBlock(roleLabel(msg)+" message", excerptOf(msg)),
				available: true,
			}
			m.copyPreview = &item
			m.copyState = ""
			m.copyScroll = 0
			m.copyReturn = "transcript"
			m.overlay = overlayCopyPreview
		}
	}
	return m, nil
}

func (m *appModel) runTranscriptSearch(t *transcriptState) {
	if m.deps.inSession == nil || strings.TrimSpace(t.query) == "" {
		t.matches = nil
		t.matchPos = -1
		t.status = ""
		return
	}
	seqs, err := m.deps.inSession(m.ctx, t.row.SourcePath, t.query)
	if err != nil {
		t.status = "Search unavailable for this session."
		return
	}
	t.matches = seqs
	if len(seqs) == 0 {
		t.matchPos = -1
		t.status = "No matches in this session."
		return
	}
	t.matchPos = 0
	if cur, ok := t.current(); ok {
		for i, seq := range seqs {
			if seq >= cur.Seq {
				t.matchPos = i
				break
			}
		}
	}
	m.transcriptRevealMatch(t)
}

func (m *appModel) transcriptMatchStep(t *transcriptState, delta int) {
	if len(t.matches) == 0 {
		t.status = "No matches. Press / to search this session."
		return
	}
	t.matchPos += delta
	if t.matchPos >= len(t.matches) {
		t.matchPos = 0
	}
	if t.matchPos < 0 {
		t.matchPos = len(t.matches) - 1
	}
	m.transcriptRevealMatch(t)
}

func (m *appModel) transcriptRevealMatch(t *transcriptState) {
	seq := t.matches[t.matchPos]
	m.transcriptGotoSeq(t, seq)
	if msg, ok := t.current(); ok && msg.Kind != "message" {
		t.expanded[msg.Seq] = true // reveal matching tool activity
	}
	t.status = fmt.Sprintf("Match %d of %d", t.matchPos+1, len(t.matches))
}

func roleLabel(msg index.TranscriptMessage) string {
	switch {
	case msg.Kind == "tool_call":
		return "Tool call"
	case msg.Kind == "tool_output":
		return "Tool output"
	case msg.Role == "user":
		return "User"
	case msg.Role == "assistant":
		return "Assistant"
	default:
		return "System"
	}
}

func excerptOf(msg index.TranscriptMessage) index.Excerpt {
	return index.Excerpt{Text: msg.Text, Seq: msg.Seq, Line: msg.Line, RecordID: msg.RecordID, Timestamp: msg.Timestamp}
}

// viewTranscript renders the transcript page.
func (m *appModel) viewTranscript(w, h int) string {
	t := m.transcript
	header := styleHeading.Render(truncate(purposeOrUnavailable(t.row), w)) + "\n" +
		styleDim.Render(truncate(projectName(t.row.CWD)+"  "+t.row.CWD+" · "+providerLabel(t.row.Provider), w))
	footer := m.transcriptFooter(t)
	bodyH := h - 3 - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	lines, cursorLine := m.transcriptLines(t, w)
	body := windowLinesScrolled(lines, cursorLine, t.lineScroll, bodyH)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *appModel) transcriptFooter(t *transcriptState) string {
	if t.searching {
		return "Search this session: " + t.query + "_  (Enter search · Esc cancel)"
	}
	parts := []string{"↑↓/jk Select  / Search  [ ] Matches  e Expand tools  Enter Toggle  c Copy  Esc Back"}
	if t.status != "" {
		parts = append([]string{t.status}, parts...)
	}
	if len(t.matches) > 0 {
		parts = append([]string{fmt.Sprintf("%d matches", len(t.matches))}, parts...)
	}
	return strings.Join(parts, "\n")
}

func (m *appModel) transcriptLines(t *transcriptState, w int) ([]string, int) {
	var lines []string
	cursorLine := 0
	matchSet := map[int]bool{}
	for _, seq := range t.matches {
		matchSet[seq] = true
	}
	for i, msg := range t.msgs {
		if i == t.cursor {
			cursorLine = len(lines)
		}
		marker := "  "
		if i == t.cursor {
			marker = styleAccent.Render("▌ ")
		}
		ts := ""
		if msg.Timestamp != nil {
			ts = " · " + msg.Timestamp.Format("Mon 02 Jan 15:04")
		}
		isTool := msg.Kind != "message"
		expanded := t.expandAll || t.expanded[msg.Seq]
		if isTool && !expanded {
			first := firstLine(sanitizeText(msg.Text))
			lines = append(lines, marker+styleDim.Render("▸ "+roleLabel(msg)+ts+"  "+truncate(first, w-12)))
			continue
		}
		lines = append(lines, marker+styleHeading.Render(roleLabel(msg)+ts))
		for _, l := range wrapPreserve(sanitizeText(msg.Text), w-2) {
			if matchSet[msg.Seq] && t.query != "" {
				l = highlightMatches(l, t.query)
			}
			lines = append(lines, "  "+l)
		}
		lines = append(lines, "")
	}
	if len(t.msgs) == 0 {
		lines = append(lines, "Transcript unavailable.")
	}
	return lines, cursorLine
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// highlightMatches accents every literal occurrence of the query
// terms. Matching is done on rune slices so ANSI styling inserted
// for one term can never be matched or split by a later term.
var styleMatch = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

func highlightMatches(line, query string) string {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return line
	}
	runes := []rune(line)
	lower := []rune(strings.ToLower(line))
	type interval struct{ start, end int }
	var hits []interval
	for _, term := range terms {
		tr := []rune(strings.ToLower(term))
		if len(tr) == 0 || len(tr) > len(runes) {
			continue
		}
		for i := 0; i+len(tr) <= len(runes); i++ {
			match := true
			for j := range tr {
				if lower[i+j] != tr[j] {
					match = false
					break
				}
			}
			if match {
				hits = append(hits, interval{i, i + len(tr)})
				i += len(tr) - 1
			}
		}
	}
	if len(hits) == 0 {
		return line
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].start < hits[j].start })
	var out strings.Builder
	pos := 0
	for _, h := range hits {
		if h.start < pos {
			continue
		}
		out.WriteString(string(runes[pos:h.start]))
		out.WriteString(styleMatch.Render(string(runes[h.start:h.end])))
		pos = h.end
	}
	out.WriteString(string(runes[pos:]))
	return out.String()
}

// sanitizeText strips terminal control and escape sequences from
// log-derived text. Newlines and tabs survive; everything that
// could move the cursor, restyle the terminal, or emit OSC
// payloads is removed.
func sanitizeText(s string) string {
	var out strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == 0x1b {
			// Skip CSI (final byte @-~) and OSC (until ST/BEL).
			if i+1 < len(runes) && runes[i+1] == '[' {
				i += 2
				for i < len(runes) && !(runes[i] >= '@' && runes[i] <= '~') {
					i++
				}
				continue
			}
			if i+1 < len(runes) && runes[i+1] == ']' {
				i += 2
				for i < len(runes) {
					if runes[i] == 0x07 {
						break
					}
					if runes[i] == 0x1b && i+1 < len(runes) && runes[i+1] == '\\' {
						i++
						break
					}
					i++
				}
				continue
			}
			continue
		}
		if r < 0x20 && r != '\n' && r != '\t' {
			continue
		}
		if r == 0x7f {
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// windowLinesScrolled windows lines around a cursor line plus an
// extra line-scroll offset (for very long single messages).
func windowLinesScrolled(lines []string, keepVisible, extraScroll, h int) string {
	if h <= 0 || len(lines) <= h {
		return strings.Join(lines, "\n")
	}
	start := keepVisible - h/2 + extraScroll
	if start < 0 {
		start = 0
	}
	if start+h > len(lines) {
		start = len(lines) - h
	}
	return strings.Join(lines[start:start+h], "\n")
}

// queryTerms extracts literal search terms and quoted phrases.
func queryTerms(query string) []string {
	var terms []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			terms = append(terms, cur.String())
			cur.Reset()
		}
	}
	for _, r := range query {
		switch {
		case r == '"':
			flush()
			inQuote = !inQuote
		case !inQuote && (r == ' ' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return terms
}

func purposeOrUnavailable(row index.SessionRow) string {
	if row.Purpose == "" {
		return "Purpose unavailable"
	}
	return row.Purpose
}
