// Package index stores Trailmark's derived session index in SQLite.
// The database is a rebuildable cache of read-only source logs; it
// holds sensitive transcript excerpts and must stay user-private.
package index

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // CGo-free SQLite driver with FTS5

	"github.com/juanrubio/trailmark/internal/adapter"
	"github.com/juanrubio/trailmark/internal/model"
)

// ParserVersion identifies the extraction logic that produced the
// indexed rows. Bumping it forces a reindex of unchanged files.
const ParserVersion = 1

// Index is the derived-data store.
type Index struct {
	db *sql.DB
}

// Open creates or opens the index at path with user-only permissions.
func Open(path string) (*Index, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dir, err)
	}
	dsnURL := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	dsnURL.RawQuery = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	ix := &Index{db: db}
	if err := ix.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
		db.Close()
		return nil, fmt.Errorf("restrict index permissions %s: %w", path, err)
	}
	return ix, nil
}

// Close releases the database handle.
func (ix *Index) Close() error { return ix.db.Close() }

func (ix *Index) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			source_path TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			native_id TEXT NOT NULL DEFAULT '',
			store TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			purpose TEXT NOT NULL DEFAULT '',
			purpose_origin TEXT NOT NULL DEFAULT 'unavailable',
			cwd TEXT NOT NULL DEFAULT '',
			parent_id TEXT NOT NULL DEFAULT '',
			forked_from TEXT NOT NULL DEFAULT '',
			sidechain INTEGER NOT NULL DEFAULT 0,
			started_at INTEGER,
			last_activity INTEGER,
			partial INTEGER NOT NULL DEFAULT 0,
			parse_errors INTEGER NOT NULL DEFAULT 0,
			file_size INTEGER NOT NULL DEFAULT 0,
			file_mtime INTEGER NOT NULL DEFAULT 0,
			parser_version INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS sessions_by_activity ON sessions(last_activity DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS sessions_identity ON sessions(provider, native_id, source_path)`,
		`CREATE TABLE IF NOT EXISTS messages (
			source_path TEXT NOT NULL,
			seq INTEGER NOT NULL,
			role TEXT NOT NULL,
			kind TEXT NOT NULL,
			text TEXT NOT NULL,
			ts INTEGER,
			line INTEGER NOT NULL,
			record_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (source_path, seq)
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(text, source_path UNINDEXED, seq UNINDEXED, kind UNINDEXED)`,
	}
	for _, stmt := range stmts {
		if _, err := ix.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate index: %w", err)
		}
	}
	// Databases created before parser_version existed gain the column
	// with default 0, which never matches ParserVersion and so forces
	// a one-time reindex.
	if _, err := ix.db.ExecContext(ctx, `ALTER TABLE sessions ADD COLUMN parser_version INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("migrate parser_version: %w", err)
	}
	return nil
}

// GetMeta reads one metadata value; ok is false when unset.
func (ix *Index) GetMeta(ctx context.Context, key string) (value string, ok bool, err error) {
	row := ix.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key)
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read meta %s: %w", key, err)
	}
	return value, true, nil
}

// SetMeta writes one metadata value.
func (ix *Index) SetMeta(ctx context.Context, key, value string) error {
	if _, err := ix.db.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		return fmt.Errorf("write meta %s: %w", key, err)
	}
	return nil
}

// FileState returns the stored fingerprint for an indexed source:
// file size, mtime in Unix nanoseconds, recorded cwd, and the parser
// version that produced the row. ok is false when not indexed.
func (ix *Index) FileState(ctx context.Context, sourcePath string) (size int64, mtimeNanos int64, cwd string, parserVersion int, ok bool, err error) {
	row := ix.db.QueryRowContext(ctx, `SELECT file_size, file_mtime, cwd, parser_version FROM sessions WHERE source_path = ?`, sourcePath)
	if err := row.Scan(&size, &mtimeNanos, &cwd, &parserVersion); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, "", 0, false, nil
		}
		return 0, 0, "", 0, false, fmt.Errorf("read file state %s: %w", sourcePath, err)
	}
	return size, mtimeNanos, cwd, parserVersion, true, nil
}

