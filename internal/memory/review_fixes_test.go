package memory_test

// Regression tests for the structured-review findings fixed during
// the reviewer pass: exclusion evidence ordering, non-regular files,
// overlapping roots, and traversal-failure coverage.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/memory"
	"github.com/juanrubio/trailmark/internal/model"
)

func TestProjectExclusionHidesConsolidatedViaRollout(t *testing.T) {
	opts := testOptions()
	opts.Excluded = func(p string) bool { return p == "/work/atlas" }
	cat := load(t, opts)
	if got := byTitle(cat, "Atlas migrations"); len(got) != 0 {
		t.Errorf("consolidated sections of excluded project visible: %d", len(got))
	}
	if got := byTitle(cat, "Atlas rollout summary"); len(got) != 0 {
		t.Error("rollout summary of excluded project visible")
	}
	for _, d := range cat.Documents {
		if strings.HasSuffix(d.Path, "2026-01-01-atlas.md") {
			t.Error("excluded rollout document still listed")
		}
	}
	for _, m := range memory.Search(cat, "quokka", memory.Filter{}) {
		if m.Entry.Category == memory.CategoryConsolidated {
			t.Error("excluded consolidated text still searchable")
		}
	}
	if len(byTitle(cat, "General preferences")) != 1 {
		t.Error("unrelated consolidated section must survive")
	}
}

func TestFIFODoesNotBlockLoad(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memory")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mem, "note.md"), []byte("# Fictional note\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(mem, "blocked.md"), 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: mem}}})
	if len(byTitle(cat, "Fictional note")) != 1 {
		t.Fatalf("regular note missing, entries=%v", cat.Entries)
	}
	for _, e := range cat.Entries {
		if strings.HasSuffix(e.Path, "blocked.md") {
			t.Error("FIFO presented as a memory entry")
		}
	}
}

func TestOverlappingRootsDedupeDocuments(t *testing.T) {
	opts := testOptions()
	opts.Roots = append(opts.Roots, memory.Root{
		Provider: model.ProviderClaudeCode,
		Path:     filepath.Join("testdata", "claude", "projects", "-work-atlas", "memory"),
	})
	cat := load(t, opts)
	seen := map[string]int{}
	for _, d := range cat.Documents {
		seen[d.Path]++
		if seen[d.Path] > 1 {
			t.Errorf("document loaded twice: %s", d.Path)
		}
	}
	seenEntry := map[string]int{}
	for _, e := range cat.Entries {
		seenEntry[e.ID]++
		if seenEntry[e.ID] > 1 {
			t.Errorf("entry loaded twice: %s", e.ID)
		}
	}
	if len(cat.Documents) != 12 || len(cat.Entries) != 17 {
		t.Errorf("counts = %d docs / %d entries, want 12/17 as without overlap", len(cat.Documents), len(cat.Entries))
	}
}

func TestTraversalFailureSurfacedInCoverage(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not apply to root")
	}
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	sub := filepath.Join(mem, "rollout_summaries")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mem, "memory_summary.md"), []byte("## Overview\n\nFictional.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "hidden.md"), []byte("# Hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })
	if _, err := os.ReadDir(sub); err == nil {
		t.Skip("directory still readable despite chmod")
	}
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderCodex, Path: mem}}})
	if len(cat.Sources) != 1 {
		t.Fatalf("sources = %+v", cat.Sources)
	}
	src := cat.Sources[0]
	if src.Detail == "" {
		t.Errorf("traversal failure not surfaced: %+v", src)
	}
	if len(byTitle(cat, "Hidden")) != 0 {
		t.Error("unreadable subtree content must not appear")
	}
}

func TestClaudeAssociationFromSessionLogPeek(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-work-secret")
	writeFile(t, filepath.Join(proj, "session-1.jsonl"),
		"{\"type\":\"summary\"}\n{\"type\":\"user\",\"cwd\":\"/work/secret\",\"message\":{}}\n")
	writeFile(t, filepath.Join(proj, "memory", "note.md"), "# Secret project note\n\nFictional.\n")
	roots := []memory.Root{{Provider: model.ProviderClaudeCode, Path: filepath.Join(dir, "projects")}}

	// No index association supplied: the recorded cwd in the
	// project's own session log still associates the store.
	cat := load(t, memory.Options{Roots: roots})
	got := byTitle(cat, "Secret project note")
	if len(got) != 1 || got[0].ProjectCWD != "/work/secret" || got[0].ProjectEvidence != "session-association" {
		t.Fatalf("peek association failed: %+v", got)
	}
	// And exclusion keeps working without any index rows.
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/secret" }})
	if len(byTitle(cat, "Secret project note")) != 0 {
		t.Error("excluded Claude project memories resurfaced as Unknown")
	}
}

