package continuation_test

import (
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/continuation"
	"github.com/juanrubio/trailmark/internal/model"
)

func init() {
	continuation.DirExists = func(path string) bool { return path != "/gone" }
}

var capCodex = model.Capability{Provider: model.ProviderCodex, ResumeTemplate: "codex resume {id}", ForkTemplate: "codex fork {id}", ForkSupported: true, CleanTemplate: "codex"}

func TestPrepareResumeQuotesArguments(t *testing.T) {
	p := continuation.Prepare(continuation.Resume, capCodex, model.ProviderCodex, "id 1", "/work/my proj", "Task")
	if !p.Available {
		t.Fatalf("unavailable: %s", p.Reason)
	}
	if !strings.Contains(p.Command, "cd -- '/work/my proj'") || !strings.Contains(p.Command, "codex resume 'id 1'") {
		t.Errorf("command = %q, want quoted cwd and id", p.Command)
	}
	if p.TargetDir != "/work/my proj" {
		t.Errorf("TargetDir = %q", p.TargetDir)
	}
}

func TestPrepareMissingIDAndDir(t *testing.T) {
	p := continuation.Prepare(continuation.Resume, capCodex, model.ProviderCodex, "", "/work/x", "Task")
	if p.Available || !strings.Contains(p.Reason, "session ID") {
		t.Errorf("missing ID: %+v", p)
	}
	p = continuation.Prepare(continuation.Clean, capCodex, model.ProviderCodex, "", "/gone", "Task")
	if p.Available || !strings.Contains(p.Reason, "no longer exists") {
		t.Errorf("missing dir: %+v", p)
	}
	p = continuation.Prepare(continuation.Clean, capCodex, model.ProviderCodex, "", "", "Task")
	if p.Available || !strings.Contains(p.Reason, "No working directory") {
		t.Errorf("missing cwd: %+v", p)
	}
}

func TestPrepareForkUnavailableAndCleanNeedsNoID(t *testing.T) {
	noFork := capCodex
	noFork.ForkSupported = false
	noFork.ForkTemplate = ""
	p := continuation.Prepare(continuation.Fork, noFork, model.ProviderCodex, "id-1", "/work/x", "Task")
	if p.Available || !strings.Contains(p.Reason, "Fork unavailable") {
		t.Errorf("fork: %+v", p)
	}
	p = continuation.Prepare(continuation.Clean, capCodex, model.ProviderCodex, "", "/work/x", "Task")
	if !p.Available || !strings.HasSuffix(p.Command, "&& codex") {
		t.Errorf("clean: %+v", p)
	}
	if strings.Contains(p.Command, "resume") {
		t.Error("clean command must not reference prior history")
	}
}

func TestShellQuote(t *testing.T) {
	if got := continuation.ShellQuote("/plain/path-1"); got != "/plain/path-1" {
		t.Errorf("plain = %q", got)
	}
	if got := continuation.ShellQuote("a'b"); got != `'a'\''b'` {
		t.Errorf("quoted = %q", got)
	}
}

func TestPrepareCraftedIDsStayLiteral(t *testing.T) {
	p := continuation.Prepare(continuation.Resume, capCodex, model.ProviderCodex, "{cwd}", "/work/x", "Task")
	if !p.Available || !strings.Contains(p.Command, "codex resume '{cwd}'") {
		t.Errorf("crafted ID must stay a literal quoted argument: %q", p.Command)
	}
	p = continuation.Prepare(continuation.Resume, capCodex, model.ProviderCodex, "--fork", "/work/x", "Task")
	if !p.Available || !strings.Contains(p.Command, "codex resume '--fork'") {
		t.Errorf("dash-leading ID must be quoted, not a flag: %q", p.Command)
	}
}
