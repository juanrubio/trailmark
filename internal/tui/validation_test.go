package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/continuation"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

// Scripted PRD section 9 validation against synthetic fixtures.
// These runs check that each task is completable from the list and
// overview alone (no transcript or content search) and record the
// outcome for docs/validation.md. They are not human-participant
// results; that limitation is recorded in the doc.

func validationSessions() []index.SessionRow {
	return []index.SessionRow{
		{SourcePath: "/v/yesterday.jsonl", Provider: model.ProviderClaudeCode, ID: "v1", Purpose: "Repair the deployment flow", PurposeOrigin: model.OriginRequestExcerpt, CWD: "/home/dev/repos/music-lab", LastActivity: at(14, 17)},
		{SourcePath: "/v/old.jsonl", Provider: model.ProviderCodex, ID: "v2", Purpose: "Migrate the billing database schema", PurposeOrigin: model.OriginNativeTitle, CWD: "/home/dev/repos/infra", LastActivity: at(10, 9)},
		{SourcePath: "/v/web.jsonl", Provider: model.ProviderCodex, ID: "v3", Purpose: "Fix the login redirect loop", CWD: "/home/dev/repos/web-app", LastActivity: at(15, 10)},
		{SourcePath: "/v/admin.jsonl", Provider: model.ProviderClaudeCode, ID: "v4", Purpose: "Fix the login redirect loop", CWD: "/home/dev/repos/admin-console", LastActivity: at(15, 9)},
		{SourcePath: "/v/main.jsonl", Provider: model.ProviderCodex, ID: "v5", Purpose: "Improve the recent-session UX", CWD: "/home/dev/repos/trailmark", LastActivity: at(15, 11)},
		{SourcePath: "/v/wt.jsonl", Provider: model.ProviderCodex, ID: "v6", Purpose: "Improve the recent-session UX", CWD: "/home/dev/worktrees/trailmark-fix", LastActivity: at(15, 8)},
	}
}

func validationModel(t *testing.T) *appModel {
	t.Helper()
	origDirExists := continuation.DirExists
	continuation.DirExists = func(string) bool { return true }
	t.Cleanup(func() { continuation.DirExists = origDirExists })
	m := newModel(context.Background(), dataDeps{overview: fixtureOverview, page: fixturePage, around: fixtureAround, inSession: fixtureInSession, content: fixtureContent, caps: testCaps(), copier: &fakeClipboard{}}, nil, "/home/dev/repos/trailmark", "/home/dev", validationSessions())
	// Recognition tasks span projects; model an All-projects
	// launch (see testModel).
	m.project = ""
	m.chooseAutomaticWindow()
	m.clampSelection()
	m.refreshDetail()
	m.now = func() time.Time { return testNow }
	m.width, m.height = 120, 40
	return m
}

func TestValidationRecognitionTasks(t *testing.T) {
	m := validationModel(t)
	view := m.View().Content
	tasks := []struct {
		name string
		want []string
	}{
		{"yesterday's session across projects", []string{"Repair the deployment flow", "music-lab"}},
		{"3-to-7-day-old session by purpose and directory", []string{"Migrate the billing database schema", "infra"}},
		{"similar purposes in different projects", []string{"web-app", "admin-console"}},
		{"intended worktree among same-repo sessions", []string{"trailmark-fix"}},
		{"session worth continuing shows latest context", []string{"Improve the recent-session UX", "trailmark"}},
	}
	for _, task := range tasks {
		for _, want := range task.want {
			if !strings.Contains(view, want) {
				t.Errorf("task %q: list view missing %q without opening a transcript", task.name, want)
			}
		}
	}
	// Day headings separate yesterday from the older session.
	if !strings.Contains(view, "Yesterday") {
		t.Error("yesterday heading missing")
	}
}

func TestValidationContinuationExplanation(t *testing.T) {
	m := validationModel(t)
	send(m, "r")
	if m.sheet == nil || !strings.Contains(m.View().Content, "Continues the same native conversation") {
		t.Error("resume sheet must explain its history behavior")
	}
	send(m, "esc")
	send(m, "n")
	if m.sheet == nil || !strings.Contains(m.View().Content, "no inherited history") {
		t.Error("clean sheet must explain that no history is inherited")
	}
	send(m, "enter") // copy command (fake clipboard)
	if m.sheetState != "copied" {
		t.Error("copy command must succeed against the fake clipboard without executing anything")
	}
}

func TestValidationPassageFinding(t *testing.T) {
	m := validationModel(t)
	start := time.Now()
	send(m, "t")
	send(m, "/")
	for _, r := range "retry" {
		send(m, string(r))
	}
	send(m, "enter")
	elapsed := time.Since(start)
	if len(m.transcript.matches) == 0 {
		t.Fatal("known passage not found")
	}
	if elapsed > 30*time.Second {
		t.Errorf("passage finding took %s, over the 30s target", elapsed)
	}
	t.Logf("scripted passage finding completed in %s", elapsed)
}
