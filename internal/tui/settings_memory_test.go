package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
)

// fakeMemoryRootsStore extends fakeSettings with the optional
// memoryRootsStore capability, like themeTestStore does for themes.
type fakeMemoryRootsStore struct {
	fakeSettings
	memRoots []MemoryRootInfo
	added    []MemoryRootInfo
	removed  []string
}

func (f *fakeMemoryRootsStore) MemoryRoots() []MemoryRootInfo {
	out := append(append([]MemoryRootInfo{}, f.memRoots...), f.added...)
	var kept []MemoryRootInfo
	for _, r := range out {
		dropped := false
		for _, p := range f.removed {
			if p == r.Path {
				dropped = true
			}
		}
		if !dropped {
			kept = append(kept, r)
		}
	}
	return kept
}

func (f *fakeMemoryRootsStore) AddMemoryRoot(provider model.Provider, path string) error {
	f.added = append(f.added, MemoryRootInfo{Provider: provider, Path: path, Configured: true})
	return nil
}

func (f *fakeMemoryRootsStore) RemoveMemoryRoot(_ model.Provider, path string) error {
	f.removed = append(f.removed, path)
	return nil
}

func findSettingsItem(t *testing.T, m *appModel, match func(settingsItem) bool) int {
	t.Helper()
	for i, item := range m.settings.items {
		if match(item) {
			return i
		}
	}
	t.Fatalf("settings item not found in %+v", m.settings.items)
	return -1
}

func TestSettingsMemoryRootsSectionAddAndRemove(t *testing.T) {
	m := testModel(t)
	fake := &fakeMemoryRootsStore{memRoots: []MemoryRootInfo{
		{Provider: model.ProviderClaudeCode, Path: "/fake/claude/projects", Derived: true},
		{Provider: model.ProviderCodex, Path: "/fake/extra/memories", Configured: true},
	}}
	m.deps.settings = fake
	rescans := 0
	m.deps.rescan = func(context.Context) ([]index.SessionRow, []string, error) { rescans++; return nil, nil, nil }
	send(m, ",")
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Memory roots", "Memory root:", "/fake/claude/projects", "(derived)", "/fake/extra/memories", "(configured)", "Add memory root"} {
		if !strings.Contains(view, want) {
			t.Errorf("settings view missing %q", want)
		}
	}
	// Order: the section sits below the session-roots Add action.
	addRootAt := findSettingsItem(t, m, func(i settingsItem) bool { return i.action == "add-root" })
	sectionAt := findSettingsItem(t, m, func(i settingsItem) bool { return i.kind == "section" })
	if sectionAt <= addRootAt {
		t.Fatal("Memory roots section must sit below the session roots")
	}

	// Derived roots are explained, never removed.
	m.settings.cursor = findSettingsItem(t, m, func(i settingsItem) bool {
		return i.kind == "memory-root" && i.memRoot.Derived
	})
	send(m, "d")
	if m.settings.mode == "confirm" || !strings.Contains(m.settings.status, "cannot be removed here") {
		t.Fatalf("derived removal must be explained, mode=%q status=%q", m.settings.mode, m.settings.status)
	}
	if len(fake.removed) != 0 {
		t.Fatal("derived root must not reach RemoveMemoryRoot")
	}

	// Add through the shared input flow (Tab switches to Codex).
	m.settings.cursor = findSettingsItem(t, m, func(i settingsItem) bool { return i.action == "add-memory-root" })
	send(m, "enter")
	if m.settings.mode != "input" || m.settings.inputKind != "memory-root" {
		t.Fatalf("mode=%q kind=%q", m.settings.mode, m.settings.inputKind)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Memory root path") {
		t.Error("input prompt must name the memory root")
	}
	m.Update(key("tab"))
	for _, r := range "/fake/new/memories" {
		send(m, string(r))
	}
	_, cmd := m.Update(key("enter"))
	if cmd != nil {
		cmd()
	}
	if len(fake.added) != 1 || fake.added[0].Provider != model.ProviderCodex || fake.added[0].Path != "/fake/new/memories" {
		t.Fatalf("added = %+v", fake.added)
	}
	if !strings.Contains(m.settings.status, "Memory root added") {
		t.Errorf("status = %q", m.settings.status)
	}

	// Configured roots remove after confirmation, without a session rescan.
	m.settings.cursor = findSettingsItem(t, m, func(i settingsItem) bool {
		return i.kind == "memory-root" && i.memRoot.Path == "/fake/extra/memories"
	})
	send(m, "d")
	if m.settings.mode != "confirm" || !strings.Contains(m.settings.confirm, "never modified") {
		t.Fatalf("removal must confirm and explain, mode=%q confirm=%q", m.settings.mode, m.settings.confirm)
	}
	_, cmd = m.Update(key("y"))
	if cmd != nil {
		cmd()
	}
	if len(fake.removed) != 1 || fake.removed[0] != "/fake/extra/memories" {
		t.Fatalf("removed = %+v", fake.removed)
	}
	if rescans != 0 {
		t.Error("memory-root changes must not rescan sessions")
	}
	if !strings.Contains(m.settings.status, "Memory root removed") {
		t.Errorf("status = %q", m.settings.status)
	}
}

func TestSettingsWithoutMemoryRootsStoreHidesSection(t *testing.T) {
	m := testModel(t)
	m.deps.settings = &fakeSettings{} // no memoryRootsStore capability
	send(m, ",")
	for _, item := range m.settings.items {
		if item.kind == "memory-root" || item.action == "add-memory-root" || item.kind == "section" {
			t.Fatalf("legacy store must not show the Memory roots section: %+v", item)
		}
	}
}

