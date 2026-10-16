# UX validation (Phase 5)

Scripted validation of the PRD section 9 tasks against synthetic
fixtures, run 2026-10-15 with `go test ./internal/tui/ -run
TestValidation -v`. Each task is exercised through the real TUI
model and key handling with fixture sessions designed for the task.

**Honest limitation:** these are scripted fixture runs, not
sessions with human participants. The MRD success definition
(correct session and directory identified within 15 seconds in 4
of 5 tasks, without opening a transcript) still requires a human
validation pass before any public claim is made. No participant
results are reported here because none were collected.

## Primary tasks (no transcript, no content search)

| Task | Scripted outcome |
| --- | --- |
| Recognize yesterday's session across projects | Pass — purpose "Repair the deployment flow" and directory `music-lab` are visible in the Yesterday group on the first screen |
| Recognize a 3-to-7-day-old session by purpose and directory | Pass — "Migrate the billing database schema" with `infra` is visible in the 7-day list (an All-time variant is one `w` selection away) |
| Distinguish similar purposes in different projects | Pass — both "Fix the login redirect loop" cards show distinct directories (`web-app`, `admin-console`) side by side |
| Find the intended worktree among same-repo sessions | Pass — the worktree card shows `/home/dev/worktrees/trailmark-fix` distinctly from the main checkout |
| Identify the session worth continuing and its latest state | Pass — the overview shows purpose, directory, initial request, and latest exchange without opening the transcript |

## Continuation task

| Task | Scripted outcome |
| --- | --- |
| Explain resume vs clean behavior and copy a command without executing | Pass — the Resume sheet states it "Continues the same native conversation", the Clean sheet states "no inherited history", and Copy command succeeds against a fake clipboard; nothing is executed |

## Secondary task (transcript/search)

| Task | Scripted outcome |
| --- | --- |
| Find a known passage within 30 seconds | Pass — in-session search for a known phrase landed on the matching message in microseconds of scripted time (fixture scale; human timing unmeasured) |

## Failures and gaps

- No human timing or recognition-rate data exists yet; the 15-second
  and 4-of-5 thresholds from the MRD/PRD are unverified with people.
- Fixture purposes are cleaner than real derived purposes; purpose
  quality on real logs remains the known product risk from the MRD.

## Visual refinement validation (2026-10-16)

