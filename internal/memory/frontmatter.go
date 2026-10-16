package memory

import "strings"

// splitLines splits on \n, dropping the empty element after a
// trailing newline so indexes are natural 1-based source lines.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	if len(lines) > 0 {
		lines[0] = strings.TrimPrefix(lines[0], "\ufeff")
	}
	return lines
}

func joinLines(lines []string) string { return strings.Join(lines, "\n") }

// parseFrontmatter reads a tolerant YAML subset: top-level
// "key: value" lines and one level of nested maps flattened as
// "key.subkey". It returns meta, the body lines, and the 1-based
// source line where the body starts. ok is false when no complete
// --- block opens the file.
func parseFrontmatter(lines []string) (meta map[string]string, body []string, bodyStart int, ok bool) {
	meta = map[string]string{}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta, lines, 1, false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" || strings.TrimSpace(lines[i]) == "..." {
			end = i
			break
		}
	}
	if end < 0 {
		return meta, lines, 1, false
	}
	parent := ""
	for _, raw := range lines[1:end] {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indented := strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
		line := strings.TrimSpace(raw)
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = stripQuotes(strings.TrimSpace(val))
		if key == "" || strings.HasPrefix(line, "-") {
			continue
		}
		if indented && parent != "" {
			meta[parent+"."+key] = val
			continue
		}
		if val == "" {
			parent = key
			continue
		}
		parent = ""
		meta[key] = val
	}
	return meta, lines[end+1:], end + 2, true
}

func stripQuotes(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// metaGet tries keys in order, then a case-insensitive pass.
func metaGet(meta map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := meta[k]; ok && v != "" {
			return v
		}
	}
	for _, k := range keys {
		for mk, v := range meta {
			if strings.EqualFold(mk, k) && v != "" {
				return v
			}
		}
	}
	return ""
}

// indentedCode reports whether a line sits in an indented code
// block (a tab or four-plus leading spaces), where ATX headings
// and fences do not apply. Up to three leading spaces still allow
// a heading or fence.
func indentedCode(line string) bool {
	if strings.HasPrefix(line, "\t") {
		return true
	}
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	return spaces >= 4
}

// headingLevel returns the ATX heading level (1-6) and text.
func headingLevel(line string) (int, string, bool) {
	if indentedCode(line) {
		return 0, "", false
	}
	s := strings.TrimLeft(line, " \t")
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(s) || (s[n] != ' ' && s[n] != '\t') {
		return 0, "", false
	}
	text := strings.TrimSpace(s[n:])
	text = strings.TrimRight(text, "#")
	return n, strings.TrimSpace(text), true
}

// fenceState tracks a Markdown fenced code block: the fence
// character and the opening run length. Only a line of the same
// character at least as long (with nothing after it) closes it.
type fenceState struct {
	char   byte
	length int
}

// feed reports whether the line is fenced code content or a fence
// boundary, i.e. never a heading candidate.
func (f *fenceState) feed(line string) bool {
	if indentedCode(line) {
		return f.char != 0
	}
	s := strings.TrimLeft(line, " \t")
	if len(s) < 3 || (s[0] != '`' && s[0] != '~') {
		return f.char != 0
	}
	n := 0
	for n < len(s) && s[n] == s[0] {
		n++
	}
	if n < 3 {
		return f.char != 0
	}
	if f.char == 0 {
		if s[0] == '`' && strings.Contains(s[n:], "`") {
			return false // backtick fences take no backticks in the info string
		}
		f.char, f.length = s[0], n
		return true
	}
	if s[0] == f.char && n >= f.length && strings.TrimSpace(s[n:]) == "" {
		f.char, f.length = 0, 0
	}
	return true
}

func firstH1(lines []string) string {
	var fence fenceState
	for _, l := range lines {
		if fence.feed(l) {
			continue
		}
		if lvl, text, ok := headingLevel(l); ok && lvl == 1 {
			return text
		}
	}
	return ""
}

func trimLeadingBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return lines
}

func trimTrailingBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func nonempty(lines []string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return true
		}
	}
	return false
}
