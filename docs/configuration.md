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
  "disable_default_roots": false,
  "theme": "green",
  "indexing_days": 30
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

## First-run indexing

Run `trailmark` normally on each machine. It scans that user's configured
local roots and builds its own local index; deploying the binary does not
copy another machine's session history or cache.

With no cached sessions and no saved indexing scope, Trailmark first asks
how much history to include: files modified in the last **7 days**, **30
days** (recommended), or **All history**. Enter saves the choice and starts
indexing; Esc leaves indexing for later. With a saved choice, an empty index
starts indexing using that scope.

`indexing_days` accepts `7`, `30`, or `0` (All history). An omitted setting
leaves existing installations with cached sessions on their previous
all-history behavior. The cutoff uses the source file's modification time,
which can change when files are copied or restored. It is not a filter on
recorded session dates. Older, unindexed files are deferred without parsing;
cached files remain available and continue to be checked for changes.
Exclusions still apply to cached sessions, and confirmed deletions are
still removed.

Change **Sources and settings** (`,`) → **Indexing scope** to expand coverage
and rescan. Narrowing the scope does not delete cached history. The `w`
selector filters the sessions already indexed; selecting All time there
does not override indexing scope or start indexing deferred files.

During an empty-index scan, a dedicated screen shows an animated
activity indicator, running file counts, elapsed time, and the current
source. Files are discovered while scanning, so the indicator does not
claim a percentage or an estimated finish time. Large individual logs can
take time even while the file counts remain unchanged.

Press **Esc** to stop indexing and browse sessions already saved. Completed
files remain cached; the interrupted file is checked again on the next
scan. **q** opens the normal quit confirmation. Cancelling or confirming
quit does not modify native session logs.

After completion, Trailmark opens the session list. If no sessions were
found, open **Sources and settings** (`,`) to check roots, add a missing
location, or choose **Rescan now**. Subsequent launches with cached sessions
open the list immediately and refresh in the background; **R** applies
pending new activity. There is no separate indexing command to run.

### How long will it take?

Large archives can take hours. The recorded development benchmark used
10,000 synthetic sessions totalling 1.19 GiB and took approximately 2 hours
18 minutes of CPU time for cold indexing, with a 2.9 GiB derived index.
Loading its cached list took 103 ms. This is a historical measurement on
an Intel Xeon E5-1620 v2, not elapsed-time guidance for every machine or a
re-measurement of the current build. See [benchmarks](benchmarks.md) for
the corpus, software baseline, and known limitations.

Choosing recent files reduces initial work when most logs are old, but
cannot guarantee a particular duration: a single recent file can be very
large. Later scans reuse unchanged indexed files. Indexing scope and
progress reporting do not improve the underlying parsing or SQLite write
throughput. No benchmark is available yet for the new scoped first run.

## Terminal appearance

Trailmark offers four color themes: **Green** (the original default),
**Blue**, **Amber**, and **Monokai**. Press `,` from the session list, select **Theme**,
and press Enter to choose one. Enter saves and applies the choice; Esc
cancels. The choice persists in Trailmark's config as `"theme": "green"`,
`"blue"`, `"amber"`, or `"monokai"`. Missing or unrecognized values use Green.

Monokai uses a pure black main background in dark mode and pure white in
light mode, with vivid accents and distinct selected/raised surfaces.

Each theme has dark and light variants. Trailmark asks the terminal for
its background color and chooses the matching variant, falling back to
dark when the terminal does not answer. Theme changes apply immediately
without rescanning sessions. If saving fails, the existing theme remains
active and settings reports the error. No theme CLI flag is needed.

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

**Sources and settings** (`,`) shows the index path and **Database size**
in B, KiB, MiB, or GiB. The size sums the current database file and any
SQLite WAL/shared-memory sidecars, so it includes pending writes. It is
refreshed when settings opens or reloads after a scan or cache operation.
If the database cannot be inspected, the size is shown as unavailable;
reading the size does not checkpoint or modify the database.

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
