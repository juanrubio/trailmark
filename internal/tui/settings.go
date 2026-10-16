package tui

import (
	"charm.land/lipgloss/v2"
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/trailmark/internal/model"
)

// RootInfo describes one discovery root for the settings view.
type RootInfo struct {
	Provider   model.Provider
	Path       string
	Configured bool
	State      string // ok, unavailable, or unknown
	LastOK     string
	Sessions   int
}

// SettingsStatus summarizes index coverage for the settings view.
type SettingsStatus struct {
	Indexed   int
	Partial   int
	Excluded  int
	IndexPath string
	LastScan  string
}

// settingsStore is the settings backend the TUI drives. Mutations
// persist configuration and follow the PRD section 8 lifecycle.
type settingsStore interface {
	Roots(ctx context.Context) ([]RootInfo, error)
	Status(ctx context.Context) (SettingsStatus, error)
	Exclusions() []string
	AddRoot(provider model.Provider, path string) error
	RemoveRoot(ctx context.Context, provider model.Provider, path string) error
	AddExclusion(path string) error
	RemoveExclusion(path string) error
	ClearCache(ctx context.Context) error
}

type settingsItem struct {
	kind      string // root, exclusion, action
	label     string
	root      RootInfo
	exclusion string
	action    string // add-root, add-exclusion, clear-cache, rescan
}

type settingsState struct {
	items     []settingsItem
	cursor    int
	mode      string // list, input, confirm
	inputKind string // root or exclusion
	input     string
	provider  model.Provider
	confirm   string
	onConfirm func() string
	status    string
	roots     []RootInfo
	settings  SettingsStatus
}

func (m *appModel) openSettings() {
	st := &settingsState{mode: "list", provider: model.ProviderClaudeCode}
	m.settings = st
	m.reloadSettings()
}

func (m *appModel) reloadSettings() {
	st := m.settings
	if st == nil || m.deps.settings == nil {
		return
	}
	roots, err := m.deps.settings.Roots(m.ctx)
	if err == nil {
		st.roots = roots
	}
	status, err := m.deps.settings.Status(m.ctx)
	if err == nil {
		st.settings = status
	}
	st.items = nil
	st.items = append(st.items, settingsItem{kind: "action", label: "Rescan now", action: "rescan"})
	for _, r := range roots {
		label := providerLabel(r.Provider) + "  " + r.Path
		if r.Configured {
			label += "  (configured)"
		}
		if r.State != "" {
			label += "  [" + r.State + "]"
		}
		st.items = append(st.items, settingsItem{kind: "root", label: label, root: r})
	}
	st.items = append(st.items, settingsItem{kind: "action", label: "Add root…", action: "add-root"})
	for _, ex := range m.deps.settings.Exclusions() {
		st.items = append(st.items, settingsItem{kind: "exclusion", label: ex, exclusion: ex})
	}
	st.items = append(st.items,
		settingsItem{kind: "action", label: "Add exclusion…", action: "add-exclusion"},
		settingsItem{kind: "action", label: "Clear cache…", action: "clear-cache"})
	if st.cursor >= len(st.items) {
		st.cursor = len(st.items) - 1
	}
	if st.cursor < 0 {
		st.cursor = 0
	}
}

