// Package config loads Trailmark's own configuration: discovery roots
// and exclusions. It never reads or writes provider configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanrubio/trailmark/internal/model"
)

// Root is one directory scanned for native session logs.
type Root struct {
	Provider model.Provider `json:"provider"`
	Path     string         `json:"path"`
}

// Config is the on-disk configuration shape.
type Config struct {
	Roots               []Root   `json:"roots,omitempty"`
	Exclusions          []string `json:"exclusions,omitempty"`
	DisableDefaultRoots bool     `json:"disable_default_roots,omitempty"`
}

// Dir returns Trailmark's configuration directory.
func Dir() (string, error) {
	if v := os.Getenv("TRAILMARK_CONFIG_DIR"); v != "" {
		return v, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, "trailmark"), nil
}

// DataDir returns the directory holding Trailmark's derived index data.
func DataDir() (string, error) {
	if v := os.Getenv("TRAILMARK_DATA_DIR"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "trailmark"), nil
	}
	return filepath.Join(home, ".local", "share", "trailmark"), nil
}

// IndexPath returns the SQLite index location inside DataDir.
func IndexPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.db"), nil
}

// DefaultRoots returns the known native log locations that exist on disk.
func DefaultRoots() []Root {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var roots []Root
	candidates := []Root{
		{Provider: model.ProviderClaudeCode, Path: filepath.Join(home, ".claude", "projects")},
		{Provider: model.ProviderCodex, Path: filepath.Join(home, ".codex", "sessions")},
	}
	for _, c := range candidates {
		if info, err := os.Stat(c.Path); err == nil && info.IsDir() {
			roots = append(roots, c)
		}
	}
	return roots
}

// Load reads the config file. A missing file yields detected default
// roots and no exclusions. Configured roots are additional to defaults
// only when they name a different path.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Roots: DefaultRoots()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	for _, r := range cfg.Roots {
		if r.Provider != model.ProviderClaudeCode && r.Provider != model.ProviderCodex {
			return nil, fmt.Errorf("parse config %s: unknown provider %q", path, r.Provider)
		}
		if r.Path == "" {
			return nil, fmt.Errorf("parse config %s: root for provider %q has an empty path", path, r.Provider)
		}
	}
	if !cfg.DisableDefaultRoots {
		cfg.Roots = append(cfg.Roots, DefaultRoots()...)
	}
	return &cfg, nil
}

// EffectiveRoots returns the configured roots deduplicated by
// provider plus canonical path (one directory may serve two
// providers, but the same provider and path scans once). Load
// populates detected defaults; a Config built directly uses exactly
// the roots it names.
func (c *Config) EffectiveRoots() []Root {
	seen := map[string]bool{}
	var out []Root
	for _, r := range c.Roots {
		key, err := canonical(r.Path)
		if err != nil {
			key = filepath.Clean(r.Path)
		}
		key = string(r.Provider) + "|" + key
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// Excluded reports whether path falls under any configured exclusion.
// Exclusions match on a path-boundary prefix of the canonical path.
func (c *Config) Excluded(path string) bool {
	canon, err := canonical(path)
	if err != nil {
		canon = filepath.Clean(path)
	}
	for _, ex := range c.Exclusions {
		exCanon, err := canonical(ex)
		if err != nil {
			exCanon = filepath.Clean(ex)
		}
		if canon == exCanon || strings.HasPrefix(canon, exCanon+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil // path may not exist yet; fall back to absolute form
	}
	return resolved, nil
}

// Canonical exposes canonical path resolution for dedupe decisions.
func Canonical(path string) string {
	c, err := canonical(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return c
}