// encodeClaudeDir mirrors Claude Code's project-directory naming
// (every non-alphanumeric byte of the cwd becomes a dash) so tests
// can build realistic stores under a temporary tree.
func encodeClaudeDir(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

func TestClaudeAssociationFromDirectoryName(t *testing.T) {
	base := t.TempDir()
	project := filepath.Join(base, "repos", "adrian-laptop")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	// Memory-only store: no session logs, no index association.
	projDir := filepath.Join(base, "projects", encodeClaudeDir(project))
	writeFile(t, filepath.Join(projDir, "memory", "machine.md"), "# Machine note\n\nFictional.\n")
	roots := []memory.Root{{Provider: model.ProviderClaudeCode, Path: filepath.Join(base, "projects")}}

	cat := load(t, memory.Options{Roots: roots})
	got := byTitle(cat, "Machine note")
	if len(got) != 1 || got[0].ProjectCWD != project || got[0].ProjectEvidence != "directory-name" {
		t.Fatalf("directory-name association failed: %+v (want cwd %s)", got, project)
	}
	// The decoded project is still excludable.
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == project }})
	if len(byTitle(cat, "Machine note")) != 0 {
		t.Error("excluded decoded project memories resurfaced")
	}
}

func TestClaudeDirectoryNameAmbiguousStaysUnknown(t *testing.T) {
	base := t.TempDir()
	// Both readings of the encoded name exist on disk.
	for _, p := range []string{filepath.Join(base, "a-b", "c"), filepath.Join(base, "a", "b-c")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	projDir := filepath.Join(base, "projects", encodeClaudeDir(filepath.Join(base, "a-b", "c")))
	writeFile(t, filepath.Join(projDir, "memory", "note.md"), "# Ambiguous note\n\nFictional.\n")
	roots := []memory.Root{{Provider: model.ProviderClaudeCode, Path: filepath.Join(base, "projects")}}
	cat := load(t, memory.Options{Roots: roots})
	got := byTitle(cat, "Ambiguous note")
	if len(got) != 1 || got[0].ProjectCWD != "" || got[0].ProjectEvidence != "unknown" {
		t.Fatalf("ambiguous directory name must stay Unknown: %+v", got)
	}
}

func TestClaudeDirectoryNameMissingStaysUnknown(t *testing.T) {
	base := t.TempDir()
	// The encoded path does not exist on disk (project moved away).
	projDir := filepath.Join(base, "projects", encodeClaudeDir(filepath.Join(base, "gone", "project")))
	writeFile(t, filepath.Join(projDir, "memory", "note.md"), "# Gone note\n\nFictional.\n")
	roots := []memory.Root{{Provider: model.ProviderClaudeCode, Path: filepath.Join(base, "projects")}}
	cat := load(t, memory.Options{Roots: roots})
	got := byTitle(cat, "Gone note")
	if len(got) != 1 || got[0].ProjectCWD != "" || got[0].ProjectEvidence != "unknown" {
		t.Fatalf("unverifiable directory name must stay Unknown: %+v", got)
	}
}

func TestRolloutEvidenceScopedPerRoot(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, cwd, title string }{
		{"storea", "/work/a", "Store A section"},
		{"storeb", "/work/b", "Store B section"},
	} {
		mem := filepath.Join(dir, tc.name, "memories")
		writeFile(t, filepath.Join(mem, "MEMORY.md"),
			"# "+tc.title+"\n\nSee rollout_summaries/same.md for detail.\n")
		writeFile(t, filepath.Join(mem, "rollout_summaries", "same.md"),
			"thread_id: t-"+tc.name+"\ncwd: "+tc.cwd+"\n\n# Same name summary\n")
	}
	roots := []memory.Root{
		{Provider: model.ProviderCodex, Path: filepath.Join(dir, "storea", "memories")},
		{Provider: model.ProviderCodex, Path: filepath.Join(dir, "storeb", "memories")},
	}
	cat := load(t, memory.Options{Roots: roots})
	for title, want := range map[string]string{"Store A section": "/work/a", "Store B section": "/work/b"} {
		got := byTitle(cat, title)
		if len(got) != 1 || got[0].ProjectCWD != want {
			t.Errorf("%s associated with %v, want cwd %s", title, got, want)
		}
	}
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/b" }})
	if len(byTitle(cat, "Store B section")) != 0 {
		t.Error("excluded store B section visible (evidence leaked across roots)")
	}
	if len(byTitle(cat, "Store A section")) != 1 {
		t.Error("store A section wrongly excluded")
	}
}

