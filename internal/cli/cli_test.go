package cli_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/cli"
)

func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := cli.NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestHelp(t *testing.T) {
	stdout, _, err := run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "trailmark") || !strings.Contains(stdout, "Remember recent coding-agent work") {
		t.Errorf("help output missing summary:\n%s", stdout)
	}
}

func TestVersion(t *testing.T) {
	stdout, _, err := run(t, "--version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, cli.Version) {
		t.Errorf("version output = %q, want it to contain %q", stdout, cli.Version)
	}
}

func TestNonTTYMessage(t *testing.T) {
	orig := cli.IsTerminal
	cli.IsTerminal = func(*os.File) bool { return false }
	defer func() { cli.IsTerminal = orig }()
	_, _, err := run(t)
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Errorf("err = %v, want a clear non-TTY explanation", err)
	}
}

func TestRejectsArgsAndSubcommands(t *testing.T) {
	if _, _, err := run(t, "list"); err == nil {
		t.Error("positional args must be rejected in the minimal launch interface")
	}
	if _, _, err := run(t, "search", "foo"); err == nil {
		t.Error("subcommands are outside the Phase 1 launch interface")
	}
	if _, _, err := run(t, "completion", "bash"); err == nil {
		t.Error("cobra's default completion command must be disabled")
	}
}
