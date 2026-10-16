package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type overviewMatchPart struct{ line, start, end int }
type overviewMatch struct {
	line  int
	parts []overviewMatchPart
}
type overviewFindBlock struct {
	source             string
	start, end, prefix int
}
type overviewFindState struct {
	source, query, draft string
	editing, reveal      bool
	matches              []overviewMatch
	current              int
}

func (m *appModel) onOverviewFindInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := &m.overviewFind
	switch msg.String() {
	case "esc":
		f.editing = false
		f.draft = f.query
	case "enter":
		f.query = f.draft
		f.editing = false
		f.current = 0
		f.reveal = true
		m.normalizeOverviewScroll()
	case "backspace":
		r := []rune(f.draft)
		if len(r) > 0 {
			f.draft = string(r[:len(r)-1])
		}
	default:
		if msg.Text != "" {
			f.draft += msg.Text
		}
	}
	return m, nil
}

func (m *appModel) stepOverviewFind(forward bool) {
	m.normalizeOverviewScroll()
	f := &m.overviewFind
	if len(f.matches) == 0 {
		return
	}
	delta := -1
	if forward {
		delta = 1
	}
	f.current = (f.current + delta + len(f.matches)) % len(f.matches)
	f.reveal = true
	m.normalizeOverviewScroll()
}

func (m *appModel) overviewFindFooter() string {
	f := &m.overviewFind
	if f.editing {
		return "Find in overview: " + f.draft + "_  (Enter apply · Esc cancel)"
	}
	if f.query == "" {
		return ""
	}
	if len(f.matches) == 0 {
		return "No matches in overview: " + f.query
	}
	return fmt.Sprintf("Overview match %d of %d · %s", f.current+1, len(f.matches), f.query)
}

// Find visible text only. Slice the styled source by terminal cells when
// inserting highlights, so ANSI attributes never become searchable text.
func (m *appModel) applyOverviewFind(lines []string, blocks []overviewFindBlock, height int) []string {
	f := &m.overviewFind
	f.matches = nil
	needle := []rune(strings.Map(unicode.ToLower, f.query))
	if len(needle) == 0 {
		f.current = 0
		f.reveal = false
		return lines
	}
	covered := make([]bool, len(lines))
	for _, block := range blocks {
		for i := block.start; i < block.end; i++ {
			covered[i] = true
		}
	}
	for i, line := range lines {
		if !covered[i] {
			blocks = append(blocks, overviewFindBlock{source: ansi.Strip(line), start: i, end: i + 1})
		}
	}
	for _, block := range blocks {
		source := []rune(block.source)
		lower := []rune(strings.Map(unicode.ToLower, block.source))
		type segment struct{ line, offset, length, prefix int }
		var segments []segment
		offset := 0
		for line := block.start; line < block.end; line++ {
			visible := []rune(ansi.Strip(lines[line]))
			prefix := min(block.prefix, len(visible))
			visible = visible[prefix:]
			at := strings.Index(string(source[offset:]), string(visible))
			if at < 0 {
				continue
			}
			offset += len([]rune(string(source[offset:])[:at]))
			segments = append(segments, segment{line, offset, len(visible), prefix})
			offset += len(visible)
		}
		for i := 0; i+len(needle) <= len(lower); i++ {
			if string(lower[i:i+len(needle)]) != string(needle) {
				continue
			}
			hit := overviewMatch{}
			for _, segment := range segments {
				start, end := max(i, segment.offset), min(i+len(needle), segment.offset+segment.length)
				if start < end {
					hit.parts = append(hit.parts, overviewMatchPart{segment.line, start - segment.offset + segment.prefix, end - segment.offset + segment.prefix})
				}
			}
			if len(hit.parts) > 0 {
				hit.line = hit.parts[0].line
				f.matches = append(f.matches, hit)
			}
			i += len(needle) - 1
		}
	}
	sort.SliceStable(f.matches, func(i, j int) bool {
		a, b := f.matches[i].parts[0], f.matches[j].parts[0]
		if a.line != b.line {
			return a.line < b.line
		}
		return a.start < b.start
	})
	if len(f.matches) == 0 {
		f.current = 0
		f.reveal = false
		return lines
	}
	f.current = max(0, min(f.current, len(f.matches)-1))
	if f.reveal {
		target := f.matches[f.current].line
		if target < m.overviewScroll {
			m.overviewScroll = target
		}
		if target >= m.overviewScroll+height {
			m.overviewScroll = target - height + 1
		}
		f.reveal = false
	}
	for i := len(f.matches) - 1; i >= 0; i-- {
		for j := len(f.matches[i].parts) - 1; j >= 0; j-- {
			hit := f.matches[i].parts[j]
			plain := []rune(ansi.Strip(lines[hit.line]))
			start := lipgloss.Width(string(plain[:hit.start]))
			end := lipgloss.Width(string(plain[:hit.end]))
			style := m.styles().match
			if i == f.current {
				style = style.Underline(true)
			}
			line := lines[hit.line]
			lines[hit.line] = ansi.Cut(line, 0, start) + style.Render(string(plain[hit.start:hit.end])) + ansi.Cut(line, end, lipgloss.Width(line))
		}
	}
	return lines
}