func TestMissingClaudeLinkNotResolvedCrossProject(t *testing.T) {
	dir := t.TempDir()
	projects := filepath.Join(dir, "projects")
	writeFile(t, filepath.Join(projects, "-work-a", "memory", "MEMORY.md"),
		"# Index A\n\n- [Deploy](deploy.md)\n")
	writeFile(t, filepath.Join(projects, "-work-b", "memory", "MEMORY.md"),
		"# Index B\n\n- [Deploy](deploy.md)\n")
	writeFile(t, filepath.Join(projects, "-work-b", "memory", "deploy.md"), "# B deploy note\n")
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: projects}}})
	for _, title := range []string{"Index A", "Index B"} {
		got := byTitle(cat, title)
		if len(got) != 1 || len(got[0].References) == 0 {
			t.Fatalf("%s references missing: %+v", title, got)
		}
		status := got[0].References[0].Status
		if title == "Index A" && status == "resolved" {
			t.Error("project A missing link resolved into project B")
		}
		if title == "Index B" && status != "resolved" {
			t.Errorf("project B same-store link should resolve, got %q", status)
		}
	}
}

func TestMixedProjectSectionExcludedViaMinorityEvidence(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "MEMORY.md"),
		"# Mixed section\n\nMostly A: rollout_summaries/a1.md and rollout_summaries/a2.md, plus rollout_summaries/b1.md.\n\n# A only section\n\nOnly rollout_summaries/a1.md here.\n")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "a1.md"), "thread_id: ta1\ncwd: /work/a\n\n# A1\n")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "a2.md"), "thread_id: ta2\ncwd: /work/a\n\n# A2\n")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "b1.md"), "thread_id: tb1\ncwd: /work/b\n\n# B1\n")
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Mixed section"); len(got) != 1 || got[0].ProjectCWD != "/work/a" {
		t.Fatalf("display association should be majority A without exclusions: %+v", got)
	}
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/b" }})
	if len(byTitle(cat, "Mixed section")) != 0 {
		t.Error("section evidencing excluded project B (minority) still visible")
	}
	if len(byTitle(cat, "A only section")) != 1 {
		t.Error("A-only section wrongly excluded")
	}
}

func TestFilterProjectRelatedMatchesNested(t *testing.T) {
	cat := &memory.Catalog{Entries: []memory.Entry{
		{ID: "1", Title: "root", ProjectCWD: "/work/repo"},
		{ID: "2", Title: "nested", ProjectCWD: "/work/repo/sub/dir"},
		{ID: "3", Title: "sibling", ProjectCWD: "/work/repo-other"},
	}}
	exact := memory.FilterEntries(cat, memory.Filter{Project: "/work/repo"})
	if len(exact) != 1 || exact[0].ID != "1" {
		t.Fatalf("exact filter = %+v", exact)
	}
	related := memory.FilterEntries(cat, memory.Filter{Project: "/work/repo", ProjectRelated: true})
	if len(related) != 2 {
		t.Fatalf("related filter must include nested dirs but not siblings: %+v", related)
	}
}

func TestSearchLinesUseParserOffsetNotFirstOccurrence(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memory")
	writeFile(t, filepath.Join(mem, "note.md"), "---\nname: needle\n---\nneedle\n")
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: mem}}})
	matches := memory.Search(cat, "needle", memory.Filter{})
	// Search covers the raw source: the frontmatter occurrence
	// (line 2) and the body occurrence (line 4) both appear, each
	// at its own source line — never collapsed onto line 2.
	if len(matches) != 1 || len(matches[0].Lines) != 2 || matches[0].Lines[0] != 2 || matches[0].Lines[1] != 4 {
		t.Fatalf("matches must carry both source lines 2 and 4: %+v", matches)
	}
}

