package tui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type initialIndexTickMsg time.Time

// Poll independently of scanner events: parsing a large log must not freeze
// the animation, and no database reads run on this path.
func (m *appModel) initialTick() tea.Cmd {
	if !m.initialIndexing {
		return nil
	}
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return initialIndexTickMsg(t) })
}

func (m *appModel) viewInitialIndexing(w, h int) string {
	st := m.styles()
	clip := func(s string) string {
		if m.ascii {
			return ansi.Truncate(strings.NewReplacer("·", "|", "…", "...").Replace(s), w, "...")
		}
		return truncate(s, w)
	}
	frames := []string{"◐", "◓", "◑", "◒"}
	if m.ascii {
		frames = []string{"|", "/", "-", "\\"}
	}
	phase := "Discovering local session logs"
	switch m.initialProgress.Phase {
	case "indexing":
		phase = "Indexing local session logs"
	case "finishing", "done":
		phase = "Finishing the local index"
	}
	if m.initialCanceling {
		phase = "Canceling; opening indexed sessions"
	}
	elapsed := max(0, int(m.now().Sub(m.initialStarted).Seconds()))
	lines := []string{st.heading.Render("Trailmark · Initial indexing"), st.muted.Render("Scope: " + indexingScopeLabel(m.indexingDays())), st.accent.Render(frames[m.initialFrame%len(frames)] + " " + phase), st.muted.Render(fmt.Sprintf("Elapsed %dm %02ds", elapsed/60, elapsed%60)), ""}
	explanation := "Reading local logs into a reusable local cache. Future launches open your cached sessions immediately."
	// Keep the controls and real counts visible even in the smallest viewport.
	if h < 16 {
		lines[4] = st.muted.Render("Local logs -> local cache")
	}
	if h >= 16 {
		lines = append(lines, wrap(explanation, w)...)
		lines = append(lines, "")
	}
	stats := m.initialProgress.Stats
	checked := stats.Indexed + stats.Skipped + stats.Failed + stats.Excluded
	lines = append(lines, st.accent.Render(fmt.Sprintf("%d discovered · %d checked", stats.Discovered, checked)), st.accent.Render(fmt.Sprintf("%d indexed · %d unchanged/skipped", stats.Indexed, stats.Skipped)))
	if h >= 14 || stats.Failed+stats.Partial+stats.Excluded > 0 {
		lines = append(lines, st.gold.Render(fmt.Sprintf("%d partial · %d failed · %d excluded", stats.Partial, stats.Failed, stats.Excluded)))
	}
	lines = append(lines, st.muted.Render(fmt.Sprintf("%d older files deferred; cache retained", stats.Deferred)))
	current := strings.NewReplacer("\n", " ", "\t", " ").Replace(sanitizeText(shortenHome(m.initialProgress.Path, m.home)))
	if current == "" {
		current = "Looking for configured sources…"
	}
	if m.ascii {
		current = strings.ReplaceAll(current, "…", "...")
	}
	provider := sanitizeText(providerLabel(m.initialProgress.Provider))
	if provider != "" {
		current = provider + ": " + current
	}
	lines = append(lines, st.path.Render(clip(current)))
	if h >= 16 {
		lines = append(lines, st.muted.Render("Esc stops indexing and opens committed sessions."))
	}
	body := make([]string, len(lines))
	for i, line := range lines {
		body[i] = clip(line)
	}
	footer := m.shortcutHints(" | ", "m", "Memories", "Esc", "Cancel and browse", "q", "Quit")
	if m.ascii {
		footer = m.shortcutHints(" | ", "m", "Memories", "Esc", "Cancel and browse", "q", "Quit")
	}
	if m.initialCanceling {
		footer = m.shortcutHints(" | ", "q", "Quit") + " | Opening indexed sessions"
		if m.ascii {
			footer = m.shortcutHints(" | ", "q", "Quit") + " | Opening indexed sessions"
		}
	}
	return fitPane(strings.Join(body, "\n"), w, max(1, h-2)) + "\n" + m.rule(w) + "\n" + clip(footer)
}
