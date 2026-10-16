package claudecode_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/adapter/claudecode"
	"github.com/juanrubio/trailmark/internal/model"
)

func metadataSession(t *testing.T, body string) *model.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.jsonl")
	os.WriteFile(path, []byte(body), 0600)
	s, err := (claudecode.Adapter{}).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestClaudeMetadataDeduplicatesResponsesAndPreservesUnknowns(t *testing.T) {
	s := metadataSession(t, `{"type":"assistant","sessionId":"s","gitBranch":"feature","perTurnEffort":"high","requestId":"request1","message":{"id":"m1","model":"model-one","role":"assistant","content":"first","stop_reason":"tool_use","usage":{"input_tokens":10,"cache_read_input_tokens":2,"cache_creation_input_tokens":0,"output_tokens":1,"output_tokens_details":{"thinking_tokens":0}}}}
{"type":"assistant","sessionId":"s","requestId":"request1","message":{"id":"m1","model":"model-one","role":"assistant","content":"continued","stop_reason":"end_turn","usage":{"input_tokens":10,"cache_read_input_tokens":2,"cache_creation_input_tokens":0,"output_tokens":5,"output_tokens_details":{"thinking_tokens":1}}}}
{"type":"assistant","sessionId":"s","effort":"low","requestId":"request1","message":{"id":"m2","model":"model-two","role":"assistant","content":"second","stop_reason":"end_turn","usage":{"input_tokens":3,"cache_read_input_tokens":0,"cache_creation_input_tokens":4,"output_tokens":2,"output_tokens_details":{"thinking_tokens":0}}}}
{"type":"system","sessionId":"s","subtype":"turn_duration","durationMs":900}
`)
	m := s.Metadata
	if m.Tokens.Input == nil || *m.Tokens.Input != 19 || *m.Tokens.UncachedInput != 13 || *m.Tokens.CacheRead != 2 || *m.Tokens.CacheWrite != 4 || *m.Tokens.Output != 7 || *m.Tokens.Reasoning != 1 {
		t.Fatalf("duplicate responses summed or usage fields lost: %+v", m.Tokens)
	}
	if len(m.Models) != 2 || len(m.ReasoningEfforts) != 2 || m.Git.Branch != "feature" || m.Outcome.Status != "completed" || m.Outcome.DurationMS == nil || *m.Outcome.DurationMS != 900 {
		t.Fatal("native model/git/effort/outcome missing")
	}
	if m.Git.Commit != "" || m.Work.CommandCount != nil {
		t.Fatal("unsupported metadata inferred")
	}
}
func TestClaudeOutcomeAndUsageUnknowns(t *testing.T) {
	for _, tc := range []struct{ stop, want string }{{"tool_use", "unfinished"}, {"max_tokens", "interrupted"}, {"end_turn", "completed"}} {
		s := metadataSession(t, `{"type":"assistant","sessionId":"s","message":{"id":"m","content":"synthetic","stop_reason":"`+tc.stop+`","usage":{"input_tokens":0,"output_tokens":0}}}`)
		if s.Metadata.Outcome.Status != tc.want || s.Metadata.Tokens.Input != nil || s.Metadata.Tokens.UncachedInput == nil || *s.Metadata.Tokens.UncachedInput != 0 || s.Metadata.Tokens.CacheRead != nil || s.Metadata.Tokens.Output == nil || *s.Metadata.Tokens.Output != 0 {
			t.Fatal("outcome or absent-vs-zero semantics lost")
		}
	}
	s := metadataSession(t, `{"type":"assistant","sessionId":"s","message":{"content":"synthetic","usage":{"input_tokens":2}}}`)
	if s.Metadata.Tokens.Input != nil || !strings.Contains(s.Metadata.Tokens.Scope, "identity") {
		t.Fatal("unidentified usage silently counted")
	}
	s = metadataSession(t, `{"type":"assistant","sessionId":"s","message":{"content":"done","stop_reason":"end_turn"}}`+"\n"+`{"type":"user","sessionId":"s","message":{"content":"next turn"}}`)
	if s.Metadata.Outcome.Status != "unfinished" {
		t.Fatal("stale completed outcome survived new turn")
	}
}

func TestMalformedClaudeOptionalUsageKeepsConversation(t *testing.T) {
	s := metadataSession(t, `{"type":"assistant","sessionId":"s","message":{"id":"m","role":"assistant","content":"keep this conversation","usage":{"input_tokens":"future-shape","output_tokens":null}}}`)
	if s.Partial || len(s.Messages) != 1 || s.Metadata.Tokens.UncachedInput != nil || s.Metadata.Tokens.Output != nil {
		t.Fatal("malformed optional metadata damaged conversation or invented zero")
	}
}
