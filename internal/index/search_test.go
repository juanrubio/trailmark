package index_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

func searchSession(path, purpose string, msgs []model.Message) *model.Session {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	return &model.Session{
		Provider: model.ProviderCodex, ID: path, Purpose: purpose,
		PurposeOrigin: model.OriginRequestExcerpt, CWD: "/work/p",
		SourcePath: path, Store: "/logs", LastActivityAt: &now, Messages: msgs,
	}
}

func msg(seq int, role, kind, text string) model.Message {
	return model.Message{Seq: seq, Role: role, Kind: kind, Text: text, Locator: model.Locator{Line: seq}}
}

func TestTranscriptPagingAndAround(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	var msgs []model.Message
	for i := 1; i <= 10; i++ {
		msgs = append(msgs, msg(i, "user", "message", "message number "+strings.Repeat("x", i)))
	}
	s := searchSession("/logs/long.jsonl", "Long session", msgs)
	if err := ix.Upsert(ctx, s, 100, time.Now()); err != nil {
		t.Fatal(err)
	}
	page, total, err := ix.TranscriptPage(ctx, s.SourcePath, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 || len(page) != 3 || page[0].Seq != 1 {
		t.Fatalf("page = %d/%d total=%d", len(page), page[0].Seq, total)
	}
	around, offset, _, err := ix.TranscriptAround(ctx, s.SourcePath, 8, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 5 || len(around) != 5 || around[2].Seq != 8 {
		t.Fatalf("around offset=%d len=%d", offset, len(around))
	}
	seqs, err := ix.SearchInSession(ctx, s.SourcePath, "number")
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 10 {
		t.Errorf("in-session seqs = %d, want 10", len(seqs))
	}
}

func TestScopedSearchRankingConversationBeforeRepetitiveTool(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	toolHeavy := searchSession("/logs/tool.jsonl", "Tool heavy", []model.Message{
		msg(1, "tool", "tool_output", "upload upload upload upload upload"),
		msg(2, "tool", "tool_output", "upload upload upload upload"),
		msg(3, "tool", "tool_output", "upload upload upload"),
	})
	convo := searchSession("/logs/convo.jsonl", "Conversation", []model.Message{
		msg(1, "user", "message", "please fix the upload worker"),
		msg(2, "assistant", "message", "upload fixed"),
	})
	for _, s := range []*model.Session{toolHeavy, convo} {
		if err := ix.Upsert(ctx, s, 100, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	groups, err := ix.SearchContentScoped(ctx, "upload", []string{toolHeavy.SourcePath, convo.SourcePath}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	// Repetitive tool output must not outrank conversation:
	// near-equal bm25 buckets count as comparable relevance, and
	// the conversation group wins the comparison.
	if groups[0].Row.SourcePath != convo.SourcePath {
		t.Fatalf("first group = %q, want the conversation session ahead of repetitive tool output", groups[0].Row.SourcePath)
	}
	var convoGroup *index.ContentGroup
	for i := range groups {
		if groups[i].Row.SourcePath == convo.SourcePath {
			convoGroup = &groups[i]
		}
	}
	if convoGroup == nil || convoGroup.Count != 2 {
		t.Errorf("convo group = %+v", convoGroup)
	}
}

func TestBuildMatchEscapesAndPhrases(t *testing.T) {
	if got := index.BuildMatch(`upload "retry helper" a"b`); !strings.Contains(got, `"retry helper"`) {
		t.Errorf("match = %q, want the quoted phrase preserved", got)
	}
	if got := index.BuildMatch(""); got != "" {
		t.Errorf("empty query match = %q", got)
	}
}