func TestCrossStoreSymlinkDoesNotImportContent(t *testing.T) {
	dir := t.TempDir()
	projects := filepath.Join(dir, "projects")
	writeFile(t, filepath.Join(projects, "-work-b", "memory", "secret.md"), "# B secret\n\nsecretbody marker\n")
	writeFile(t, filepath.Join(projects, "-work-b", "s.jsonl"), "{\"cwd\":\"/work/b\"}\n")
	aMem := filepath.Join(projects, "-work-a", "memory")
	writeFile(t, filepath.Join(aMem, "own.md"), "# A own note\n")
	if err := os.Symlink(filepath.Join(projects, "-work-b", "memory", "secret.md"), filepath.Join(aMem, "link.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	roots := []memory.Root{{Provider: model.ProviderClaudeCode, Path: projects}}
	cat := load(t, memory.Options{Roots: roots})
	if got := memory.Search(cat, "secretbody", memory.Filter{}); len(got) != 1 {
		t.Fatalf("secret must appear once (via its own store), got %d matches", len(got))
	}
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/b" }})
	if got := memory.Search(cat, "secretbody", memory.Filter{}); len(got) != 0 {
		t.Error("excluded project B content leaked through project A symlink")
	}
	if len(byTitle(cat, "A own note")) != 1 {
		t.Error("project A note missing")
	}
}

func TestCodexSymlinkAliasKeepsRolloutEvidence(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "private.md"),
		"thread_id: tp\ncwd: /work/private\n\n# Private summary\n\nprivatebody marker\n")
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "# Consolidated\n\nSee rollout_summaries/private.md.\n")
	if err := os.Symlink(filepath.Join(mem, "rollout_summaries", "private.md"), filepath.Join(mem, "alias.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Private summary"); len(got) != 1 || got[0].ProjectCWD != "/work/private" {
		t.Fatalf("alias must not shadow rollout evidence: %+v", got)
	}
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/private" }})
	if got := memory.Search(cat, "privatebody", memory.Filter{}); len(got) != 0 {
		t.Error("excluded rollout content visible through root-level alias")
	}
	if len(byTitle(cat, "Consolidated")) != 0 {
		t.Error("consolidated section evidencing excluded rollout still visible")
	}
}

func TestTildeAndLongFencesDoNotSplitSections(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "MEMORY.md"), strings.Join([]string{
		"# Real section",
		"",
		"See rollout_summaries/r1.md.",
		"",
		"~~~markdown",
		"# Example heading inside tilde fence",
		"~~~",
		"",
		"````",
		"```",
		"# Example heading inside long fence",
		"````",
		"",
		"Trailing body.",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(mem, "rollout_summaries", "r1.md"), "thread_id: tr1\ncwd: /work/fenced\n\n# R1\n")
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if len(byTitle(cat, "Example heading inside tilde fence")) != 0 || len(byTitle(cat, "Example heading inside long fence")) != 0 {
		t.Fatal("fenced headings became sections")
	}
	got := byTitle(cat, "Real section")
	if len(got) != 1 || !strings.Contains(got[0].Text, "Trailing body.") {
		t.Fatalf("real section must contain the fenced examples: %+v", got)
	}
	// The whole section inherits the rollout evidence, so excluding
	// the project hides the fenced content with it.
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/fenced" }})
	if got := memory.Search(cat, "Example heading", memory.Filter{}); len(got) != 0 {
		t.Error("fenced content of excluded project still searchable")
	}
}

