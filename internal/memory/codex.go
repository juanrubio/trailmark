package memory

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/juanrubio/trailmark/internal/model"
)

// codexMemoriesDir resolves a Codex root to its memories directory.
func codexMemoriesDir(root string) string {
	if filepath.Base(root) == "memories" {
		return root
	}
	sub := filepath.Join(root, "memories")
	if info, err := os.Lstat(sub); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return canonical(sub)
	}
	return root
}

type codexKind int

const (
	kindSkip codexKind = iota
	kindConsolidated
	kindOverview
	kindRaw
	kindRollout
	kindSupporting // extension notes and unknown root-level Markdown
)

func classifyCodex(memDir, path string) codexKind {
	rel, err := filepath.Rel(memDir, path)
	if err != nil {
		return kindSkip
	}
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "MEMORY.md":
		return kindConsolidated
	case rel == "memory_summary.md":
		return kindOverview
	case rel == "raw_memories.md":
		return kindRaw
	case strings.HasPrefix(rel, "rollout_summaries/"):
		return kindRollout
	case strings.HasPrefix(rel, "extensions/"):
		parts := strings.Split(rel, "/")
		if len(parts) >= 4 && parts[2] == "notes" {
			return kindSupporting
		}
		return kindSkip // includes extensions/*/instructions.md
	case strings.HasPrefix(rel, "skills/"):
		return kindSkip
	case !strings.Contains(rel, "/"):
		return kindSupporting
	default:
		return kindSkip
	}
}

// loadCodex returns a coverage note naming the first traversal
// failure, if any, alongside the documents that could be read.
func loadCodex(ctx context.Context, cat *Catalog, root string, opts Options) (string, error) {
	memDir := codexMemoriesDir(root)
	if opts.excluded(memDir) {
		return "", nil
	}
	files, outside, err := collectMarkdown(memDir, memDir, func(p string) bool {
		return classifyCodex(memDir, p) != kindSkip
	})
	note := ""
	if err != nil {
		note = err.Error()
	}
	catStartDocs, catStartEntries := len(cat.Documents), len(cat.Entries)
	for _, p := range outside {
		if classifyCodex(memDir, p) == kindSkip {
			continue
		}
		abs, _ := filepath.Abs(p)
		cat.Documents = append(cat.Documents, Document{
			ID: abs, Path: abs, Root: root, Provider: model.ProviderCodex,
			Category: codexCategory(classifyCodex(memDir, p)), Title: baseNoExt(p),
			ModTime: fileModTime(p), Status: "outside-root",
			StatusDetail: "symlink target escapes the approved root", ProjectEvidence: "unknown",
		})
	}
	for _, p := range files {
		if err := ctx.Err(); err != nil {
			return note, err
		}
		if opts.excluded(p) {
			continue
		}
		// Classify by the canonical target, matching document
		// identity: a same-root symlink alias of a rollout summary
		// must parse as that rollout (with its recorded cwd), or
		// deduplication could keep an Unknown alias and drop the
		// evidenced entry, bypassing project exclusion.
		kind := classifyCodex(memDir, canonical(p))
		if kind == kindSkip {
			continue
		}
		// Project exclusion is applied in Load after consolidated
		// sections inherit rollout project evidence; discarding an
		// excluded rollout here would strip that evidence and leave
		// its consolidated sections visible as Unknown.
		doc, entries := parseCodexDoc(p, root, kind, opts)
		cat.Documents = append(cat.Documents, doc)
		cat.Entries = append(cat.Entries, entries...)
	}
	_ = catStartDocs
	_ = catStartEntries
	return note, nil
}

func codexCategory(k codexKind) Category {
	switch k {
	case kindConsolidated:
		return CategoryConsolidated
	case kindOverview:
		return CategoryOverview
	case kindRaw:
		return CategoryRaw
	default:
		return CategorySupporting
	}
}

type section struct {
	headingPath []string
	title       string
	start, end  int // 1-based inclusive
	lines       []string
}

