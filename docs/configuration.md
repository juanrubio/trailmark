# Configuration and data locations

## Configuration

Trailmark reads an optional JSON config at
`$XDG_CONFIG_HOME/trailmark/config.json` (usually
`~/.config/trailmark/config.json`). The `TRAILMARK_CONFIG_DIR`
environment variable overrides the config directory.

```json
{
  "roots": [
    {"provider": "codex", "path": "/extra/codex/sessions"}
  ],
  "exclusions": ["/work/private-project"],
  "disable_default_roots": false
}
```

- `roots` adds directories to scan. `provider` must be `claude-code`
  or `codex`. Defaults are detected automatically:
  `~/.claude/projects` for Claude Code and `~/.codex/sessions` for
  Codex. Set `disable_default_roots` to `true` to scan only the
  configured roots — this is also how a default root is removed.
- `exclusions` are absolute paths matched on a path boundary. An
  exclusion applies both to source-log paths and to a session's
  recorded working directory (project exclusion). Excluded
  transcripts are never indexed, and applying an exclusion removes
  any already-derived index rows for it. Source logs are never
  modified.

## Derived data

The index lives at `$XDG_DATA_HOME/trailmark/index.db` (usually
`~/.local/share/trailmark/index.db`); `TRAILMARK_DATA_DIR` overrides
the data directory. The directory is created `0700` and the database
file `0600`.

The index is a rebuildable cache of excerpted transcript text and is
as sensitive as the logs it derives from. Deleting the data
directory clears it completely; source logs and configuration are
untouched, and the next launch rebuilds the index from sources that
remain included. Clearing is a cache reset, not forensic erasure.
