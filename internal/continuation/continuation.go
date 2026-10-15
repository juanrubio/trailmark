// Package continuation prepares native continuation commands for a
// selected session. Preparing and copying are the only operations:
// nothing in this package executes a prepared command, creates a
// directory, or modifies a source log.
package continuation

import (
	"os"
	"strings"

	"github.com/juanrubio/trailmark/internal/model"
)

// Action identifies one continuation choice.
type Action string

const (
	Resume Action = "resume"
	Fork   Action = "fork"
	Clean  Action = "clean"
)

// Prepared is one continuation sheet's content.
type Prepared struct {
	Action    Action
	Title     string // Resume session, Fork session, Clean session
	History   string // plain-language history behavior
	Command   string // empty when Available is false
	Available bool
	Reason    string // why the command cannot be prepared
	TargetDir string // recorded cwd, exactly as stored
	Purpose   string
	Provider  model.Provider
	SessionID string
	DirExists bool
}

// DirExists reports whether a directory currently exists. It is a
// variable so tests can simulate missing directories.
var DirExists = func(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Prepare builds the sheet content for one action and session. The
// target directory is always the session's recorded cwd; no other
// directory is ever substituted.
func Prepare(action Action, cap model.Capability, provider model.Provider, sessionID, cwd, purpose string) Prepared {
	p := Prepared{
		Action:    action,
		TargetDir: cwd,
		Purpose:   purpose,
		Provider:  provider,
		SessionID: sessionID,
	}
	switch action {
	case Resume:
		p.Title = "Resume session"
		p.History = "Continues the same native conversation with its existing history."
	case Fork:
		p.Title = "Fork session"
		p.History = "Creates a separate native conversation branched from this session's history. The original session is unchanged."
	case Clean:
		p.Title = "Clean session"
		p.History = "Starts a new conversation with no inherited history in the same directory. Normal provider and project configuration still applies; no transcript or copied context is attached."
	}
	if cwd == "" {
		p.Reason = "No working directory was recorded for this session, so no target directory can be shown or used."
		return p
	}
	p.DirExists = DirExists(cwd)
	if !p.DirExists {
		p.Reason = "The recorded directory no longer exists: " + cwd + ". Trailmark will not substitute another directory or create it."
		return p
	}
	var template string
	switch action {
	case Resume:
		template = cap.ResumeTemplate
	case Fork:
		if !cap.ForkSupported || cap.ForkTemplate == "" {
			p.Reason = "Fork unavailable for this provider: no validated native fork command is recorded."
			return p
		}
		template = cap.ForkTemplate
	case Clean:
		template = cap.CleanTemplate
	}
	if template == "" {
		p.Reason = "No validated native command is recorded for this action and provider."
		return p
	}
	if action != Clean && sessionID == "" {
		p.Reason = "The native session ID is unavailable in the source log, so this session cannot be targeted."
		return p
	}
	// Single-pass replacement: a crafted ID containing "{cwd}" must
	// stay literal, never be re-substituted with the directory.
	cmd := strings.NewReplacer("{id}", ShellQuote(sessionID), "{cwd}", ShellQuote(cwd)).Replace(template)
	p.Command = "cd -- " + ShellQuote(cwd) + " && " + cmd
	p.Available = true
	return p
}

// ShellQuote quotes one argument for a POSIX shell command line.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := !strings.HasPrefix(s, "-") // a leading dash must never read as a flag
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune("_@%+=:,./-", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