// splitAt splits lines at headings of the given level (optionally
// filtered by heading text). A non-empty preamble comes first.
func splitAt(lines []string, level int, accept func(heading string) bool) []section {
	var secs []section
	cur := section{start: 1}
	var h1 string
	flush := func(end int) {
		cur.end = end
		cur.lines = lines[cur.start-1 : end]
		secs = append(secs, cur)
	}
	var fence fenceState
	for i, l := range lines {
		if fence.feed(l) {
			continue
		}
		lvl, text, ok := headingLevel(l)
		if !ok {
			continue
		}
		if lvl == 1 && level != 1 {
			h1 = text
		}
		if lvl != level || (accept != nil && !accept(text)) {
			continue
		}
		if i > 0 {
			flush(i) // previous section ends on the line before
		}
		cur = section{start: i + 1, title: text}
		if level == 1 || h1 == "" || h1 == text {
			cur.headingPath = []string{text}
		} else {
			cur.headingPath = []string{h1, text}
		}
		if lvl == 1 {
			h1 = text
		}
	}
	flush(len(lines))
	// Drop an empty preamble; keep named sections even when bodyless.
	out := secs[:0]
	for i, s := range secs {
		if i == 0 && s.title == "" {
			rest := s.lines
			if len(rest) > 0 {
				if lvl, _, ok := headingLevel(rest[0]); ok && lvl == 1 && level != 1 {
					rest = rest[1:]
				}
			}
			if !nonempty(rest) {
				continue
			}
		}
		out = append(out, s)
	}
	// A preamble of only a higher-level heading still counts as
	// content via nonempty above; give it that heading as title.
	for i := range out {
		if out[i].title == "" {
			if h := firstH1(out[i].lines); h != "" {
				out[i].title = h
				out[i].headingPath = []string{h}
			}
		}
	}
	return out
}

// sectionBody drops the splitting heading line when present.
func sectionBody(s section) []string {
	lines := s.lines
	if s.title != "" && len(lines) > 0 {
		if _, text, ok := headingLevel(lines[0]); ok && text == s.title {
			lines = lines[1:]
		}
	}
	return lines
}

var headerLineRe = regexp.MustCompile(`^([A-Za-z0-9_-]+):[ \t]+(.*)$`)

// parseKeyHeader reads leading "key: value" lines (rollout summary
// headers) up to a blank line, a heading, or a non-matching line.
func parseKeyHeader(lines []string) (meta map[string]string, bodyStart int) {
	meta = map[string]string{}
	bodyStart = 1
	for i, l := range lines {
		m := headerLineRe.FindStringSubmatch(l)
		if m == nil {
			break
		}
		meta[m[1]] = stripQuotes(strings.TrimSpace(m[2]))
		bodyStart = i + 2
	}
	if len(meta) == 0 {
		return meta, 1
	}
	return meta, bodyStart
}

func parseRecordedAt(v string) *time.Time {
	v = strings.TrimSpace(v)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t
		}
	}
	return nil
}

var backtickRe = regexp.MustCompile("`([^`]+)`")