// Upsert replaces the indexed copy of one parsed session atomically.
func (ix *Index) Upsert(ctx context.Context, s *model.Session, size int64, mtime time.Time) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert %s: %w", s.SourcePath, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE source_path = ?`, s.SourcePath); err != nil {
		return fmt.Errorf("clear messages %s: %w", s.SourcePath, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE source_path = ?`, s.SourcePath); err != nil {
		return fmt.Errorf("clear fts %s: %w", s.SourcePath, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions
		(source_path, provider, native_id, store, title, purpose, purpose_origin, cwd, parent_id, forked_from, sidechain, started_at, last_activity, partial, parse_errors, file_size, file_mtime, parser_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_path) DO UPDATE SET
			provider=excluded.provider, native_id=excluded.native_id, store=excluded.store,
			title=excluded.title, purpose=excluded.purpose, purpose_origin=excluded.purpose_origin,
			cwd=excluded.cwd, parent_id=excluded.parent_id, forked_from=excluded.forked_from,
			sidechain=excluded.sidechain, started_at=excluded.started_at, last_activity=excluded.last_activity,
			partial=excluded.partial, parse_errors=excluded.parse_errors,
			file_size=excluded.file_size, file_mtime=excluded.file_mtime, parser_version=excluded.parser_version`,
		s.SourcePath, string(s.Provider), s.ID, s.Store, s.Title, s.Purpose, string(s.PurposeOrigin),
		s.CWD, s.ParentID, s.ForkedFromID, boolInt(s.Sidechain), unixOrNil(s.StartedAt), unixOrNil(s.LastActivityAt),
		boolInt(s.Partial), s.ParseErrors, size, mtime.UnixNano(), ParserVersion)
	if err != nil {
		return fmt.Errorf("upsert session %s: %w", s.SourcePath, err)
	}
	msgStmt, err := tx.PrepareContext(ctx, `INSERT INTO messages (source_path, seq, role, kind, text, ts, line, record_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare messages: %w", err)
	}
	defer msgStmt.Close()
	ftsStmt, err := tx.PrepareContext(ctx, `INSERT INTO messages_fts (text, source_path, seq, kind) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare fts: %w", err)
	}
	defer ftsStmt.Close()
	for _, m := range s.Messages {
		if _, err := msgStmt.ExecContext(ctx, s.SourcePath, m.Seq, m.Role, m.Kind, m.Text, unixOrNil(m.Timestamp), m.Locator.Line, m.Locator.RecordID); err != nil {
			return fmt.Errorf("insert message %s#%d: %w", s.SourcePath, m.Seq, err)
		}
		if m.Text != "" {
			if _, err := ftsStmt.ExecContext(ctx, m.Text, s.SourcePath, m.Seq, m.Kind); err != nil {
				return fmt.Errorf("insert fts %s#%d: %w", s.SourcePath, m.Seq, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert %s: %w", s.SourcePath, err)
	}
	return nil
}

// Remove deletes the derived data for one source path.
func (ix *Index) Remove(ctx context.Context, sourcePath string) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove %s: %w", sourcePath, err)
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM sessions WHERE source_path = ?`,
		`DELETE FROM messages WHERE source_path = ?`,
		`DELETE FROM messages_fts WHERE source_path = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, sourcePath); err != nil {
			return fmt.Errorf("remove %s: %w", sourcePath, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove %s: %w", sourcePath, err)
	}
	return nil
}

// SourcePaths lists every indexed source path.
func (ix *Index) SourcePaths(ctx context.Context) ([]string, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT source_path FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("list source paths: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan source path: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SessionRow is the list-facing projection of a session.
type SessionRow struct {
	SourcePath    string
	Provider      model.Provider
	ID            string
	Title         string
	Purpose       string
	PurposeOrigin model.PurposeOrigin
	CWD           string
	Store         string
	ParentID      string
	ForkedFrom    string
	Sidechain     bool
	LastActivity  *time.Time
	Partial       bool
}

// ListSessions returns sessions newest-activity first.
func (ix *Index) ListSessions(ctx context.Context, limit int) ([]SessionRow, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT source_path, provider, native_id, title, purpose, purpose_origin, cwd, store, parent_id, forked_from, sidechain, last_activity, partial
		FROM sessions ORDER BY COALESCE(last_activity, 0) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		var provider, origin string
		var last sql.NullInt64
		var partial, sidechain int
		if err := rows.Scan(&r.SourcePath, &provider, &r.ID, &r.Title, &r.Purpose, &origin, &r.CWD, &r.Store, &r.ParentID, &r.ForkedFrom, &sidechain, &last, &partial); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		r.Provider = model.Provider(provider)
		r.PurposeOrigin = model.PurposeOrigin(origin)
		r.Partial = partial != 0
		r.Sidechain = sidechain != 0
		if last.Valid {
			t := time.Unix(last.Int64, 0)
			r.LastActivity = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Excerpt is one indexed message shown in an overview, with the
// source locator needed for provenance when copying.
type Excerpt struct {
	Text      string
	Line      int
	RecordID  string
	Timestamp *time.Time
}

// Overview carries the selected-session detail shown beside the list.
type Overview struct {
	Session         SessionRow
	InitialRequest  string
	LatestUser      string
	LatestAssistant string
	Initial         Excerpt
	LatestUserEx    Excerpt
	LatestAsstEx    Excerpt
}

// Overview loads one session's row plus its initial request and
// latest substantive exchange from the index.
func (ix *Index) Overview(ctx context.Context, sourcePath string) (*Overview, error) {
	row := ix.db.QueryRowContext(ctx, `SELECT source_path, provider, native_id, title, purpose, purpose_origin, cwd, store, parent_id, forked_from, sidechain, last_activity, partial
		FROM sessions WHERE source_path = ?`, sourcePath)
	var o Overview
	var provider, origin string
	var last sql.NullInt64
	var partial, sidechain int
	r := &o.Session
	if err := row.Scan(&r.SourcePath, &provider, &r.ID, &r.Title, &r.Purpose, &origin, &r.CWD, &r.Store, &r.ParentID, &r.ForkedFrom, &sidechain, &last, &partial); err != nil {
		return nil, fmt.Errorf("load overview %s: %w", sourcePath, err)
	}
	r.Provider = model.Provider(provider)
	r.PurposeOrigin = model.PurposeOrigin(origin)
	r.Partial = partial != 0
	r.Sidechain = sidechain != 0
	if last.Valid {
		t := time.Unix(last.Int64, 0)
		r.LastActivity = &t
	}
	// Excerpts must skip injected setup text exactly as purpose
	// derivation does, and the latest exchange must pair the latest
	// substantive user message with the final assistant reply that
	// follows it — never with an answer from an earlier exchange.
	type userMsg struct {
		seq  int64
		text string
		line int
		rec  string
		ts   sql.NullInt64
	}
	rows, err := ix.db.QueryContext(ctx, `SELECT seq, text, line, record_id, ts FROM messages WHERE source_path = ? AND role = 'user' AND kind = 'message' ORDER BY seq ASC`, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("load overview messages %s: %w", sourcePath, err)
	}
	var users []userMsg
	for rows.Next() {
		var u userMsg
		if err := rows.Scan(&u.seq, &u.text, &u.line, &u.rec, &u.ts); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan overview message: %w", err)
		}
		users = append(users, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load overview messages %s: %w", sourcePath, err)
	}
	excerpt := func(u userMsg) Excerpt {
		e := Excerpt{Text: u.text, Line: u.line, RecordID: u.rec}
		if u.ts.Valid {
			t := time.Unix(u.ts.Int64, 0)
			e.Timestamp = &t
		}
		return e
	}
	var latestUser *userMsg
	for i := range users {
		if !adapter.Substantive(users[i].text) {
			continue
		}
		if o.InitialRequest == "" {
			o.InitialRequest = users[i].text
			o.Initial = excerpt(users[i])
		}
		latestUser = &users[i]
	}
	asst := func(query string, args ...any) Excerpt {
		var e Excerpt
		var ts sql.NullInt64
		if err := ix.db.QueryRowContext(ctx, query, args...).Scan(&e.Text, &e.Line, &e.RecordID, &ts); err != nil {
			return Excerpt{}
		}
		if ts.Valid {
			t := time.Unix(ts.Int64, 0)
			e.Timestamp = &t
		}
		return e
	}
	if latestUser != nil {
		o.LatestUser = latestUser.text
		o.LatestUserEx = excerpt(*latestUser)
		o.LatestAsstEx = asst(`SELECT text, line, record_id, ts FROM messages WHERE source_path = ? AND role = 'assistant' AND kind = 'message' AND seq > ? ORDER BY seq DESC LIMIT 1`, sourcePath, latestUser.seq)
		o.LatestAssistant = o.LatestAsstEx.Text
	} else {
		o.LatestAsstEx = asst(`SELECT text, line, record_id, ts FROM messages WHERE source_path = ? AND role = 'assistant' AND kind = 'message' ORDER BY seq DESC LIMIT 1`, sourcePath)
		o.LatestAssistant = o.LatestAsstEx.Text
	}
	return &o, nil
}

// SearchHit is one FTS5 content match.
type SearchHit struct {
	SourcePath string
	Seq        int
	Kind       string
	Snippet    string
	Rank       float64
}

// SearchContent runs a literal phrase search over indexed message
// text. The phrase is quoted so user input cannot alter FTS syntax.
func (ix *Index) SearchContent(ctx context.Context, phrase string, limit int) ([]SearchHit, error) {
	match := `"` + strings.ReplaceAll(phrase, `"`, `""`) + `"`
	rows, err := ix.db.QueryContext(ctx, `SELECT source_path, seq, kind, snippet(messages_fts, 0, '[', ']', '…', 12), bm25(messages_fts)
		FROM messages_fts WHERE messages_fts MATCH ? ORDER BY bm25(messages_fts) LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search content: %w", err)
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.SourcePath, &h.Seq, &h.Kind, &h.Snippet, &h.Rank); err != nil {
			return nil, fmt.Errorf("scan search hit: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Counts summarizes index coverage.
type Counts struct {
	Sessions   int
	Messages   int
	Partial    int
	ByProvider map[model.Provider]int
}

// Stats returns coverage counts for status reporting.
func (ix *Index) Stats(ctx context.Context) (Counts, error) {
	c := Counts{ByProvider: map[model.Provider]int{}}
	if err := ix.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(partial),0) FROM sessions`).Scan(&c.Sessions, &c.Partial); err != nil {
		return c, fmt.Errorf("count sessions: %w", err)
	}
	if err := ix.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&c.Messages); err != nil {
		return c, fmt.Errorf("count messages: %w", err)
	}
	rows, err := ix.db.QueryContext(ctx, `SELECT provider, COUNT(*) FROM sessions GROUP BY provider`)
	if err != nil {
		return c, fmt.Errorf("count providers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			return c, fmt.Errorf("scan provider count: %w", err)
		}
		c.ByProvider[model.Provider(p)] = n
	}
	return c, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func unixOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}
