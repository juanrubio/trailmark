package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
)

type themeTestStore struct {
	fakeSettings
	backend settingsBackend
}

func (s *themeTestStore) Theme() string              { return s.backend.Theme() }
func (s *themeTestStore) SetTheme(name string) error { return s.backend.SetTheme(name) }

func openThemeChooser(t *testing.T, m *appModel) {
	t.Helper()
	send(m, ",")
	for i, item := range m.settings.items {
		if item.action == "theme" {
			m.settings.cursor = i
			send(m, "enter")
			return
		}
	}
	t.Fatal("missing Theme action")
}

func TestThemeSettingsSaveCancelFailureAndReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAILMARK_CONFIG_DIR", dir)
	store := &themeTestStore{backend: settingsBackend{cfg: &config.Config{Theme: "green", DisableDefaultRoots: true}}}
	m := testModel(t)
	m.deps.settings = store
	m.themeName = store.Theme()
	rescanned := false
	m.deps.rescan = func(context.Context) ([]index.SessionRow, []string, error) { rescanned = true; return nil, nil, nil }
	openThemeChooser(t, m)
	if !strings.Contains(ansi.Strip(m.View().Content), "Green (current)") {
		t.Fatal("chooser does not show current choice")
	}
	send(m, "down")
	send(m, "esc")
	if m.themeName != "green" || store.Theme() != "green" {
		t.Fatal("cancel mutated theme")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("cancel wrote config")
	}
	send(m, "enter")
	send(m, "down")
	_, cmd := m.onKey(key("enter"))
	if cmd != nil || rescanned || m.themeName != "blue" || store.Theme() != "blue" {
		t.Fatal("save did not apply Blue without scanning")
	}
	cfg, err := config.Load()
	if err != nil || cfg.Theme != "blue" {
		t.Fatal("choice was not persisted")
	}
	// Recreate a model from the persisted settings contract.
	reloaded := newModel(context.Background(), dataDeps{settings: &themeTestStore{backend: settingsBackend{cfg: cfg}}}, nil, "/fictional", "/fictional", nil)
	if reloaded.themeName != "blue" {
		t.Fatal("saved theme did not load on startup")
	}
	// A config-directory failure leaves both model and backend on Blue.
	blocked := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocked, []byte("blocked"), 0600)
	t.Setenv("TRAILMARK_CONFIG_DIR", blocked)
	send(m, "enter")
	send(m, "down")
	_, cmd = m.onKey(key("enter"))
	if cmd != nil || rescanned || m.themeName != "blue" || store.Theme() != "blue" || !strings.Contains(m.settings.status, "Could not save theme") {
		t.Fatal("failed save changed theme or omitted error")
	}
}

func TestThemePaletteCacheSurfacesAndDimensions(t *testing.T) {
	m := testModel(t)
	m.noColor = false
	previous := m.styles().accent.GetForeground()
	m.themeName = "blue"
	if m.styles().accent.GetForeground() == previous || m.palette().selection == darkPalette.selection {
		t.Fatal("theme change did not invalidate style cache")
	}
	m.themeName = "unsupported"
	if m.palette() != darkPalette {
		t.Fatal("unknown palette not Green")
	}
	for _, name := range []string{"green", "blue", "amber"} {
		for _, light := range []bool{false, true} {
			for _, size := range [][2]int{{144, 40}, {80, 24}, {40, 12}} {
				m := testModel(t)
				m.width, m.height = size[0], size[1]
				m.themeName = name
				m.lightTheme = light
				m.noColor = false
				for _, mode := range []string{"main", "transcript", "theme"} {
					m.transcript = nil
					m.settings = nil
					if mode == "transcript" {
						send(m, "t")
					}
					if mode == "theme" {
						m.settings = &settingsState{mode: "theme"}
					}
					view := m.View().Content
					if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
						t.Fatalf("%s/%v/%s: bad dimensions", name, light, mode)
					}
					assertFrameGutters(t, m, view)
				}
			}
		}
	}
	m.noColor = true
	m.ascii = true
	m.settings = nil
	m.transcript = nil
	for _, name := range []string{"green", "blue", "amber"} {
		m.themeName = name
		if m.styles().base.GetBackground() != (lipgloss.NoColor{}) || strings.Contains(m.View().Content, "38;2;") || strings.Contains(m.View().Content, "48;2;") {
			t.Fatal("theme ignored NO_COLOR/dumb")
		}
	}
}