The visual refinement implements the [UX palette and component
rules](../UX.md#8-visual-and-state-rules) and is checked against the
[PRD visual criteria](../PRD.md#visual-acceptance). This pass used
fictional sessions, not private session logs.

| Check | Result and scope |
| --- | --- |
| Repository checks and build | `make check` and `make build` passed; the local `trailmark` executable was rebuilt. |
| Frame size and structural rules | Automated checks cover main and secondary views at wide, 80×24, and 40×12 sizes. Main-frame assertions check the full-width header, bottom footer, and continuous pane divider. |
| Dark/light visual comparison | Inspected ANSI fixture previews at 120×40, with additional captures at 160×45 and 80×24, against the supplied mockups. Inspected selectors, narrow overview, transcript search, overview find, and the quit dialog. These previews render the application's ANSI output to an image; they are not screenshots of a physical terminal. |
| Selection and surface colors | Verified all three selected-card rows retain the selection fill across text and padding in both palettes. Fixed nested ANSI resets that initially left gaps in card and panel backgrounds; applied the same restoration to selected options and the primary action. |
| Theme and no-color behavior | Tests exercise light/dark background messages and uncolored styles. A no-color fixture emits no foreground/background color escapes while retaining selection markers and separators. |
| Long content and copying | Regression tests verify that long words/paths wrap without losing content, a long target path remains reachable by scrolling, and a failed clipboard copy retains an unbroken command longer than 300 characters at 80 columns. |
| Real-terminal visual acceptance | **Unverified.** Actual font rendering, color approximation, terminal background reporting, and manual selection/copying still need inspection in the user's terminal. |

The dedicated rendering regressions can be run with:

```bash
go test ./internal/tui -run '^TestVisual' -count=1
```

The existing human recognition targets above remain unverified.
The prior performance measurements were not rerun as part of this
visual pass.

### Terminal spacing regression

A user screenshot exposed a failure beyond the model's layout:
Kitty 0.16 ignored the renderer's REP (`CSI n b`) sequence. A run of
spaces in selected-card metadata lost five cells, shifting the pane
divider and overview text left. Trailmark now uses a conservative
program-local xterm rendering profile for `TERM=xterm-kitty`, without
changing the shell environment or terminal settings.

Verified with the actual installed Kitty parser and a fictional
144×40 fixture: the original emitted output mismatched the metadata
row; the corrected output matched all 40 rows. The regression test
also exercises the terminal renderer, rather than checking only the
model's string widths:

```bash
go test ./internal/tui -run 'TestTerminalEnvironment|TestSelectedMetadataTerminalRendering' -count=1
```

### Exit and scrolling regressions

Quit confirmation tests cover the default Cancel choice, Enter/Esc/n
cancellation, explicit confirmation, repeated `q`, preservation of
browsing state, nested-view back behavior, and dark/light/no-color
modal dimensions. The interactive test harness now confirms before
exiting.

Scrolling tests exercise long content in the transcript and both
overview layouts. Repeated movement beyond either edge no longer
accumulates an invisible offset; the first reverse key changes the
visible text. Tests cover actual Page Up/Down key codes, resize,
resetting the offset on a transcript match jump, and moving upward
across a loaded-message boundary on the first key. An 850-message fixture
verifies Page Down reaches the final message and Page Up returns to the
first while retaining the bounded message buffer and reloading evicted history.


### Overview search and automatic time scope

Overview find tests cover case-insensitive literal queries, repeated and
offscreen matches, match navigation, no results, clearing/cancelling, and
input ownership of action keys. Wrapped phrases and long path substrings
remain searchable across lines and resizing. Tests check highlighted
matches, session-change reset, and the independent list filter.

Automatic time-window tests cover cached startup with recent, 15-day-old,
80-day-old, unknown-date, and absent sessions; selecting an older project;
late arrival of the first scan; project/harness intersections; returning
to a recent scope; and an explicitly selected empty 7-day window surviving
a refreshed session list. Metadata text filtering does not drive expansion. These are
synthetic model tests, not human acceptance observations.


## Claude Code discovery regression (2026-10-16)

Added synthetic regression coverage for plugin telemetry JSONL under a
Claude project root. The adapter ignores confirmed unrelated records,
while retaining native metadata, empty messages, incomplete writes, and
malformed logs conservatively. Scanner tests seed a row with the previous
parser version and an unchanged file fingerprint, then verify that a scan
removes the stale telemetry card without changing its source file.
Parser version 2 triggers this reclassification on the next normal scan.

A separate read-only local audit verified the same classification against
surviving files. Private transcript contents, identifiers, and paths were
not copied into fixtures or this report. Missing source transcripts cannot
be validated or recovered through parser changes.


### Background removal and displayed counts

A successful startup scan previously corrected the index while leaving
stale cards and harness counts in memory until `R`. Regression tests now
cover twelve cached Claude rows becoming six while the harness selector
is open, with no manual refresh. Surviving rows retain their ordering and
selection; new or reordered activity remains pending. Tests also cover
selected-session removal, overview/find reset, the final session being
removed, superseded pending results, and failed scans preserving cached
content.


### Status-bar color refinement

Inspected ANSI fixture previews at 144×40 in dark mode and 80×24 in light
mode with new-activity and partial-coverage status visible. Shortcut keys
use bold green, descriptions use muted text, warnings use amber, and
information/find text uses blue. Explicit key/description pairs keep
user-entered text out of shortcut styling. Narrow navigation keeps its
two-line layout, and the existing no-color fallback retains plain labels.
These remain rendered previews, not physical-terminal screenshots.


### Application gutters and transcript wrapping

The application now renders within a shared inner viewport: normally two
blank columns per side and one blank row above/below. At widths of 40–43
columns, side gutters shrink to one column; below 14 rows, vertical gutters
are omitted. Tests verify exact terminal dimensions and clear edges across
main, overview, transcript, search, settings, and modal views, including
40×12. Paging and resize use the same inner dimensions as rendering.

Inspected updated dark/light fixture previews, including a 144×40
transcript with long prose, indentation, and an oversized path, plus the
80×24 list/status bar. Prose wraps at word boundaries, long tokens wrap
inside the gutters, and original copy payloads remain unchanged. The
existing unbroken clipboard-failure command fallback remains an explicit
exception to fixed-width rendering. `make check build` passed, including
scroll-boundary, resize, 850-message paging, overview-search, and copy tests.
These previews do not replace physical-terminal acceptance.


### Selectable color themes

Green remains the default; Blue and Amber add complete dark/light palettes.
Inspected 120×40 ANSI previews of both new themes in both variants and the
theme chooser. Selected cards, header surfaces, continuation actions, paths,
status hints, and warnings use the chosen colors without background gaps.
A numeric check of the intended body, secondary, accent, path, warning,
selected-body, and search-match text pairs yielded contrast ratios above
5.5:1 in all six palettes; terminal color approximation remains unverified.

Automated tests cover saving and reloading a theme, preserving unrelated
configuration fields, unknown-name fallback, cancelling without a write,
save failure preserving the prior choice, and changing themes without a
rescan. Rendering tests cover all three themes in light/dark modes at
144×40, 80×24, and 40×12, including transcript and chooser gutters and
NO_COLOR/dumb behavior. `make check build` passed; the binary was rebuilt.


### Database size in settings

Sources and Settings now shows a human-readable database size beneath the
opened index path. Tests verify aggregation of the main SQLite file and
existing WAL/shared-memory sidecars, absent optional files, unavailable
measurements, unit formatting, and the displayed value. A temporary SQLite
integration test checks that measurement uses the actual opened database
even when the configured data directory differs, and that reloading
settings refreshes availability. `make check build` passed.
