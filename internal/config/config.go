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

// Config is the on-disk configuration shape. Roots holds only
// user-configured roots; detected defaults live in extraRoots and
// are never persisted back as if the user had configured them.
type Config struct {
	Theme               string   `json:"theme,omitempty"`
	Roots               []Root   `json:"roots,omitempty"`
	Exclusions          []string `json:"exclusions,omitempty"`
	DisableDefaultRoots bool     `json:"disable_default_roots,omitempty"`

	extraRoots []Root
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
		return &Config{extraRoots: DefaultRoots()}, nil
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
	cfg.Theme = NormalizeTheme(cfg.Theme)
	if !cfg.DisableDefaultRoots {
		cfg.extraRoots = DefaultRoots()
	}
	return &cfg, nil
}

// Save persists the user-owned configuration (configured roots,
// exclusions, and the default-roots flag) with user-only
// permissions. Detected defaults are never written back.
func (c *Config) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "config.json")
	// Preserve unknown fields from an existing file, overlay the
	// known keys, and write atomically via a temp file plus rename
	// so a crash cannot corrupt the config.
	merged := map[string]json.RawMessage{}
	if existing, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(existing, &merged)
	}
	set := func(key string, v any) error {
		if v == nil {
			delete(merged, key)
			return nil
		}
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		merged[key] = data
		return nil
	}
	if err := set("theme", NormalizeTheme(c.Theme)); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if len(c.Roots) > 0 {
		if err := set("roots", c.Roots); err != nil {
			return fmt.Errorf("encode config: %w", err)
		}
	} else {
		delete(merged, "roots")
	}
	if len(c.Exclusions) > 0 {
		if err := set("exclusions", c.Exclusions); err != nil {
			return fmt.Errorf("encode config: %w", err)
		}
	} else {
		delete(merged, "exclusions")
	}
	if c.DisableDefaultRoots {
		if err := set("disable_default_roots", true); err != nil {
			return fmt.Errorf("encode config: %w", err)
		}
	} else {
		delete(merged, "disable_default_roots")
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}

// AddRoot records a configured root and reports whether it changed.
func (c *Config) AddRoot(r Root) bool {
	for _, existing := range c.Roots {
		if existing.Provider == r.Provider && Canonical(existing.Path) == Canonical(r.Path) {
			return false
		}
	}
	c.Roots = append(c.Roots, r)
	return true
}

// RemoveConfiguredRoot drops a user-configured root.
func (c *Config) RemoveConfiguredRoot(provider model.Provider, path string) bool {
	for i, r := range c.Roots {
		if r.Provider == provider && Canonical(r.Path) == Canonical(path) {
			c.Roots = append(c.Roots[:i], c.Roots[i+1:]...)
			return true
		}
	}
	return false
}

// IsConfiguredRoot reports whether the root came from the config file.
func (c *Config) IsConfiguredRoot(provider model.Provider, path string) bool {
	for _, r := range c.Roots {
		if r.Provider == provider && Canonical(r.Path) == Canonical(path) {
			return true
		}
	}
	return false
}

// AddExclusion records an exclusion path.
func (c *Config) AddExclusion(path string) bool {
	for _, ex := range c.Exclusions {
		if Canonical(ex) == Canonical(path) {
			return false
		}
	}
	c.Exclusions = append(c.Exclusions, path)
	return true
}

// RemoveExclusion drops one exclusion path.
func (c *Config) RemoveExclusion(path string) bool {
	for i, ex := range c.Exclusions {
		if Canonical(ex) == Canonical(path) {
			c.Exclusions = append(c.Exclusions[:i], c.Exclusions[i+1:]...)
			return true
		}
	}
	return false
}

// EffectiveRoots returns configured roots plus detected defaults
// (unless disabled), deduplicated by provider plus canonical path.
// A Config built directly uses exactly the roots it names.
func (c *Config) EffectiveRoots() []Root {
	seen := map[string]bool{}
	var out []Root
	all := append([]Root{}, c.Roots...)
	if !c.DisableDefaultRoots {
		all = append(all, c.extraRoots...)
	}
	for _, r := range all {
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

// NormalizeTheme keeps absent or unsupported theme names on the default.
func NormalizeTheme(name string) string {
	switch name {
	case "blue", "amber":
		return name
	default:
		return "green"
	}
}
