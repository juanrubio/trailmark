package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/scan"
)

func TestDatabaseFileSizeAndFormatting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	if _, known := databaseFileSize(path); known {
		t.Fatal("missing database reported as zero")
	}
	os.WriteFile(path, make([]byte, 1000), 0600)
	if size, known := databaseFileSize(path); !known || size != 1000 {
		t.Fatal("missing optional sidecars not accepted")
	}
	os.WriteFile(path+"-wal", make([]byte, 2000), 0600)
	os.WriteFile(path+"-shm", make([]byte, 500), 0600)
	if size, known := databaseFileSize(path); !known || size != 3500 {
		t.Fatal("sidecars not included")
	}
	os.Remove(path + "-wal")
	os.Mkdir(path+"-wal", 0700)
	if _, known := databaseFileSize(path); known {
		t.Fatal("unreadable/nonfile sidecar reported available")
	}
	for _, tc := range []struct {
		bytes int64
		want  string
	}{{0, "0 B"}, {512, "512 B"}, {1024, "1.0 KiB"}, {13 * 1024 * 1024, "13.0 MiB"}, {3 * 1024 * 1024 * 1024, "3.0 GiB"}} {
		if got := formatDatabaseSize(tc.bytes); got != tc.want {
			t.Fatalf("%d: %s want %s", tc.bytes, got, tc.want)
		}
	}
}

func TestSettingsDatabaseSizeUsesActualIndexAndReloads(t *testing.T) {
	actual := filepath.Join(t.TempDir(), "actual.db")
	ix, err := index.Open(actual)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	// A different configured data directory must not change the opened DB path.
	t.Setenv("TRAILMARK_DATA_DIR", t.TempDir())
	backend := &settingsBackend{cfg: &config.Config{DisableDefaultRoots: true}, ix: ix, scanner: &scan.Scanner{Index: ix}}
	m := testModel(t)
	m.deps.settings = backend
	send(m, ",")
	if m.settings.settings.IndexPath != actual || !m.settings.settings.DatabaseSizeKnown {
		t.Fatal("settings measured configured path instead of opened index")
	}
	want, known := databaseFileSize(actual)
	if !known || m.settings.settings.DatabaseBytes != want {
		t.Fatal("settings size does not include active SQLite files")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Database size: "+formatDatabaseSize(want)) {
		t.Fatal("database size missing from settings")
	}
	// Force a failed main-file stat without touching the open database contents.
	os.Rename(actual, actual+".moved")
	m.reloadSettings()
	if m.settings.settings.DatabaseSizeKnown || !strings.Contains(ansi.Strip(m.View().Content), "Database size: Unavailable") {
		t.Fatal("missing database did not refresh to Unavailable")
	}
	os.Rename(actual+".moved", actual)
}
