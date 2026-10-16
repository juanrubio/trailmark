package memory

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// relatedProjectDir mirrors the session list's current-project
// matching: the same directory, or one nested inside the other.
func relatedProjectDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b ||
		strings.HasPrefix(a, b+string(filepath.Separator)) ||
		strings.HasPrefix(b, a+string(filepath.Separator))
}

// FilterEntries returns entries matching f, sorted by modification
// time descending, then title ascending, then ID.
func FilterEntries(c *Catalog, f Filter) []Entry {
	if c == nil {
		return nil
	}
	var cats map[Category]bool
	if len(f.Categories) > 0 {
		cats = map[Category]bool{}
		for _, cat := range f.Categories {
			cats[cat] = true
		}
	}
	needle := strings.ToLower(f.Text)
	var out []Entry
	for _, e := range c.Entries {
		if f.Provider != "" && e.Provider != f.Provider {
			continue
		}
		if cats != nil && !cats[e.Category] {
			continue
		}
		switch {
		case f.Project == "":
		case f.Project == "unknown":
			if e.ProjectCWD != "" {
				continue
			}
		default:
			matches := e.ProjectCWD == f.Project ||
				(f.ProjectRelated && relatedProjectDir(e.ProjectCWD, f.Project))
			if !matches && e.ProjectEvidence != "global" {
				continue
			}
		}
		if needle != "" {
			hay := strings.ToLower(e.Title + "\n" + e.Description + "\n" + strings.Join(e.HeadingPath, " "))
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Projects lists sorted distinct non-empty project working dirs.
func Projects(c *Catalog) []string {
	if c == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range c.Entries {
		if e.ProjectCWD != "" && !seen[e.ProjectCWD] {
			seen[e.ProjectCWD] = true
			out = append(out, e.ProjectCWD)
		}
	}
	sort.Strings(out)
	return out
}

// Search runs a literal case-insensitive search over title,
// description, and body text of entries matching f, grouped by
// entry. An empty query returns nil.
func Search(c *Catalog, query string, f Filter) []Match {
	if c == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	needle := strings.ToLower(query)
	var matches []Match
	for _, e := range FilterEntries(c, f) {
		// Search the original Markdown (RawText), so recorded
		// metadata stripped from Text — frontmatter and rollout
		// headers such as originSessionId or thread_id — stays
		// findable with exact source lines. RawText begins at
		// StartLine; entries built without it fall back to Text.
		body, startLine := e.RawText, e.StartLine
		if body == "" {
			body, startLine = e.Text, textStartLine(e)
		}
		if startLine < 1 {
			startLine = 1
		}
		bodyLower := strings.ToLower(body)
		count := strings.Count(bodyLower, needle)
		// Title/description are usually extracted from the same
		// source; count them separately only when their text is
		// absent from the searchable body (for example a
		// filename-derived title), never twice.
		for _, field := range []string{e.Title, e.Description} {
			if field != "" && !strings.Contains(bodyLower, strings.ToLower(field)) {
				count += strings.Count(strings.ToLower(field), needle)
			}
		}
		if count == 0 {
			continue
		}
		m := Match{Entry: e, Count: count}
		for i, line := range splitLines(body) {
			if strings.Contains(strings.ToLower(line), needle) {
				m.Lines = append(m.Lines, startLine+i)
				m.Snippets = append(m.Snippets, snippet(line, needle))
			}
		}
		if len(m.Snippets) == 0 {
			// Match lives only in title or description.
			for _, s := range []string{e.Title, e.Description} {
				if strings.Contains(strings.ToLower(s), needle) {
					m.Snippets = append(m.Snippets, snippet(s, needle))
				}
			}
		}
		matches = append(matches, m)
	}
	return matches
}

// textStartLine maps body lines back to source lines using the
// parser-recorded Text start. The substring recovery below is only
// a fallback for entries built without a parser offset.
func textStartLine(e Entry) int {
	if e.TextLine > 0 {
		return e.TextLine
	}
	if e.Text == "" || e.RawText == "" {
		return e.StartLine
	}
	probe := e.Text
	// Use the first non-blank chunk so leading-blank trimming in
	// section bodies does not break the substring search.
	trimmed := strings.TrimLeft(probe, "\n\r\t ")
	if trimmed == "" {
		return e.StartLine
	}
	idx := strings.Index(e.RawText, trimmed)
	if idx < 0 {
		// Fall back to the first line of the trimmed text.
		first, _, _ := strings.Cut(trimmed, "\n")
		idx = strings.Index(e.RawText, first)
		if idx < 0 {
			return e.StartLine
		}
	}
	skipped := strings.Count(e.Text[:len(e.Text)-len(trimmed)], "\n")
	return e.StartLine + strings.Count(e.RawText[:idx], "\n") - skipped
}

func snippet(line, needleLower string) string {
	s := strings.TrimSpace(line)
	const maxLen = 160 // runes
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	// Search in runes with a 1:1 per-rune lowercase mapping:
	// whole-string lowercasing can change UTF-8 byte lengths, so
	// byte offsets from a lowered copy do not slice the original.
	hay := make([]rune, len(runes))
	for i, r := range runes {
		hay[i] = unicode.ToLower(r)
	}
	needle := []rune(needleLower)
	idx := 0
outer:
	for i := 0; i+len(needle) <= len(hay); i++ {
		for j, nr := range needle {
			if hay[i+j] != unicode.ToLower(nr) {
				continue outer
			}
		}
		idx = i
		break
	}
	start := max(0, idx-60)
	end := min(len(runes), start+maxLen)
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}
