package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Optional so existing stores retain the legacy all-history behavior.
type indexingScopeStore interface {
	IndexingDays() (int, bool)
	SetIndexingDays(int) error
}

var indexingChoices = []int{7, 30, 0}

func indexingScopeLabel(days int) string {
	if days == 0 {
		return "All history"
	}
	return fmt.Sprintf("Files modified in last %d days", days)
}

func (m *appModel) indexingDays() int {
	if store, ok := m.deps.settings.(indexingScopeStore); ok {
		days, _ := store.IndexingDays()
		return days
	}
	return 0
}

func (m *appModel) indexingCoverageHint() string {
	days := m.indexingDays()
	if days == 0 {
		return ""
	}
	return fmt.Sprintf("Indexing: new files modified within %dd; cached history retained. All time shows indexed data. Change in Settings (,).", days)
}

func (m *appModel) onIndexingScopeKey(s string, startup bool) (tea.Model, tea.Cmd) {
	cursor := &m.scopeCursor
	if !startup {
		cursor = &m.settings.scopeCursor
	}
	switch s {
	case "j", "down":
		*cursor = min(len(indexingChoices)-1, *cursor+1)
	case "k", "up":
		*cursor = max(0, *cursor-1)
	case "q":
		if startup {
			m.overlay, m.quitSelected = overlayQuit, false
		} else {
			m.settings.mode = "list"
		}
	case "esc":
		if startup {
			m.chooseIndexingScope = false
			m.scanFinished = true
			m.notice = "Indexing skipped. Open Settings (,) to choose coverage and rescan."
		} else {
			m.settings.mode = "list"
		}
	case "enter":
		store, ok := m.deps.settings.(indexingScopeStore)
		if !ok {
			return m, nil
		}
		if err := store.SetIndexingDays(indexingChoices[*cursor]); err != nil {
			if startup {
				m.scopeError = "Could not save indexing scope: " + sanitizeText(err.Error())
			} else {
				m.settings.status = "Could not save indexing scope: " + sanitizeText(err.Error())
			}
			return m, nil
		}
		if startup {
			m.chooseIndexingScope = false
			m.initialIndexing = true
			m.initialStarted = m.now()
			m.scopeError = ""
			return m, tea.Batch(m.scanCmd, m.initialTick())
		}
		m.settings.mode = "list"
		m.settings.status = "Indexing scope saved; cached history retained. Rescanning…"
		m.reloadSettings()
		return m, m.rescanCmd(true)
	}
	return m, nil
}

func (m *appModel) viewIndexingScope(w, h int, startup bool) string {
	cursor := m.scopeCursor
	title := "Choose initial indexing scope"
	errorText := m.scopeError
	if !startup {
		cursor = m.settings.scopeCursor
		title = "Indexing scope"
		errorText = m.settings.status
	}
	lines := []string{m.styles().heading.Render(title), "New files: use file modification time.", "Cached history kept; logs stay local."}
	for i, days := range indexingChoices {
		label := "All history"
		if days != 0 {
			label = fmt.Sprintf("Last %d days", days)
		}
		if days == 30 {
			label += " (recommended)"
		}
		row := "  " + label
		if i == cursor {
			row = m.styles().accent.Render("> " + label)
		}
		lines = append(lines, row)
	}
	lines = append(lines, m.styles().gold.Render("All history may take hours."))
	if errorText != "" {
		lines = append(lines, m.styles().gold.Render(errorText))
	}
	footer := m.shortcutHints("  ", "↑↓/jk", "Choose", "Enter", "Save", "Esc", "Back")
	if startup {
		footer = m.shortcutHints("  ", "↑↓/jk", "Choose", "Enter", "Start") + "\n" + m.shortcutHints("  ", "Esc", "Skip indexing", "q", "Quit")
	}
	if m.ascii {
		footer = strings.ReplaceAll(footer, "↑↓", "Up/Down")
	}
	bodyH := max(1, h-1-strings.Count(footer, "\n")-1)
	// Selection and save errors remain visible at 40x12.
	body := windowLines(lines, len(lines)-1, bodyH)
	return fitPane(body, w, bodyH) + "\n" + m.rule(w) + "\n" + footer
}
