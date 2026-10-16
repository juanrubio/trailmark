package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
	"strings"
	"testing"
)

func TestRecordedMetadataUnknownAndZero(t *testing.T) {
	unknown := strings.Join(overviewMetadataLines(model.SessionMetadata{}, model.Provider("codex")), "\n")
	if !strings.Contains(unknown, "Tokens input: Unavailable") || !strings.Contains(unknown, "Latest turn outcome: Unavailable") {
		t.Fatal(unknown)
	}
	zero := int64(0)
	meta := model.SessionMetadata{Tokens: model.TokenUsage{Input: &zero, Reasoning: &zero}, Outcome: model.TurnOutcome{Status: "completed", DurationMS: &zero}}
	got := strings.Join(overviewMetadataLines(meta, model.Provider("codex")), "\n")
	for _, want := range []string{"Tokens input: 0", "reasoning (output subset): 0", "duration: 0s", "task success unverified"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestOverviewWorkExpansionFindAndSelectionReset(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 12}, {144, 40}} {
		m := testModel(t)
		m.width, m.height = size[0], size[1]
		send(m, "enter")
		m.detail = &index.Overview{Metadata: model.SessionMetadata{Models: []string{"modelneedle"}, Work: model.WorkSummary{ChangedFiles: []string{"uniquefile.go"}}}}
		m.View()
		typeOverviewFind(m, "uniquefile")
		if len(m.overviewFind.matches) != 0 {
			t.Fatal("collapsed work searchable")
		}
		send(m, "e")
		m.View()
		if !m.workExpanded || len(m.overviewFind.matches) != 1 {
			t.Fatalf("expanded search incorrect at %v: %d", size, len(m.overviewFind.matches))
		}
		send(m, "e")
		m.View()
		if m.workExpanded || len(m.overviewFind.matches) != 0 {
			t.Fatal("collapsed work retained matches")
		}
		send(m, "/")
		send(m, "e")
		if m.workExpanded {
			t.Fatal("find input toggled work")
		}
		send(m, "esc")
		send(m, "e")
		send(m, "esc")
		send(m, "j")
		m.refreshDetail()
		if m.workExpanded {
			t.Fatal("selection retained expansion")
		}
	}
}

func TestOverviewMetadataFindAndBounds(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 12}, {144, 40}} {
		m := testModel(t)
		m.width, m.height = size[0], size[1]
		m.noColor = true
		send(m, "enter")
		m.detail = &index.Overview{Metadata: model.SessionMetadata{Git: model.GitMetadata{Branch: "feature/metadata-needle"}}}
		typeOverviewFind(m, "metadata-needle")
		view := ansi.Strip(m.View().Content)
		for _, action := range []string{"[r] Resume", "[f] Fork", "[n] Clean"} {
			if !strings.Contains(view, action) {
				t.Errorf("missing pinned action %q at %v: %s", action, size, view)
			}
		}
		if len(m.overviewFind.matches) != 1 {
			t.Fatalf("metadata unsearchable at %v", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("overflow at %v: %q", size, line)
			}
		}
	}
}

func TestRecordedCountGrouping(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{{0, "0"}, {123, "123"}, {1234, "1,234"}, {12345678, "12,345,678"}, {-1234, "-1,234"}} {
		if got := recordedCount(&tc.n); got != tc.want {
			t.Errorf("%d: %s", tc.n, got)
		}
	}
}

func TestOverviewWorkToggleRevealsHeading(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 80, 24
	send(m, "enter")
	if m.overviewScroll != 0 {
		t.Fatal("unexpected scroll")
	}
	send(m, "e")
	if m.overviewScroll == 0 || !strings.Contains(ansi.Strip(m.View().Content), "Recorded work · expanded") {
		t.Fatal("expanding work did not reveal heading")
	}
}

func TestIdenticalScanRefreshesOverviewMetadataPreservingState(t *testing.T) {
	m := testModel(t)
	send(m, "enter")
	m.workExpanded = true
	m.overviewFind.query = "old-model"
	m.overviewFind.draft = "old-model"
	m.overviewFind.current = 2
	m.overviewScroll = 7
	selected, _ := m.selected()
	oldOrder := sessionsSignature(m.sessions)
	m.deps.overview = func(_ context.Context, path string) (*index.Overview, error) {
		if path != selected.row.SourcePath {
			t.Fatalf("wrong selection: %s", path)
		}
		return &index.Overview{Metadata: model.SessionMetadata{Models: []string{"fresh-model"}}}, nil
	}
	m.Update(scanDoneMsg{sessions: m.sessions})
	if m.detail.Metadata.Models[0] != "fresh-model" {
		t.Fatal("overview metadata stayed cached")
	}
	current, _ := m.selected()
	if current.row.SourcePath != selected.row.SourcePath || sessionsSignature(m.sessions) != oldOrder || m.hasPending {
		t.Fatal("scan altered list state")
	}
	if !m.workExpanded || m.overviewScroll != 7 || m.overviewFind.query != "old-model" || m.overviewFind.current != 2 {
		t.Fatal("scan altered overview interaction state")
	}
	cached := m.detail
	m.deps.overview = func(context.Context, string) (*index.Overview, error) { return nil, fmt.Errorf("unavailable") }
	m.Update(scanDoneMsg{sessions: m.sessions})
	if m.detail != cached {
		t.Fatal("detail read failure discarded cached data")
	}
	called := false
	m.deps.overview = func(context.Context, string) (*index.Overview, error) { called = true; return nil, nil }
	m.Update(scanDoneMsg{sessions: m.sessions, err: fmt.Errorf("scan failed")})
	if called || m.detail != cached {
		t.Fatal("failed scan changed cached detail")
	}
}
