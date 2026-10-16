package memory

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/model"
)

func canonical(path string) string { return config.Canonical(path) }

func (o Options) maxBytes() int64 {
	if o.MaxFileBytes > 0 {
		return o.MaxFileBytes
	}
	return DefaultMaxFileBytes
}

func (o Options) excluded(path string) bool {
	return o.Excluded != nil && path != "" && o.Excluded(path)
}

// dropNestedRoots removes roots whose effective store directory
// sits inside another same-provider root's store. The ancestor
// root parses those files with full provider context (for example
// rollout headers), while a nested root would parse them
// generically and first-wins deduplication could keep the
// evidence-free version.
func dropNestedRoots(roots []Root) []Root {
	effective := func(r Root) string {
		p := canonical(r.Path)
		if r.Provider == model.ProviderCodex {
			return codexMemoriesDir(p)
		}
		return p
	}
	var out []Root
	for i, r := range roots {
		eff := effective(r)
		nested := false
		for j, other := range roots {
			if i == j || other.Provider != r.Provider {
				continue
			}
			otherEff := effective(other)
			if eff == otherEff || !strings.HasPrefix(eff, otherEff+string(filepath.Separator)) {
				continue
			}
			// Directory ancestry only counts when the ancestor
			// actually loads the nested store's files. Codex
			// loads only its recognized layout, so an
			// independently configured store elsewhere below it
			// (for example an archive) must stay. Claude loads
			// recursively within each discovered store, so a
			// nested root is covered only when it falls inside
			// a store the ancestor discovers (an immediate
			// */memory directory or the root itself).
			if r.Provider != model.ProviderCodex {
				stores, err := claudeStores(otherEff)
				if err == nil {
					for _, store := range stores {
						if eff == store.memDir || strings.HasPrefix(eff, store.memDir+string(filepath.Separator)) {
							nested = true
							break
						}
					}
				}
				if nested {
					break
				}
				continue
			}
			rel := strings.TrimPrefix(eff, otherEff+string(filepath.Separator))
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if parts[0] == "rollout_summaries" || (len(parts) >= 3 && parts[0] == "extensions" && parts[2] == "notes") {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, r)
		}
	}
	return out
}

// Load reads every root read-only. Per-file and per-root failures are
// reported in Document.Status / Sources; only context cancellation
// (or an internal error) returns a non-nil error.
func Load(ctx context.Context, opts Options) (*Catalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cat := &Catalog{}
	seenRoot := map[string]bool{}
	for _, root := range dropNestedRoots(opts.Roots) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := string(root.Provider) + "|" + canonical(root.Path)
		if seenRoot[key] {
			continue
		}
		seenRoot[key] = true
		src := SourceStatus{Provider: root.Provider, Root: root.Path}
		info, err := os.Stat(root.Path)
		switch {
		case os.IsNotExist(err):
			src.State = "missing"
			src.Detail = "root does not exist"
			cat.Sources = append(cat.Sources, src)
			continue
		case err != nil:
			src.State = "unreadable"
			src.Detail = err.Error()
			cat.Sources = append(cat.Sources, src)
			continue
		case !info.IsDir():
			src.State = "unreadable"
			src.Detail = "root is not a directory"
			cat.Sources = append(cat.Sources, src)
			continue
		}
		src.Root = canonical(root.Path)
		if opts.excluded(root.Path) || opts.excluded(src.Root) {
			src.State = "excluded"
			src.Detail = "root excluded by configuration"
			cat.Sources = append(cat.Sources, src)
			continue
		}
		beforeDocs, beforeEntries := len(cat.Documents), len(cat.Entries)
		var loadErr error
		var loadNote string
		switch root.Provider {
		case model.ProviderClaudeCode:
			loadNote, loadErr = loadClaude(ctx, cat, src.Root, opts)
		case model.ProviderCodex:
			loadNote, loadErr = loadCodex(ctx, cat, src.Root, opts)
		default:
			src.State = "unreadable"
			src.Detail = fmt.Sprintf("unknown provider %q", root.Provider)
			cat.Sources = append(cat.Sources, src)
			continue
		}
		if loadErr != nil {
			return nil, loadErr
		}
		src.Documents = len(cat.Documents) - beforeDocs
		src.Entries = len(cat.Entries) - beforeEntries
		switch {
		case src.Documents == 0 && loadNote != "":
			// Traversal failed: distinguish inaccessible stores
			// from genuinely empty ones.
			src.State = "unreadable"
			src.Detail = loadNote
		case src.Documents == 0:
			src.State = "empty"
			src.Detail = "no memory documents found"
		default:
			src.State = "ok"
			src.Detail = loadNote
		}
		cat.Sources = append(cat.Sources, src)
	}
	dedupeDocuments(cat)
	resolveReferences(cat, opts)
	applyCodexConsolidatedProjects(cat, opts)
	applyProjectExclusions(cat, opts)
	// Recount documents and entries per source after dedupe and
	// per-entry exclusions.
	docRoot := map[string]string{}
	for _, d := range cat.Documents {
		docRoot[d.ID] = d.Root
	}
	for i := range cat.Sources {
		docs, n := 0, 0
		for _, d := range cat.Documents {
			if d.Root == cat.Sources[i].Root && d.Provider == cat.Sources[i].Provider {
				docs++
			}
		}
		for _, e := range cat.Entries {
			if docRoot[e.DocumentID] == cat.Sources[i].Root && e.Provider == cat.Sources[i].Provider {
				n++
			}
		}
		cat.Sources[i].Documents = docs
		cat.Sources[i].Entries = n
	}
	return cat, nil
}

