# Trailmark — TUI experience

- **Status:** Proposed interaction design; implementation not started
- **Date:** 2026-10-14
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

Offer Today, 3 days, 7 days, 30 days, and All time in the time selector (`w`). These are view filters, not retention limits: sessions older than seven days remain indexed and accessible. The list footer offers **Show all time** as a direct route to older work. The initial release starts each launch at 7 days/all projects and preserves changes while navigating within that run. The header must always show the current scope. Dates with no sessions need no empty headings.

### Harness filtering

The header includes a **Harness** selector alongside time and project. Press `h` or select the control to choose **All harnesses**, **Codex**, or **Claude Code**. Show per-harness session counts within the selected time/project scope. Harness refers to the coding tool that created a session, rather than its model or a child-agent role.

The active selection remains visible in the header and combines with existing time, project, and text filters. Opening a session or returning from its transcript preserves the selection. Cross-session content search uses the same harness scope. **All harnesses** clears only this filter; a zero-result combination explains which filters are active and lets the user adjust them.

### Wide layout: 110 columns or more

The list takes about 45% of the width and the overview takes the rest. Selecting another card updates the overview without opening a new screen. This wireframe uses fictional session content; it shows information placement rather than exact terminal dimensions.

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

Each card normally uses three lines: purpose; project/path; provider/activity. Cap the purpose at two lines when necessary, keeping card height predictable. Do not draw a heavy box around every card. Use whitespace, day headings, and one selection marker to establish structure.

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
| Arrows or `j`/`k` | Move selection; scroll when overview has focus. | Scroll transcript or select result. |
| Enter | Focus/open selected overview. | Open the selected search result at its matching passage. |
| Tab / Shift-Tab | Move between visible focusable panes/controls. | Move between available controls. |
| Esc | Close input/sheet, then return from overview to list. | Return to the originating overview/results with state preserved. |
| `p` | Project selector, including All projects and Current project. | Available through help/menu if needed. |
| `w` | Time-window selector. | Scope selector in cross-session results. |
| `h` | Harness selector: All harnesses, Codex, Claude Code. | Harness scope in cross-session results. |
| `/` | Filter visible purpose/project/path/provider labels. | Search within the open transcript. |
| `s` | Enter explicit cross-session content search. | Edit the cross-session content query in results. |
| `t` | Open the selected session's transcript. | Return path is Esc. |
| `r` / `f` / `n` | Prepare resume / fork / clean session. | No continuation shortcuts while reading; return to overview. |
| `]` / `[` | No action. | Next / previous match. |
| `c` | Copy menu: directory or available context. | Copy selected message/exchange with provenance. |
| `e` | Expand/collapse explicit child sessions for selected parent. | No action. |
| `R` | Refresh and apply the latest session ordering. | Refresh available from home after returning. |
| `,` | Sources/settings: roots, exclusions, scan status, cache controls. | Return to home for settings. |
| `?` | Contextual help. | Contextual help. |
| `q` | Quit from home; close a nested overview first. | Return from the reading/search view. |

These are initial bindings to validate in a working TUI. Keep the overview and transcript bindings documented consistently; do not repurpose `n` for next match when it means clean session elsewhere.

## 7. Secondary transcript reading and search

Transcript reading is one explicit action away from a session. Show readable roles, timestamps where present, wrapped text, and collapsed tool activity. Preserve session purpose and directory in the view header. Allow opening an overview excerpt at its source message.

`/` in the transcript searches that session, shows a match count, and highlights results. `[` and `]` move through matches. Reveal a matching tool output even when tools are normally collapsed.

Cross-session content search is an explicit `s` mode. Its header says **Search session content** and displays project, time, and harness scope. Results show matching snippets and open at the matching passage. Preserve the recent list's selection and filters when returning. The seven-day window still applies until the user explicitly broadens it; offer All time when no match is found.

The home-screen `/` filter remains a lightweight way to narrow recognizable cards. It must not unexpectedly replace the day-grouped overview with relevance-ranked transcript results.

## 8. Visual and state rules

- Use a single accent for selection/focus and text labels for providers. Reserve warning color for actionable issues; color must never be the only status cue.
- Keep purpose text high contrast, project/path readable, and age/provider visually secondary. Avoid dimming paths until they are difficult to read.
- Support light/dark terminal themes and a no-color mode. Do not require a special icon font; provide ASCII border/marker fallbacks.
- Show focus explicitly. Use space and alignment before adding borders. Avoid animation or spinners that shift card positions.
- Full paths wrap in the overview; list paths truncate intelligently. Do not rely on horizontal scrolling for basic identification.
- Background scans update a small status indicator. If new data changes order or day groups, show **New activity · R to refresh** and keep the selected row steady. Removed/excluded content must still disappear as required by the PRD lifecycle; select the nearest remaining entry and explain the change.
- Explicit child sessions stay collapsed beneath their parent when both match the active filters, with a visible child count. If the parent is missing or outside the filters, show the matching child as a labeled entry under its own activity date. Expanding a parent shows individual child purposes, paths, and timestamps; do not present child activity as the parent's own last activity.
- Last activity comes from available session events. If only a fallback timestamp is available, label it inferred in details. Unknown dates remain reachable under All time.

## 9. Empty, partial, and error states

| Situation | What the TUI shows and offers |
| --- | --- |
| Initial scan in progress | Progress and discovered cards as they become available; navigation remains usable. |
| No sessions in seven days | **No sessions in the last 7 days** with All time and source-status actions. |
| No configured/detected sources | **No session sources found** with Add root and source settings. |
| No metadata filter matches | The active filter and Clear filter; preserve the selected scope. |
| No transcript-content matches | Active project/time/harness scope, edit query, and broaden scope actions. |
| Some logs cannot be parsed | Valid sessions remain visible; a compact partial-coverage notice opens source details. |
| Source temporarily unavailable | Cached cards labeled stale, last successful scan visible, unavailable source actions explained. |
| Purpose/cwd missing | Explicit unavailable labels; do not invent a goal or substitute a directory. |
| Clipboard unavailable | Selectable command/context text and a clear failed-copy state. |

Settings expose roots, exclusions, and cache clearing within the TUI. The cache-clear action explains that it removes Trailmark's derived data, preserves source logs and configuration, and that scanning can rebuild it. Keep maintenance controls out of the normal recent-work flow.

## 10. UX acceptance

The [PRD](PRD.md) defines measurable recognition and performance targets. The design must additionally demonstrate:

- Purpose and project/path are recognizable before opening a transcript.
- Selecting a session immediately updates the overview without a second action in the wide layout.
- Resume, fork, and clean session have understandable history semantics and an unambiguous target directory.
- A user can complete the main flow by keyboard with visible hints, without memorizing commands.
- At 80×24, all primary information and actions remain reachable without overlap or clipped controls.
- Leaving a transcript, search, filter selector, or continuation sheet returns to a stable, familiar overview.
- Supporting search and source-management features do not crowd out recent-session recognition.
