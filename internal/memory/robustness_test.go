package memory_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/memory"
	"github.com/juanrubio/trailmark/internal/model"
)

// Fictional robustness fixtures are generated in t.TempDir.

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func claudeRoot(t *testing.T) (root, memDir string) {
	t.Helper()
	memDir = filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return memDir, memDir
}

func TestUnclosedFrontmatterIsWholeNote(t *testing.T) {
	root, mem := claudeRoot(t)
	writeFile(t, filepath.Join(mem, "broken.md"), "---\nname: Hidden name\n# Visible heading\n\nBody stays readable.\n")
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: root}}})
	if len(cat.Documents) != 1 || cat.Documents[0].Status != "ok" {
		t.Fatalf("docs = %+v", cat.Documents)
	}
	got := byTitle(cat, "Visible heading")
	if len(got) != 1 {
		t.Fatalf("unclosed frontmatter must fall back to heading title, entries=%+v", cat.Entries)
	}
	if len(got[0].Meta) != 0 || !strings.Contains(got[0].Text, "name: Hidden name") || !strings.Contains(got[0].Text, "Body stays readable") {
		t.Errorf("unclosed block must remain body text with no meta: %+v", got[0])
	}
}

func TestEmptyStoresBothProviders(t *testing.T) {
	claudeDir := filepath.Join(t.TempDir(), "projects")
	codexDir := filepath.Join(t.TempDir(), "memories")
	for _, d := range []string{claudeDir, codexDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cat := load(t, memory.Options{Roots: []memory.Root{
		{Provider: model.ProviderClaudeCode, Path: claudeDir},
		{Provider: model.ProviderCodex, Path: codexDir},
	}})
	if len(cat.Documents) != 0 || len(cat.Entries) != 0 || len(cat.Sources) != 2 {
		t.Fatalf("catalog = %+v", cat)
	}
	for _, s := range cat.Sources {
		if s.State != "empty" {
			t.Errorf("source %s state=%q want empty", s.Provider, s.State)
		}
	}
}

func TestUnreadableFileReportedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod 000 is still readable as root")
	}
	root, mem := claudeRoot(t)
	writeFile(t, filepath.Join(mem, "ok.md"), "# Readable note\nbody\n")
	locked := filepath.Join(mem, "locked.md")
	writeFile(t, locked, "# Locked note\nsecret body\n")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o600) //nolint:errcheck
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: root}}})
	var lockedDoc *memory.Document
	for i := range cat.Documents {
		if strings.HasSuffix(cat.Documents[i].Path, "locked.md") {
			lockedDoc = &cat.Documents[i]
		}
	}
	if lockedDoc == nil || lockedDoc.Status != "unreadable" || lockedDoc.StatusDetail == "" {
		t.Fatalf("locked doc = %+v", lockedDoc)
	}
	if len(byTitle(cat, "Locked note")) != 0 || len(byTitle(cat, "Readable note")) != 1 {
		t.Errorf("readable note must survive, locked content must not appear: %+v", cat.Entries)
	}
}

func TestDanglingSymlinkInsideRootIsUnreadable(t *testing.T) {
	root, mem := claudeRoot(t)
	writeFile(t, filepath.Join(mem, "ok.md"), "# Present note\nbody\n")
	// Target lives inside the root but does not exist (file removed
	// between discovery and read).
	if err := os.Symlink(filepath.Join(mem, "gone.md"), filepath.Join(mem, "dangling.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: root}}})
	var dangling *memory.Document
	for i := range cat.Documents {
		if strings.HasSuffix(cat.Documents[i].Path, "dangling.md") {
			dangling = &cat.Documents[i]
		}
		if cat.Documents[i].Status == "outside-root" {
			t.Errorf("dangling inside-root link misreported as outside-root: %+v", cat.Documents[i])
		}
	}
	if dangling == nil || (dangling.Status != "unreadable" && dangling.Status != "unsupported") {
		t.Fatalf("dangling doc = %+v, want unreadable/unsupported", dangling)
	}
	if len(byTitle(cat, "Present note")) != 1 {
		t.Error("present note missing")
	}
}

func TestSecondLoadReflectsConcurrentChange(t *testing.T) {
	root, mem := claudeRoot(t)
	path := filepath.Join(mem, "evolving.md")
	writeFile(t, path, "# First version\noriginal body\n")
	opts := memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: root}}}
	first := load(t, opts)
	if len(byTitle(first, "First version")) != 1 {
		t.Fatalf("first load = %+v", first.Entries)
	}
	writeFile(t, path, "# Second version\nchanged body\n")
	second := load(t, opts)
	if len(byTitle(second, "First version")) != 0 || len(byTitle(second, "Second version")) != 1 {
		t.Errorf("stateless Load must reflect the change: %+v", second.Entries)
	}
	if len(byTitle(first, "First version")) != 1 {
		t.Error("first catalog must remain its own snapshot")
	}
}