// dedupeDocuments drops documents (and their entries) already loaded
// through an earlier overlapping root, keyed by provider plus
// canonical path. Entry IDs embed the document path, so duplicate
// entries share IDs and are dropped with their document.
func dedupeDocuments(cat *Catalog) {
	seen := map[string]bool{}
	dropped := map[string]bool{}
	docs := cat.Documents[:0]
	for _, d := range cat.Documents {
		key := string(d.Provider) + "|" + canonical(d.Path)
		if d.Status == "outside-root" {
			// Rejected-link records key by their lexical path:
			// canonicalizing would let a link into another store
			// shadow that store's legitimate document record.
			key = string(d.Provider) + "|outside|" + d.Path
		}
		if seen[key] {
			dropped[d.ID] = true
			continue
		}
		seen[key] = true
		docs = append(docs, d)
	}
	cat.Documents = docs
	if len(dropped) == 0 {
		return
	}
	seenEntry := map[string]bool{}
	entries := cat.Entries[:0]
	for _, e := range cat.Entries {
		if seenEntry[e.ID] {
			continue
		}
		seenEntry[e.ID] = true
		entries = append(entries, e)
	}
	cat.Entries = entries
}

// applyProjectExclusions removes entries confidently associated with
// an excluded project, plus documents of that project left with no
// entries. It runs after consolidated sections inherit rollout
// project evidence, so an excluded rollout also excludes the
// consolidated sections that reference it.
func applyProjectExclusions(cat *Catalog, opts Options) {
	if opts.Excluded == nil {
		return
	}
	entries := cat.Entries[:0]
	for _, e := range cat.Entries {
		if e.ProjectCWD != "" && opts.excluded(e.ProjectCWD) {
			continue
		}
		entries = append(entries, e)
	}
	cat.Entries = entries
	hasEntries := map[string]bool{}
	for _, e := range cat.Entries {
		hasEntries[e.DocumentID] = true
	}
	docs := cat.Documents[:0]
	for _, d := range cat.Documents {
		if d.ProjectCWD != "" && opts.excluded(d.ProjectCWD) && !hasEntries[d.ID] {
			continue
		}
		docs = append(docs, d)
	}
	cat.Documents = docs
}

// readFile reads up to maxBytes, reporting truncation and non-UTF8
// replacement in status/detail. Status is ok, truncated, unreadable.
func readFile(path string, maxBytes int64) (text, status, detail string) {
	// Reject non-regular files (FIFOs, devices) before opening:
	// os.Open on a FIFO with no writer blocks indefinitely.
	if info, err := os.Stat(path); err != nil {
		return "", "unreadable", err.Error()
	} else if !info.Mode().IsRegular() {
		return "", "unreadable", "not a regular file"
	}
	f, err := os.Open(path)
	if err != nil {
		return "", "unreadable", err.Error()
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", "unreadable", err.Error()
	}
	status = "ok"
	if int64(len(data)) > maxBytes {
		data = data[:maxBytes]
		status = "truncated"
		detail = fmt.Sprintf("file exceeds %d bytes; showing first %d bytes", maxBytes, maxBytes)
	}
	for !utf8.FullRune(data) && len(data) > 0 && status == "truncated" {
		data = data[:len(data)-1]
	}
	if !utf8.Valid(data) {
		data = []byte(strings.ToValidUTF8(string(data), "�"))
		if detail != "" {
			detail += "; "
		}
		detail += "non-UTF-8 content replaced"
	}
	return string(data), status, detail
}

// withinRoot reports whether canonical target stays inside root.
func withinRoot(root, target string) bool {
	root = canonical(root)
	target = canonical(target)
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}

