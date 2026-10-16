package codex_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/adapter/codex"
	"github.com/juanrubio/trailmark/internal/model"
)

func metadataSession(t *testing.T, body string) *model.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.jsonl")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := (codex.Adapter{}).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestRecordedMetadataSnapshotsWorkAndCustomInput(t *testing.T) {
	s := metadataSession(t, `{"type":"session_meta","payload":{"id":"s","forked_from_id":"parent","git":{"branch":"feature","commit_hash":"abc123","repository_url":"https://example.invalid/repo"}}}
{"type":"turn_context","payload":{"model":"model-one","effort":"high"}}
{"type":"turn_context","payload":{"model":"model-two","effort":"low"}}
{"type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch\n*** End Patch"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":10,"reasoning_output_tokens":5}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":10,"reasoning_output_tokens":5}}}}
{"type":"token_usage_record","payload":{"thread_token_usage":{"input_tokens":200,"cached_input_tokens":30,"cache_write_input_tokens":0,"output_tokens":25,"reasoning_output_tokens":8},"turn_token_usage":{"input_tokens":999}}}
{"type":"event_msg","payload":{"type":"item_completed","item":{"id":"cmd1","type":"CommandExecution","command":["go","test","./..."],"status":"Completed","exit_code":0}}}
{"type":"event_msg","payload":{"type":"item_completed","item":{"id":"cmd1","type":"CommandExecution","command":["go","test","./..."],"status":"Completed","exit_code":0}}}
{"type":"response_item","payload":{"type":"function_call_output","output":"duplicate command output"}}
{"type":"event_msg","payload":{"type":"item_completed","item":{"id":"cmd2","type":"CommandExecution","command":["false"],"status":"Failed","exit_code":-1}}}
{"type":"event_msg","payload":{"type":"item_completed","item":{"id":"file1","type":"FileChange","status":"Completed","changes":{"src/a.go":{"type":"add","content":"synthetic"},"src/b.go":{"type":"update"}}}}}
{"type":"event_msg","payload":{"type":"item_completed","item":{"id":"file2","type":"FileChange","status":"Failed","changes":{"not-changed.go":{"type":"add"}}}}}
{"type":"event_msg","payload":{"type":"task_complete","duration_ms":1234}}
`)
	m := s.Metadata
	if len(m.Models) != 2 || len(m.ReasoningEfforts) != 2 || m.Git.Branch != "feature" || m.Git.Commit != "abc123" || m.Git.Repository == "" {
		t.Fatalf("missing native metadata: %+v", m)
	}
	if len(s.Messages) != 2 || !strings.Contains(s.Messages[0].Text, "*** Begin Patch") {
		t.Fatal("custom tool input omitted or work events duplicated into transcript")
	}
	if m.Tokens.Input == nil || *m.Tokens.Input != 200 || m.Tokens.CacheWrite == nil || *m.Tokens.CacheWrite != 0 || !strings.Contains(m.Tokens.Scope, "inherited") {
		t.Fatal("cumulative snapshots double-counted or known zero lost")
	}
	if m.Work.CommandCount == nil || *m.Work.CommandCount != 2 || m.Work.FailedCommands == nil || *m.Work.FailedCommands != 1 || m.Work.LatestExitCode == nil || *m.Work.LatestExitCode != -1 || len(m.Work.ChangedFiles) != 2 {
		t.Fatalf("bad structured work summary: %+v", m.Work)
	}
	if m.Outcome.Status != "completed" || m.Outcome.DurationMS == nil || *m.Outcome.DurationMS != 1234 {
		t.Fatal("missing recorded outcome")
	}
}
func TestOutcomeLatestTurnUnknownsAndBoundedWork(t *testing.T) {
	for _, tc := range []struct{ event, want string }{{`{"type":"task_started"}`, "unfinished"}, {`{"type":"turn_aborted","duration_ms":0}`, "interrupted"}, {`{"type":"task_complete","error":"recorded failure"}`, "error"}} {
		s := metadataSession(t, `{"type":"event_msg","payload":{"type":"task_complete","duration_ms":50}}`+"\n"+`{"type":"event_msg","payload":`+tc.event+`}`)
		if s.Metadata.Outcome.Status != tc.want {
			t.Fatalf("latest outcome=%s want%s", s.Metadata.Outcome.Status, tc.want)
		}
		if tc.want == "unfinished" && s.Metadata.Outcome.DurationMS != nil {
			t.Fatal("stale duration retained")
		}
	}
	changes := map[string]any{}
	for i := 0; i < 101; i++ {
		changes[fmt.Sprintf("file-%03d", i)] = map[string]string{"type": "add"}
	}
	payload, _ := json.Marshal(map[string]any{"type": "event_msg", "payload": map[string]any{"type": "item_completed", "item": map[string]any{"type": "FileChange", "id": "cap", "status": "Completed", "changes": changes}}})
	s := metadataSession(t, string(payload)+"\n"+`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"unknown","command":"echo synthetic","status":"Completed"}}}`)
	if len(s.Metadata.Work.ChangedFiles) != 100 || !s.Metadata.Work.Truncated || s.Metadata.Work.FailedCommands != nil || s.Metadata.Work.CommandCount == nil {
		t.Fatal("work bounds or unknown command outcome lost")
	}
	if s.Metadata.Tokens.Input != nil || s.Partial {
		t.Fatal("missing optional metadata became zero/parse error")
	}
}

func TestMalformedOptionalMetadataKeepsTranscript(t *testing.T) {
	s := metadataSession(t, `{"type":"turn_context","payload":{"model":{"future":"shape"},"effort":12}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":"unknown","output_tokens":null}}}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"keep this conversation"}]}}
`)
	if s.Partial || len(s.Messages) != 1 || s.Metadata.Tokens.Input != nil || s.Metadata.Tokens.Output != nil {
		t.Fatal("malformed optional metadata damaged conversation or invented zero")
	}
}
