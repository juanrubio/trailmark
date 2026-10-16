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

## Terminal appearance

Trailmark uses the [mockup palettes](../UX.md#palette) for dark and
light terminals. It asks the terminal for its background color and
chooses the matching palette; it falls back to dark if the terminal
does not answer. There is no additional theme setting or CLI flag.

Set `NO_COLOR=1` to disable application colors. `TERM=dumb` also
disables colors. Selection markers, labels, spacing, and separators
remain available without color. Terminals with limited color support
may approximate the palette's true-color values.

For `TERM=xterm-kitty`, Trailmark uses a conservative xterm rendering
profile inside the application. Older Kitty versions ignore the repeat
character sequence used to compress spaces, which can shift columns
and overlap text. The compatibility profile avoids that sequence while
retaining color support; it does not change the shell's environment or
terminal configuration.

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

## Missing Claude Code sessions

Choose **All projects**, **Claude Code** in the harness selector (`h`),
and **All time** in the time selector (`w`) to distinguish filtering from
missing source files. Source settings (`,`) show the scanned roots and
scan status. If Claude Code uses another configuration directory, add its
`projects` directory as a `claude-code` root.

Trailmark reads surviving transcript files under its configured roots.
Claude's `history.jsonl` contains prompt-history references, not complete
conversations; a reference there does not mean its transcript still exists.
Trailmark cannot reconstruct an absent conversation from that history.
Its derived index is not a transcript backup. Claude Code documents a
[30-day transcript retention setting](https://code.claude.com/docs/en/sessions#where-transcripts-are-stored),
`cleanupPeriodDays`. Provider cleanup can therefore remove history before
Trailmark scans it; Trailmark does not change provider retention settings.

Unrelated JSONL files, such as plugin telemetry inside a project directory,
must not become session cards. Files confidently identified as non-session
data are ignored, and any previously cached cards for them are removed on
rescan. Confirmed removals update the open list and selector counts immediately;
`R` is needed only to apply pending additions or ordering changes. Empty,
malformed, or incomplete files remain conservatively handled
so a damaged genuine log is not discarded solely for failing to parse.