func TestPeekBoundedToFirstThreeLogs(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-work-bound")
	for _, name := range []string{"a.jsonl", "b.jsonl", "c.jsonl"} {
		writeFile(t, filepath.Join(proj, name), "{\"type\":\"user\"}\n")
	}
	writeFile(t, filepath.Join(proj, "d.jsonl"), "{\"cwd\":\"/work/bound\"}\n")
	writeFile(t, filepath.Join(proj, "memory", "note.md"), "# Bound note\n")
	roots := []memory.Root{{Provider: model.ProviderClaudeCode, Path: filepath.Join(dir, "projects")}}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Bound note"); len(got) != 1 || got[0].ProjectCWD != "" {
		t.Fatalf("peek must stop after 3 logs, got association %+v", got)
	}
	// A cwd inside the bound is found.
	writeFile(t, filepath.Join(proj, "b.jsonl"), "{\"cwd\":\"/work/bound\"}\n")
	cat = load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Bound note"); len(got) != 1 || got[0].ProjectCWD != "/work/bound" {
		t.Fatalf("peek within bound failed: %+v", got)
	}
}

func TestRootsFromConfigCodexHomeRoot(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "codexhome")
	writeFile(t, filepath.Join(home, "memories", "MEMORY.md"), "# Fictional\n")
	writeFile(t, filepath.Join(home, "sessions", "s.jsonl"), "{}\n")
	sibling := filepath.Join(dir, "memories") // unrelated adjacent store
	writeFile(t, filepath.Join(sibling, "MEMORY.md"), "# Unrelated\n")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "no-claude"))
	t.Setenv("CODEX_HOME", "")
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderCodex, Path: home}}}
	roots := memory.RootsFromConfig(cfg)
	want := config.Canonical(filepath.Join(home, "memories"))
	found := false
	for _, r := range roots {
		if r.Path == want {
			found = true
		}
		if r.Path == config.Canonical(sibling) {
			t.Error("codex home root derived the unrelated sibling memories directory")
		}
	}
	if !found {
		t.Fatalf("home memories not derived: %+v", roots)
	}
}

func TestPathExcludedRolloutDoesNotRevealConsolidated(t *testing.T) {
	rollDir, _ := filepath.Abs(filepath.Join("testdata", "codex", "memories", "rollout_summaries"))
	for _, tc := range []struct {
		name       string
		exclusions []string
	}{
		{"project only", []string{"/work/atlas"}},
		{"project plus rollout dir", []string{"/work/atlas", rollDir}},
		{"rollout dir only", []string{rollDir}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Exclusions: tc.exclusions}
			opts := testOptions()
			opts.Excluded = cfg.Excluded
			cat := load(t, opts)
			if got := byTitle(cat, "Atlas migrations"); len(got) != 0 {
				t.Errorf("atlas consolidated sections visible with exclusions %v", tc.exclusions)
			}
		})
	}
}

func TestRootsFromConfigArbitrarySessionRootName(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "backup", "codex-logs")
	writeFile(t, filepath.Join(logs, "loose.md"), "# Not a memory\n")
	writeFile(t, filepath.Join(dir, "backup", "memories", "MEMORY.md"), "# Fictional\n")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "no-claude"))
	t.Setenv("CODEX_HOME", "")
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderCodex, Path: logs}}}
	roots := memory.RootsFromConfig(cfg)
	want := config.Canonical(filepath.Join(dir, "backup", "memories"))
	found := false
	for _, r := range roots {
		if r.Path == want {
			found = true
		}
		if r.Path == config.Canonical(logs) {
			t.Error("session log directory itself treated as a memory store")
		}
	}
	if !found {
		t.Fatalf("sibling memories not derived for arbitrary session root: %+v", roots)
	}
}

func TestAliasReferenceCarriesTargetEvidence(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "private.md"),
		"thread_id: tp\ncwd: /work/private\n\n# Private summary\n")
	if err := os.Symlink(filepath.Join(mem, "rollout_summaries", "private.md"), filepath.Join(mem, "alias.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "# Via alias\n\nSee [the summary](alias.md) for detail.\n")
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Via alias"); len(got) != 1 || got[0].ProjectCWD != "/work/private" {
		t.Fatalf("resolved alias must carry target evidence: %+v", got)
	}
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/private" }})
	if len(byTitle(cat, "Via alias")) != 0 {
		t.Error("section referencing excluded rollout via alias still visible")
	}
}

