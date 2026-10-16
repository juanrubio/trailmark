// Package codex parses native Codex CLI session logs.
//
// Verified against the local store on 2026-10-15: logs live at
// ~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<id>.jsonl. The
// first record is session_meta with id, cwd, and timestamps; later
// response_item records carry messages and tool calls. Thread names
// from ~/.codex/session_index.jsonl provide native titles.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/model"
)

// Adapter parses Codex rollout JSONL logs.
type Adapter struct {
	ThreadNames map[string]string // native session ID -> thread name
}

func (Adapter) Provider() model.Provider { return model.ProviderCodex }

// Capability records the contract verified from `codex resume --help`
// and `codex fork --help`: both take a session ID argument, and a
// clean session is codex launched with no resume or fork command.
func (Adapter) Capability() model.Capability {
	return model.Capability{
		Provider:       model.ProviderCodex,
		ResumeTemplate: "codex resume {id}",
		ForkTemplate:   "codex fork {id}",
		ForkSupported:  true,
		CleanTemplate:  "codex",
		Notes:          "Run in the recorded cwd. Templates verified against local codex --help on 2026-10-15; never executed by Trailmark.",
	}
}

// LoadThreadNames reads a Codex session_index.jsonl into a title map.
// A missing index is not an error; titles fall back to excerpts.
func LoadThreadNames(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open codex session index %s: %w", path, err)
	}
	defer f.Close()
	names := map[string]string{}
	err = adapter.ScanLines(f, func(line []byte, _ int) error {
		var row struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if err := json.Unmarshal(line, &row); err == nil && row.ID != "" && row.ThreadName != "" {
			names[row.ID] = row.ThreadName
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read codex session index %s: %w", path, err)
	}
	return names, nil
}

type record struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	ID           string `json:"id"`
	Timestamp    string `json:"timestamp"`
	CWD          string `json:"cwd"`
	ForkedFromID string `json:"forked_from_id"`
	ParentID     string `json:"parent_thread_id"`
}

type responseItem struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Name      string          `json:"name"`
	CallID    string          `json:"call_id"`
	Input     string          `json:"input"`
	Arguments string          `json:"arguments"`
	Output    string          `json:"output"`
	Content   json.RawMessage `json:"content"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseFile parses one Codex rollout log with the same tolerance
// contract as the Claude Code adapter.
func (a Adapter) ParseFile(ctx context.Context, path string) (*model.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open codex log %s: %w", path, err)
	}
	defer f.Close()

	s := &model.Session{
		Provider:      model.ProviderCodex,
		SourcePath:    path,
		Store:         storeOf(path),
		PurposeOrigin: model.OriginUnavailable,
	}
	seq := 0
	metadata := metadataReader{}
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
		ts := adapter.ParseTime(rec.Timestamp)
		if ts != nil && rec.Type == "response_item" {
			s.LastActivityAt = ts
		}
		metadata.read(s, rec)
		loc := model.Locator{SourcePath: path, Line: lineNo}
		switch rec.Type {
		case "session_meta":
			var meta sessionMeta
			if err := json.Unmarshal(rec.Payload, &meta); err != nil {
				s.ParseErrors++
				s.Partial = true
				return nil
			}
			s.ID = meta.ID
			s.CWD = meta.CWD
			s.ForkedFromID = meta.ForkedFromID
			s.ParentID = meta.ParentID
			if t := adapter.ParseTime(meta.Timestamp); t != nil {
				s.StartedAt = t
				if s.LastActivityAt == nil {
					s.LastActivityAt = t
				}
			}
		case "response_item":
			msgs, ok := convertItem(rec, loc, ts, &seq)
			if !ok {
				s.ParseErrors++
				s.Partial = true
			}
			s.Messages = append(s.Messages, msgs...)
		case "turn_context":
			if s.CWD == "" {
				var tc struct {
					CWD string `json:"cwd"`
				}
				if err := json.Unmarshal(rec.Payload, &tc); err == nil {
					s.CWD = tc.CWD
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse codex log %s: %w", path, err)
	}
	if name, ok := a.ThreadNames[s.ID]; ok && name != "" {
		s.Title = name
	}
	adapter.DerivePurpose(s)
	return s, nil
}

func storeOf(path string) string {
	return filepath.Dir(path)
}

func convertItem(rec record, loc model.Locator, ts *time.Time, seq *int) ([]model.Message, bool) {
	var item responseItem
	if err := json.Unmarshal(rec.Payload, &item); err != nil {
		return nil, false
	}
	recordID := item.CallID
	if recordID == "" {
		recordID = item.ID
	}
	switch item.Type {
	case "message":
		role := item.Role
		if role != "user" && role != "assistant" {
			role = "system"
		}
		text := joinParts(item.Content)
		if strings.TrimSpace(text) == "" {
			return nil, true
		}
		*seq++
		loc.RecordID = recordID
		return []model.Message{{Seq: *seq, Role: role, Kind: "message", Text: text, Timestamp: ts, Locator: loc}}, true
	case "function_call", "custom_tool_call":
		*seq++
		loc.RecordID = recordID
		arguments := item.Arguments
		if item.Type == "custom_tool_call" {
			arguments = item.Input
		}
		callText := strings.TrimSpace(item.Name + " " + arguments)
		return []model.Message{{Seq: *seq, Role: "tool", Kind: "tool_call", Text: callText, Timestamp: ts, Locator: loc}}, true
	case "function_call_output", "custom_tool_call_output":
		*seq++
		loc.RecordID = recordID
		return []model.Message{{Seq: *seq, Role: "tool", Kind: "tool_output", Text: item.Output, Timestamp: ts, Locator: loc}}, true
	}
	return nil, true
}

func joinParts(raw json.RawMessage) string {
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}