func TestLongLinesNoTrailingNewlineCRLFAndBOM(t *testing.T) {
	root, mem := claudeRoot(t)
	long := strings.Repeat("fictional", 2000) // one ~18k line, no trailing newline
	writeFile(t, filepath.Join(mem, "long.md"), "# Long line note\n\n"+long)
	writeFile(t, filepath.Join(mem, "crlf.md"), "---\r\nname: CRLF note\r\ndescription: windows lines\r\n---\r\n# CRLF heading\r\n\r\nCRLF body\r\n")
	writeFile(t, filepath.Join(mem, "bom.md"), "\ufeff---\nname: BOM note\n---\n# BOM heading\n\nBOM body")
	cat := load(t, memory.Options{Roots: []memory.Root{{Provider: model.ProviderClaudeCode, Path: root}}})
	if got := byTitle(cat, "Long line note"); len(got) != 1 || !strings.HasSuffix(got[0].Text, long) || got[0].EndLine != 3 {
		t.Errorf("long/no-trailing-newline note = %+v", got)
	}
	if got := byTitle(cat, "CRLF note"); len(got) != 1 || got[0].Description != "windows lines" || strings.Contains(got[0].Text, "\r") || !strings.Contains(got[0].Text, "CRLF body") {
		t.Errorf("CRLF note = %+v", got)
	}
	if got := byTitle(cat, "BOM note"); len(got) != 1 || !strings.Contains(got[0].Text, "BOM body") || strings.Contains(got[0].Text, "name: BOM") {
		t.Errorf("BOM note must parse frontmatter past the BOM: %+v", got)
	}
	for _, d := range cat.Documents {
		if d.Status != "ok" {
			t.Errorf("doc %s status=%q", d.Path, d.Status)
		}
	}
}

func TestDuplicateCodexHeadingsGetDistinctStableIDs(t *testing.T) {
	memDir := filepath.Join(t.TempDir(), "memories")
	var b strings.Builder
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b, "# Repeated heading\n\nsection body %d with quokka\n\n", i)
	}
	writeFile(t, filepath.Join(memDir, "MEMORY.md"), b.String())
	opts := memory.Options{Roots: []memory.Root{{Provider: model.ProviderCodex, Path: memDir}}}
	cat := load(t, opts)
	got := byTitle(cat, "Repeated heading")
	if len(got) != 3 {
		t.Fatalf("entries = %+v", cat.Entries)
	}
	ids := map[string]bool{}
	for i, e := range got {
		if ids[e.ID] {
			t.Errorf("duplicate ID %s", e.ID)
		}
		ids[e.ID] = true
		if !strings.HasSuffix(e.ID, fmt.Sprintf("[%d]", i)) {
			t.Errorf("ID %q should carry occurrence index %d", e.ID, i)
		}
		if !strings.Contains(e.Text, fmt.Sprintf("section body %d", i)) {
			t.Errorf("section %d text = %q", i, e.Text)
		}
	}
	again := load(t, opts)
	for _, e := range byTitle(again, "Repeated heading") {
		if !ids[e.ID] {
			t.Errorf("ID not stable across loads: %s", e.ID)
		}
	}
}

func TestLoadSearchPerformanceSmoke(t *testing.T) {
	base := t.TempDir()
	codexDir := filepath.Join(base, "memories")
	var b strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "# Fictional section %d\n\nBody about deployment ordering and cache invalidation for project %d.\nThe quokka marker appears in section %d.\n\n", i, i%10, i)
	}
	writeFile(t, filepath.Join(codexDir, "MEMORY.md"), b.String())
	claudeMem := filepath.Join(base, "projects", "-work-fictional", "memory")
	for i := 0; i < 100; i++ {
		writeFile(t, filepath.Join(claudeMem, fmt.Sprintf("note-%03d.md", i)),
			fmt.Sprintf("---\nname: Fictional note %d\ndescription: synthetic note %d\n---\n# Fictional note %d\n\nSynthetic body mentioning migrations and quokka %d.\n", i, i, i, i))
	}
	opts := memory.Options{Roots: []memory.Root{
		{Provider: model.ProviderClaudeCode, Path: filepath.Join(base, "projects")},
		{Provider: model.ProviderCodex, Path: codexDir},
	}}
	start := time.Now()
	cat, err := memory.Load(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	matches := memory.Search(cat, "quokka", memory.Filter{})
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("Load+Search took %v, want <5s", elapsed)
	}
	if len(cat.Entries) != 300 {
		t.Fatalf("entries = %d, want 300", len(cat.Entries))
	}
	if len(matches) != 300 {
		t.Fatalf("matches = %d, want 300", len(matches))
	}
	t.Logf("Load+Search over 300 entries: %v", elapsed)
}
