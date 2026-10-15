// Package clipboard copies text to the system clipboard using the
// first available platform helper. It never invokes a shell.
package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrUnavailable reports that no clipboard helper could copy text.
var ErrUnavailable = errors.New("clipboard unavailable")

// Copier copies text to the clipboard.
type Copier interface {
	Copy(text string) error
}

// System is the production Copier.
type System struct{}

// helpers are tried in order; each receives the text on stdin.
var helpers = [][]string{
	{"wl-copy"},
	{"xclip", "-selection", "clipboard"},
	{"xsel", "--clipboard", "--input"},
	{"pbcopy"},
	{"clip.exe"},
}

// Copy runs the first available helper with the text on stdin. A
// helper that exists but fails (for example wl-copy without a
// Wayland session) falls through to the next one. Each attempt is
// time-bounded so a hanging helper cannot freeze the caller.
func (System) Copy(text string) error {
	var lastErr error
	for _, helper := range helpers {
		path, err := exec.LookPath(helper[0])
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd := exec.CommandContext(ctx, path, helper[1:]...)
		cmd.Stdin = strings.NewReader(text)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			lastErr = err
			cancel()
			continue
		}
		cancel()
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("%w: clipboard helpers failed: %v", ErrUnavailable, lastErr)
	}
	return fmt.Errorf("%w: no clipboard helper found (tried wl-copy, xclip, xsel, pbcopy, clip.exe)", ErrUnavailable)
}
