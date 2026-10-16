package claudecode

import (
	"encoding/json"
	"math"

	"github.com/juanrubio/trailmark/internal/model"
)

type metadataReader struct {
	usage             map[string]map[string]json.RawMessage
	unidentifiedUsage bool
}

func metadataObject(raw json.RawMessage) map[string]json.RawMessage {
	var value map[string]json.RawMessage
	_ = json.Unmarshal(raw, &value)
	return value
}
func metadataString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}
func metadataNumber(raw json.RawMessage) *int64 {
	var value int64
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil || value < 0 {
		return nil
	}
	return &value
}
func metadataUnique(values *[]string, value string) {
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
func (r *metadataReader) read(s *model.Session, line []byte, rec record) {
	p := metadataObject(line)
	message := metadataObject(rec.Message)
	if branch := metadataString(p["gitBranch"]); branch != "" {
		s.Metadata.Git.Branch = branch
	}
	for _, raw := range []json.RawMessage{p["effort"], p["reasoning_effort"], p["perTurnEffort"], message["effort"], message["reasoning_effort"]} {
		metadataUnique(&s.Metadata.ReasoningEfforts, metadataString(raw))
	}
	if rec.IsMeta {
		return
	}
	if rec.Type == "user" && !rec.IsMeta {
		s.Metadata.Outcome = model.TurnOutcome{Status: "unfinished"}
	}
	if rec.Type == "assistant" {
		s.Metadata.Outcome = model.TurnOutcome{Status: "unfinished"}
		metadataUnique(&s.Metadata.Models, metadataString(message["model"]))
		key := metadataString(message["id"])
		if key == "" {
			key = metadataString(p["requestId"])
			if key == "" {
				key = metadataString(p["request_id"])
			}
		}
		if key == "" && len(metadataObject(message["usage"])) > 0 {
			r.unidentifiedUsage = true
		}
		if key != "" {
			if r.usage == nil {
				r.usage = map[string]map[string]json.RawMessage{}
			}
			usage := metadataObject(message["usage"])
			// Later records of the same streaming response carry final usage. Merge
			// only present fields; repeated snapshots are replacements, never sums.
			previous := r.usage[key]
			if previous == nil {
				previous = map[string]json.RawMessage{}
			}
			details := metadataObject(usage["output_tokens_details"])
			if _, present := usage["reasoning_output_tokens"]; !present && len(details["thinking_tokens"]) > 0 {
				usage["reasoning_output_tokens"] = details["thinking_tokens"]
			}
			for field, value := range usage {
				previous[field] = value
			}
			r.usage[key] = previous
		}
		switch metadataString(message["stop_reason"]) {
		case "end_turn", "stop_sequence":
			s.Metadata.Outcome = model.TurnOutcome{Status: "completed"}
		case "max_tokens":
			s.Metadata.Outcome = model.TurnOutcome{Status: "interrupted"}
		case "tool_use", "pause_turn":
			s.Metadata.Outcome = model.TurnOutcome{Status: "unfinished"}
		}
		if raw := p["error"]; len(raw) > 0 && string(raw) != "null" && string(raw) != `""` && string(raw) != "false" {
			s.Metadata.Outcome = model.TurnOutcome{Status: "error"}
		}
	}
	if rec.Type == "system" && metadataString(p["subtype"]) == "turn_duration" {
		s.Metadata.Outcome.DurationMS = metadataNumber(p["durationMs"])
	}
	if rec.Type == "system" && metadataString(p["subtype"]) == "api_error" {
		s.Metadata.Outcome = model.TurnOutcome{Status: "error"}
	}
}
func (r *metadataReader) finish(s *model.Session) {
	if r.unidentifiedUsage {
		s.Metadata.Tokens = model.TokenUsage{Scope: "Recorded usage lacks response identity; complete totals unknown"}
		return
	}
	if len(r.usage) == 0 {
		return
	}
	sum := func(field string) *int64 {
		total := int64(0)
		for _, usage := range r.usage {
			n := metadataNumber(usage[field])
			if n == nil {
				return nil
			}
			if *n > math.MaxInt64-total {
				return nil
			}
			total += *n
		}
		return &total
	}
	usage := model.TokenUsage{UncachedInput: sum("input_tokens"), CacheRead: sum("cache_read_input_tokens"), CacheWrite: sum("cache_creation_input_tokens"), Output: sum("output_tokens"), Reasoning: sum("reasoning_output_tokens"), Scope: "Recorded unique assistant-message totals; input includes cache read/write when all components recorded"}
	if usage.UncachedInput != nil && usage.CacheRead != nil && usage.CacheWrite != nil {
		if *usage.CacheRead <= math.MaxInt64-*usage.UncachedInput && *usage.CacheWrite <= math.MaxInt64-*usage.UncachedInput-*usage.CacheRead {
			total := *usage.UncachedInput + *usage.CacheRead + *usage.CacheWrite
			usage.Input = &total
		}
	}
	s.Metadata.Tokens = usage
}
