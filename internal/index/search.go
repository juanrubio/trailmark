package index

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juanrubio/trailmark/internal/model"
)

func modelProvider(s string) model.Provider           { return model.Provider(s) }
func modelPurposeOrigin(s string) model.PurposeOrigin { return model.PurposeOrigin(s) }
func timeUnix(sec int64) *time.Time {
	t := time.Unix(sec, 0)
	return &t
}

// TranscriptMessage is one indexed message for transcript rendering.
type TranscriptMessage struct {
	Seq       int
	Role      string
	Kind      string
	Text      string
	Timestamp *time.Time
	Line      int
	RecordID  string
}

// TranscriptPage returns up to limit messages starting at offset,
// plus the session's total message count. Paging keeps large
// transcripts bounded in memory.
func (ix *Index) TranscriptPage(ctx context.Context, sourcePath string, offset, limit int) ([]TranscriptMessage, int, error) {
	var total int
	if err := ix.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE source_path = ?`, sourcePath).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count transcript %s: %w", sourcePath, err)
	}
	rows, err := ix.db.QueryContext(ctx, `SELECT seq, role, kind, text, ts, line, record_id FROM messages
		WHERE source_path = ? ORDER BY seq ASC LIMIT ? OFFSET ?`, sourcePath, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("read transcript %s: %w", sourcePath, err)
	}
	defer rows.Close()
	var out []TranscriptMessage
	for rows.Next() {
		var m TranscriptMessage
		var ts *int64
		if err := rows.Scan(&m.Seq, &m.Role, &m.Kind, &m.Text, &ts, &m.Line, &m.RecordID); err != nil {
			return nil, 0, fmt.Errorf("scan transcript message: %w", err)
		}
		if ts != nil {
			t := time.Unix(*ts, 0)
			m.Timestamp = &t
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// TranscriptAround returns a window of messages centered on seq,
// plus the offset of the first returned message and the total.
func (ix *Index) TranscriptAround(ctx context.Context, sourcePath string, seq, before, after int) ([]TranscriptMessage, int, int, error) {
	var offset int
	err := ix.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE source_path = ? AND seq < ?`,
		sourcePath, seq-before).Scan(&offset)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("locate transcript window %s: %w", sourcePath, err)
	}
	msgs, total, err := ix.TranscriptPage(ctx, sourcePath, offset, before+after+1)
	if err != nil {
		return nil, 0, 0, err
	}
	return msgs, offset, total, nil
}

