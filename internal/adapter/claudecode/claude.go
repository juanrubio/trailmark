// Package claudecode parses native Claude Code session logs.
//
// Verified against the local store on 2026-10-15: logs live at
// ~/.claude/projects/<encoded-cwd>/<session-id>.jsonl. Records are
// JSONL objects with type user/assistant/attachment/system and shared
// sessionId, cwd, timestamp, uuid, and parentUuid fields. Message
// content is either a string or an array of typed blocks.
package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/model"
)

// Adapter parses Claude Code JSONL session logs.
type Adapter struct{}

func (Adapter) Provider() model.Provider { return model.ProviderClaudeCode }

// Capability records the contract verified from `claude --help`:
// resume via --resume <id>, fork via --resume <id> --fork-session,
// and a clean session by launching claude with no resume flags.
func (Adapter) Capability() model.Capability {
	return model.Capability{
		Provider:       model.ProviderClaudeCode,
		ResumeTemplate: "claude --resume {id}",
		ForkTemplate:   "claude --resume {id} --fork-session",
		ForkSupported:  true,
		CleanTemplate:  "claude",
		Notes:          "Run in the recorded cwd. Templates verified against local claude --help on 2026-10-15; never executed by Trailmark.",
	}
}

type record struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	UUID        string          `json:"uuid"`
	IsSidechain bool            `json:"isSidechain"`
	IsMeta      bool            `json:"isMeta"`
	Summary     string          `json:"summary"`
	Message     json.RawMessage `json:"message"`
}

type messageBody struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
}

// ParseFile parses one Claude Code session log. Malformed lines are
// counted and skipped; a truncated final line marks the session partial.
func (a Adapter) ParseFile(ctx context.Context, path string) (*model.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open claude log %s: %w", path, err)
	}
	defer f.Close()

	s := &model.Session{
		Provider:      model.ProviderClaudeCode,
		SourcePath:    path,
		Store:         filepath.Dir(path),
		PurposeOrigin: model.OriginUnavailable,
	}
	seq := 0
	validRecords, nativeEvidence := 0, false
	err = adapter.ScanLines(f, func(line []byte, lineNo int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(strings.TrimSpace(string(line))) == 0 {
			return nil
		}
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			s.ParseErrors++
			s.Partial = true
			return nil
		}
		validRecords++
		if rec.SessionID != "" {
			nativeEvidence = true
		}
		switch rec.Type {
		case "user", "assistant", "system", "attachment", "summary", "progress", "file-history-snapshot", "queue-operation", "last-prompt", "custom-title":
			nativeEvidence = true
		}
		if s.ID == "" {
			s.ID = rec.SessionID
		}
		if s.CWD == "" {
			s.CWD = rec.CWD
		}
		if rec.IsSidechain {
			s.Sidechain = true
		}
		ts := adapter.ParseTime(rec.Timestamp)
		if ts != nil {
			if s.StartedAt == nil || ts.Before(*s.StartedAt) {
				s.StartedAt = ts
			}
			if s.LastActivityAt == nil || ts.After(*s.LastActivityAt) {
				s.LastActivityAt = ts
			}
		}
		switch rec.Type {
		case "summary":
			if rec.Summary != "" {
				s.Title = rec.Summary
			}
		case "user", "assistant":
			if rec.IsMeta {
				return nil // injected/meta records are not conversation
			}
			msgs, ok := convertMessage(rec, lineNo, path, &seq)
			if !ok {
				s.ParseErrors++
				s.Partial = true
			}
			s.Messages = append(s.Messages, msgs...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse claude log %s: %w", path, err)
	}
	// Only reject confirmed unrelated JSONL. Empty files and files containing
	// malformed records remain eligible partial sessions, even without an ID yet.
	if validRecords > 0 && !nativeEvidence && s.ParseErrors == 0 {
		return nil, adapter.ErrNotSession
	}
	adapter.DerivePurpose(s)
	return s, nil
}

func convertMessage(rec record, lineNo int, path string, seq *int) ([]model.Message, bool) {
	var body messageBody
	if err := json.Unmarshal(rec.Message, &body); err != nil {
		return nil, false
	}
	role := body.Role
	if role == "" {
		role = rec.Type
	}
	loc := model.Locator{SourcePath: path, Line: lineNo, RecordID: rec.UUID}
	ts := adapter.ParseTime(rec.Timestamp)

	// String content is a single message.
	var text string
	if err := json.Unmarshal(body.Content, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, true
		}
		*seq++
		return []model.Message{{Seq: *seq, Role: role, Kind: "message", Text: text, Timestamp: ts, Locator: loc}}, true
	}
	var blocks []contentBlock
	if err := json.Unmarshal(body.Content, &blocks); err != nil {
		return nil, false
	}
	var out []model.Message
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				*seq++
				out = append(out, model.Message{Seq: *seq, Role: role, Kind: "message", Text: b.Text, Timestamp: ts, Locator: loc})
			}
		case "tool_use":
			*seq++
			callText := b.Name
			if len(b.Input) > 0 {
				callText = strings.TrimSpace(b.Name + " " + string(b.Input))
			}
			out = append(out, model.Message{Seq: *seq, Role: "tool", Kind: "tool_call", Text: callText, Timestamp: ts, Locator: loc})
		case "tool_result":
			*seq++
			out = append(out, model.Message{Seq: *seq, Role: "tool", Kind: "tool_output", Text: blockText(b.Content), Timestamp: ts, Locator: loc})
		}
	}
	return out, true
}

func blockText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
