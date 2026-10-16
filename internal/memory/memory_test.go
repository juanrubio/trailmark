package memory_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/memory"
	"github.com/juanrubio/trailmark/internal/model"
)

func testOptions() memory.Options {
	atlasProject, _ := filepath.Abs(filepath.Join("testdata", "claude", "projects", "-work-atlas"))
	return memory.Options{
		Roots: []memory.Root{
			{Provider: model.ProviderClaudeCode, Path: filepath.Join("testdata", "claude", "projects")},
			{Provider: model.ProviderCodex, Path: filepath.Join("testdata", "codex", "memories")},
		},
		ClaudeProjectByDir: map[string]string{config.Canonical(atlasProject): "/work/atlas"},
	}
}

func load(t *testing.T, opts memory.Options) *memory.Catalog {
	t.Helper()
	cat, err := memory.Load(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func byTitle(cat *memory.Catalog, title string) []memory.Entry {
	var out []memory.Entry
	for _, e := range cat.Entries {
		if e.Title == title {
			out = append(out, e)
		}
	}
	return out
}

func TestDiscoveryCountsAndCategories(t *testing.T) {
	cat := load(t, testOptions())
	if len(cat.Documents) != 12 {
		t.Errorf("documents = %d, want 12", len(cat.Documents))
		for _, d := range cat.Documents {
			t.Logf("doc %s %s %s", d.Provider, d.Category, d.Path)
		}
	}
	if len(cat.Entries) != 17 {
		t.Errorf("entries = %d, want 17", len(cat.Entries))
		for _, e := range cat.Entries {
			t.Logf("entry %s %s %q lines %d-%d", e.Provider, e.Category, e.Title, e.StartLine, e.EndLine)
		}
	}
	counts := map[memory.Category]int{}
	for _, d := range cat.Documents {
		counts[d.Category]++
		if d.Status != "ok" {
			t.Errorf("doc %s status = %q (%s)", d.Path, d.Status, d.StatusDetail)
		}
		if strings.Contains(d.Path, "SKILL.md") || strings.Contains(d.Path, "instructions.md") {
			t.Errorf("excluded artifact catalogued: %s", d.Path)
		}
	}
	want := map[memory.Category]int{
		memory.CategoryNote: 4, memory.CategoryIndex: 1, memory.CategoryConsolidated: 1,
		memory.CategoryOverview: 1, memory.CategoryRaw: 1, memory.CategorySupporting: 4,
	}
	for c, n := range want {
		if counts[c] != n {
			t.Errorf("category %s documents = %d, want %d", c, counts[c], n)
		}
	}
	if len(memory.DefaultCategories()) != 2 {
		t.Errorf("DefaultCategories = %v", memory.DefaultCategories())
	}
	for _, s := range cat.Sources {
		if s.State != "ok" {
			t.Errorf("source %s state = %q (%s)", s.Root, s.State, s.Detail)
		}
	}
}

func TestProjectAssociation(t *testing.T) {
	cat := load(t, testOptions())
	mig := byTitle(cat, "Migration ordering")
	if len(mig) != 1 || mig[0].ProjectCWD != "/work/atlas" || mig[0].ProjectEvidence != "session-association" {
		t.Errorf("claude association = %+v", mig)
	}
	orphan := byTitle(cat, "Orphan note")
	if len(orphan) != 1 || orphan[0].ProjectCWD != "" || orphan[0].ProjectEvidence != "unknown" {
		t.Errorf("orphan must stay unknown, got %+v", orphan)
	}
	atlas := byTitle(cat, "Atlas migrations")
	if len(atlas) != 2 {
		t.Fatalf("atlas consolidated entries = %d, want 2", len(atlas))
	}
	for _, e := range atlas {
		if e.ProjectCWD != "/work/atlas" || e.ProjectEvidence != "rollout-header" {
			t.Errorf("consolidated association = %q/%q", e.ProjectCWD, e.ProjectEvidence)
		}
	}
	prefs := byTitle(cat, "General preferences")
	if len(prefs) != 1 || prefs[0].ProjectCWD != "" || prefs[0].ProjectEvidence != "unknown" {
		t.Errorf("unreferenced consolidated section must stay unknown: %+v", prefs)
	}
	summary := byTitle(cat, "Preferences")
	if len(summary) != 1 || summary[0].ProjectEvidence != "global" || summary[0].ProjectCWD != "" {
		t.Errorf("overview must be global: %+v", summary)
	}
	roll := byTitle(cat, "Beacon rollout summary")
	if len(roll) != 1 || roll[0].ProjectCWD != "/work/beacon" || roll[0].ThreadID != "thread-333" || roll[0].RecordedAt == nil {
		t.Errorf("rollout header parse = %+v", roll)
	}
	got := memory.Projects(cat)
	if strings.Join(got, ",") != "/work/atlas,/work/beacon" {
		t.Errorf("Projects = %v", got)
	}
}

func TestFrontmatterVariants(t *testing.T) {
	cat := load(t, testOptions())
	mig := byTitle(cat, "Migration ordering")[0]
	if mig.Meta["metadata.type"] != "project" || mig.Type != "project" || mig.OriginSessionID != "sess-atlas-1" {
		t.Errorf("nested frontmatter = %+v type=%q origin=%q", mig.Meta, mig.Type, mig.OriginSessionID)
	}
	if strings.Contains(mig.Text, "originSessionId") || !strings.Contains(mig.RawText, "originSessionId") {
		t.Error("Text must drop frontmatter while RawText keeps it")
	}
	nested := byTitle(cat, "Nested cache note")
	if len(nested) != 1 || nested[0].Description != "Cache invalidation rules" || nested[0].Type != "reference" {
		t.Errorf("quoted/top-level frontmatter = %+v", nested)
	}
	plain := byTitle(cat, "Plain deployment note")
	if len(plain) != 1 || plain[0].Type != "" || len(plain[0].Meta) != 0 {
		t.Errorf("plain note without frontmatter = %+v", plain)
	}
	// Session reference from frontmatter.
	found := false
	for _, r := range mig.References {
		if r.Kind == "session" && r.SessionID == "sess-atlas-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("migration refs = %+v", mig.References)
	}
}

func TestIndexLinksResolvedUnresolvedExternal(t *testing.T) {
	cat := load(t, testOptions())
	idx := byTitle(cat, "Atlas memory index")
	if len(idx) != 1 {
		t.Fatalf("index entries = %d", len(idx))
	}
	byRaw := map[string]memory.Reference{}
	for _, r := range idx[0].References {
		byRaw[r.Raw] = r
	}
	migID := byTitle(cat, "Migration ordering")[0].ID
	if r := byRaw["migrations.md"]; r.Status != "resolved" || r.EntryID != migID || r.Kind != "note" {
		t.Errorf("migrations link = %+v, want resolved to %s", r, migID)
	}
	if r := byRaw["missing.md"]; r.Status != "unresolved" || r.EntryID != "" {
		t.Errorf("missing link = %+v", r)
	}
	if r := byRaw["https://example.com/docs"]; r.Status != "external" || r.Kind != "url" {
		t.Errorf("external link = %+v", r)
	}
}

func TestSectionSplitsLineSpansAndDuplicateIDs(t *testing.T) {
	cat := load(t, testOptions())
	ids := map[string]bool{}
	for _, e := range cat.Entries {
		if ids[e.ID] {
			t.Errorf("duplicate entry ID %s", e.ID)
		}
		ids[e.ID] = true
		if e.StartLine < 1 || e.EndLine < e.StartLine {
			t.Errorf("bad span for %q: %d-%d", e.Title, e.StartLine, e.EndLine)
		}
	}
	atlas := byTitle(cat, "Atlas migrations")
	if len(atlas) != 2 || atlas[0].ID == atlas[1].ID {
		t.Fatalf("duplicate headings must get distinct IDs: %+v", atlas)
	}
	if atlas[0].StartLine != 3 || atlas[0].EndLine != 10 {
		t.Errorf("first atlas section span = %d-%d, want 3-10", atlas[0].StartLine, atlas[0].EndLine)
	}
	if len(atlas[0].HeadingPath) != 1 || atlas[0].HeadingPath[0] != "Atlas migrations" {
		t.Errorf("heading path = %v", atlas[0].HeadingPath)
	}
	if !strings.HasPrefix(atlas[0].RawText, "# Atlas migrations") || strings.HasPrefix(atlas[0].Text, "# Atlas") {
		t.Error("RawText keeps the heading, Text drops it")
	}
	raw := byTitle(cat, "Thread `thread-111`")
	if len(raw) != 1 || raw[0].ThreadID != "thread-111" || raw[0].Category != memory.CategoryRaw {
		t.Errorf("raw thread = %+v", raw)
	}
	// Codex consolidated rollout reference resolves to rollout entry.
	rollID := byTitle(cat, "Atlas rollout summary")[0].ID
	resolved := false
	for _, r := range atlas[0].References {
		if r.Status == "resolved" && r.EntryID == rollID {
			resolved = true
		}
	}
	if !resolved {
		t.Errorf("atlas refs = %+v, want resolved rollout %s", atlas[0].References, rollID)
	}
}

func TestMissingAndEmptyRoots(t *testing.T) {
	empty := t.TempDir()
	cat := load(t, memory.Options{Roots: []memory.Root{
		{Provider: model.ProviderClaudeCode, Path: filepath.Join(empty, "nope")},
		{Provider: model.ProviderCodex, Path: empty},
	}})
	if len(cat.Sources) != 2 || cat.Sources[0].State != "missing" || cat.Sources[1].State != "empty" {
		t.Errorf("sources = %+v", cat.Sources)
	}
}

func TestExclusionByPathAndProject(t *testing.T) {
	opts := testOptions()
	opts.Excluded = func(p string) bool {
		return strings.HasSuffix(p, "plain.md") || p == "/work/atlas"
	}
	cat := load(t, opts)
	for _, e := range cat.Entries {
		if strings.HasSuffix(e.Path, "plain.md") {
			t.Error("path-excluded note present")
		}
		if e.ProjectCWD == "/work/atlas" {
			t.Errorf("project-excluded entry present: %q (%s)", e.Title, e.Category)
		}
	}
	// Consolidated doc itself survives with its unknown sections.
	if len(byTitle(cat, "General preferences")) != 1 {
		t.Error("non-excluded consolidated section must survive")
	}
	if len(byTitle(cat, "Beacon rollout summary")) != 1 {
		t.Error("beacon rollout must survive atlas exclusion")
	}
}

func TestSymlinkOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "proj", "memory")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "secret.md")
	if err := os.WriteFile(outside, []byte("# Secret\noutside body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mem, "ok.md"), []byte("# Ok note\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(mem, "linked.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: filepath.Join(dir, "proj")}}})
	// proj dir holds memory/ directly? root=proj has */memory? No:
	// memory is a direct child dir, so use the memory dir root too.
	if len(cat.Documents) == 0 {
		cat = load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: mem}}})
	}
	var outsideDoc *memory.Document
	for i := range cat.Documents {
		if cat.Documents[i].Status == "outside-root" {
			outsideDoc = &cat.Documents[i]
		}
	}
	if outsideDoc == nil {
		t.Fatalf("no outside-root document in %+v", cat.Documents)
	}
	for _, e := range cat.Entries {
		if strings.Contains(e.Text, "outside body") {
			t.Error("outside-root content was read")
		}
	}
	if len(byTitle(cat, "Ok note")) != 1 {
		t.Error("regular note missing")
	}
}

