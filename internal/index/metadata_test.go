package index_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/adapter/codex"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
	"github.com/juanrubio/trailmark/internal/scan"
)

func TestOverviewMetadataRoundTripKeepsKnownZeroAndUnknown(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	zero := int64(0)
	input := int64(123)
	count := int64(2)
	exit := -1
	metadata := model.SessionMetadata{Models: []string{"one", "two"}, ReasoningEfforts: []string{"high"}, Git: model.GitMetadata{Branch: "feature", Commit: "abc", Repository: "https://example.invalid/repo"}, Tokens: model.TokenUsage{Input: &input, CacheRead: &zero, Scope: "recorded"}, Outcome: model.TurnOutcome{Status: "interrupted", DurationMS: &zero}, Work: model.WorkSummary{ChangedFiles: []string{"src/a.go"}, Truncated: true, CommandCount: &count, FailedCommands: &zero, LatestCommand: "go test", LatestExitCode: &exit, LatestCommandStatus: "failed"}}
	session := &model.Session{SourcePath: "/fictional/session", Provider: model.ProviderCodex, ID: "synthetic", Metadata: metadata}
	if err := ix.Upsert(ctx, session, 100, time.Now()); err != nil {
		t.Fatal(err)
	}
	overview, err := ix.Overview(ctx, session.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(overview.Metadata, metadata) {
		t.Fatalf("metadata changed in index roundtrip: %+v", overview.Metadata)
	}
	session.Metadata = model.SessionMetadata{}
	if err := ix.Upsert(ctx, session, 100, time.Now()); err != nil {
		t.Fatal(err)
	}
	overview, err = ix.Overview(ctx, session.SourcePath)
	if err != nil || !reflect.DeepEqual(overview.Metadata, model.SessionMetadata{}) {
		t.Fatal("stale metadata survived reparse without evidence")
	}
}

func TestMetadataMigrationPreservesRowsAndForcesReindex(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "native.jsonl")
	body := []byte(`{"type":"session_meta","payload":{"id":"native","cwd":"/fictional"}}` + "\n" + `{"type":"turn_context","payload":{"model":"recorded-model","effort":"high"}}` + "\n" + `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":7,"output_tokens":0}}}}`)
	if err := os.WriteFile(source, body, 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	dbpath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", dbpath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sessions (
 source_path TEXT PRIMARY KEY, provider TEXT NOT NULL,native_id TEXT NOT NULL DEFAULT '',store TEXT NOT NULL DEFAULT '',title TEXT NOT NULL DEFAULT '',purpose TEXT NOT NULL DEFAULT '',purpose_origin TEXT NOT NULL DEFAULT 'unavailable',cwd TEXT NOT NULL DEFAULT '',parent_id TEXT NOT NULL DEFAULT '',forked_from TEXT NOT NULL DEFAULT '',sidechain INTEGER NOT NULL DEFAULT 0,started_at INTEGER,last_activity INTEGER,partial INTEGER NOT NULL DEFAULT 0,parse_errors INTEGER NOT NULL DEFAULT 0,file_size INTEGER NOT NULL DEFAULT 0,file_mtime INTEGER NOT NULL DEFAULT 0,parser_version INTEGER NOT NULL DEFAULT 0)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO sessions (source_path,provider,native_id,file_size,file_mtime,parser_version) VALUES(?,?,?,?,?,?)`, source, model.ProviderCodex, "native", info.Size(), info.ModTime().UnixNano(), index.ParserVersion-1)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(dbpath)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	before, err := ix.Overview(ctx, source)
	if err != nil || before.Session.ID != "native" || !reflect.DeepEqual(before.Metadata, model.SessionMetadata{}) {
		t.Fatal("additive migration lost legacy row or invented metadata")
	}
	_, _, _, version, ok, err := ix.FileState(ctx, source)
	if err != nil || !ok || version == index.ParserVersion {
		t.Fatal("legacy fingerprint no longer signals required reindex")
	}
	scanner := &scan.Scanner{Index: ix, Config: &config.Config{DisableDefaultRoots: true, Roots: []config.Root{{Provider: model.ProviderCodex, Path: root}}}, Adapters: map[model.Provider]adapter.Adapter{model.ProviderCodex: codex.Adapter{}}}
	stats, err := scanner.Scan(ctx)
	if err != nil || stats.Indexed != 1 {
		t.Fatalf("unchanged old-parser log did not reindex: %+v %v", stats, err)
	}
	after, err := ix.Overview(ctx, source)
	if err != nil || len(after.Metadata.Models) != 1 || after.Metadata.Tokens.Input == nil || *after.Metadata.Tokens.Input != 7 || after.Metadata.Tokens.Output == nil || *after.Metadata.Tokens.Output != 0 {
		t.Fatal("reindex did not populate recorded metadata")
	}
}
