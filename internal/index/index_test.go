package index_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

func openTestIndex(t *testing.T) *index.Index {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

func fixtureSession() *model.Session {
	start := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	last := start.Add(3 * time.Minute)
	return &model.Session{
		Provider:       model.ProviderClaudeCode,
		ID:             "sess-1",
		Purpose:        "Fix the flaky checkout test",
		PurposeOrigin:  model.OriginRequestExcerpt,
		CWD:            "/work/example-project",
		SourcePath:     "/logs/session-1.jsonl",
		Store:          "/logs",
		StartedAt:      &start,
		LastActivityAt: &last,
		Messages: []model.Message{
			{Seq: 1, Role: "user", Kind: "message", Text: "Fix the flaky checkout test", Timestamp: &start, Locator: model.Locator{Line: 1}},
			{Seq: 2, Role: "tool", Kind: "tool_output", Text: "ok example 0.5s", Timestamp: &last, Locator: model.Locator{Line: 3}},
		},
	}
}

func TestUpsertListAndSearch(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	s := fixtureSession()
	if err := ix.Upsert(ctx, s, 100, time.Now()); err != nil {
		t.Fatal(err)
	}
	rows, err := ix.ListSessions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Purpose != "Fix the flaky checkout test" {
		t.Fatalf("rows = %+v", rows)
	}
	hits, err := ix.SearchContent(ctx, "flaky checkout", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Seq != 1 {
		t.Fatalf("hits = %+v, want the user message", hits)
	}
	toolHits, err := ix.SearchContent(ctx, "ok example", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(toolHits) != 1 || toolHits[0].Kind != "tool_output" {
		t.Fatalf("tool hits = %+v", toolHits)
	}
}

func TestUpsertReplacesWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	s := fixtureSession()
	if err := ix.Upsert(ctx, s, 100, time.Now()); err != nil {
		t.Fatal(err)
	}
	s.Messages = s.Messages[:1]
	if err := ix.Upsert(ctx, s, 90, time.Now()); err != nil {
		t.Fatal(err)
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Sessions != 1 || counts.Messages != 1 {
		t.Errorf("counts = %+v, want 1 session / 1 message after replace", counts)
	}
	if _, _, cwd, version, ok, err := ix.FileState(ctx, s.SourcePath); err != nil || !ok {
		t.Errorf("FileState ok=%v err=%v", ok, err)
	} else if cwd != "/work/example-project" || version != index.ParserVersion {
		t.Errorf("FileState cwd=%q version=%d", cwd, version)
	}
}

func TestRemoveAndSearchEscaping(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	s := fixtureSession()
	if err := ix.Upsert(ctx, s, 100, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.SearchContent(ctx, `flaky" OR checkout`, 10); err != nil {
		t.Errorf("quoted phrase search must not error: %v", err)
	}
	if err := ix.Remove(ctx, s.SourcePath); err != nil {
		t.Fatal(err)
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Sessions != 0 || counts.Messages != 0 {
		t.Errorf("counts after remove = %+v", counts)
	}
}
