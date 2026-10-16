package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/config"
)

// Palettes follow the fictional UX mockups. Terminal background reports choose
// the light palette; dark is the fallback for terminals without OSC 11 support.
type palette struct{ panel, text, muted, accent, selection, path, separator, gold, ink, raised, warning string }

var darkPalette = palette{"#191e20", "#e5e9e4", "#a2aeae", "#ade6c2", "#253c32", "#b2d3dc", "#364144", "#ddc38c", "#24342d", "#202729", "#ddc38c"}
var lightPalette = palette{"#fafbf7", "#24342d", "#59685e", "#246941", "#dfeddf", "#315e70", "#c1c9bf", "#f2dcaa", "#24342d", "#eef1e9", "#795b20"}

type themeStyles struct{ base, accent, muted, heading, purpose, selected, path, rule, match, gold, raised lipgloss.Style }

func themePalette(name string, light bool) palette {
	switch config.NormalizeTheme(name) {
	case "monokai":
		if light {
			return palette{"#ffffff", "#272822", "#66665e", "#486b00", "#eef5db", "#006b80", "#c4c4b8", "#f5e8a0", "#272822", "#f5f5f2", "#965000"}
		}
		return palette{"#000000", "#f8f8f2", "#a6a69c", "#a6e22e", "#272822", "#66d9ef", "#49483e", "#e6db74", "#191919", "#161616", "#fd971f"}
	case "blue":
		if light {
			return palette{"#f6f9fe", "#20334b", "#52677b", "#215fa0", "#dfebfa", "#246663", "#bccbdc", "#f2dcaa", "#24342d", "#eaf0f9", "#795b20"}
		}
		return palette{"#171e29", "#e5edf5", "#a4b3c5", "#91c7ff", "#243c58", "#a8dcd7", "#35465c", "#ddc38c", "#24342d", "#202b3a", "#ddc38c"}
	case "amber":
		if light {
			return palette{"#fdf9f1", "#3d3021", "#73634f", "#845000", "#f4e4c8", "#3d6b53", "#d6c8b1", "#f2d6bd", "#3d3021", "#f3ecdf", "#963b2d"}
		}
		return palette{"#242019", "#f0e8db", "#bdb09a", "#edc17a", "#473720", "#b8d2c2", "#514536", "#edb599", "#3d3021", "#30291f", "#f0a28b"}
	default:
		if light {
			return lightPalette
		}
		return darkPalette
	}
}
func (m *appModel) palette() palette { return themePalette(m.themeName, m.lightTheme) }

func newStyles(light, noColor bool, names ...string) themeStyles {
	name := "green"
	if len(names) > 0 {
		name = names[0]
	}
	p := themePalette(name, light)
	base := lipgloss.NewStyle()
	fg := func(hex string) lipgloss.Style {
		if noColor {
			return base
		}
		return base.Foreground(lipgloss.Color(hex))
	}
	st := themeStyles{gold: fg(p.warning), raised: fg(p.text), base: base, accent: fg(p.accent), muted: fg(p.muted), heading: fg(p.text).Bold(true), purpose: fg(p.text).Bold(true), selected: base.Bold(true), path: fg(p.path), rule: fg(p.separator), match: base.Bold(true)}
	if !noColor {
		st.base = fg(p.text).Background(lipgloss.Color(p.panel))
		st.raised = fg(p.text).Background(lipgloss.Color(p.raised))
		st.selected = fg(p.text).Background(lipgloss.Color(p.selection))
		st.match = fg(p.ink).Background(lipgloss.Color(p.gold)).Bold(true)
	}
	return st
}

func (m *appModel) styles() themeStyles {
	if m.theme == nil || m.themeLight != m.lightTheme || m.themeNoColor != m.noColor || m.themeCachedName != config.NormalizeTheme(m.themeName) {
		styles := newStyles(m.lightTheme, m.noColor, m.themeName)
		m.theme = &styles
		m.themeLight, m.themeNoColor = m.lightTheme, m.noColor
		m.themeCachedName = config.NormalizeTheme(m.themeName)
	}
	return *m.theme
}
func (m *appModel) rule(w int) string {
	return m.styles().rule.Render(strings.Repeat(m.horizontalRule(), max(0, w)))
}
func colorDisabled() bool { return os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" }

// fitPane enforces allocation even when its content is short or has long words.
// ANSI-aware truncation preserves styles and counts terminal cells, not bytes.
func fitPane(content string, w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, line := range lines {
		line = ansi.Truncate(line, w, "")
		lines[i] = line + strings.Repeat(" ", max(0, w-lipgloss.Width(line)))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func (m *appModel) horizontalRule() string {
	if m.ascii {
		return "-"
	}
	return "─"
}
func (m *appModel) verticalRule() string {
	if m.ascii {
		return "|"
	}
	return "│"
}
func (m *appModel) selectionMarker() string {
	if m.ascii {
		return "| "
	}
	return "▌ "
}

// frame paints the allocated terminal without wrapping source text. In
// particular, manual command-copy fallback remains one unbroken source line.
func (m *appModel) frame(content string, w, h int) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = line + strings.Repeat(" ", max(0, w-lipgloss.Width(line)))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return m.paintBackground(m.styles().base.Render(strings.Join(lines, "\n")), false)
}

// paintBackground restores the surrounding surface after nested SGR resets.
// Lipgloss foreground spans reset all attributes, including the parent fill.
func (m *appModel) paintBackground(s string, selected bool) string {
	if m.noColor {
		return s
	}
	p := m.palette()
	bg := p.panel
	if selected {
		bg = p.selection
	}
	return m.restoreSurface(s, bg)
}

func (m *appModel) restoreSurface(s, bg string) string {
	p := m.palette()
	code := func(hex string) string {
		n, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", n>>16, (n>>8)&255, n&255)
	}
	fg, _ := strconv.ParseUint(strings.TrimPrefix(p.text, "#"), 16, 32)
	restore := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", fg>>16, (fg>>8)&255, fg&255) + code(bg)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+restore)
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+restore)
	s = strings.ReplaceAll(s, "\x1b[49m", restore)
	return s + "\x1b[0m"
}

// shortcutHints styles known key labels explicitly; user text and paths are
// never parsed as shortcuts. Separators preserve the existing plain labels.
func (m *appModel) shortcutHints(separator string, pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		if m.noColor {
			parts = append(parts, pairs[i]+" "+pairs[i+1])
			continue
		}
		parts = append(parts, m.styles().accent.Bold(true).Render(pairs[i])+m.styles().muted.Render(" "+pairs[i+1]))
	}
	return strings.Join(parts, m.styles().muted.Render(separator))
}
