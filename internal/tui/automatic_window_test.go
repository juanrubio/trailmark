package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestAutomaticWindowStartup(t *testing.T) {
	for _, tc := range []struct {
		name           string
		age, want      int
		unknown, empty bool
	}{
		{name: "recent", age: 2, want: 7}, {name: "older", age: 15, want: 30}, {name: "ancient", age: 80, want: 0}, {name: "unknown", unknown: true, want: 0}, {name: "empty", empty: true, want: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			when := time.Now().AddDate(0, 0, -tc.age)
			rows := []index.SessionRow{{SourcePath: "/fictional/session", ID: "a", CWD: "/fictional/project", LastActivity: &when}}
			if tc.unknown {
				rows[0].LastActivity = nil
			}
			if tc.empty {
				rows = nil
			}
			m := newModel(context.Background(), dataDeps{}, nil, "/fictional/project", "/fictional", rows)
			if m.windowDays != tc.want {
				t.Fatalf("got %d want %d", m.windowDays, tc.want)
			}
			if !tc.empty && len(m.visibleSessions()) == 0 {
				t.Fatal("startup still empty")
			}
			// Launch defaults to the project containing the
			// launch directory when it has recorded sessions.
			want := "current"
			if tc.empty {
				want = ""
			}
			if m.project != want {
				t.Fatalf("startup project = %q, want %q", m.project, want)
			}
		})
	}
}

func TestAutomaticWindowScopeAndManualSelection(t *testing.T) {
	m := testModel(t)
	old := testNow.AddDate(0, 0, -15)
	m.sessions = append(m.sessions, index.SessionRow{SourcePath: "/fictional/older", ID: "older", CWD: "/fictional/older-project", LastActivity: &old, Provider: model.ProviderClaudeCode})
	m.selector = selectorProject
	for _, option := range m.selectorOptions() {
		if option.label == "/fictional/older-project" {
			m.applySelector(option)
		}
	}
	if m.windowDays != 30 || len(m.visibleSessions()) != 1 {
		t.Fatal("project selection did not widen to 30 days")
	}
	m.selector = selectorTime
	for _, option := range m.selectorOptions() {
		if option.label == "7 days" {
			m.applySelector(option)
		}
	}
	if m.windowDays != 7 || len(m.visibleSessions()) != 0 || !m.manualWindow {
		t.Fatal("explicit empty 7-day selection was overridden")
	}
	m.selector = selectorProject
	for _, option := range m.selectorOptions() {
		if option.label == "/fictional/older-project" {
			m.applySelector(option)
		}
	}
	if m.windowDays != 7 {
		t.Fatal("project selection overrode manual time")
	}
}

func TestAutomaticWindowFirstScanAndTextFilter(t *testing.T) {
	m := testModel(t)
	m.sessions = nil
	old := testNow.AddDate(0, 0, -80)
	rows := []index.SessionRow{{SourcePath: "/fictional/ancient", ID: "a", CWD: "/fictional", LastActivity: &old}}
	m.Update(scanDoneMsg{sessions: rows})
	if m.windowDays != 0 || len(m.visibleSessions()) != 1 {
		t.Fatal("initial asynchronous scan did not choose All time")
	}
	m = testModel(t)
	m.filterText = "no matching metadata"
	m.chooseAutomaticWindow()
	if m.windowDays != 7 {
		t.Fatal("text filter silently widened history")
	}
	m.harness = model.ProviderClaudeCode
	m.project = "/fictional/absent"
	m.chooseAutomaticWindow()
	if m.windowDays != 7 {
		t.Fatal("truly empty scope did not retain default")
	}
}

func TestAutomaticWindowProjectHarnessIntersectionAndReturn(t *testing.T) {
	m := testModel(t)
	recent, old := testNow.AddDate(0, 0, -1), testNow.AddDate(0, 0, -20)
	m.sessions = []index.SessionRow{
		{SourcePath: "/fictional/shared-codex", ID: "c", CWD: "/fictional/shared", Provider: model.ProviderCodex, LastActivity: &recent},
		{SourcePath: "/fictional/shared-claude", ID: "a", CWD: "/fictional/shared", Provider: model.ProviderClaudeCode, LastActivity: &old},
		{SourcePath: "/fictional/recent-claude", ID: "b", CWD: "/fictional/recent", Provider: model.ProviderClaudeCode, LastActivity: &recent},
	}
	choose := func(kind selectorKind, label string) {
		t.Helper()
		m.selector = kind
		for _, option := range m.selectorOptions() {
			if strings.HasPrefix(option.label, label) {
				m.applySelector(option)
				return
			}
		}
		t.Fatalf("missing selector %s", label)
	}
	choose(selectorProject, "/fictional/shared")
	if m.windowDays != 7 {
		t.Fatal("recent project unexpectedly widened")
	}
	choose(selectorHarness, "Claude Code")
	if m.windowDays != 30 || len(m.visibleSessions()) != 1 || m.visibleSessions()[0].SourcePath != "/fictional/shared-claude" {
		t.Fatal("recent sessions outside project+harness intersection prevented fallback")
	}
	choose(selectorProject, "/fictional/recent")
	if m.windowDays != 7 || len(m.visibleSessions()) != 1 {
		t.Fatal("returning to a recent scope did not restore seven days")
	}
}