// collectMarkdown walks dir recursively for .md files, skipping
// dot-directories/.git and never following directory symlinks.
// outside receives .md file symlinks whose target escapes root.
// The returned error is the first traversal failure, if any; files
// collected before it are still returned so callers can surface
// incomplete coverage instead of silently dropping results.
func collectMarkdown(dir, root string, accept func(path string) bool) (files, outside []string, err error) {
	var firstErr error
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if firstErr == nil {
				firstErr = walkErr
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != dir && d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			// WalkDir does not descend into symlinked dirs.
			if !strings.EqualFold(filepath.Ext(path), ".md") {
				return nil
			}
			target, terr := filepath.EvalSymlinks(path)
			if terr != nil {
				// A dangling link (e.g. the file was removed after
				// discovery) whose lexical target stays inside the
				// root is not an escape: surface it as a regular
				// file so reading reports it unreadable.
				link, lerr := os.Readlink(path)
				if lerr == nil {
					if !filepath.IsAbs(link) {
						link = filepath.Join(filepath.Dir(path), link)
					}
					if withinRoot(root, filepath.Clean(link)) && (accept == nil || accept(path)) {
						files = append(files, path)
						return nil
					}
				}
				outside = append(outside, path)
				return nil
			}
			if !withinRoot(root, target) {
				outside = append(outside, path)
				return nil
			}
			if info, serr := os.Stat(target); serr != nil || !info.Mode().IsRegular() {
				return nil // symlink to a non-regular file: not a document
			}
			if accept == nil || accept(path) {
				files = append(files, path)
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() || !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		if accept == nil || accept(path) {
			files = append(files, path)
		}
		return nil
	})
	if err == nil {
		err = firstErr
	}
	sort.Strings(files)
	sort.Strings(outside)
	return files, outside, err
}

