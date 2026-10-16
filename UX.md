# Trailmark — TUI experience

- **Status:** Normative interaction design; MVP and visual refinement implemented, terminal acceptance tracked in [validation](docs/validation.md#visual-refinement-validation-2026-10-16)
- **Date:** 2026-10-14
- **Visual requirements updated:** 2026-10-16
- **Related:** [PRD](PRD.md), [MRD](MRD.md)
- **Interactive concept:** [Open the terminal-style mockup](docs/mockups/index.html)

The standalone HTML concept uses fictional data and requires no server or external assets. It includes recent work, older-session scope, continuation decisions, and transcript search. Click controls or use the documented keyboard hints; the outer controls switch light/dark and wide/narrow presentations. Provider capability examples are illustrative; it reads no real logs and prepares or executes no native commands.

### Concept screen gallery

| Screen | Preview |
| --- | --- |
| Recent work and selected-session overview | [Wide layout](docs/mockups/01-recent-work.png) |
| Time-window selector | [Today through All time](docs/mockups/02-time-window.png) |
| Older sessions | [All time with a session from 38 days ago](docs/mockups/03-older-sessions.png) |
| Resume / fork / clean-session choice | [Continuation sheet](docs/mockups/04-continuation.png) |
| Transcript with highlighted matches | [Transcript search](docs/mockups/05-transcript-search.png) |
| Narrow layout | [Recent list](docs/mockups/06-narrow-list.png), [session overview](docs/mockups/07-narrow-overview.png) |
| Light theme | [Recent-work overview](docs/mockups/08-light-theme.png) |
| Harness filtering | [Harness selector](docs/mockups/09-harness-filter.png), [Codex sessions only](docs/mockups/10-codex-sessions.png) |

The concept demonstrates browser interactions and visual hierarchy; it does not validate native terminal rendering or the 80×24 implementation. Source settings, indexing, and real provider commands are outside this visual prototype. Its outer scene controls are presentation controls, not proposed TUI navigation tabs.

## 1. Experience goal

Open Trailmark and remember what you have been working on over the last few days. Recognize the purpose and directory of a session, glance at where you left off, and choose whether to resume, fork, or start a clean conversation in that area.

The home screen must be useful before the user types anything. Transcript viewing and content search are secondary MVP tools for answering questions that the overview cannot resolve.

The first release is a TUI with a minimal launch interface: `trailmark`, `--help`, and `--version`. Navigation, filters, search, refresh, source status, and cache controls belong inside the TUI.

## 2. Information hierarchy

Every session card answers these questions in this order:

1. **What was this for?** A prominent purpose label from the native title or a clearly labeled request excerpt.
2. **Where was I working?** Project name and enough cwd/worktree path to distinguish it from other sessions.
3. **When, and with which agent?** Relative last activity and a textual provider label.

The selected overview adds the full path, initial request, latest substantive exchange, and continuation choices. IDs, raw log locations, parser details, and indexing diagnostics belong in a details/status view. They must not displace purpose or directory in ordinary browsing.

Avoid a dashboard of counts, charts, usage statistics, or large decorative headers. Use the terminal space for recognizable work and its context.

## 3. Home: recent sessions

Default scope is all projects over today plus the previous six local calendar days. Group by last-active day, then newest first. Use Today, Yesterday, and weekday/date headings. Show each session once. A session started a month ago and used today belongs under Today.

Offer Today, 3 days, 7 days, 30 days, and All time in the time selector (`w`). These are view filters, not retention limits: sessions older than seven days remain indexed and accessible. The list footer offers **Show all time** as a direct route to older work. Each launch prefers 7 days/all projects. If no sessions exist in the active project/harness scope, try 30 days, then All time, stopping at the first populated window. Repeat this automatic choice when selecting a project or harness and when applying scan results. An explicit choice in the time selector fixes that window for the rest of the run, even if empty. Metadata text filtering never triggers expansion. If the scope has no sessions at any age, retain the 7-day empty state; unknown dates can appear under All time. The header must always show the current scope. Dates with no sessions need no empty headings.

### Harness filtering

The header includes a **Harness** selector alongside time and project. Press `h` or select the control to choose **All harnesses**, **Codex**, or **Claude Code**. Show per-harness session counts within the selected time/project scope. Harness refers to the coding tool that created a session, rather than its model or a child-agent role.

The active selection remains visible in the header and combines with existing time, project, and text filters. Opening a session or returning from its transcript preserves the selection. Cross-session content search uses the same harness scope. **All harnesses** clears only this filter; a zero-result combination explains which filters are active and lets the user adjust them.

### Wide layout: 110 content columns or more

The list takes about 45% of the inner content width and the overview takes the rest. With the normal two-column side gutters, this layout starts at 114 terminal columns. Selecting another card updates the overview without opening a new screen. This wireframe uses fictional session content; it shows information placement rather than exact terminal dimensions.

```text
 Trailmark    Last 7 days · All projects · All harnesses                  Updated just now
 ─────────────────────────────────────────────────────────────────────────────────────
 RECENT SESSIONS                           │ SESSION OVERVIEW
                                          │
 Today                                    │ Improve the recent-session UX
 ▌ Improve the recent-session UX          │ trailmark · Codex · Today 10:42
 ▌ trailmark  ~/repos/trailmark            │
 ▌ Codex · 20m ago                        │ Directory
                                          │ /home/dev/repos/trailmark
   Repair the deployment flow             │
   music-lab  ~/repos/music-lab            │ Purpose                         Native title
   Claude Code · 2h ago                    │ Improve the recent-session UX
                                          │
 Yesterday                                │ Initial request
   Investigate refresh failures           │ Make recent work easy to recognize by
   dashboard  ~/worktrees/dashboard-fix    │ session purpose and project directory.
   Codex · Yesterday 17:30                 │
                                          │ Latest exchange                 Agent reported
 Mon 12 Oct                               │ Updated the overview and navigation design.
   Review the migration plan              │ Implementation has not started.
   infra  ~/repos/infra                    │
   Claude Code · Mon 09:15                 │ Continue with this work
                                          │ [r] Resume   [f] Fork   [n] Clean session
                                          │ Opens command preparation; no agent is launched.
 ─────────────────────────────────────────────────────────────────────────────────────
 ↑↓/jk Select   Enter Overview   p Project   w When   / Filter   t Transcript   ? Help
```

Each card normally uses three lines: purpose; project/path; provider/activity. Cap the purpose at two lines when necessary, keeping card height predictable. Separate cards with one blank row and follow day labels with a muted horizontal rule. Do not draw a heavy box around every card. The selected card uses a full-width background and a left selection marker on every card row. Align provider left and activity right when space permits; preserve readable metadata in compact layouts.

Purpose text may be truncated with an ellipsis in the list; the overview exposes the full label. Shorten the home directory to `~` and elide middle path segments if necessary. Retain distinctive parent/worktree segments when basenames collide. The full cwd must always be accessible in the overview and copyable.

### Narrow layout: 80–109 columns

Show a full-width recent-session list. Enter opens the same session overview as a full-width view; Esc returns to the original selection and scroll position. The list still shows purpose, directory, provider, and recency. Transcript reading remains a separate `t` action.

```text
 Trailmark     Last 7 days · All projects · All harnesses
 ────────────────────────────────────────────────────────────────────
 Today
 ▌ Improve the recent-session UX
 ▌ trailmark  ~/repos/trailmark
 ▌ Codex · 20m ago

   Repair the deployment flow
   music-lab  ~/repos/music-lab
   Claude Code · 2h ago

 Yesterday
   Investigate refresh failures
   dashboard  ~/worktrees/dashboard-fix
   Codex · Yesterday 17:30
 ────────────────────────────────────────────────────────────────────
 ↑↓/jk Select   Enter Overview   p Project   w When   / Filter
 t Transcript   ? Help   q Quit
```

Support 80×24 without clipped actions or overlapping panels. Below that size, show a concise size notice if the core interface cannot fit. Resizing across layouts must preserve the selected session, filters, and reading position.

## 4. Purpose and latest context

Use a native title where available. Otherwise extract a concise portion of the first substantive user request and label its origin **Request excerpt**. Ignore recognized setup/injected instructions. If neither is usable, display **Purpose unavailable** and allow transcript inspection.

The overview labels the origin and shows initial request and latest exchange separately. A session that changed direction may have an old title; displaying its latest context helps the user notice that. Do not present an extractive preview as a verified account of the whole session.

Label completion claims **Agent reported** and tool evidence **Recorded tool result**. Neither warrants a generic green “Done” badge or a recommendation that a session is safe to resume. Display recency as last recorded activity, not a claim that an agent process is currently running.

Purpose quality is central to acceptance: a list full of IDs, setup text, or indistinguishable “continue” labels fails the recognition goal even if the parser is correct. Use the PRD recognition tasks to evaluate this before adding more providers or generated summaries.

### Recorded session details

After the initial request and latest exchange, the scrollable Overview shows recorded models and reasoning efforts, Git context, token usage, and latest-turn outcome/timing. Keep the purpose/path and conversation excerpts first and continuation actions pinned. Multiple observed models or efforts remain visible; Git labels describe the log's recorded state rather than the current checkout.

Input token totals include cache reads/writes. Cache values are subsets of input, and reasoning is a subset of output; do not add these breakdowns to totals a second time. Display missing values as **Unavailable**, distinguish them from recorded zero, and qualify usage that may include inherited history. Duration is a recorded measurement, not elapsed time since the session began. A completed turn does not establish task success or current process status.

The **Recorded work** section starts collapsed. With Overview focused, `e` expands/collapses recorded file changes and command results. Show bounds and missing coverage explicitly. Expansion does not select a different session or expand its child sessions, and changing sessions resets it. Overview find includes metadata and currently expanded work; toggling the section recomputes matches and clamps the scroll position. Existing gutters, themes, page navigation, and no-color behavior apply to these details.

### Find within the overview

With the overview focused (Enter from the list), `/` opens a local
find input. Search the current session's displayed details, including
purpose, directory, initial request, and latest exchange. This is a
literal, case-insensitive text search; it does not search the complete
transcript or change the session-list filter.

Enter applies the query, highlights matches, and reveals a matching
passage. Show the match count or an explicit no-match state; `[` and
`]` move between matches and scroll the detail while the continuation
actions stay in place. Distinguish the active match with an additional
focus cue. An empty query clears the search; Esc while editing cancels
the draft and restores the previous applied query. Printable action
keys belong to the input while editing. Changing sessions resets the
overview search, and resizing keeps matches aligned with rewrapped text.

## 5. Continuation choices

Expose the three choices in the selected overview. Their semantics are stable across providers; availability depends on the adapter's validated native capabilities.

| Choice | Meaning | Required context |
| --- | --- | --- |
| Resume | Continue the same native conversation with its existing history. | Native session identity, source provider/store, and original cwd. |
| Fork | Create a separate native conversation from the selected history where supported. | Validated native fork support, session identity, and target cwd. |
| Clean session | Start with no inherited conversation in the selected directory. Normal project instructions/configuration still apply. | Selected provider and original cwd; no transcript is attached automatically. |

Choosing an action opens a compact preparation sheet, not a confirmation followed by another modal. The sheet shows:

- The selected action and a plain-language description of its history behavior.
- Session purpose, provider, and exact target directory.
- A validated native command, including cwd and provider-store context where needed.
- **Copy command** and Back. If clipboard access fails, provide selectable text and explain that copying did not succeed.

The first release prepares commands only. Labels must not suggest that an agent has been launched. Native execution and terminal handoff are a later design decision.

For unsupported forks, keep the choice visible but label it unavailable with the reason. Do not pretend a fresh session with pasted text is a native fork. For a missing directory, show the recorded path and disable command preparation; never silently use Trailmark's launch directory. The UI must not create directories or rewrite native sessions.

Selected-context copying is available as a separate supporting action. It includes provider, session identity, and passage provenance and shows what will be copied. A clean-session action does not insert that content automatically.

## 6. Navigation and keyboard contract

Show only relevant keys in each view, with full help available through `?`. A footer may use two lines at 80 columns. Text inputs own printable keys while focused, so typing `q`, `r`, or `n` cannot quit or trigger actions.

| Key | Recent list / overview | Transcript / content-search results |
| --- | --- | --- |
| Arrows or `j`/`k` | Move selection; scroll when overview has focus. | Select a transcript message or search result. |
| `J` / `K` | No action in the session list. | Scroll transcript text down/up by one rendered line. |
| Page Down / Page Up | Page the focused overview. | Page transcript text down/up across the full session. |
| Enter | Focus/open selected overview. | Open the selected search result at its matching passage. |
| Tab / Shift-Tab | Move between visible focusable panes/controls. | Move between available controls. |
| Esc | Close input/sheet, then return from overview to list. | Return to the originating overview/results with state preserved. |
| `p` | Project selector, including All projects and Current project. | Available through help/menu if needed. |
| `w` | Time-window selector. | Scope selector in cross-session results. |
| `h` | Harness selector: All harnesses, Codex, Claude Code. | Harness scope in cross-session results. |
| `/` | List: filter visible purpose/project/path/provider labels. Focused overview: find within its details. | Search within the open transcript. |
| `s` | Enter explicit cross-session content search. | Edit the cross-session content query in results. |
| `t` | Open the selected session's transcript. | Return path is Esc. |
| `r` / `f` / `n` | Prepare resume / fork / clean session. | No continuation shortcuts while reading; return to overview. |
| `]` / `[` | Focused overview: next / previous local match. | Next / previous match. |
| `c` | Copy menu: directory or available context. | Copy selected message/exchange with provenance. |
| `e` | List: expand/collapse explicit child sessions. Focused overview: expand/collapse recorded work. | Transcript: expand/collapse tool activity. |
| `R` | Refresh and apply the latest session ordering. | Refresh available from home after returning. |
| `,` | Sources/settings: theme, roots, exclusions, scan status, database size, cache controls. | Return to home for settings. |
| `?` | Contextual help. | Contextual help. |
| `q` | Ask to quit from home; close a nested overview first. | Return from the reading/search view. |

These are initial bindings to validate in a working TUI. Keep the overview and transcript bindings documented consistently; do not repurpose `n` for next match when it means clean session elsewhere.

When `q` would exit the application, show a centered **Quit Trailmark?**
modal with **Cancel** selected by default. Arrows or Tab select Cancel
or Quit; Enter activates that choice, `y` confirms quitting, and `n`
or Esc cancels. Repeated `q` must not bypass confirmation. Cancelling
preserves selection, filters, and reading position. Nested views retain
their back/close behavior; Ctrl+C remains an immediate exit.

Give the quit modal a raised surface, a theme-accent border and bold title,
and a short explanatory line. Use padded, clearly separated Cancel and Quit
buttons: the selected Cancel uses an accent fill and the selected Quit uses
the warning color, both with contrasting text. Retain a visible selection
marker in no-color mode. Separate the keyboard hints from the actions with
a rule, and keep the title, both buttons and complete hints readable down
to 40×12. This styling is specific to the quit confirmation.

## 7. Secondary transcript reading and search

Transcript reading is one explicit action away from a session. Arrows and `j`/`k` select messages; `J`/`K` scroll the text one rendered line at a time, while Page Down/Up scroll by a viewport. Show readable roles, timestamps where present, wrapped text, and collapsed tool activity. Wrap prose at word boundaries within the inner frame, preserving source line breaks and indentation; split overlong tokens when necessary. Copy actions retain the original text. Preserve session purpose and directory in the view header. Allow opening an overview excerpt at its source message.

`/` in the transcript searches that session, shows a match count, and highlights results. `[` and `]` move through matches. Reveal a matching tool output even when tools are normally collapsed.

Cross-session content search is an explicit `s` mode. Its header says **Search session content** and displays project, time, and harness scope. Results show matching snippets and open at the matching passage. Preserve the recent list's selection and filters when returning. The current selected or automatically expanded time window applies; offer All time when no match is found.

Clamp scrolling to the actual beginning and end of the visible content
in both the transcript and overview. Repeated scrolling beyond an edge
must not accumulate hidden movement: the first key in the opposite
direction moves immediately when more content exists. Resizing and
expanding/collapsing content must retain valid scroll bounds. A search
or next/previous-match jump must reveal its target without retaining an
unrelated text-scroll offset. Both line scrolling and Page Up/Down load adjacent transcript pages in either direction, including previously evicted messages, so the loading boundary never becomes a scrolling endpoint.

The home-screen `/` filter remains a lightweight way to narrow recognizable cards. It must not unexpectedly replace the day-grouped overview with relevance-ranked transcript results.

## 8. Visual and state rules

The 2026-10-16 visual review found that the initial renderer retained the information hierarchy but omitted much of the mockup styling: the selection colored only one marker, paths inherited the terminal foreground, cards lacked spacing and day rules, the pane divider occupied one row, and the footer followed short content. The requirements below define the approved refinement. They are acceptance targets, not a claim that every terminal has been visually verified.

### Palette

Use a shared semantic palette across all views. **Green** retains the
original [mockup palette](docs/mockups/index.html) and remains the default;
**Blue** and **Amber** offer cool and warm alternatives. **Monokai** adds
vivid accents on pure black or white. Choose a theme in
settings (`,`, Theme, Enter). Enter applies and saves it, while Esc cancels.
The choice survives restart and changes no session filters or scan state.
A save failure keeps the prior theme and reports the problem. Each theme
adapts to light/dark terminal backgrounds; no-color mode overrides color
rendering while retaining the saved preference.

The table below defines Green's true-color targets. The alternatives keep
the same roles and hierarchy; shortcut/focus accents, selection fills,
paths, rules, panel surfaces, and text all come from the chosen palette.
Warnings and search highlights retain distinct semantic treatment.
Reduced-color terminals may use their closest supported equivalents.

| Role | Dark | Light | Use |
| --- | --- | --- | --- |
| Panel | `#191e20` | `#fafbf7` | Application content surface, including blank cells |
| Raised surface | `#202729` | `#eef1e9` | Headers and secondary surface emphasis |
| Main text | `#e5e9e4` | `#24342d` | Purpose and body text |
| Secondary text | `#a2aeae` | `#59685e` | Provider, dates, section labels, hints |
| Accent | `#ade6c2` | `#246941` | Selection edge, focus, keys, primary action, transcript roles |
| Selection fill | `#253c32` | `#dfeddf` | Entire selected card/option and primary action |
| Path | `#b2d3dc` | `#315e70` | Project directories and worktree paths |
| Separator | `#364144` | `#c1c9bf` | Pane, day, message, header/footer, and action-area rules |
| Gold | `#ddc38c` | `#795b20` | Search-match treatment and qualified status labels |

Use contrasting foreground/background pairs for highlighted matches, not gold text alone. The implementation uses dark ink (`#24342d`) on a gold fill (`#ddc38c` in dark mode, `#f2dcaa` in light mode); the light status-label foreground remains `#795b20`. Keep the active match distinguishable by an additional focus cue. Any gold status label retains explicit wording such as **Agent reported**; the color does not establish success or verification.

### Alternative palettes

| Theme / variant | Panel | Accent | Selection fill | Path | Warning |
| --- | --- | --- | --- | --- | --- |
| Blue / dark | `#171e29` | `#91c7ff` | `#243c58` | `#a8dcd7` | `#ddc38c` |
| Blue / light | `#f6f9fe` | `#215fa0` | `#dfebfa` | `#246663` | `#795b20` |
| Amber / dark | `#242019` | `#edc17a` | `#473720` | `#b8d2c2` | `#f0a28b` |
| Amber / light | `#fdf9f1` | `#845000` | `#f4e4c8` | `#3d6b53` | `#963b2d` |
| Monokai / dark | `#000000` | `#a6e22e` | `#272822` | `#66d9ef` | `#fd971f` |
| Monokai / light | `#ffffff` | `#486b00` | `#eef5db` | `#006b80` | `#965000` |

Amber uses peach/terracotta warning colors to distinguish warnings from
its gold accent. Theme-specific body text, secondary text, raised surfaces,
and rules complete each palette in `internal/tui/theme.go`.

Monokai is inspired by its namesake rather than a literal editor-theme port.
Its main surface and outer gutters are `#000000` in dark mode and `#ffffff`
in light mode. Selection fills and raised headers remain distinct so item
boundaries are visible. Lime/olive accents, cyan/teal paths, orange warnings,
and yellow search highlights preserve the established semantic roles. Light
mode uses darker accent colors for readable contrast on white. The existing
terminal background report chooses the variant; dark remains the fallback
when no report arrives. No-color mode leaves background painting disabled.

### Structure and component treatment

- **Screen frame:** fill the terminal background, reserving two blank columns on each side and one blank row above and below the application content. Use one side column at widths of 40–43 and omit the top/bottom row below 14 rows to retain the 40×12 minimum. Apply the same inner viewport to headers, footers, panes, transcripts, selectors, and sheets; header/footer rules span that inner width. The footer stays at the bottom of the inner frame even for an empty or short list. Wrapping, page movement, and resize calculations must all use the inner viewport so ordinary text never touches the terminal edge.
- **Status bar:** use distinct semantic colors within the footer, following the supplied Codex status-bar reference: theme-accent bold shortcut keys and new-activity cues (green by default), muted action descriptions and separators, amber partial-coverage/source warnings, and blue informational/find text. Keep explicit wording for every state and retain clear key labels without color. Preserve the pinned full-width rule and readable narrow layouts.
- **Wide panes:** allocate approximately 45% to the list and the remainder to the overview, accounting for padding and the divider. Keep the divider continuous throughout the body. Short content must not move it sideways or shorten it.
- **Cards and day groups:** use the three-line card treatment in section 3, a blank row between cards, and day rules. The selection fill reaches the card's right edge and the marker spans its full height. Keep child indentation and relationship labels readable.
- **Overview:** use a prominent purpose, muted section labels, and a blue directory with a left rule. Separate continuation actions with a top rule and reserve their area below the scrolling detail. Resume receives the primary accent/fill treatment; labels and keyboard hints continue to identify all three actions.
- **Selectors and sheets:** use a consistent bordered panel, internal padding, selected-option fill, and separated heading/help or action rows. Fit the panel to the terminal; compact layouts must retain scrolling and reachable actions. Visual formatting must not alter copied commands or context.
- **Transcript and search:** use theme-accent role labels, muted timestamps, distinct path colors, and muted horizontal separators between messages or result groups. Highlight matches with a contrasting background, retaining the match count and navigation cues.
- **Supporting screens:** copy previews, help, and sources/settings reuse the same text, path, focus, and separator roles rather than introducing another palette.
- **Terminal adaptation:** use boldness, color, spacing, and alignment in place of the mockup's differing font sizes. Do not reproduce browser shadows, scene tabs, or presentation controls. Borders and padding consume terminal cells and must be included in width/height and scrolling calculations.

### Accessibility and state

- Use a single accent for selection/focus and text labels for providers. Reserve warning color for actionable issues; color must never be the only status cue.
- Keep purpose text high contrast, project/path readable, and age/provider visually secondary. Avoid dimming paths until they are difficult to read.
- Support light/dark terminal themes and a no-color mode. Do not require a special icon font; provide ASCII border/marker fallbacks.
- Show focus explicitly. Use space and alignment before adding borders. Avoid animation or spinners that shift card positions.
- Full paths wrap in the overview; list paths truncate intelligently. Do not rely on horizontal scrolling for basic identification.
- Background scans update a small status indicator. If new data changes order or day groups, show **New activity · R to refresh** and keep the selected row steady. Removed/excluded content and reclassified non-session files disappear immediately after a successful scan, including from selector counts, without requiring `R`. Preserve surviving selection and ordering; when the selected entry disappears, select the nearest remaining entry, refresh its overview, and explain the change.
- Explicit child sessions stay collapsed beneath their parent when both match the active filters, with a visible child count. If the parent is missing or outside the filters, show the matching child as a labeled entry under its own activity date. Expanding a parent shows individual child purposes, paths, and timestamps; do not present child activity as the parent's own last activity.
- Last activity comes from available session events. If only a fallback timestamp is available, label it inferred in details. Unknown dates remain reachable under All time.

## 9. Empty, partial, and error states

| Situation | What the TUI shows and offers |
| --- | --- |
| Empty index, no saved indexing scope | Ask which files to index: modified in the last 7 days, last 30 days (recommended), or All history. State that large archives can take hours; Enter saves and starts, Esc skips, and `q` opens quit confirmation. Do not start parsing before confirmation. |
| Initial scan with no cached sessions | A dedicated screen with an animated activity indicator, running file counts, elapsed time, and the current source. Discovery has no known total, so show activity rather than a percentage. Esc stops the scan and opens sessions already saved; `q` opens the usual quit confirmation. Keep these controls responsive during indexing. |
| Initial scan stopped or failed | Leave the progress screen, explain the outcome, and show any committed sessions. With no sessions, retain access to source settings and Rescan now. The next launch checks sources again, reusing unchanged indexed files. |
| Launch with cached sessions | Open the session list immediately and refresh in the background, preserving selection and explicit application of new ordering. |
| No sessions in the automatic seven-day scope | Try 30 days, then All time; show the chosen scope in the header. If no sessions exist at any age, retain the empty state and source-status actions. |
| No sessions in an explicitly selected window | Keep the chosen window and show its empty state with scope/source-status actions. |
| No configured/detected sources | **No session sources found** with Add root and source settings. |
| No metadata filter matches | The active filter and Clear filter; preserve the selected scope. |
| No transcript-content matches | Active project/time/harness scope, edit query, and broaden scope actions. |
| Some logs cannot be parsed | Valid sessions remain visible; a compact partial-coverage notice opens source details. |
| Source temporarily unavailable | Cached cards labeled stale, last successful scan visible, unavailable source actions explained. |
| Purpose/cwd missing | Explicit unavailable labels; do not invent a goal or substitute a directory. |
| Clipboard unavailable | Selectable command/context text and a clear failed-copy state. |

Settings expose roots, exclusions, and cache clearing within the TUI. The cache-clear action explains that it removes Trailmark's derived data, preserves source logs and configuration, and that scanning can rebuild it. Keep maintenance controls out of the normal recent-work flow.

Settings also expose **Indexing scope**. Its file-modification cutoff
defers older files not yet indexed; cached history is retained and kept
fresh. Changing scope saves the choice and rescans. Show limited coverage
in browsing and empty states, with Settings as the route to older history.
The list's **All time** filter covers indexed sessions only and does not
expand indexing scope. Keep deferred files distinct from failed or excluded
files in progress counts.

## 10. UX acceptance

The [PRD](PRD.md) defines measurable recognition and performance targets. The design must additionally demonstrate:

- Purpose and project/path are recognizable before opening a transcript.
- Selecting a session immediately updates the overview without a second action in the wide layout.
- Resume, fork, and clean session have understandable history semantics and an unambiguous target directory.
- A user can complete the main flow by keyboard with visible hints, without memorizing commands.
- At 80×24, all primary information and actions remain reachable without overlap or clipped controls.
- Dark/light and no-color renders meet the [PRD visual acceptance criteria](PRD.md#visual-acceptance): full-card selection, readable paths, clear card/day separation, continuous pane rules, and stable footer/action placement. Compare against the mockup gallery; record real-terminal inspection separately from automated checks.
- Leaving a transcript, search, filter selector, or continuation sheet returns to a stable, familiar overview.
- Supporting search and source-management features do not crowd out recent-session recognition.
