package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/juanrubio/trailmark/internal/index"
)

// card is one selectable row in the display list.
type card struct {
	row        index.SessionRow
	child      bool // shown as an expanded or orphaned child entry
	childCount int
	expanded   bool
}

// visibleSessions applies time, harness, project, and text filters
// by intersection. Order stays newest-activity first.
func (m *appModel) visibleSessions() []index.SessionRow {
	var out []index.SessionRow
	for _, s := range m.sessions {
		if !m.inWindow(s) {
			continue
		}
		if m.harness != "" && s.Provider != m.harness {
			continue
		}
		if !m.inProject(s) {
			continue
		}
		if m.filterText != "" && !matchesText(s, m.filterText) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (m *appModel) inWindow(s index.SessionRow) bool {
	if m.windowDays == 0 {
		return true
	}
	if s.LastActivity == nil {
		return false // unknown dates live under All time only
	}
	now := m.now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	cutoff := dayStart.AddDate(0, 0, -(m.windowDays - 1))
	return !s.LastActivity.Before(cutoff)
}

func (m *appModel) inProject(s index.SessionRow) bool {
	switch m.project {
	case "":
		return true
	case "current":
		return relatedDir(s.CWD, m.currentProjectRoot())
	default:
		return s.CWD == m.project
	}
}

// currentProjectRoot resolves the launch directory to its Git
// worktree root when inside one, else the launch directory itself.
func (m *appModel) currentProjectRoot() string {
	if m.projectRootSet {
		return m.projectRoot
	}
	m.projectRootSet = true
	m.projectRoot = m.launchDir
	dir := m.launchDir
	for dir != "" {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			m.projectRoot = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return m.projectRoot
}

// relatedDir reports whether two directories are the same or nested.
func relatedDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}

func matchesText(s index.SessionRow, q string) bool {
	hay := strings.ToLower(s.Purpose + "\n" + s.Title + "\n" + s.CWD + "\n" + providerLabel(s.Provider))
	return strings.Contains(hay, strings.ToLower(q))
}

// cards flattens visible sessions into selectable entries with
// explicit children collapsed beneath their parent when the parent
// is also visible. An orphaned child stays a labeled top-level card.
func (m *appModel) cards() []card {
	visible := m.visibleSessions()
	identity := func(s index.SessionRow) string {
		return string(s.Provider) + "|" + s.Store + "|" + s.ID
	}
	byIdentity := map[string]index.SessionRow{}
	for _, s := range visible {
		if s.ID != "" {
			byIdentity[identity(s)] = s
		}
	}
	// A session attaches only to a top-level parent. Grandchildren
	// and cycle members stay visible as labeled orphan entries
	// rather than vanishing.
	attached := func(s index.SessionRow) (index.SessionRow, bool) {
		if s.ParentID == "" {
			return index.SessionRow{}, false
		}
		parent, ok := byIdentity[string(s.Provider)+"|"+s.Store+"|"+s.ParentID]
		if !ok || parent.ParentID != "" {
			return index.SessionRow{}, false
		}
		return parent, true
	}
	childOf := map[string][]index.SessionRow{}
	var top []index.SessionRow
	for _, s := range visible {
		if parent, ok := attached(s); ok {
			childOf[parent.SourcePath] = append(childOf[parent.SourcePath], s)
			continue
		}
		top = append(top, s)
	}
	var out []card
	for _, s := range top {
		children := childOf[s.SourcePath]
		c := card{row: s, childCount: len(children), expanded: m.expanded[s.SourcePath]}
		if (s.ParentID != "" && s.ID != "") || s.Sidechain {
			if _, isAttached := attached(s); !isAttached {
				c.child = true // orphan child or sidechain: labeled entry
			}
		}
		out = append(out, c)
		if c.expanded {
			for _, ch := range children {
				out = append(out, card{row: ch, child: true})
			}
		}
	}
	return out
}

// orphanChild reports whether a card is a child whose parent is not
// in the visible set, so it needs its own labeled entry.
func (m *appModel) isOrphanChild(s index.SessionRow) bool {
	if s.ParentID == "" {
		return false
	}
	for _, v := range m.visibleSessions() {
		if v.Provider == s.Provider && v.Store == s.Store && v.ID == s.ParentID && v.ParentID == "" {
			return false
		}
	}
	return true
}

func (m *appModel) selected() (card, bool) {
	cards := m.cards()
	if m.sel < 0 || m.sel >= len(cards) {
		return card{}, false
	}
	return cards[m.sel], true
}

func (m *appModel) clampSelection() {
	n := len(m.cards())
	if n == 0 {
		m.sel = 0
		return
	}
	if m.sel >= n {
		m.sel = n - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
}

// projects lists the distinct session directories, sorted.
func (m *appModel) projects() []string {
	set := map[string]bool{}
	for _, s := range m.sessions {
		if s.CWD != "" {
			set[s.CWD] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// currentProjectAvailable reports whether the launch directory maps
// to a known session directory or sits inside a Git worktree.
func (m *appModel) currentProjectAvailable() bool {
	if m.launchDir == "" {
		return false
	}
	if m.currentProjectRoot() != m.launchDir {
		return true // inside a Git worktree
	}
	for _, s := range m.sessions {
		if s.CWD == m.launchDir {
			return true
		}
	}
	return false
}

// relativeTime renders quiet recency for card lines.
func relativeTime(t *time.Time, now time.Time) string {
	if t == nil {
		return "date unknown"
	}
	d := now.Sub(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour && t.Day() == now.Day():
		return itoa(int(d.Hours())) + "h ago"
	default:
		return t.Format("Mon 15:04")
	}
}

// dayHeading labels a session's last-active day.
func dayHeading(t *time.Time, now time.Time) string {
	if t == nil {
		return "Unknown date"
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case day.Equal(today):
		return "Today"
	case day.Equal(today.AddDate(0, 0, -1)):
		return "Yesterday"
	default:
		return t.Format("Mon 02 Jan")
	}
}
