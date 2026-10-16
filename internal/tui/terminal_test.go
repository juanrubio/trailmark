package tui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func TestTerminalEnvironment(t *testing.T) {
	original := []string{"TERM=xterm-kitty", "COLORTERM=truecolor", "KITTY_WINDOW_ID=1"}
	got := terminalEnvironment(original)
	if got[0] != "TERM=xterm-256color" || got[1] != original[1] || got[2] != original[2] || original[0] != "TERM=xterm-kitty" {
		t.Fatalf("environment override changed unrelated or original values: %v, %v", got, original)
	}
	for _, term := range []string{"xterm-256color", "screen", "tmux-256color", "dumb"} {
		if got := terminalEnvironment([]string{"TERM=" + term}); got[0] != "TERM="+term {
			t.Fatalf("changed %s", term)
		}
	}
}

// Exercise the actual renderer: View width checks cannot catch unsupported
// control sequences emitted after layout, such as REP on older Kitty releases.
func TestSelectedMetadataTerminalRendering(t *testing.T) {
	m := testModel(t)
	m.noColor = false
	c, _ := m.selected()
	c.row.Partial, c.childCount, c.row.LastActivity = true, 2, &testNow
	rows := m.cardLines(c, true, 62)
	if !strings.Contains(ansi.Strip(rows[2]), "2 child sessions (e expand)") || !strings.Contains(ansi.Strip(rows[2]), "just now") {
		t.Fatal("fixture is missing long metadata")
	}
	content := strings.Join(rows, "\n")
	env := []string{"TERM=xterm-kitty", "COLORTERM=truecolor"}
	render := func(env []string) string {
		var output bytes.Buffer
		r := uv.NewTerminalRenderer(&output, env)
		r.SetColorProfile(colorprofile.TrueColor)
		r.SetFullscreen(true)
		r.Resize(62, 3)
		screen := uv.NewRenderBuffer(62, 3)
		uv.NewStyledString(content).Draw(terminalTestScreen{screen}, uv.Rect(0, 0, 62, 3))
		r.Render(screen)
		if err := r.Flush(); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}
	rep := regexp.MustCompile("\x1b\\[[0-9]*b")
	if !rep.MatchString(render(env)) {
		t.Fatalf("fixture no longer exercises REP optimization: %q", render(env))
	}
	if got := render(terminalEnvironment(env)); rep.MatchString(got) {
		t.Fatalf("unsupported REP emitted: %q", got)
	}
}

type terminalTestScreen struct{ *uv.RenderBuffer }

func (terminalTestScreen) WidthMethod() uv.WidthMethod { return ansi.GraphemeWidth }