func TestRejectedLinkDoesNotShadowRealDocument(t *testing.T) {
	dir := t.TempDir()
	projects := filepath.Join(dir, "projects")
	bMem := filepath.Join(projects, "-work-b", "memory")
	writeFile(t, filepath.Join(bMem, "real.md"), "# B real note\n\nBody.\n")
	aMem := filepath.Join(projects, "-work-a", "memory")
	writeFile(t, filepath.Join(aMem, "own.md"), "# A note\n")
	if err := os.Symlink(filepath.Join(bMem, "real.md"), filepath.Join(aMem, "link.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: projects}}})
	docIDs := map[string]bool{}
	for _, d := range cat.Documents {
		docIDs[d.ID] = true
	}
	for _, e := range cat.Entries {
		if !docIDs[e.DocumentID] {
			t.Errorf("entry %s references missing document %s (shadowed by rejected link)", e.ID, e.DocumentID)
		}
	}
	if len(byTitle(cat, "B real note")) != 1 {
		t.Error("B's real note must remain browsable via its own store")
	}
}

func TestExcludedExplicitTargetGetsNoBasenameFallback(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "private", "shared.md"),
		"thread_id: tp\ncwd: /work/private\n\n# Private shared\n\nprivatemarker\n")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "shared.md"),
		"thread_id: tu\ncwd: /work/public\n\n# Public shared\n")
	writeFile(t, filepath.Join(mem, "MEMORY.md"),
		"# Section citing private\n\nSee rollout_summaries/private/shared.md for the privatemarker detail.\n")
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Section citing private"); len(got) != 1 || got[0].ProjectCWD != "/work/private" {
		t.Fatalf("explicit target must supply evidence: %+v", got)
	}
	cfg := &config.Config{Exclusions: []string{filepath.Join(mem, "rollout_summaries", "private")}}
	cat = load(t, memory.Options{Roots: roots, Excluded: cfg.Excluded})
	if len(byTitle(cat, "Section citing private")) != 0 {
		t.Error("section referencing excluded explicit target fell back to same-basename public file")
	}
	if got := memory.Search(cat, "privatemarker", memory.Filter{}); len(got) != 0 {
		t.Error("excluded rollout text remains searchable via consolidated section")
	}
}

func TestSearchFindsRecordedMetadataInRawSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "memory", "note.md"),
		"---\nname: Meta note\noriginSessionId: sess-xyz-123\n---\nBody text.\n")
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: filepath.Join(dir, "memory")}}})
	matches := memory.Search(cat, "sess-xyz-123", memory.Filter{})
	if len(matches) != 1 || len(matches[0].Lines) != 1 || matches[0].Lines[0] != 3 {
		t.Fatalf("frontmatter metadata must be searchable at its source line: %+v", matches)
	}
}

func TestAbsoluteExcludedTargetSuppressed(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	priv := filepath.Join(mem, "rollout_summaries", "private", "x.md")
	writeFile(t, priv, "thread_id: tp\ncwd: /work/private\n\n# Private abs\n\nabsmarker\n")
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "# Abs section\n\nSee "+priv+" for the absmarker detail.\n")
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Abs section"); len(got) != 1 || got[0].ProjectCWD != "/work/private" {
		t.Fatalf("absolute reference must carry evidence: %+v", got)
	}
	cfg := &config.Config{Exclusions: []string{filepath.Join(mem, "rollout_summaries", "private")}}
	cat = load(t, memory.Options{Roots: roots, Excluded: cfg.Excluded})
	if len(byTitle(cat, "Abs section")) != 0 {
		t.Error("section referencing excluded absolute target still visible")
	}
}

func TestIndentedCodeHeadingDoesNotSplit(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "MEMORY.md"), strings.Join([]string{
		"# Deploy section",
		"",
		"See rollout_summaries/r1.md.",
		"",
		"    # Private deployment command",
		"    run --private",
		"",
		"After the code.",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(mem, "rollout_summaries", "r1.md"), "thread_id: tr1\ncwd: /work/ind\n\n# R1\n")
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if len(byTitle(cat, "Private deployment command")) != 0 {
		t.Fatal("indented code comment became a section")
	}
	if got := byTitle(cat, "Deploy section"); len(got) != 1 || !strings.Contains(got[0].Text, "After the code.") {
		t.Fatalf("deploy section must contain the indented code: %+v", got)
	}
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/ind" }})
	if got := memory.Search(cat, "Private deployment", memory.Filter{}); len(got) != 0 {
		t.Error("indented code of excluded project still searchable")
	}
}