func (m *appModel) onSettingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := m.settings
	s := msg.String()
	switch st.mode {
	case "input":
		switch s {
		case "esc":
			st.mode = "list"
			st.input = ""
		case "enter":
			return m, m.finishSettingsInput()
		case "backspace":
			runes := []rune(st.input)
			if len(runes) > 0 {
				st.input = string(runes[:len(runes)-1])
			}
		case "tab":
			if st.provider == model.ProviderClaudeCode {
				st.provider = model.ProviderCodex
			} else {
				st.provider = model.ProviderClaudeCode
			}
		default:
			if msg.Text != "" {
				st.input += msg.Text
			}
		}
		return m, nil
	case "confirm":
		switch s {
		case "y", "enter":
			msgText := ""
			if st.onConfirm != nil {
				msgText = st.onConfirm()
			}
			st.mode = "list"
			st.onConfirm = nil
			st.status = msgText
			m.reloadSettings()
			return m, m.rescanCmd(true)
		case "esc", "n", "q":
			st.mode = "list"
			st.onConfirm = nil
		}
		return m, nil
	}
	switch s {
	case "esc", "q":
		m.settings = nil
	case "j", "down":
		if st.cursor < len(st.items)-1 {
			st.cursor++
		}
	case "k", "up":
		if st.cursor > 0 {
			st.cursor--
		}
	case "a":
		st.mode = "input"
		st.inputKind = "root"
		st.input = ""
	case "x":
		st.mode = "input"
		st.inputKind = "exclusion"
		st.input = ""
	case "d":
		m.confirmDeleteSettingsItem()
	case "R":
		return m, m.rescanCmd(true)
	case "enter":
		if st.cursor >= 0 && st.cursor < len(st.items) {
			switch st.items[st.cursor].action {
			case "rescan":
				return m, m.rescanCmd(true)
			case "add-root":
				st.mode = "input"
				st.inputKind = "root"
				st.input = ""
			case "add-exclusion":
				st.mode = "input"
				st.inputKind = "exclusion"
				st.input = ""
			case "clear-cache":
				st.confirm = "Clearing removes Trailmark's derived index, snippets, and search data. Configuration and exclusions are kept, and source logs are never deleted. The next scan rebuilds the index from sources that remain included."
				st.onConfirm = func() string {
					if err := m.deps.settings.ClearCache(m.ctx); err != nil {
						return "Clear failed: " + err.Error()
					}
					return "Cache cleared. Rescanning…"
				}
				st.mode = "confirm"
			}
		}
	}
	return m, nil
}

func (m *appModel) confirmDeleteSettingsItem() {
	st := m.settings
	if m.deps.settings == nil || st.cursor < 0 || st.cursor >= len(st.items) {
		return
	}
	item := st.items[st.cursor]
	switch item.kind {
	case "root":
		if item.root.Configured {
			st.confirm = fmt.Sprintf("Remove the configured root %s? Its derived index data is removed unless the same source remains included through another root. Source logs are not deleted.", item.root.Path)
		} else {
			st.confirm = fmt.Sprintf("This is a detected default root (%s). Removing it disables all default roots; configured roots remain. Its derived data is removed unless included through another root. Source logs are not deleted.", item.root.Path)
		}
		provider, path := item.root.Provider, item.root.Path
		st.onConfirm = func() string {
			if err := m.deps.settings.RemoveRoot(m.ctxProvider(), provider, path); err != nil {
				return "Remove failed: " + err.Error()
			}
			return "Root removed. Rescanning…"
		}
		st.mode = "confirm"
	case "exclusion":
		st.confirm = fmt.Sprintf("Remove the exclusion %s? Matching sources will be indexed again on the next scan.", item.exclusion)
		path := item.exclusion
		st.onConfirm = func() string {
			if err := m.deps.settings.RemoveExclusion(path); err != nil {
				return "Remove failed: " + err.Error()
			}
			return "Exclusion removed. Rescanning…"
		}
		st.mode = "confirm"
	}
}

func (m *appModel) ctxProvider() context.Context { return m.ctx }

func (m *appModel) finishSettingsInput() tea.Cmd {
	st := m.settings
	value := strings.TrimSpace(st.input)
	st.mode = "list"
	st.input = ""
	if value == "" {
		st.status = "Nothing entered."
		return nil
	}
	if st.inputKind == "exclusion" {
		// Exclusions are destructive to derived data: explain and
		// confirm before applying, like root removal and clearing.
		st.confirm = fmt.Sprintf("Apply the exclusion %s? Matching sessions' derived text, snippets, and search results are removed from the index immediately and on every scan. Source logs are not deleted.", value)
		st.onConfirm = func() string {
			if err := m.deps.settings.AddExclusion(value); err != nil {
				return "Failed: " + err.Error()
			}
			return "Exclusion applied; matching derived data removed. Rescanning…"
		}
		st.mode = "confirm"
		return nil
	}
	if err := m.deps.settings.AddRoot(st.provider, value); err != nil {
		st.status = "Failed: " + err.Error()
		return nil
	}
	st.status = "Root added. Rescanning…"
	m.reloadSettings()
	return m.rescanCmd(true)
}