func fileModTime(path string) time.Time {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	if info, err := os.Lstat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

func baseNoExt(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// idMaker assigns stable unique section IDs within one document:
// canonicalPath + headingPath + occurrenceIndex among same paths.
type idMaker struct {
	docPath string
	counts  map[string]int
}

func newIDMaker(docPath string) *idMaker {
	return &idMaker{docPath: docPath, counts: map[string]int{}}
}

func (m *idMaker) next(headingPath []string) string {
	key := strings.Join(headingPath, "/")
	idx := m.counts[key]
	m.counts[key]++
	return fmt.Sprintf("%s#%s[%d]", m.docPath, key, idx)
}

// --- references ---

var (
	linkRe    = regexp.MustCompile(`\[[^\]]*\]\(<([^>]+)>\)|\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	mdTokenRe = regexp.MustCompile(`[A-Za-z0-9_.\-/]+\.md\b`)
)

func isURL(s string) bool {
	return strings.Contains(s, "://") || strings.HasPrefix(s, "mailto:")
}

// linkReferences extracts markdown links, classified but unresolved
// (except external URLs) until resolveReferences runs.
func linkReferences(text string) []Reference {
	var refs []Reference
	seen := map[string]bool{}
	for _, m := range linkRe.FindAllStringSubmatch(text, -1) {
		target := m[1]
		if target == "" {
			target = m[2]
		}
		if target == "" || strings.HasPrefix(target, "#") || seen[target] {
			continue
		}
		seen[target] = true
		ref := Reference{Raw: target, Status: "unresolved"}
		switch {
		case isURL(target):
			ref.Kind = "url"
			ref.Status = "external"
		case strings.EqualFold(filepath.Ext(strings.SplitN(target, "#", 2)[0]), ".md"):
			ref.Kind = "note"
		default:
			ref.Kind = "path"
		}
		refs = append(refs, ref)
	}
	return refs
}

// mdTokenReferences extracts bare *.md mentions (rollout summaries
// listed in consolidated Codex memory).
func mdTokenReferences(text string, existing []Reference) []Reference {
	seen := map[string]bool{}
	for _, r := range existing {
		seen[r.Raw] = true
	}
	out := existing
	for _, tok := range mdTokenRe.FindAllString(text, -1) {
		tok = strings.Trim(tok, "`'\"")
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, Reference{Raw: tok, Kind: "note", Status: "unresolved"})
	}
	return out
}

func sessionReference(id string) Reference {
	return Reference{Raw: id, Kind: "session", SessionID: id, Status: "unresolved"}
}

// sameStoreDir reports whether two document directories belong to
// the same store: identical, or one the parent of the other (a
// Codex memories root versus its rollout_summaries directory).
// Sibling project memory directories under one shared projects
// root are different stores, so a bare-basename link missing from
// one project must not resolve into another project.
func sameStoreDir(a, b string) bool {
	a, b = canonical(a), canonical(b)
	return a == b || filepath.Dir(a) == b || filepath.Dir(b) == a
}

// uniqueBasenameMatch returns the single same-store candidate for a
// basename fallback, or "" when there is none or it is ambiguous.
func uniqueBasenameMatch(candidates []string, e *Entry, docRoot map[string]string) string {
	match := ""
	for _, p := range candidates {
		if p == e.Path || docRoot[p] != docRoot[e.Path] || !sameStoreDir(filepath.Dir(p), filepath.Dir(e.Path)) {
			continue
		}
		if match != "" {
			return ""
		}
		match = p
	}
	return match
}

// slugifyHeading converts a heading to the fragment slug
// Markdown renderers generate for it (lowercase, spaces to
// hyphens, punctuation dropped), so a link like
// MEMORY.md#deployment-notes can find its section.
func slugifyHeading(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '\t':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// resolveReferences links note references to the first entry of the
// target document when it lives inside an approved root. A
// #fragment selects the matching section entry instead, and a
// fragment that matches no section stays unresolved rather than
// opening the document's first section.
func resolveReferences(cat *Catalog, opts Options) {
	firstEntry := map[string]string{}   // canonical doc path -> entry ID
	byBasename := map[string][]string{} // basename -> canonical doc paths (same provider key added by caller scan)
	sectionBySlug := map[string]string{}
	slugCounts := map[string]int{}
	for _, e := range cat.Entries {
		if _, ok := firstEntry[e.Path]; !ok {
			firstEntry[e.Path] = e.ID
		}
		heading := e.Title
		if len(e.HeadingPath) > 0 {
			heading = e.HeadingPath[len(e.HeadingPath)-1]
		}
		base := slugifyHeading(heading)
		if base == "" {
			continue
		}
		key := e.Path + "|" + base
		n := slugCounts[key]
		slugCounts[key]++
		slug := base
		if n > 0 {
			slug = fmt.Sprintf("%s-%d", base, n)
		}
		sectionBySlug[e.Path+"#"+slug] = e.ID
	}
	docRoot := map[string]string{}
	for _, d := range cat.Documents {
		docRoot[d.Path] = d.Root
		byBasename[filepath.Base(d.Path)] = append(byBasename[filepath.Base(d.Path)], d.Path)
	}
	for i := range cat.Entries {
		e := &cat.Entries[i]
		for j := range e.References {
			ref := &e.References[j]
			if ref.Kind != "note" || ref.Status == "external" {
				continue
			}
			target, fragment, _ := strings.Cut(ref.Raw, "#")
			var cand string
			if filepath.IsAbs(target) {
				cand = canonical(target)
			} else if strings.Contains(target, "/") {
				// Explicit paths resolve only against literal
				// locations: relative to the entry's directory, its
				// root, or the root's memories directory (when the
				// root is a provider home). A missing explicit
				// target never falls back to a same-basename file
				// elsewhere — that would open the wrong note and
				// attach the wrong evidence.
				candidates := []string{canonical(filepath.Join(filepath.Dir(e.Path), filepath.FromSlash(target)))}
				if root := docRoot[e.Path]; root != "" {
					candidates = append(candidates,
						canonical(filepath.Join(root, filepath.FromSlash(target))),
						canonical(filepath.Join(root, "memories", filepath.FromSlash(target))))
				}
				for _, c := range candidates {
					if _, ok := firstEntry[c]; ok && !opts.excluded(c) {
						cand = c
						break
					}
				}
			} else {
				relDir := canonical(filepath.Join(filepath.Dir(e.Path), target))
				if _, ok := firstEntry[relDir]; ok {
					cand = relDir
				} else if !opts.excluded(relDir) {
					cand = uniqueBasenameMatch(byBasename[filepath.Base(target)], e, docRoot)
				}
			}
			if id, ok := firstEntry[cand]; ok && cand != "" {
				// Never resolve outside the entry's approved root.
				if root := docRoot[e.Path]; root == "" || withinRoot(root, cand) || docRoot[cand] == root {
					if fragment != "" {
						// A fragment must name its section; an
						// unmatched fragment stays unresolved
						// instead of opening the first section.
						if sid, ok := sectionBySlug[cand+"#"+slugifyHeading(fragment)]; ok {
							ref.EntryID = sid
							ref.Status = "resolved"
						}
					} else {
						ref.EntryID = id
						ref.Status = "resolved"
					}
				}
			}
		}
	}
}
