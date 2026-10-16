package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
	for _, name := range []string{"green", "blue", "amber", "monokai"} {
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
	for _, name := range []string{"green", "blue", "amber", "monokai"} {
		m.themeName = name
		if m.styles().base.GetBackground() != (lipgloss.NoColor{}) || strings.Contains(m.View().Content, "38;2;") || strings.Contains(m.View().Content, "48;2;") {
			t.Fatal("theme ignored NO_COLOR/dumb")
		}
	}
}

func TestMonokaiChooserPersistenceAndBackgroundReports(t *testing.T) {
	t.Setenv("TRAILMARK_CONFIG_DIR", t.TempDir())
	store := &themeTestStore{backend: settingsBackend{cfg: &config.Config{Theme: "green", DisableDefaultRoots: true}}}
	m := testModel(t)
	m.deps.settings = store
	m.noColor = false
	openThemeChooser(t, m)
	for i := 0; i < 3; i++ {
		send(m, "down")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Monokai") {
		t.Fatal("fourth choice missing")
	}
	send(m, "enter")
	cfg, err := config.Load()
	if err != nil || cfg.Theme != "monokai" || m.themeName != "monokai" {
		t.Fatalf("choice not saved: %+v %v", cfg, err)
	}
	reloaded := newModel(context.Background(), dataDeps{settings: &themeTestStore{backend: settingsBackend{cfg: cfg}}}, nil, "/fictional", "/fictional", nil)
	if reloaded.themeName != "monokai" || reloaded.palette().panel != "#000000" {
		t.Fatal("Monokai dark fallback did not reload")
	}
	for _, tc := range []struct {
		color string
		want  string
	}{{"#ffffff", "#ffffff"}, {"#000000", "#000000"}} {
		m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color(tc.color)})
		if m.palette().panel != tc.want {
			t.Fatalf("background report %s gave %s", tc.color, m.palette().panel)
		}
		assertFrameGutters(t, m, m.View().Content)
	}
}

func TestMonokaiReadableContrast(t *testing.T) {
	luminance := func(hex string) float64 {
		n, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		channel := func(v uint64) float64 {
			c := float64(v) / 255
			if c <= 0.04045 {
				return c / 12.92
			}
			return math.Pow((c+0.055)/1.055, 2.4)
		}
		return 0.2126*channel(n>>16) + 0.7152*channel((n>>8)&255) + 0.0722*channel(n&255)
	}
	for _, light := range []bool{false, true} {
		p := themePalette("monokai", light)
		for _, pair := range [][2]string{{p.text, p.panel}, {p.muted, p.panel}, {p.accent, p.panel}, {p.path, p.panel}, {p.warning, p.panel}, {p.text, p.selection}, {p.ink, p.gold}} {
			a, b := luminance(pair[0]), luminance(pair[1])
			if a < b {
				a, b = b, a
			}
			if ratio := (a + 0.05) / (b + 0.05); ratio < 4.5 {
				t.Errorf("light=%v %s on %s contrast %.2f", light, pair[0], pair[1], ratio)
			}
		}
	}
}