func TestTruncatedAndNonUTF8(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memory")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	big := "# Big note\n" + strings.Repeat("fictional filler line\n", 500)
	if err := os.WriteFile(filepath.Join(mem, "big.md"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mem, "bad.md"), []byte("# Bad bytes\n\xff\xfe body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cat := load(t, memory.Options{
		Roots:        []memory.Root{{Provider: model.ProviderClaudeCode, Path: mem}},
		MaxFileBytes: 1024,
	})
	byPath := map[string]memory.Document{}
	for _, d := range cat.Documents {
		byPath[filepath.Base(d.Path)] = d
	}
	if d := byPath["big.md"]; d.Status != "truncated" || !strings.Contains(d.StatusDetail, "1024") {
		t.Errorf("big doc = %+v", d)
	}
	if d := byPath["bad.md"]; !strings.Contains(d.StatusDetail, "non-UTF-8") {
		t.Errorf("bad doc = %+v", d)
	}
	if len(byTitle(cat, "Big note")) != 1 {
		t.Error("truncated file must still parse its prefix")
	}
}

func TestSearchGroupingAndSnippets(t *testing.T) {
	cat := load(t, testOptions())
	if got := memory.Search(cat, "", memory.Filter{}); got != nil {
		t.Errorf("empty query = %v, want nil", got)
	}
	matches := memory.Search(cat, "QUOKKA", memory.Filter{})
	if len(matches) != 3 {
		t.Fatalf("quokka matches = %d, want 3 (claude note, consolidated, raw)", len(matches))
	}
	for _, m := range matches {
		if m.Count < 1 || len(m.Lines) == 0 || len(m.Snippets) == 0 {
			t.Errorf("match %+v", m)
		}
		if !strings.Contains(strings.ToLower(strings.Join(m.Snippets, " ")), "quokka") {
			t.Errorf("snippets lack term: %v", m.Snippets)
		}
	}
	// Source line for the claude note: quokka is on file line 11.
	for _, m := range matches {
		if m.Entry.Title == "Migration ordering" {
			if len(m.Lines) != 1 || m.Lines[0] != 11 {
				t.Errorf("migration match lines = %v, want [11]", m.Lines)
			}
		}
		if m.Entry.Title == "Atlas migrations" && (len(m.Lines) != 1 || m.Lines[0] != 5) {
			t.Errorf("consolidated match lines = %v, want [5]", m.Lines)
		}
	}
	only := memory.Search(cat, "quokka", memory.Filter{Provider: model.ProviderClaudeCode})
	if len(only) != 1 {
		t.Errorf("provider-scoped search = %d, want 1", len(only))
	}
}