// BuildMatch converts a user query into an FTS5 MATCH expression.
// Unquoted words become ANDed literal terms; double-quoted spans
// become phrases. Every token is quoted so user input cannot alter
// FTS syntax, and path-like queries keep working as token sequences.
func BuildMatch(query string) string {
	var terms []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			terms = append(terms, `"`+strings.ReplaceAll(cur.String(), `"`, `""`)+`"`)
			cur.Reset()
		}
	}
	for _, r := range query {
		switch {
		case r == '"':
			if inQuote {
				flush()
				inQuote = false
			} else {
				flush()
				inQuote = true
			}
		case !inQuote && (r == ' ' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return strings.Join(terms, " ")
}

// SearchInSession returns the seq numbers of messages in one session
// matching the query, in transcript order.
func (ix *Index) SearchInSession(ctx context.Context, sourcePath, query string) ([]int, error) {
	match := BuildMatch(query)
	if match == "" {
		return nil, nil
	}
	rows, err := ix.db.QueryContext(ctx, `SELECT seq FROM messages_fts
		WHERE messages_fts MATCH ? AND source_path = ? ORDER BY seq ASC`, match, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("search in session: %w", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var seq int
		if err := rows.Scan(&seq); err != nil {
			return nil, fmt.Errorf("scan session match: %w", err)
		}
		out = append(out, seq)
	}
	return out, rows.Err()
}

// ContentGroup is one session's grouped content-search results.
type ContentGroup struct {
	Row       SessionRow
	Hits      []SearchHit
	Count     int
	ConvCount int // conversation-kind matches, counted exactly
}

// SearchContentScoped searches message text across the given
// sessions only. Counts come from an exact GROUP BY so repetitive
// sessions cannot hide others behind a global row window, and each
// group keeps its three best snippets via a window function.
// Ranking buckets bm25 scores so near-equal relevance counts as
// comparable: conversation groups then beat tool-only groups, exact
// rank orders within a bucket, and recency breaks final ties.
func (ix *Index) SearchContentScoped(ctx context.Context, query string, paths []string, limit int) ([]ContentGroup, error) {
	match := BuildMatch(query)
	if match == "" || len(paths) == 0 {
		return nil, nil
	}
	type groupData struct {
		group     ContentGroup
		path      string
		best      float64
		hasBest   bool
		convCount int
	}
	byPath := map[string]*groupData{}
	var order []*groupData
	get := func(path string) *groupData {
		g, ok := byPath[path]
		if !ok {
			g = &groupData{path: path}
			byPath[path] = g
			order = append(order, g)
		}
		return g
	}
	for start := 0; start < len(paths); start += 400 {
		end := start + 400
		if end > len(paths) {
			end = len(paths)
		}
		chunk := paths[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		countArgs := make([]any, 0, len(chunk)+1)
		countArgs = append(countArgs, match)
		for _, pth := range chunk {
			countArgs = append(countArgs, pth)
		}
		rows, err := ix.db.QueryContext(ctx, `SELECT source_path, kind, COUNT(*) FROM messages_fts
			WHERE messages_fts MATCH ? AND source_path IN (`+placeholders+`) GROUP BY source_path, kind`, countArgs...)
		if err != nil {
			return nil, fmt.Errorf("scoped content search counts: %w", err)
		}
		for rows.Next() {
			var path, kind string
			var n int
			if err := rows.Scan(&path, &kind, &n); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan scoped count: %w", err)
			}
			g := get(path)
			g.group.Count += n
			if kind == "message" {
				g.convCount += n
				g.group.ConvCount += n
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		hitRows, err := ix.db.QueryContext(ctx, `SELECT source_path, seq, kind, bm25(messages_fts) FROM messages_fts
			WHERE messages_fts MATCH ? AND source_path IN (`+placeholders+`) ORDER BY bm25(messages_fts) LIMIT 5000`, countArgs...)
		if err != nil {
			return nil, fmt.Errorf("scoped content search hits: %w", err)
		}
		for hitRows.Next() {
			var h SearchHit
			if err := hitRows.Scan(&h.SourcePath, &h.Seq, &h.Kind, &h.Rank); err != nil {
				hitRows.Close()
				return nil, fmt.Errorf("scan scoped hit: %w", err)
			}
			g := get(h.SourcePath)
			if len(g.group.Hits) < 3 {
				g.group.Hits = append(g.group.Hits, h)
			}
			if !g.hasBest || h.Rank < g.best {
				g.best, g.hasBest = h.Rank, true
			}
		}
		hitRows.Close()
		if err := hitRows.Err(); err != nil {
			return nil, err
		}
	}
	// Attach session rows only for discovered groups.
	if len(order) > 0 {
		discovered := make([]string, 0, len(order))
		for path := range byPath {
			discovered = append(discovered, path)
		}
		rowByPath, err := ix.sessionRowsByPaths(ctx, discovered)
		if err != nil {
			return nil, err
		}
		for _, g := range order {
			if len(g.group.Hits) > 0 {
				g.group.Row = rowByPath[g.group.Hits[0].SourcePath]
			}
		}
	}
	out := make([]ContentGroup, 0, len(order))
	for _, g := range order {
		out = append(out, g.group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		bi, bj := bucket(bestRank(out[i])), bucket(bestRank(out[j]))
		if bi != bj {
			return bi < bj
		}
		ci, cj := hasConversation(out[i]), hasConversation(out[j])
		if ci != cj {
			return ci
		}
		ri, rj := bestRank(out[i]), bestRank(out[j])
		if ri != rj {
			return ri < rj
		}
		return after(out[i].Row.LastActivity, out[j].Row.LastActivity)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	// Retained groups get their exact top hits with snippets in
	// one query per group. Groups outside the global relevance
	// window keep their exact counts and rank after window groups.
	for gi := range out {
		path := out[gi].Row.SourcePath
		if path == "" && len(out[gi].Hits) > 0 {
			path = out[gi].Hits[0].SourcePath
		}
		if path == "" {
			continue
		}
		rows, err := ix.db.QueryContext(ctx, `SELECT seq, kind, snippet(messages_fts, 0, '[', ']', '…', 12), bm25(messages_fts)
			FROM messages_fts WHERE messages_fts MATCH ? AND source_path = ? ORDER BY bm25(messages_fts) LIMIT 3`, match, path)
		if err != nil {
			return nil, fmt.Errorf("fetch group hits: %w", err)
		}
		var hits []SearchHit
		for rows.Next() {
			var h SearchHit
			h.SourcePath = path
			if err := rows.Scan(&h.Seq, &h.Kind, &h.Snippet, &h.Rank); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan group hit: %w", err)
			}
			hits = append(hits, h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if hits != nil {
			out[gi].Hits = hits
		}
	}
	return out, nil
}

// bucket coarsens a bm25 score so near-equal relevance compares equal.
func bucket(rank float64) float64 {
	return float64(int(rank*2)) / 2
}

// sessionRowsByPaths loads session rows for the given source paths.
func (ix *Index) sessionRowsByPaths(ctx context.Context, paths []string) (map[string]SessionRow, error) {
	out := map[string]SessionRow{}
	for start := 0; start < len(paths); start += 400 {
		end := start + 400
		if end > len(paths) {
			end = len(paths)
		}
		chunk := paths[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, 0, len(chunk))
		for _, p := range chunk {
			args = append(args, p)
		}
		rows, err := ix.db.QueryContext(ctx, `SELECT source_path, provider, native_id, title, purpose, purpose_origin, cwd, store, parent_id, forked_from, sidechain, last_activity, partial
			FROM sessions WHERE source_path IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("load search session rows: %w", err)
		}
		for rows.Next() {
			var r SessionRow
			var provider, origin string
			var last *int64
			var partial, sidechain int
			if err := rows.Scan(&r.SourcePath, &provider, &r.ID, &r.Title, &r.Purpose, &origin, &r.CWD, &r.Store, &r.ParentID, &r.ForkedFrom, &sidechain, &last, &partial); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan search session row: %w", err)
			}
			r.Provider = modelProvider(provider)
			r.PurposeOrigin = modelPurposeOrigin(origin)
			r.Partial = partial != 0
			r.Sidechain = sidechain != 0
			if last != nil {
				t := timeUnix(*last)
				r.LastActivity = t
			}
			out[r.SourcePath] = r
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func bestRank(g ContentGroup) float64 {
	best := 0.0
	for i, h := range g.Hits {
		if i == 0 || h.Rank < best {
			best = h.Rank
		}
	}
	return best
}

func hasConversation(g ContentGroup) bool {
	if g.ConvCount > 0 {
		return true
	}
	for _, h := range g.Hits {
		if h.Kind == "message" {
			return true
		}
	}
	return false
}

func after(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}