func TestSettingsBackendMemoryRootsPersist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "no-claude"))
	t.Setenv("CODEX_HOME", filepath.Join(dir, "no-codex"))
	extra := filepath.Join(dir, "extra-memories")
	backend := &settingsBackend{cfg: &config.Config{DisableDefaultRoots: true}}
	if err := backend.AddMemoryRoot(model.ProviderCodex, "relative/path"); err == nil {
		t.Error("relative memory root must be rejected")
	}
	if err := backend.AddMemoryRoot(model.Provider("nope"), extra); err == nil {
		t.Error("unknown provider must be rejected")
	}
	if err := backend.AddMemoryRoot(model.ProviderCodex, extra); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil || len(cfg.MemoryRoots) != 1 || cfg.MemoryRoots[0].Path != extra {
		t.Fatalf("persisted memory roots = %+v (%v)", cfg, err)
	}
	// A configured-but-missing root stays listed as configured.
	roots := backend.MemoryRoots()
	found := false
	for _, r := range roots {
		if r.Path == extra && r.Configured && !r.Derived {
			found = true
		}
	}
	if !found {
		t.Fatalf("configured missing root not listed: %+v", roots)
	}
	if err := backend.RemoveMemoryRoot(model.ProviderCodex, filepath.Join(dir, "derived")); err == nil {
		t.Error("removing a non-configured root must fail with an explanation")
	}
	if err := backend.RemoveMemoryRoot(model.ProviderCodex, extra); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil || len(cfg.MemoryRoots) != 0 {
		t.Fatalf("removal not persisted: %+v (%v)", cfg, err)
	}
}

func TestSettingsBackendMemoryRootRollbackOnSaveFailure(t *testing.T) {
	dir := t.TempDir()
	// Point the config dir at a regular file so Save always fails.
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRAILMARK_CONFIG_DIR", blocker)
	existing := config.Root{Provider: model.ProviderCodex, Path: filepath.Join(dir, "existing")}
	backend := &settingsBackend{cfg: &config.Config{MemoryRoots: []config.Root{existing}}}
	if err := backend.AddMemoryRoot(model.ProviderCodex, filepath.Join(dir, "new")); err == nil {
		t.Fatal("save must fail with an unwritable config dir")
	}
	if len(backend.cfg.MemoryRoots) != 1 || backend.cfg.MemoryRoots[0] != existing {
		t.Errorf("failed add must roll back: %+v", backend.cfg.MemoryRoots)
	}
	if err := backend.RemoveMemoryRoot(model.ProviderCodex, existing.Path); err == nil {
		t.Fatal("save must fail with an unwritable config dir")
	}
	if len(backend.cfg.MemoryRoots) != 1 || backend.cfg.MemoryRoots[0] != existing {
		t.Errorf("failed removal must roll back: %+v", backend.cfg.MemoryRoots)
	}
}

func TestMemoriesRepeatedRefreshTwice(t *testing.T) {
	m, loader := memTestModel(t)
	openMemoriesForTest(t, m)
	selectMemoryForTest(t, m, "e-note")
	for round := 1; round <= 2; round++ {
		_, cmd := m.Update(key("R"))
		if cmd == nil {
			t.Fatalf("refresh %d must start a load", round)
		}
		m.Update(cmd())
		if loader.calls != 1+round {
			t.Fatalf("after refresh %d loader calls = %d", round, loader.calls)
		}
		e, ok := m.mem.selectedEntry()
		if !ok || e.ID != "e-note" {
			t.Fatalf("refresh %d lost selection: %v", round, e.ID)
		}
		if view := m.View().Content; lipgloss.Width(view) != m.width || lipgloss.Height(view) != m.height {
			t.Fatalf("refresh %d broke frame geometry", round)
		}
	}
}

func TestMemoriesRenderAllThemesLightAndDark(t *testing.T) {
	for _, name := range []string{"green", "blue", "amber", "monokai"} {
		for _, light := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/light=%v", name, light), func(t *testing.T) {
				m, _ := memTestModel(t)
				m.width, m.height = 120, 40
				m.themeName = name
				m.lightTheme = light
				m.noColor = false
				openMemoriesForTest(t, m)
				for _, stage := range []string{"rendered", "raw"} {
					if stage == "raw" {
						send(m, "v")
					}
					view := m.View().Content
					if lipgloss.Width(view) != 120 || lipgloss.Height(view) != 40 {
						t.Fatalf("%s: got %dx%d", stage, lipgloss.Width(view), lipgloss.Height(view))
					}
					for _, line := range strings.Split(view, "\n") {
						if lipgloss.Width(line) > 120 {
							t.Fatalf("%s: line width %d exceeds 120: %q", stage, lipgloss.Width(line), ansi.Strip(line))
						}
					}
					if !strings.Contains(ansi.Strip(view), "Memories") {
						t.Fatalf("%s: view lost its heading", stage)
					}
				}
				// Narrow under the same theme.
				m.width, m.height = 80, 24
				view := m.View().Content
				if lipgloss.Width(view) != 80 || lipgloss.Height(view) != 24 {
					t.Fatalf("narrow: got %dx%d", lipgloss.Width(view), lipgloss.Height(view))
				}
			})
		}
	}
	// No-color / ASCII rendering must not panic under any theme.
	m, _ := memTestModel(t)
	m.noColor = true
	m.ascii = true
	openMemoriesForTest(t, m)
	for _, name := range []string{"green", "blue", "amber", "monokai"} {
		m.themeName = name
		view := m.View().Content
		if strings.Contains(view, "38;2;") || strings.Contains(view, "48;2;") {
			t.Fatalf("%s ignored NO_COLOR", name)
		}
	}
}