func parseCodexDoc(path, root string, kind codexKind, opts Options) (Document, []Entry) {
	canon := canonical(path)
	category := codexCategory(kind)
	mod := fileModTime(path)
	doc := Document{
		ID: canon, Path: canon, Root: root, Provider: model.ProviderCodex,
		Category: category, ModTime: mod, Status: "ok", ProjectEvidence: "unknown",
	}
	text, status, detail := readFile(path, opts.maxBytes())
	doc.Status, doc.StatusDetail = status, detail
	if status == "unreadable" {
		doc.Title = baseNoExt(path)
		return doc, nil
	}
	lines := splitLines(text)
	ids := newIDMaker(canon)
	base := Entry{
		DocumentID: canon, Path: canon, Provider: model.ProviderCodex,
		Category: category, ModTime: mod, Meta: map[string]string{},
		ProjectEvidence: "unknown",
	}
	var entries []Entry
	switch kind {
	case kindConsolidated:
		doc.Title = firstNonEmpty(firstH1(lines), baseNoExt(path))
		for _, s := range splitAt(lines, 1, nil) {
			e := base
			e.ID = ids.next(s.headingPath)
			e.Title = firstNonEmpty(s.title, baseNoExt(path))
			e.HeadingPath = s.headingPath
			e.StartLine, e.EndLine = s.start, max(s.end, s.start)
			e.RawText = joinLines(s.lines)
			bodyLines := sectionBody(s)
			e.TextLine = s.start + len(s.lines) - len(bodyLines)
			e.Text = joinLines(bodyLines)
			e.References = mdTokenReferences(e.RawText, linkReferences(e.RawText))
			entries = append(entries, e)
		}
	case kindOverview:
		doc.Title = firstNonEmpty(firstH1(lines), baseNoExt(path))
		doc.ProjectEvidence = "global"
		for _, s := range splitAt(lines, 2, nil) {
			e := base
			e.ID = ids.next(s.headingPath)
			e.Title = firstNonEmpty(s.title, baseNoExt(path))
			e.HeadingPath = s.headingPath
			e.StartLine, e.EndLine = s.start, max(s.end, s.start)
			e.RawText = joinLines(s.lines)
			bodyLines := sectionBody(s)
			e.TextLine = s.start + len(s.lines) - len(bodyLines)
			e.Text = joinLines(bodyLines)
			e.ProjectEvidence = "global"
			e.References = linkReferences(e.RawText)
			entries = append(entries, e)
		}
	case kindRaw:
		doc.Title = firstNonEmpty(firstH1(lines), baseNoExt(path))
		for _, s := range splitAt(lines, 2, func(h string) bool {
			return strings.HasPrefix(strings.ToLower(h), "thread")
		}) {
			e := base
			e.ID = ids.next(s.headingPath)
			e.Title = firstNonEmpty(s.title, baseNoExt(path))
			e.HeadingPath = s.headingPath
			e.StartLine, e.EndLine = s.start, max(s.end, s.start)
			e.RawText = joinLines(s.lines)
			bodyLines := sectionBody(s)
			e.TextLine = s.start + len(s.lines) - len(bodyLines)
			e.Text = joinLines(bodyLines)
			if m := backtickRe.FindStringSubmatch(s.title); m != nil {
				e.ThreadID = strings.TrimSpace(m[1])
			}
			if e.ThreadID == "" {
				if m := regexp.MustCompile(`(?i)thread[_ ]?id[:=][ \t]*` + "`?([A-Za-z0-9._-]+)").FindStringSubmatch(e.RawText); m != nil {
					e.ThreadID = m[1]
				}
			}
			if e.ThreadID != "" {
				e.Meta = map[string]string{"thread_id": e.ThreadID}
				e.References = append(linkReferences(e.RawText), sessionReference(e.ThreadID))
			} else {
				e.References = linkReferences(e.RawText)
			}
			entries = append(entries, e)
		}
	case kindRollout:
		meta, bodyStart := parseKeyHeader(lines)
		body := lines[bodyStart-1:]
		cwd := metaGet(meta, "cwd")
		thread := metaGet(meta, "thread_id", "threadId")
		title := firstNonEmpty(firstH1(body), baseNoExt(path))
		doc.Title = title
		doc.ProjectCWD = cwd
		if cwd != "" {
			doc.ProjectEvidence = "rollout-header"
		}
		e := base
		e.ID = ids.next(nil)
		e.Title = title
		e.StartLine, e.EndLine = 1, max(1, len(lines))
		e.TextLine = bodyStart
		e.RawText = text
		e.Text = joinLines(body)
		e.Meta = meta
		e.ThreadID = thread
		e.OriginSessionID = thread
		e.RecordedAt = parseRecordedAt(metaGet(meta, "updated_at", "recorded_at"))
		e.ProjectCWD = cwd
		e.ProjectEvidence = doc.ProjectEvidence
		e.References = linkReferences(text)
		if thread != "" {
			e.References = append(e.References, sessionReference(thread))
		}
		entries = []Entry{e}
	default: // supporting whole-doc fallback (incl. extension notes)
		meta, body, bodyStart, _ := parseFrontmatter(lines)
		title := metaGet(meta, "name")
		if title == "" {
			title = firstH1(body)
		}
		if title == "" {
			title = baseNoExt(path)
		}
		doc.Title = title
		e := base
		e.ID = ids.next(nil)
		e.Title = title
		e.Description = metaGet(meta, "description")
		e.Type = metaGet(meta, "type", "metadata.type")
		e.StartLine, e.EndLine = 1, max(1, len(lines))
		e.TextLine = bodyStart
		e.RawText = text
		e.Text = joinLines(body)
		e.Meta = meta
		e.References = linkReferences(text)
		entries = []Entry{e}
	}
	return doc, entries
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// applyCodexConsolidatedProjects assigns a project to consolidated
// sections from the rollout summaries they reference (majority cwd,
// earliest first appearance on ties), then applies per-entry project
// exclusion to those sections. Runs after resolveReferences, but
// works off raw reference basenames so unresolved-by-root links to
// known rollouts still carry their recorded cwd.
func applyCodexConsolidatedProjects(cat *Catalog, opts Options) {
	docRoot := map[string]string{}
	for _, d := range cat.Documents {
		docRoot[d.ID] = d.Root
	}
	// Evidence is scoped to the referencing document's own root:
	// two stores may hold rollout summaries with the same filename
	// but different recorded directories.
	rolloutCWD := map[string]string{} // root|basename -> cwd
	rolloutEntry := map[string]*Entry{}
	ambiguous := map[string]bool{}
	for i := range cat.Entries {
		e := &cat.Entries[i]
		if e.Provider == model.ProviderCodex && e.Category == CategorySupporting && e.ProjectEvidence == "rollout-header" && e.ProjectCWD != "" {
			key := docRoot[e.DocumentID] + "|" + filepath.Base(e.Path)
			if prev, ok := rolloutCWD[key]; ok && prev != e.ProjectCWD {
				ambiguous[key] = true // conflicting evidence: trust neither
				continue
			}
			rolloutCWD[key] = e.ProjectCWD
			rolloutEntry[key] = e
		}
	}
	for key := range ambiguous {
		delete(rolloutCWD, key)
		delete(rolloutEntry, key)
	}
	// Loaded rollout summaries per root, for telling "referenced
	// rollout was read" apart from "referenced rollout was skipped
	// (for example path-excluded)".
	loadedRollout := map[string]bool{}
	for _, d := range cat.Documents {
		if d.Provider == model.ProviderCodex && filepath.Base(filepath.Dir(d.Path)) == "rollout_summaries" {
			loadedRollout[d.Root+"|"+filepath.Base(d.Path)] = true
		}
	}
	suppress := map[string]bool{}
	loadedDoc := map[string]bool{}
	for _, d := range cat.Documents {
		loadedDoc[d.Path] = true
	}
	// explicitCandidates resolves a reference's literal target
	// paths (relative to the entry's directory and its memories
	// root) without any basename fallback.
	explicitCandidates := func(e *Entry, target string) []string {
		if filepath.IsAbs(target) {
			return []string{canonical(target)}
		}
		cands := []string{canonical(filepath.Join(filepath.Dir(e.Path), filepath.FromSlash(target)))}
		if strings.Contains(target, "/") {
			cands = append(cands, canonical(filepath.Join(codexMemoriesDir(docRoot[e.DocumentID]), filepath.FromSlash(target))))
		}
		return cands
	}
	explicitExcluded := func(e *Entry, target string) bool {
		if opts.Excluded == nil {
			return false
		}
		for _, cand := range explicitCandidates(e, target) {
			if opts.excluded(cand) {
				return true
			}
		}
		return false
	}
	explicitLoaded := func(e *Entry, target string) bool {
		for _, cand := range explicitCandidates(e, target) {
			if loadedDoc[cand] {
				return true
			}
		}
		return false
	}
	entryByID := map[string]*Entry{}
	for i := range cat.Entries {
		entryByID[cat.Entries[i].ID] = &cat.Entries[i]
	}
	{
		for i := range cat.Entries {
			e := &cat.Entries[i]
			if e.Provider != model.ProviderCodex || e.Category != CategoryConsolidated {
				continue
			}
			counts := map[string]int{}
			order := []string{}
			for _, ref := range e.References {
				// Resolved references carry the target's own
				// evidence (an alias may name a rollout by a
				// different basename); raw basenames are the
				// fallback for unresolved mentions.
				cwd, ok := "", false
				if ref.EntryID != "" {
					if target := entryByID[ref.EntryID]; target != nil && target.ProjectCWD != "" {
						cwd, ok = target.ProjectCWD, true
					}
				}
				target := strings.SplitN(ref.Raw, "#", 2)[0]
				// Basename evidence is only for bare-basename
				// references (or a literal target that loaded):
				// an explicit path to a different same-basename
				// file must not inherit this rollout's project.
				if !ok && !explicitExcluded(e, target) && (!strings.Contains(target, "/") || explicitLoaded(e, target)) {
					cwd, ok = rolloutCWD[docRoot[e.DocumentID]+"|"+filepath.Base(target)]
				}
				if !ok {
					continue
				}
				if counts[cwd] == 0 {
					order = append(order, cwd)
				}
				counts[cwd]++
			}
			best, bestN := "", 0
			for _, cwd := range order {
				if counts[cwd] > bestN {
					best, bestN = cwd, counts[cwd]
				}
			}
			if best != "" {
				e.ProjectCWD = best
				e.ProjectEvidence = "rollout-header"
			}
			// Privacy check uses every evidenced project, not just
			// the displayed majority: a section citing material
			// from an excluded project is hidden whole, since its
			// text cannot be split by project.
			if opts.Excluded != nil {
				for _, cwd := range order {
					if opts.excluded(cwd) {
						e.ProjectCWD = cwd
						e.ProjectEvidence = "rollout-header"
						break
					}
				}
				// A path-excluded rollout never reaches the
				// catalog, so it cannot supply project evidence.
				// Suppress sections referencing one instead of
				// letting an added exclusion reveal them as
				// Unknown.
				memDir := codexMemoriesDir(docRoot[e.DocumentID])
				for _, ref := range e.References {
					if ref.Kind != "note" {
						continue
					}
					if ref.EntryID != "" {
						continue // target was loaded; evidence handled above
					}
					target := strings.SplitN(ref.Raw, "#", 2)[0]
					base := filepath.Base(target)
					// The literal target's exclusion is checked
					// before any loaded shortcut: a different
					// same-basename file must not mask it.
					if explicitExcluded(e, target) {
						suppress[e.ID] = true
						continue
					}
					if explicitLoaded(e, target) || (!strings.Contains(target, "/") && loadedRollout[docRoot[e.DocumentID]+"|"+base]) {
						continue
					}
					candidates := []string{
						filepath.Join(memDir, "rollout_summaries", base),
						filepath.Join(memDir, base),
					}
					if strings.Contains(target, "/") {
						candidates = append(candidates, filepath.Join(memDir, filepath.FromSlash(target)))
					}
					for _, cand := range candidates {
						if opts.excluded(cand) {
							suppress[e.ID] = true
							break
						}
					}
				}
			}
		}
	}
	if opts.Excluded == nil {
		return
	}
	kept := cat.Entries[:0]
	for _, e := range cat.Entries {
		if suppress[e.ID] {
			continue
		}
		if e.Category == CategoryConsolidated && e.ProjectCWD != "" && opts.excluded(e.ProjectCWD) {
			continue
		}
		kept = append(kept, e)
	}
	cat.Entries = kept
	_ = rolloutEntry
}