// rescanCmd runs a background rescan; apply makes the result replace
// the visible list immediately (settings-triggered), preserving the
// selected session where it still exists.
func (m *appModel) rescanCmd(apply bool) tea.Cmd {
	if m.deps.rescan == nil {
		return nil
	}
	m.applyNextScan = apply
	m.scanSeq++
	id := m.scanSeq
	if apply {
		m.applyScanID = id
	}
	rescan := m.deps.rescan
	ctx := m.ctx
	return func() tea.Msg {
		sessions, failed, err := rescan(ctx)
		if err != nil {
			return scanDoneMsg{id: id, err: err}
		}
		return scanDoneMsg{id: id, sessions: sessions, failedRoots: failed}
	}
}

// viewSettings renders the sources/settings view.
func (m *appModel) viewSettings(w, h int) string {
	st := m.settings
	lines := []string{m.styles().heading.Render("Sources and settings"), ""}
	unavailable := 0
	for _, r := range st.roots {
		if r.State == "unavailable" {
			unavailable++
		}
	}
	lines = append(lines, fmt.Sprintf("Indexed: %d sessions · Partial: %d · Unavailable roots: %d · Excluded (last scan): %d", st.settings.Indexed, st.settings.Partial, unavailable, st.settings.Excluded))
	if st.settings.LastScan != "" {
		lines = append(lines, "Last successful scan: "+st.settings.LastScan)
	}
	if st.settings.IndexPath != "" {
		lines = append(lines, "Index: "+st.settings.IndexPath)
	}
	lines = append(lines, "", m.styles().heading.Render("Roots and actions"))
	cursorLine := 0
	for i, item := range st.items {
		marker := "  "
		if i == st.cursor {
			marker = m.styles().accent.Render(m.selectionMarker())
			cursorLine = len(lines)
		}
		prefix := ""
		label := item.label
		switch item.kind {
		case "root":
			prefix = "Root: "
			label += fmt.Sprintf(" · %d sessions", item.root.Sessions)
			if item.root.LastOK != "" {
				label += " · last ok " + item.root.LastOK
			}
		case "exclusion":
			prefix = "Exclusion: "
		}
		row := marker + truncate(prefix+label, w-2)
		if item.kind == "root" || item.kind == "exclusion" {
			row = marker + m.styles().path.Render(truncate(prefix+label, w-2))
		}
		if i == st.cursor {
			row = m.paintBackground(m.styles().selected.Width(w).Render(row), true)
		}
		lines = append(lines, row)
	}
	_ = cursorLine
	if st.status != "" {
		lines = append(lines, "", st.status)
	}
	footerStart := len(lines)
	switch st.mode {
	case "input":
		label := "Path: "
		if st.inputKind == "root" {
			label = "Root path (" + providerLabel(st.provider) + ", Tab switches provider): "
		} else {
			label = "Exclusion path: "
		}
		lines = append(lines, "", label+st.input+"_", m.shortcutHints(" · ", "Enter", "Save", "Esc", "Cancel"))
	case "confirm":
		lines = append(lines, "", m.styles().heading.Render("Confirm"))
		lines = append(lines, wrap(st.confirm, w)...)
		lines = append(lines, m.shortcutHints(" · ", "y/Enter", "Confirm", "Esc/n", "Cancel"))
	default:
		lines = append(lines, "", m.shortcutHints("  ", "↑↓/jk", "Select", "a", "Add root", "x", "Add exclusion", "d", "Remove", "R", "Rescan", "Esc", "Back"))
	}
	footer := m.rule(w) + "\n" + m.footerLines(lines[footerStart:], w)
	bodyH := max(1, h-lipgloss.Height(footer))
	return fitPane(windowLines(lines[:footerStart], cursorLine, bodyH), w, bodyH) + "\n" + footer
}