func TestSearchCountsOccurrencesOnce(t *testing.T) {
	cat := &memory.Catalog{Entries: []memory.Entry{{
		ID: "1", Title: "Needle", Description: "A note",
		StartLine: 1, Text: "Body only.", RawText: "---\nname: Needle\ndescription: A note\n---\nBody only.",
	}}}
	matches := memory.Search(cat, "needle", memory.Filter{})
	if len(matches) != 1 || matches[0].Count != 1 || len(matches[0].Lines) != 1 {
		t.Fatalf("single source occurrence must count once: %+v", matches)
	}
}

func TestMissingExplicitPathStaysUnresolved(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memory")
	writeFile(t, filepath.Join(mem, "deploy.md"), "# Root deploy note\n")
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "# Index\n\n- [Archived deploy](archive/deploy.md)\n")
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: mem}}})
	got := byTitle(cat, "Index")
	if len(got) != 1 || len(got[0].References) == 0 {
		t.Fatalf("index entry missing: %+v", got)
	}
	if got[0].References[0].Status == "resolved" {
		t.Error("missing explicit archive/deploy.md resolved to unrelated root deploy.md")
	}
}

func TestNestedRootDoesNotShadowEvidence(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "store", "memories")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "r1.md"),
		"thread_id: tr1\ncwd: /work/nested\n\n# Nested summary\n\nnestedmarker\n")
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "# Section\n\nSee rollout_summaries/r1.md.\n")
	// Subdirectory root listed FIRST must not win deduplication
	// with a generic, evidence-free parse.
	roots := []memory.Root{
		{Provider: model.ProviderCodex, Path: filepath.Join(mem, "rollout_summaries")},
		{Provider: model.ProviderCodex, Path: mem},
	}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Nested summary"); len(got) != 1 || got[0].ProjectCWD != "/work/nested" {
		t.Fatalf("ancestor root's evidenced parse must win: %+v", got)
	}
	cat = load(t, memory.Options{Roots: roots, Excluded: func(p string) bool { return p == "/work/nested" }})
	if got := memory.Search(cat, "nestedmarker", memory.Filter{}); len(got) != 0 {
		t.Error("excluded nested-root rollout still searchable")
	}
}

func TestNestedArchiveRootNotCoveredByAncestorStays(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "# Main section\n\nBody.\n")
	archive := filepath.Join(mem, "archive", "memories")
	writeFile(t, filepath.Join(archive, "MEMORY.md"), "# Archive section\n\nArchived body.\n")
	roots := []memory.Root{
		{Provider: model.ProviderCodex, Path: mem},
		{Provider: model.ProviderCodex, Path: archive},
	}
	cat := load(t, memory.Options{Roots: roots})
	if len(byTitle(cat, "Main section")) != 1 || len(byTitle(cat, "Archive section")) != 1 {
		t.Fatalf("explicitly configured archive store must stay browsable: %v entries", len(cat.Entries))
	}
	if len(cat.Sources) != 2 {
		t.Errorf("both roots must appear in coverage, got %d sources", len(cat.Sources))
	}
}

func TestNestedClaudeRootOutsideDiscoveredStoresStays(t *testing.T) {
	dir := t.TempDir()
	projects := filepath.Join(dir, "projects")
	writeFile(t, filepath.Join(projects, "-work-a", "memory", "note.md"), "# A note\n")
	custom := filepath.Join(projects, "archive", "custom-notes")
	writeFile(t, filepath.Join(custom, "custom.md"), "# Custom archived note\n")
	roots := []memory.Root{
		{Provider: model.ProviderClaudeCode, Path: projects},
		{Provider: model.ProviderClaudeCode, Path: custom},
	}
	cat := load(t, memory.Options{Roots: roots})
	if len(byTitle(cat, "A note")) != 1 || len(byTitle(cat, "Custom archived note")) != 1 {
		t.Fatalf("explicitly configured nested Claude root must stay: %d entries", len(cat.Entries))
	}
}

func TestSearchSnippetUnicodeOffsetsDoNotPanic(t *testing.T) {
	line := strings.Repeat("Ⱥ", 100) + " needle " + strings.Repeat("x", 100)
	cat := &memory.Catalog{Entries: []memory.Entry{{
		ID: "1", Title: "Unicode", StartLine: 1, Text: line, RawText: line,
	}}}
	matches := memory.Search(cat, "needle", memory.Filter{})
	if len(matches) != 1 || len(matches[0].Snippets) != 1 || !strings.Contains(matches[0].Snippets[0], "needle") {
		t.Fatalf("unicode snippet must contain the match: %+v", matches)
	}
}

