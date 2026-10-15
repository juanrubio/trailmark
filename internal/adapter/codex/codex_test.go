package codex_test

import (
	"context"
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/adapter/codex"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestParseBasic(t *testing.T) {
	s, err := codex.Adapter{}.ParseFile(context.Background(), "testdata/rollout-basic.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "019d0000-0000-7000-8000-000000000001" {
		t.Errorf("ID = %q", s.ID)
	}
	if s.CWD != "/work/codex-project" {
		t.Errorf("CWD = %q", s.CWD)
	}
	if s.Purpose != "Add a retry to the upload worker" || s.PurposeOrigin != model.OriginRequestExcerpt {
		t.Errorf("Purpose = %q (%s)", s.Purpose, s.PurposeOrigin)
	}
	if len(s.Messages) != 4 {
		t.Fatalf("Messages = %d, want 4", len(s.Messages))
	}
	if s.Messages[1].Kind != "tool_call" || s.Messages[2].Kind != "tool_output" {
		t.Errorf("tool kinds = %q, %q", s.Messages[1].Kind, s.Messages[2].Kind)
	}
	if s.Messages[0].Locator.Line != 2 {
		t.Errorf("first message line = %d, want 2", s.Messages[0].Locator.Line)
	}
}

func TestParseForkedPartial(t *testing.T) {
	s, err := codex.Adapter{}.ParseFile(context.Background(), "testdata/rollout-forked.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.ForkedFromID != "019d0000-0000-7000-8000-000000000099" {
		t.Errorf("ForkedFromID = %q", s.ForkedFromID)
	}
	if s.ParentID != "019d0000-0000-7000-8000-000000000098" {
		t.Errorf("ParentID = %q", s.ParentID)
	}
	if !s.Partial || s.ParseErrors != 1 {
		t.Errorf("Partial=%v errors=%d, want partial with 1 error", s.Partial, s.ParseErrors)
	}
}

func TestThreadNameTitle(t *testing.T) {
	a := codex.Adapter{ThreadNames: map[string]string{"019d0000-0000-7000-8000-000000000001": "Upload worker retry"}}
	s, err := a.ParseFile(context.Background(), "testdata/rollout-basic.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.Purpose != "Upload worker retry" || s.PurposeOrigin != model.OriginNativeTitle {
		t.Errorf("Purpose = %q (%s), want native title", s.Purpose, s.PurposeOrigin)
	}
}

func TestInjectedContextSkippedAndToolArgumentsKept(t *testing.T) {
	s, err := codex.Adapter{}.ParseFile(context.Background(), "testdata/rollout-injected.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.Purpose != "Ship the actual codex task" {
		t.Errorf("Purpose = %q, want the first substantive user request", s.Purpose)
	}
	var toolText string
	for _, m := range s.Messages {
		if m.Kind == "tool_call" {
			toolText = m.Text
		}
	}
	if toolText == "" || !strings.Contains(toolText, "internal/worker") {
		t.Errorf("tool_call text = %q, want it to include the tool arguments for search", toolText)
	}
}

func TestCapability(t *testing.T) {
	c := codex.Adapter{}.Capability()
	if !c.ForkSupported || c.ResumeTemplate != "codex resume {id}" || c.ForkTemplate != "codex fork {id}" {
		t.Errorf("capability = %+v", c)
	}
}
