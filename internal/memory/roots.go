package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/model"
)

// RootsFromConfig derives memory roots from effective session roots,
// provider environment overrides, Claude's autoMemoryDirectory
// setting, and explicitly configured memory_roots. Only existing
// directories are returned, deduplicated by provider + canonical
// path. Missing roots passed directly to Load are reported there.
func RootsFromConfig(cfg *config.Config) []Root {
	var candidates []Root
	if cfg != nil {
		for _, r := range cfg.EffectiveRoots() {
			switch r.Provider {
			case model.ProviderClaudeCode:
				candidates = append(candidates, Root{Provider: r.Provider, Path: r.Path})
			case model.ProviderCodex:
				// A root that is itself a memories directory, or a
				// Codex home containing memories/, resolves there;
				// any other session root (whatever its name)
				// derives the sibling memories directory. A
				// session-log directory is never itself treated
				// as a memory store.
				path := filepath.Clean(r.Path)
				if filepath.Base(path) != "memories" {
					if info, err := os.Stat(filepath.Join(path, "memories")); err == nil && info.IsDir() {
						path = filepath.Join(path, "memories")
					} else {
						path = filepath.Join(filepath.Dir(path), "memories")
					}
				}
				candidates = append(candidates, Root{Provider: r.Provider, Path: path})
			}
		}
	}
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		candidates = append(candidates, Root{Provider: model.ProviderClaudeCode, Path: filepath.Join(dir, "projects")})
	}
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		candidates = append(candidates, Root{Provider: model.ProviderCodex, Path: filepath.Join(dir, "memories")})
	}
	if dir := readAutoMemoryDirectory(); dir != "" {
		candidates = append(candidates, Root{Provider: model.ProviderClaudeCode, Path: expandHome(dir)})
	}
	if cfg != nil {
		for _, r := range cfg.MemoryRoots {
			candidates = append(candidates, Root{Provider: r.Provider, Path: r.Path})
		}
	}
	seen := map[string]bool{}
	var out []Root
	for _, c := range candidates {
		if c.Path == "" {
			continue
		}
		info, err := os.Stat(c.Path)
		if err != nil || !info.IsDir() {
			continue
		}
		key := string(c.Provider) + "|" + canonical(c.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Root{Provider: c.Provider, Path: canonical(c.Path)})
	}
	return out
}

// claudeSettingsPath locates the user-level Claude settings file.
func claudeSettingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// readAutoMemoryDirectory reads ONLY the autoMemoryDirectory key
// from user-level Claude settings; no other setting is decoded.
func readAutoMemoryDirectory() string {
	path := claudeSettingsPath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}
	v, ok := raw["autoMemoryDirectory"]
	if !ok {
		return ""
	}
	var dir string
	if err := json.Unmarshal(v, &dir); err != nil {
		return ""
	}
	return strings.TrimSpace(dir)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