func TestLaunchProjectDefault(t *testing.T) {
	when := time.Now()
	rows := []index.SessionRow{
		{SourcePath: "/fictional/here", ID: "here", CWD: "/fictional/project", LastActivity: &when},
		{SourcePath: "/fictional/else", ID: "else", CWD: "/fictional/other", LastActivity: &when},
	}
	// Inside a project with sessions: scoped to it.
	m := newModel(context.Background(), dataDeps{}, nil, "/fictional/project", "/fictional", rows)
	if m.project != "current" || len(m.visibleSessions()) != 1 || m.visibleSessions()[0].ID != "here" {
		t.Fatalf("launch in project: project=%q visible=%d", m.project, len(m.visibleSessions()))
	}
	// Outside any known project: All projects.
	m = newModel(context.Background(), dataDeps{}, nil, "/fictional/nowhere", "/fictional", rows)
	if m.project != "" || len(m.visibleSessions()) != 2 {
		t.Fatalf("launch elsewhere: project=%q visible=%d", m.project, len(m.visibleSessions()))
	}
	// From a subdirectory of a Git worktree: scoped to the root.
	gitRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(gitRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gitRoot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRows := []index.SessionRow{
		{SourcePath: "/fictional/here", ID: "here", CWD: gitRoot, LastActivity: &when},
		{SourcePath: "/fictional/else", ID: "else", CWD: "/fictional/other", LastActivity: &when},
	}
	m = newModel(context.Background(), dataDeps{}, nil, filepath.Join(gitRoot, "sub"), "/fictional", gitRows)
	if m.project != "current" || len(m.visibleSessions()) != 1 {
		t.Fatalf("worktree subdir: project=%q visible=%d", m.project, len(m.visibleSessions()))
	}
	// A Git worktree with no sessions of its own keeps All projects.
	emptyRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(emptyRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	m = newModel(context.Background(), dataDeps{}, nil, emptyRoot, "/fictional", rows)
	if m.project != "" {
		t.Fatalf("empty worktree must keep All projects, got %q", m.project)
	}
	// A current project with only older sessions widens the
	// window inside that project rather than leaking others.
	old := when.AddDate(0, 0, -15)
	rows[0].LastActivity = &old
	m = newModel(context.Background(), dataDeps{}, nil, "/fictional/project", "/fictional", rows)
	if m.project != "current" || m.windowDays != 30 || len(m.visibleSessions()) != 1 {
		t.Fatalf("older current project: project=%q window=%d visible=%d", m.project, m.windowDays, len(m.visibleSessions()))
	}
}

func TestLaunchProjectDefaultAfterFirstScan(t *testing.T) {
	when := time.Now()
	m := newModel(context.Background(), dataDeps{}, nil, "/home/dev/repos/trailmark", "/home/dev", nil)
	m.Update(scanDoneMsg{sessions: []index.SessionRow{
		{SourcePath: "/fictional/here", ID: "here", CWD: "/home/dev/repos/trailmark", LastActivity: &when},
		{SourcePath: "/fictional/else", ID: "else", CWD: "/home/dev/other", LastActivity: &when},
	}})
	if m.project != "current" || len(m.visibleSessions()) != 1 {
		t.Fatalf("first scan: project=%q visible=%d", m.project, len(m.visibleSessions()))
	}
}

func TestManualTimeSurvivesAppliedRefresh(t *testing.T) {
	m := testModel(t)
	m.selector = selectorTime
	for _, option := range m.selectorOptions() {
		if option.label == "7 days" {
			m.applySelector(option)
		}
	}
	old := testNow.AddDate(0, 0, -50)
	rows := []index.SessionRow{{SourcePath: "/fictional/old-refresh", ID: "old", CWD: "/fictional", LastActivity: &old}}
	m.applyScanID = 42
	m.Update(scanDoneMsg{id: 42, sessions: rows})
	if m.windowDays != 7 || !m.manualWindow || len(m.visibleSessions()) != 0 {
		t.Fatal("applied refresh overrode explicit empty seven-day scope")
	}
}
