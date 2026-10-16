package codex

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/juanrubio/trailmark/internal/model"
)

type metadataReader struct {
	seenItems             map[string]bool
	commandFailures       int64
	unknownCommandOutcome bool
}

func addUnique(values *[]string, value string) {
	if value == "" {
		return
	}
	for _, v := range *values {
		if v == value {
			return
		}
	}
	*values = append(*values, value)
}
func number(raw json.RawMessage) *int64 {
	var value int64
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil || value < 0 {
		return nil
	}
	return &value
}
func object(raw json.RawMessage) map[string]json.RawMessage {
	var result map[string]json.RawMessage
	_ = json.Unmarshal(raw, &result)
	return result
}
func str(raw json.RawMessage) string { var s string; _ = json.Unmarshal(raw, &s); return s }
func (r *metadataReader) read(s *model.Session, rec record) {
	p := object(rec.Payload)
	kind := str(p["type"])
	switch rec.Type {
	case "session_meta":
		git := object(p["git"])
		s.Metadata.Git = model.GitMetadata{Branch: str(git["branch"]), Commit: str(git["commit_hash"]), Repository: str(git["repository_url"])}
	case "turn_context":
		addUnique(&s.Metadata.Models, str(p["model"]))
		effort := str(p["effort"])
		if effort == "" {
			effort = str(p["reasoning_effort"])
		}
		addUnique(&s.Metadata.ReasoningEfforts, effort)
	}
	if rec.Type != "event_msg" && rec.Type != "token_usage_record" {
		return
	}
	if rec.Type == "token_usage_record" {
		kind = "token_usage_record"
	}
	switch kind {
	case "token_count", "token_usage_record":
		usage := object(p["thread_token_usage"])
		if kind == "token_count" {
			usage = object(object(p["info"])["total_token_usage"])
		}
		if len(usage) > 0 {
			s.Metadata.Tokens = model.TokenUsage{Input: number(usage["input_tokens"]), CacheRead: number(usage["cached_input_tokens"]), CacheWrite: number(usage["cache_write_input_tokens"]), Output: number(usage["output_tokens"]), Reasoning: number(usage["reasoning_output_tokens"]), Scope: "Recorded cumulative thread totals; input includes cache read/write; reasoning is part of output"}
			if s.ParentID != "" || s.ForkedFromID != "" {
				s.Metadata.Tokens.Scope += "; may include inherited parent/fork usage"
			}
		}
	case "task_started":
		s.Metadata.Outcome = model.TurnOutcome{Status: "unfinished"}
	case "task_complete", "turn_aborted", "turn_error", "error":
		status := "completed"
		if kind == "turn_aborted" {
			status = "interrupted"
		}
		if kind == "turn_error" || kind == "error" || str(p["reason"]) == "error" || (len(p["error"]) > 0 && string(p["error"]) != "null" && string(p["error"]) != "false" && string(p["error"]) != `""`) {
			status = "error"
		}
		s.Metadata.Outcome = model.TurnOutcome{Status: status, DurationMS: number(p["duration_ms"])}
	case "item_completed":
		item := object(p["item"])
		id := str(item["id"])
		if id != "" {
			id = str(item["type"]) + "/" + id
		}
		if id != "" {
			if r.seenItems[id] {
				return
			}
			if r.seenItems == nil {
				r.seenItems = map[string]bool{}
			}
			r.seenItems[id] = true
		}
		status := str(item["status"])
		switch str(item["type"]) {
		case "CommandExecution", "command_execution":
			w := &s.Metadata.Work
			if w.CommandCount == nil {
				v := int64(0)
				w.CommandCount = &v
			}
			*w.CommandCount++
			var signedExit int
			if json.Unmarshal(item["exit_code"], &signedExit) == nil && string(item["exit_code"]) != "null" {
				w.LatestExitCode = &signedExit
			} else {
				w.LatestExitCode = nil
			}
			failed := w.LatestExitCode != nil && *w.LatestExitCode != 0 || strings.EqualFold(status, "failed") || strings.EqualFold(status, "error")
			known := w.LatestExitCode != nil || failed || strings.EqualFold(status, "success") || strings.EqualFold(status, "succeeded")
			if failed {
				r.commandFailures++
			}
			if !known {
				r.unknownCommandOutcome = true
			}
			w.FailedCommands = nil
			if !r.unknownCommandOutcome {
				count := r.commandFailures
				w.FailedCommands = &count
			}
			command := str(item["command"])
			if command == "" {
				var argv []string
				if json.Unmarshal(item["command"], &argv) == nil {
					command = strings.Join(argv, " ")
				}
			}
			runes := []rune(command)
			if len(runes) > 1000 {
				command = string(runes[:1000])
				w.Truncated = true
			}
			w.LatestCommand = command
			w.LatestCommandStatus = status
		case "FileChange", "file_change":
			if !strings.EqualFold(status, "completed") && !strings.EqualFold(status, "success") && !strings.EqualFold(status, "succeeded") {
				return
			}
			changes := object(item["changes"])
			paths := make([]string, 0, len(changes))
			for path := range changes {
				paths = append(paths, path)
			}
			if len(paths) == 0 {
				var list []struct {
					Path string `json:"path"`
				}
				if json.Unmarshal(item["changes"], &list) == nil {
					for _, change := range list {
						paths = append(paths, change.Path)
					}
				}
			}
			sort.Strings(paths)
			for _, path := range paths {
				runes := []rune(path)
				if len(runes) > 1000 {
					path = string(runes[:1000])
					s.Metadata.Work.Truncated = true
				}
				duplicate := false
				for _, existing := range s.Metadata.Work.ChangedFiles {
					if existing == path {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				if len(s.Metadata.Work.ChangedFiles) >= 100 {
					s.Metadata.Work.Truncated = true
					break
				}
				addUnique(&s.Metadata.Work.ChangedFiles, path)
			}
		}
	}
}
