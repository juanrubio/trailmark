package memory

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanrubio/trailmark/internal/model"
)

type claudeStore struct {
	memDir     string // canonical memory directory
	projectDir string // canonical parent (project) directory
}

// claudeStores classifies a Claude root: a direct memory directory,
// a projects directory enumerating */memory, or a directory holding
// Markdown notes directly.
func claudeStores(root string) ([]claudeStore, error) {
	if filepath.Base(root) == "memory" {
		return []claudeStore{{memDir: root, projectDir: filepath.Dir(root)}}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var stores []claudeStore
	hasMarkdown := false
	for _, e := range entries {
		if strings.EqualFold(filepath.Ext(e.Name()), ".md") && !e.IsDir() {
			hasMarkdown = true
		}
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		mem := filepath.Join(root, e.Name(), "memory")
		info, err := os.Lstat(mem)
		if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			stores = append(stores, claudeStore{memDir: canonical(mem), projectDir: canonical(filepath.Join(root, e.Name()))})
		}
	}
	if len(stores) > 0 {
		sort.Slice(stores, func(i, j int) bool { return stores[i].memDir < stores[j].memDir })
		return stores, nil
	}
	if hasMarkdown {
		return []claudeStore{{memDir: root, projectDir: filepath.Dir(root)}}, nil
	}
	return nil, nil
}

func (s claudeStore) project(ctx context.Context, opts Options) (cwd, evidence string) {
	for _, dir := range []string{s.projectDir, s.memDir} {
		if cwd, ok := opts.ClaudeProjectByDir[canonical(dir)]; ok && cwd != "" {
			return cwd, "session-association"
		}
		if cwd, ok := opts.ClaudeProjectByDir[dir]; ok && cwd != "" {
			return cwd, "session-association"
		}
	}
	// The session index cannot be the only association source: an
	// excluded project's rows are deleted from it, which would
	// resurrect its memories as Unknown. Fall back to the cwd
	// recorded in this project directory's own session logs.
	if cwd := peekClaudeProjectCWD(ctx, s.projectDir); cwd != "" {
		return cwd, "session-association"
	}
	return "", "unknown"
}

// peekClaudeProjectCWD reads the recorded working directory from
// session logs in a Claude project directory. It is a bounded
// metadata peek — at most 3 regular .jsonl files, each at most
// 256 KiB and 100 lines, honoring cancellation — never a
// transcript scan.
func peekClaudeProjectCWD(ctx context.Context, projectDir string) string {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return ""
	}
	var logs []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.EqualFold(filepath.Ext(e.Name()), ".jsonl") {
			logs = append(logs, filepath.Join(projectDir, e.Name()))
		}
	}
	sort.Strings(logs)
	if len(logs) > 3 {
		logs = logs[:3]
	}
	for _, path := range logs {
		if ctx.Err() != nil {
			return ""
		}
		if cwd := peekSessionLogCWD(path); cwd != "" {
			return cwd
		}
	}
	return ""
}

func peekSessionLogCWD(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 256*1024))
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for lines := 0; lines < 100 && sc.Scan(); lines++ {
		var rec struct {
			CWD string `json:"cwd"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.CWD != "" {
			return rec.CWD
		}
	}
	return ""
}

// loadClaude returns a coverage note naming the first traversal
// failure, if any, alongside the documents that could be read.
func loadClaude(ctx context.Context, cat *Catalog, root string, opts Options) (string, error) {
	stores, storesErr := claudeStores(root)
	note := ""
	if storesErr != nil {
		note = storesErr.Error()
	}
	for _, store := range stores {
		if err := ctx.Err(); err != nil {
			return note, err
		}
		cwd, evidence := store.project(ctx, opts)
		// Whole-store exclusion by path or confidently known project.
		if opts.excluded(store.memDir) || (cwd != "" && opts.excluded(cwd)) {
			continue
		}
		// The symlink boundary is the individual memory store, not
		// the shared projects root: a link from one project's store
		// into another's would otherwise import that project's
		// content under this store's project association and
		// bypass its project exclusion.
		files, outside, err := collectMarkdown(store.memDir, store.memDir, nil)
		if err != nil && note == "" {
			note = err.Error()
		}
		for _, p := range outside {
			abs, _ := filepath.Abs(p)
			cat.Documents = append(cat.Documents, Document{
				ID: abs, Path: abs, Root: root, Provider: model.ProviderClaudeCode,
				Category: claudeCategory(p), Title: baseNoExt(p), ModTime: fileModTime(p),
				Status: "outside-root", StatusDetail: "symlink target escapes the approved root",
				ProjectCWD: cwd, ProjectEvidence: evidence,
			})
		}
		for _, p := range files {
			if err := ctx.Err(); err != nil {
				return note, err
			}
			if opts.excluded(p) {
				continue
			}
			doc, entries := parseClaudeDoc(p, root, cwd, evidence, opts)
			cat.Documents = append(cat.Documents, doc)
			cat.Entries = append(cat.Entries, entries...)
		}
	}
	return note, nil
}

func claudeCategory(path string) Category {
	if filepath.Base(path) == "MEMORY.md" {
		return CategoryIndex
	}
	return CategoryNote
}

func parseClaudeDoc(path, root, cwd, evidence string, opts Options) (Document, []Entry) {
	canon := canonical(path)
	category := claudeCategory(canon)
	mod := fileModTime(path)
	doc := Document{
		ID: canon, Path: canon, Root: root, Provider: model.ProviderClaudeCode,
		Category: category, ModTime: mod, Status: "ok",
		ProjectCWD: cwd, ProjectEvidence: evidence,
	}
	text, status, detail := readFile(path, opts.maxBytes())
	doc.Status, doc.StatusDetail = status, detail
	if status == "unreadable" {
		doc.Title = baseNoExt(path)
		return doc, nil
	}
	lines := splitLines(text)
	meta, body, bodyStart, _ := parseFrontmatter(lines)
	title := metaGet(meta, "name")
	if title == "" {
		title = firstH1(body)
	}
	if title == "" {
		title = baseNoExt(path)
	}
	doc.Title = title
	origin := metaGet(meta, "originSessionId", "originSessionID", "origin_session_id",
		"metadata.originSessionId", "metadata.originSessionID")
	refs := linkReferences(text)
	if origin != "" {
		refs = append(refs, sessionReference(origin))
	}
	ids := newIDMaker(canon)
	entry := Entry{
		ID: ids.next(nil), DocumentID: canon, Path: canon,
		Provider: model.ProviderClaudeCode, Category: category,
		Title: title, Description: metaGet(meta, "description"),
		Type:            metaGet(meta, "type", "metadata.type"),
		StartLine:       1,
		EndLine:         max(1, len(lines)),
		TextLine:        bodyStart,
		Text:            joinLines(body),
		RawText:         text,
		Meta:            meta,
		OriginSessionID: origin,
		ModTime:         mod,
		ProjectCWD:      cwd, ProjectEvidence: evidence,
		References: refs,
	}
	return doc, []Entry{entry}
}