func TestFragmentLinkResolvesToSection(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "MEMORY.md"),
		"# First section\n\nIntro.\n\n# Deployment notes\n\nDeploy body.\n")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "r.md"),
		"thread_id: tr\n\n# Summary\n\nSee [deploy](../MEMORY.md#deployment-notes) and [missing](../MEMORY.md#no-such-section).\n")
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderCodex, Path: mem}}})
	summary := byTitle(cat, "Summary")
	if len(summary) != 1 || len(summary[0].References) < 2 {
		t.Fatalf("summary references missing: %+v", summary)
	}
	byRaw := map[string]memory.Reference{}
	for _, r := range summary[0].References {
		byRaw[r.Raw] = r
	}
	deploy := byTitle(cat, "Deployment notes")
	if len(deploy) != 1 {
		t.Fatalf("deployment section missing")
	}
	if got := byRaw["../MEMORY.md#deployment-notes"]; got.Status != "resolved" || got.EntryID != deploy[0].ID {
		t.Errorf("fragment must resolve to its section entry: %+v", got)
	}
	if got := byRaw["../MEMORY.md#no-such-section"]; got.Status == "resolved" {
		t.Error("unmatched fragment must stay unresolved")
	}
}

func TestExcludedLiteralTargetNotMaskedBySameBasename(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "secret", "private.md"),
		"thread_id: ts\ncwd: /work/secret\n\n# Secret private\n\nsecretmarker\n")
	writeFile(t, filepath.Join(mem, "rollout_summaries", "private.md"),
		"thread_id: tu\ncwd: /work/public\n\n# Public private\n")
	if err := os.Symlink(filepath.Join(mem, "rollout_summaries", "secret", "private.md"), filepath.Join(mem, "private.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "# Section\n\nSee [details](private.md) for the secretmarker.\n")
	roots := []memory.Root{{Provider: model.ProviderCodex, Path: mem}}
	cat := load(t, memory.Options{Roots: roots})
	if got := byTitle(cat, "Section"); len(got) != 1 || got[0].ProjectCWD != "/work/secret" {
		t.Fatalf("literal symlink target must supply evidence: %+v", got)
	}
	cfg := &config.Config{Exclusions: []string{filepath.Join(mem, "rollout_summaries", "secret")}}
	cat = load(t, memory.Options{Roots: roots, Excluded: cfg.Excluded})
	if len(byTitle(cat, "Section")) != 0 {
		t.Error("section referencing excluded literal target masked by same-basename public rollout")
	}
}

func TestRootsFromConfigTrailingSeparator(t *testing.T) {
	dir := t.TempDir()
	sessions := filepath.Join(dir, "codex", "sessions")
	writeFile(t, filepath.Join(sessions, "s.jsonl"), "{}\n")
	writeFile(t, filepath.Join(dir, "codex", "memories", "MEMORY.md"), "# Fictional\n")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "no-claude"))
	t.Setenv("CODEX_HOME", "")
	cfg := &config.Config{Roots: []config.Root{{Provider: model.ProviderCodex, Path: sessions + string(filepath.Separator)}}}
	roots := memory.RootsFromConfig(cfg)
	want := config.Canonical(filepath.Join(dir, "codex", "memories"))
	found := false
	for _, r := range roots {
		if r.Path == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("trailing-separator session root must derive sibling memories: %+v", roots)
	}
}

func TestUnreadableRootIsNotReportedEmpty(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not apply to root")
	}
	dir := t.TempDir()
	mem := filepath.Join(dir, "memories")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mem, "MEMORY.md"), []byte("# Fictional\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(mem, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(mem, 0o755) })
	if _, err := os.ReadDir(mem); err == nil {
		t.Skip("directory still readable despite chmod")
	}
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderCodex, Path: mem}}})
	if len(cat.Sources) != 1 || cat.Sources[0].State != "unreadable" {
		t.Errorf("unreadable root must not report empty: %+v", cat.Sources)
	}
}