func TestFilterSortAndScopes(t *testing.T) {
	cat := load(t, testOptions())
	def := memory.FilterEntries(cat, memory.Filter{Categories: memory.DefaultCategories()})
	for _, e := range def {
		if e.Category != memory.CategoryNote && e.Category != memory.CategoryConsolidated {
			t.Errorf("default filter leaked %s", e.Category)
		}
	}
	if len(def) != 8 { // 4 notes + 4 consolidated sections
		t.Errorf("default entries = %d, want 8", len(def))
	}
	atlas := memory.FilterEntries(cat, memory.Filter{Project: "/work/atlas"})
	for _, e := range atlas {
		if e.ProjectCWD != "/work/atlas" && e.ProjectEvidence != "global" {
			t.Errorf("project filter leaked %q cwd=%q ev=%q", e.Title, e.ProjectCWD, e.ProjectEvidence)
		}
	}
	unknown := memory.FilterEntries(cat, memory.Filter{Project: "unknown"})
	for _, e := range unknown {
		if e.ProjectCWD != "" {
			t.Errorf("unknown filter leaked %q", e.Title)
		}
	}
	text := memory.FilterEntries(cat, memory.Filter{Text: "cache"})
	if len(text) != 1 || text[0].Title != "Nested cache note" {
		t.Errorf("text filter = %+v", text)
	}
	// Sort: ModTime desc then title asc — synthetic catalog.
	synth := &memory.Catalog{Entries: []memory.Entry{
		{ID: "b", Title: "Beta"},
		{ID: "a", Title: "Alpha"},
		{ID: "c", Title: "Alpha"},
	}}
	got := memory.FilterEntries(synth, memory.Filter{})
	if got[0].ID != "a" || got[1].ID != "c" || got[2].ID != "b" {
		t.Errorf("sort order = %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestLoadIsReadOnly(t *testing.T) {
	hashTree := func() map[string]string {
		out := map[string]string{}
		err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			out[p] = hex.EncodeToString(sum[:])
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := hashTree()
	load(t, testOptions())
	after := hashTree()
	if len(before) != len(after) {
		t.Fatalf("file inventory changed: %d -> %d", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Errorf("file changed during Load: %s", p)
		}
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := memory.Load(ctx, testOptions()); err == nil {
		t.Error("cancelled context must return an error")
	}
}

func TestDirectRootsAndCodexHome(t *testing.T) {
	// Direct claude memory dir.
	cat := load(t, memory.Options{Roots: []memory.Root{
		{Provider: model.ProviderClaudeCode, Path: filepath.Join("testdata", "claude", "projects", "-work-atlas", "memory")},
	}})
	if len(cat.Documents) != 4 {
		t.Errorf("direct memory dir docs = %d, want 4", len(cat.Documents))
	}
	// Codex home (parent of memories/).
	cat = load(t, memory.Options{Roots: []memory.Root{
		{Provider: model.ProviderCodex, Path: filepath.Join("testdata", "codex")},
	}})
	if len(cat.Documents) != 7 || len(cat.Entries) != 12 {
		t.Errorf("codex home docs/entries = %d/%d, want 7/12", len(cat.Documents), len(cat.Entries))
	}
}
