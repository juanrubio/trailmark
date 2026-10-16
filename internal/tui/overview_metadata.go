package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/juanrubio/trailmark/internal/model"
)

func recordedText(s string) string {
	if s == "" {
		return "Unavailable"
	}
	return s
}
func recordedCount(n *int64) string {
	if n == nil {
		return "Unavailable"
	}
	digits := fmt.Sprint(*n)
	start := 0
	if strings.HasPrefix(digits, "-") {
		start = 1
	}
	for i := len(digits) - 3; i > start; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func overviewMetadataLines(meta model.SessionMetadata, _ model.Provider) []string {
	status := "Unavailable"
	switch meta.Outcome.Status {
	case "completed":
		status = "Completed turn (task success unverified)"
	case "unfinished":
		status = "No completion recorded"
	case "interrupted":
		status = "Interrupted"
	case "error":
		status = "Error"
	}
	duration := "Unavailable"
	if meta.Outcome.DurationMS != nil {
		duration = (time.Duration(*meta.Outcome.DurationMS) * time.Millisecond).String()
	}
	lines := []string{
		"Model: " + recordedText(strings.Join(meta.Models, ", ")),
		"Reasoning effort: " + recordedText(strings.Join(meta.ReasoningEfforts, ", ")),
		"Recorded git branch: " + recordedText(meta.Git.Branch),
		"Recorded git commit: " + recordedText(meta.Git.Commit),
		"Recorded git repository: " + recordedText(meta.Git.Repository),
		"Latest turn outcome: " + status,
		"Recorded turn duration: " + duration,
		"Tokens input: " + recordedCount(meta.Tokens.Input) + " · output: " + recordedCount(meta.Tokens.Output),
		"Tokens cache read (input subset): " + recordedCount(meta.Tokens.CacheRead) + " · cache write (input subset): " + recordedCount(meta.Tokens.CacheWrite),
		"Tokens reasoning (output subset): " + recordedCount(meta.Tokens.Reasoning),
		"Token scope: " + recordedText(meta.Tokens.Scope),
	}
	if meta.Tokens.UncachedInput != nil {
		lines = append(lines, "Tokens uncached input: "+recordedCount(meta.Tokens.UncachedInput))
	}
	return lines
}

func overviewWorkLines(work model.WorkSummary) []string {
	lines := []string{"Recorded command results: " + recordedCount(work.CommandCount) + " · failed: " + recordedCount(work.FailedCommands)}
	lines = append(lines, "Latest command: "+recordedText(work.LatestCommand))
	exit := "Unavailable"
	if work.LatestExitCode != nil {
		exit = fmt.Sprint(*work.LatestExitCode)
	}
	lines = append(lines, "Latest command status: "+recordedText(work.LatestCommandStatus)+" · exit code: "+exit)
	if work.Truncated {
		lines = append(lines, "Work summary truncated to recorded limits")
	}
	if len(work.ChangedFiles) == 0 {
		return append(lines, "Files changed: Unavailable")
	}
	lines = append(lines, "Recorded files changed (tool evidence):")
	for _, file := range work.ChangedFiles {
		lines = append(lines, file)
	}
	return lines
}

// Style values before wrapping so search retains the same plain source text.
func (m *appModel) styleRecordedLine(text string, work bool) string {
	styles := m.styles()
	if strings.HasPrefix(text, "Token scope:") {
		return styles.muted.Render(text)
	}
	label, value, ok := strings.Cut(text, ": ")
	if !ok {
		if work && text != "Recorded files changed (tool evidence):" && text != "Work summary truncated to recorded limits" {
			return styles.path.Render(text)
		}
		return styles.muted.Render(text)
	}
	valueStyle := styles.base
	switch {
	case strings.HasPrefix(label, "Recorded git"), label == "Files changed":
		valueStyle = styles.path
	case label == "Model", label == "Reasoning effort":
		valueStyle = styles.accent
	}
	if value == "Unavailable" {
		valueStyle = styles.muted
	}
	return styles.muted.Render(label+": ") + valueStyle.Render(value)
}
