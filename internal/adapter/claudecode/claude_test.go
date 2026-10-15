package claudecode_test

import (
	"context"
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/adapter/claudecode"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestParseBasic(t *testing.T) {
	s, err := claudecode.Adapter{}.ParseFile(context.Background(), "testdata/session-basic.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("ID = %q", s.ID)
	}
	if s.CWD != "/work/example-project" {
		t.Errorf("CWD = %q", s.CWD)
	}
	if s.Purpose != "Fix the flaky checkout test" || s.PurposeOrigin != model.OriginRequestExcerpt {
		t.Errorf("Purpose = %q (%s)", s.Purpose, s.PurposeOrigin)
	}
	if s.Partial || s.ParseErrors != 0 {
		t.Errorf("Partial=%v errors=%d, want clean parse", s.Partial, s.ParseErrors)
	}
	if len(s.Messages) != 5 {
		t.Fatalf("Messages = %d, want 5 (user, text, tool_call, tool_output, text)", len(s.Messages))
	}
	if s.Messages[0].Locator.Line != 1 || s.Messages[0].Locator.RecordID != "u-1" {
		t.Errorf("first locator = %+v", s.Messages[0].Locator)
	}
	var sawTool bool
	for _, m := range s.Messages {
		if m.Kind == "tool_output" && m.Text == "ok example 0.5s" {
			sawTool = true
		}
	}
	if !sawTool {
		t.Error("tool_output message missing")
	}
	if s.StartedAt == nil || s.LastActivityAt == nil || !s.LastActivityAt.After(*s.StartedAt) {
		t.Errorf("timestamps started=%v last=%v", s.StartedAt, s.LastActivityAt)
	}
}

func TestParsePartialAndTitle(t *testing.T) {
	s, err := claudecode.Adapter{}.ParseFile(context.Background(), "testdata/session-partial.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Native titled session" || s.PurposeOrigin != model.OriginNativeTitle {
		t.Errorf("Title=%q origin=%s", s.Title, s.PurposeOrigin)
	}
	if !s.Partial || s.ParseErrors != 1 {
		t.Errorf("Partial=%v errors=%d, want partial with 1 error", s.Partial, s.ParseErrors)
	}
	if !s.Sidechain {
		t.Error("Sidechain not detected")
	}
	if len(s.Messages) != 2 {
		t.Errorf("Messages = %d, want 2 (truncated line skipped)", len(s.Messages))
	}
}

func TestInjectedSetupSkippedAndToolInputKept(t *testing.T) {
	s, err := claudecode.Adapter{}.ParseFile(context.Background(), "testdata/session-injected.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.Purpose != "Real task after setup" {
		t.Errorf("Purpose = %q, want the first substantive user request", s.Purpose)
	}
	var toolText string
	for _, m := range s.Messages {
		if m.Kind == "tool_call" {
			toolText = m.Text
		}
	}
	if toolText == "" || !strings.Contains(toolText, "internal/upload") {
		t.Errorf("tool_call text = %q, want it to include the tool input for search", toolText)
	}
}

func TestCapability(t *testing.T) {
	c := claudecode.Adapter{}.Capability()
	if !c.ForkSupported || c.ResumeTemplate == "" || c.ForkTemplate == "" || c.CleanTemplate == "" {
		t.Errorf("capability incomplete: %+v", c)
	}
}
