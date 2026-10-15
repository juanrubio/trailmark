// Package adapter defines the read-only provider adapter contract.
// Adapters parse one native log file into the normalized model and must
// tolerate partial writes, truncated records, and unknown fields.
package adapter

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/juanrubio/trailmark/internal/model"
)

// Adapter parses the native logs of one provider.
type Adapter interface {
	Provider() model.Provider
	ParseFile(ctx context.Context, path string) (*model.Session, error)
	Capability() model.Capability
}

// ScanLines calls fn for each JSONL line with its 1-based line number.
// Malformed-line tolerance is the caller's job; this helper only splits.
func ScanLines(r io.Reader, fn func(line []byte, lineNo int) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if err := fn(sc.Bytes(), lineNo); err != nil {
			return err
		}
	}
	return sc.Err()
}

const maxPurposeRunes = 140

// Excerpt returns a single-line, rune-safe excerpt of text.
func Excerpt(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= maxPurposeRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxPurposeRunes-1]) + "…"
}

// Substantive reports whether a user text can serve as a purpose or
// latest-exchange excerpt. Empty text and recognized injected setup
// blocks are not substantive.
func Substantive(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	for _, marker := range []string{
		"<system-reminder", "<command-name>", "<local-command",
		"<environment_context", "<instructions", "<user_instructions",
		"# agents.md instructions", "agents.md instructions",
		"caveat: the messages below",
	} {
		if strings.HasPrefix(lower, marker) {
			return false
		}
	}
	return true
}

// DerivePurpose fills Purpose and PurposeOrigin from the native title
// or the first substantive user message.
func DerivePurpose(s *model.Session) {
	if s.Title != "" {
		s.Purpose = s.Title
		s.PurposeOrigin = model.OriginNativeTitle
		return
	}
	for _, m := range s.Messages {
		if m.Role == "user" && m.Kind == "message" && Substantive(m.Text) {
			s.Purpose = Excerpt(m.Text)
			s.PurposeOrigin = model.OriginRequestExcerpt
			return
		}
	}
	s.Purpose = ""
	s.PurposeOrigin = model.OriginUnavailable
}

// ParseTime parses an RFC 3339 timestamp, returning nil when absent
// or unparseable. Unknown timestamp formats must not fail a parse.
func ParseTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z0700"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	return nil
}
