# Trailmark — Product Requirements Document

- **Working name:** Trailmark (not name-cleared)
- **Status:** Draft
- **Date:** 2026-10-14
- **Visual requirements updated:** 2026-10-16
- **Related:** [MRD](MRD.md)
- **Interaction design:** [TUI experience and wireframes](UX.md)

## 1. Summary

Trailmark is a local Go TUI for remembering recent coding-agent work and deciding how to continue it. Its primary screen prefers the last seven days of sessions and expands to older history when that scope is empty, with purpose and project/path taking visual priority.

The primary workflow is to recognize recent work, recall where it happened, and choose resume, fork, or a clean session. Transcript reading and content search remain supporting MVP features. The TUI is the product surface; a separate command suite is outside the MVP.

## 2. Problem and goals

Session history is fragmented across agent-specific directories and formats. After several days across projects, a developer needs reminders of what each session was for and which directory it belongs to. A useful overview should support recognition without requiring a remembered query or a transcript read. When the overview is insufficient, the user can inspect or search the session log.

### Goals

- Open directly onto recent work across supported agents and projects.
- Make session purpose and project/path recognizable in the list; use agent and recency as supporting context.
- Provide a selected-session overview with latest context and clear resume/fork/clean-session choices.
- Keep the primary flow usable without typing a query, reading a transcript, or invoking subcommands.
- Support transcript reading, content search, and copying selected context when needed.
- Keep provider parsing isolated so new log formats can be added incrementally.
- Leave original log files untouched and keep transcript data on-device by default.

### Non-goals for MVP

- Starting, resuming, forking, attaching to, steering, or terminating coding agents directly. The MVP helps choose a continuation mode and prepares/copies validated native commands; executing them is deferred.
- Requiring sessions to be created through Trailmark or ACP.
- Cloud sync, shared team history, telemetry, or a hosted account.
- Writing to, migrating, or cleaning up native agent logs.
- A separate list/search/show/scan CLI, machine-readable output, scripting APIs, or shell automation.
- Model-generated summaries, semantic search, and additional providers before validating the recent-work overview.

## 3. Users and core workflows

### Remember recent work — primary

The user opens Trailmark after a break. The last seven days are visible across projects, grouped by day; when empty, the initial view expands to 30 days, then All time, until available sessions appear. Each compact card foregrounds what the session was for and its project/path. Moving the selection updates an overview with the full cwd, initial request, and latest substantive exchange. The user can narrow to a project or extend the time window without leaving the screen.

### Choose how to continue — primary

The user decides whether the selected session's history remains useful. Resume continues the same native session; fork creates a separate native conversation from existing history where the provider supports it; clean starts with no inherited conversation in the chosen directory. A continuation sheet shows the exact target directory, history behavior, provider capability, and prepared command. The MVP copies or displays the command; it does not launch the provider.

### Inspect or search a transcript — secondary, included in MVP

The user opens a transcript only when they need more detail. They can scroll, search within that session, and move between highlighted matches. An explicit content-search mode can search across sessions within the visible project/time scope and jump to a matching passage. Returning restores the overview's selection and filters. Selected exchanges can be copied with provenance.

Across these workflows, a malformed or unavailable source must not prevent browsing other sessions. Scan coverage and freshness must explain the limits of the results.

## 4. Scope

### MVP harnesses

- Linux is the first release and acceptance platform.
- Claude Code native session logs.
- Codex CLI native session logs.

`trailmark` launches the recent-work TUI. MVP purpose labels and previews are local and deterministic. All normal operations are available inside the TUI.

### Later adapters

- GitHub Copilot CLI.
- Muse Code.
- Pi.
- OpenCode, if its log format and access are practical.

### MVP capabilities

1. **Discovery and indexing**
   - Detect known Claude Code and Codex log locations on Linux, with configurable additional roots.
   - Support root/project exclusions before indexing and remove already derived data when a new exclusion is applied.
   - Read source logs without modifying them; tolerate partial writes, truncated records, and unknown fields. Ignore confidently identified non-session JSONL such as plugin telemetry, and remove its stale derived cards on reindex. Do not reject a damaged or incomplete native log solely because it lacks successfully parsed messages.
   - Normalize available session IDs, title/first prompt, agent, timestamps, original cwd, project/worktree identity, source path, and explicit session relationships.
   - Maintain an incremental local index/cache so repeated launches do not need to reparse every transcript. Storage format and exact location are implementation decisions.
   - Show scanned roots, last successful scan, progress, and counts of indexed, partial, unavailable, and excluded sources. Explain empty results when coverage is incomplete.
2. **Session list and cards**
   - Default to today plus the previous six calendar days in the user's local timezone. Include sessions by last recorded activity, even if they started earlier. Offer Today, 3 days, 7 days, 30 days, and All time windows, plus a **Show all time** shortcut in the list footer. These are view filters, not retention limits; older sessions remain indexed and accessible.
   - At startup and when changing project/harness in automatic mode, select the first populated window from 7 days, 30 days, and All time. Apply the same policy when initial scan results arrive and refreshed sessions are applied. Test availability within the active project and harness, ignoring metadata text filters; display the actual chosen window in the header. An explicit time selection disables automatic adjustment for the rest of the run. Truly empty scopes retain an honest empty state; unknown timestamps remain reachable through All time.
   - Group sessions by their last-active day, using Today, Yesterday, then weekday/date headings. Within each day, sort newest first. A session appears once, not once per event. Unknown activity dates remain discoverable under All time with an explicit unknown label.
   - Make the purpose label the first line and project/path the second; show provider and relative last activity as quieter supporting information. Retain enough path context to distinguish same-named directories/worktrees.
   - Show a selected-session overview beside the list where space permits: purpose, full original cwd, provider, precise last activity, initial request, and latest substantive exchange. Provide the same information in a keyboard-accessible single-pane view at narrower widths.
   - Keep explicit child sessions collapsed beneath their parent when both satisfy the active filters, with a visible child count and an expand action. A matching child whose parent is missing or outside the filters remains a visible labeled entry under its own activity date. Children remain searchable; do not infer parentage from timing.
   - Keep selection stable during background scans. Indicate new activity and let the user apply an updated ordering explicitly rather than moving the selected row under them. Apply confirmed removals immediately, including files reclassified as non-session data: cards and selector counts must agree with the refreshed index without requiring `R`. Preserve surviving selection and ordering; if the selection disappears, show the nearest remaining session and refresh its overview.
   - Clearly mark missing or inferred metadata rather than presenting guesses as facts.
3. **Search and filters**
   - Keep list filtering and transcript-content search distinct. `/` in the session list filters visible purpose labels, project/path, and provider; it preserves chronological order and does not implicitly search every transcript.
   - The `/` action depends on focus: in the session list it filters cards; in the focused selected-session overview it finds literal, case-insensitive text within the displayed details. Overview find highlights and reveals matches, reports no results explicitly, and supports `[`/`]` navigation without changing list filters. Input owns printable action keys, cancellation restores the applied query, and changing sessions resets local find state.
   - Provide a clearly labeled **Harness** selector (`h`) alongside project and time filters: **All harnesses**, **Codex**, and **Claude Code** for the MVP. Harness means the coding tool that created the session; model and child-agent role are separate concepts.
   - Combine harness, repository/path, and time filters by intersection. Keep the active harness visible, preserve the other filters when it changes, and retain it when opening/closing an overview, transcript, or continuation sheet. Apply the same harness scope to cross-session content search. Choosing All harnesses clears only the harness filter.
   - Start with all projects visible; provide a current-project shortcut and an obvious toggle back to all projects. Show the active scope. Outside a recognized project, explain why the shortcut is unavailable and retain path filtering.
   - In a transcript, `/` searches that session. An explicit content-search mode searches conversation and tool text across sessions within the current project/time scope, with visible controls to broaden it. Default to literal, case-insensitive term search with quoted phrases; paths and filenames must remain useful queries.
   - Return session-grouped results with highlighted matching snippets, match counts, and an indication of whether the match is in metadata, conversation, or tool output.
   - Rank textual relevance before recency. For otherwise comparable matches, prioritize conversation over tool output; use recency as a tie-breaker. Repeated tool output must not dominate simply by repetition. Allow explicit conversation/tool/all-text search scope.
   - Open a selected transcript match at its message and offer next/previous matches. A metadata-only match opens the session overview and identifies the matching field.
   - Keep search useful when a provider log cannot be fully parsed; show partial-index status.
4. **Session detail**
   - Open a selected card to read the available transcript in chronological order.
   - Show roles and timestamps where present; tool activity may be collapsed by default and expandable.
   - Reveal a selected tool-output match even when tool activity is normally collapsed. Preserve search, filters, selection, and scroll position when returning to results.
   - Keep the session's source path and working directory easy to inspect/copy.
5. **Purpose labels, previews, and evidence**
   - Use the native session title when available; otherwise use a concise excerpt of the first substantive user request and label it as a request excerpt. If neither is meaningful, show **Purpose unavailable**. Do not substitute session IDs or injected setup text for purpose.
   - Show the label's origin in the overview. A native title or initial request may be stale after a change of direction; keep the latest request/exchange visible alongside it rather than treating either as a verified summary of the entire session.
   - Show excerpts labeled **Initial request** and **Latest substantive exchange**. These describe recorded messages; they do not assert a single lasting goal or verified session outcome.
   - Prefer the first substantive user request and the latest substantive user message plus its subsequent assistant response when available. Exclude provider metadata and recognized injected setup text. Version these extraction rules with the adapter fixtures.
   - Mark truncation and missing messages explicitly. If only a request or response is present, label it accordingly; if no suitable content exists, show an unavailable state. Do not synthesize missing text.
   - Every excerpt must navigate to its source message. Distinguish **Agent reported** claims from **Recorded tool result** evidence. Neither establishes the current repository state or that the whole task succeeded.
   - Include file-change hints only when supported by source events, label their provenance, and avoid treating a mentioned filename as a verified modification.
6. **Continuation choices and context copying**
   - Offer Resume, Fork, and Clean session from the selected overview. Explain the history behavior of each in the continuation sheet. All choices retain the selected original cwd as their proposed target; show the exact path before preparing a command.
   - Resume targets the original native session and provider store. Fork requires validated native support and sufficient session metadata; otherwise show **Fork unavailable for this provider/session** with a reason. Do not emulate it by modifying source logs or silently starting a clean session.
   - Clean session proposes the selected provider in the selected directory with no inherited transcript. Normal provider/project configuration may still apply. It must not silently inject a generated handoff or copied conversation. Manual context copying is a separate explicit action.
   - Allow explicit selection and copying of a message or exchange. Include provider, session identity, source path, and a message ID or source record locator; include timestamps when available.
   - Show what will be copied. If clipboard access is unavailable, provide selectable plain text with the same provenance and do not report a successful copy.
   - Display/copy native continuation commands only when the adapter has a validated contract and enough source metadata. Include required cwd and provider-home context, quote arguments correctly, and explain unsupported or missing-directory cases. Do not substitute the directory from which Trailmark was launched when the recorded cwd is missing.
   - Label the final action **Copy command**, not **Launch**. Never execute it in the MVP or imply that copying resumed, forked, or created a session. Direct native launch handoff can be designed separately after the overview is validated.

### Project identity and session relationships

- Preserve the cwd recorded in the log even if it no longer exists. Keep it separate from any current resolved path or project grouping.
- Use explicit source metadata or verified local Git common-directory information to group worktrees under a project. Keep each worktree path visible and separately filterable.
- Do not merge independent clones or moved directories solely because their basename or remote URL matches. Where identity cannot be established, retain separate path groups and mark unavailable metadata. Non-Git sessions remain browseable by cwd.
- Identify logical sessions using provider, native session ID, and source-store identity. Rediscovering the same source through overlapping roots or symlinks must not create duplicate cards. Conflicting sources must not be silently merged.
- Preserve and display explicit parent/child, fork, or continuation references when the provider supplies them. Keep distinct native sessions distinct; do not infer relationships from similar text or timing. Full related-session navigation is a later enhancement.

### Next enhancements

| Enhancement | Value and boundary |
| --- | --- |
| Bookmarks and personal labels | Revisit useful sessions; store annotations in Trailmark's own data, without modifying native logs. |
| Previewable Markdown context export | Create a handoff from selected content with provenance and explicit omissions; let the user inspect the export before writing it. |
| Related-session navigation | Follow explicitly linked work across sessions while preserving source boundaries. |

Semantic search, generated summaries, and additional adapters remain deferred until the recent-work overview is validated against the tasks in section 9 and the direct alternatives in the MRD. Purpose-label quality must be assessed before adding more features; if deterministic labels fail the recognition target, revisit their design. Any future model backend requires a separate privacy and quality contract.

## 5. TUI interaction

- The normative [UX specification](UX.md) defines the screen hierarchy, wireframes, key map, continuation sheet, visual rules, and empty/error states.
- Launch with focus on the recent-session list and its newest entry; no welcome wizard, empty search prompt, or command palette should block the overview.
- Use arrows/j/k for list movement, Enter for the selected overview, Esc to return, and q to quit from the home screen. Contextual key hints must explain scope-dependent actions.
- When `q` would quit, require a confirmation modal with Cancel selected by default. Cancelling preserves browsing state; nested-view back/close behavior and Ctrl+C remain unchanged.
- Use two panes at 110 inner content columns or wider (normally 114 terminal columns with side gutters) and a single-pane flow below that. Core operations must remain usable at 80×24; resizing must preserve selection and context.
- Give purpose and directory the strongest visual emphasis. Use restrained borders and color, textual provider labels, visible keyboard focus, and readable contrast in light/dark and no-color terminals.
- Treat the mockup palette and separation rules in [UX section 8](UX.md#8-visual-and-state-rules) as the visual target. Use green for selection/focus, blue for paths, muted metadata and rules, and a contrasting search-match background. A selected session has a full-card background and a continuous left marker; color must not be its only selection cue.
- Make the status bar visually scannable with green shortcut keys, muted descriptions, amber coverage/source warnings, and blue informational/find status. Preserve labels and no-color usability.
- Reserve a small blank gutter around application content: normally two columns on each side and one row above/below, reducing it at the minimum terminal size. Compute text wrapping and scrolling from the remaining inner viewport; ordinary transcript text must fit without horizontal scrolling. Preserve exact copy payloads.
- Keep wide panes at their allocated widths with a full-height divider. Use full-width header/footer rules, pin the footer to the bottom, and separate continuation actions from scrolling overview content. Day rules and whitespace distinguish sessions without a heavy border around every card.
- Apply the same palette and hierarchy to selectors, continuation preparation, copying, transcript/search, and settings. Terminal cell dimensions replace browser typography and pixel spacing; the mockup's outer presentation controls and browser chrome are not application features.
- Keep transcript and search views one explicit action away and preserve the overview's filters, selection, and scroll position when returning.
- Refresh, source status, exclusions, and cache clearing are TUI operations; routine use must not require a CLI subcommand.
- No action in the MVP may mutate or delete provider logs.

## 6. Minimal launch interface

The MVP requires only the TUI entry point and standard informational flags:

~~~text
trailmark                 # open the recent-work overview
trailmark --help
trailmark --version
~~~

List/search/show/scan/status/cache subcommands, a `tui` alias, and machine-readable output are outside MVP scope. Reconsider them only when an actual automation need arises. An interactive-terminal check should give a concise explanation when a TUI cannot run.

Internal session references must still disambiguate provider and source store. Cache clearing in the TUI retains configuration and exclusions and never deletes provider logs. A subsequent scan rebuilds the cache from sources that remain included.

## 7. Technical direction

- **Language:** Go.
- **TUI stack:** reuse gh-boardwalk's Go stack: Bubble Tea v2 for the TUI; Bubbles for reusable widgets; Lip Gloss v2 for layout and styling. Cobra may handle the minimal launch/help/version interface; its availability does not require a subcommand hierarchy. Use Huh v2 for interactive prompts if needed, Glamour v2 for Markdown transcript rendering, and teatest for TUI tests where appropriate.
- **Reference:** gh-boardwalk's ADR 0007, “Go stack: Cobra + Charm,” and its current Go module versions. Trailmark should choose compatible versions when implementation begins rather than copying a stale lock blindly.
- **Architecture:** provider-specific read-only adapters feed a shared normalized session model; indexing, excerpt extraction, search, and TUI operate on that model. Preserve source message/record locators for search, copying, and excerpt navigation.
- **Responsiveness:** cached browsing must not wait for a full rescan. Indexing must report progress and permit cancellation; large transcripts must render incrementally with bounded memory rather than loading/rendering the whole transcript before showing a passage.
- **Portability:** develop and test on Linux first; keep paths and adapters isolated so other operating systems can be added.

## 8. Privacy, safety, and reliability

- Treat transcripts as sensitive: the MVP makes no network requests, including for telemetry, model calls, or automatic updates. Explicit user copying is the only context transfer described by the MVP; no automatic export or upload is permitted.
- Keep the source logs read-only. Store only derived index/cache data in Trailmark's own application-data location.
- Treat full-text indexes and excerpts as sensitive copies of source content and use permissions restricted to the current user. Document the data location and clearing behavior.
- Do not log transcript bodies or secrets to diagnostic output.
- Handle missing directories, permission errors, malformed JSONL, partial final lines, and provider schema changes without hiding other valid sessions.

### Derived-data lifecycle

- Apply root/path exclusions before opening matching files. If a project exclusion requires source metadata to identify the project, read only the metadata needed to decide inclusion and do not index the excluded transcript. Applying a new exclusion must remove its derived text, snippets, and search results without modifying the sources.
- When a successful complete scan of an accessible root confirms a source was deleted, remove its derived content and search entries. Show the change in scan status.
- A missing root, permission failure, interrupted scan, or parse error is not proof of deletion. Preserve existing cached content, mark it stale/unavailable or partial as appropriate, and expose its last successful scan time.
- If cached content is shown while a source is unavailable, label it as cached and disable unavailable source navigation. Restoring access and rescanning must reconcile the session without duplicate cards.
- Explicitly removing a configured root must remove its derived content unless the same source remains included through another root. Explain this behavior in configuration help.
- Cache clearing removes all derived transcript/index data while retaining configuration and exclusions. It does not promise forensic secure erasure. Later user annotations must have a separate lifecycle from rebuildable caches.

## 9. MVP acceptance criteria

### Functional acceptance

- Claude Code and Codex sessions from sanitized fixture datasets appear in one list with correct provider, ID, original cwd, source path, and timestamps when available.
- The initial screen shows seven local calendar days grouped by last-active day. Fixtures cover a long-running session started before the window, timezone/day boundaries, unknown timestamps, older-only history, and explicit child sessions. Background refresh preserves selection and does not silently reorder the list.
- Purpose and project/path are readable without opening a transcript or entering a query. The overview distinguishes native titles from request excerpts and displays full cwd, latest context, and continuation choices. Same-named projects/worktrees remain distinguishable.
- Harness filtering is available by keyboard and visible in the header. Codex and Claude Code choices combine correctly with project/time filters and scope content-search results; All harnesses restores the combined view without clearing project/time. Zero-result combinations have an explicit empty state.
- A user can navigate using only the keyboard, assess a preview, search for a phrase or filename, open at the matching message, move between matches, and return with their filters and selection preserved.
- Search snippets identify their source field, highlight matches, and expose tool-output matches even when tools are collapsed. Ranking fixtures include repetitive tool output and relevant conversation matches.
- Excerpts navigate to source messages, mark missing/truncated content, and distinguish agent claims from recorded results. Fixtures include changed goals, abrupt endings, setup-only logs, and unsupported completion claims.
- Current-project scope groups verified worktrees while retaining their cwd; independent clones and unknown/non-Git paths remain distinguishable. Fixtures cover missing/moved directories, overlapping discovery roots, explicit child/fork references, and conflicting IDs across stores.
- Resume/fork/clean-session sheets explain history behavior and show the exact target directory. Commands use validated provider contracts and source-store context. Unsupported forks and missing directories have explicit reasons; clean-session commands inherit no transcript and no agent is launched.
- Copying a selected exchange includes its provenance. Clipboard failure is visible and offers selectable text.
- Incremental rescans correctly notice new/changed logs, handle truncation/replacement, avoid duplicate cards, and leave source file content unchanged.
- A malformed or actively written log does not prevent other sessions from appearing; coverage and freshness identify the affected source.
- Exclusions, confirmed deletion, temporary root unavailability, removal of a configured root, and cache clearing follow the lifecycle rules in section 8.
- The MVP operates without network requests. Parser tests use representative, sanitized fixtures; TUI tests cover wide and 80×24 layouts, resize, list/overview, project/time filtering, continuation sheets, search, transcript, copying, empty states, and partial/error states. Refresh and source/cache management are reachable without subcommands.

### Visual acceptance

- Compare fictional fixture renders with the [mockup gallery](UX.md#concept-screen-gallery) at 120×40 and 160×45, and exercise the list/overview flow at 80×24. Check dark and light palettes plus no-color output.
- Verify full-screen dimensions, continuous pane separation, full-width header/footer rules, and a footer that stays at the bottom with short or empty content. No content may overflow into another pane.
- The selected card's background and left marker span every card row, including padded cells. Paths remain distinct from metadata; adjacent cards and day groups remain recognizable without color.
- Overview sections and the continuation action area are visibly separated. Selectors and preparation sheets have consistent framing; transcript messages and search-result groups have separators; matching text uses a contrasting background.
- Resize, long paths/titles, expanded child sessions, and scrolling must preserve readable content, visible selection/focus, reachable actions, and exact copy payloads. Borders and padding count toward the available terminal dimensions.
- Page Up/Down must traverse the full transcript across loaded-message boundaries, including history evicted from the bounded reading buffer.
- Scrolling beyond either end must not accumulate an invisible offset. The first reverse scroll must move immediately, and search jumps must reveal their target after prior scrolling. Exercise this in both the transcript and overview with long content.
- Automatic time selection must show older-only sessions at startup, after an initial scan, and when choosing an inactive project/harness. Verify 30-day and All-time fallback, unknown timestamps, truly empty scopes, and preservation of explicit time choices and text-filter scope.
- Overview find must work with offscreen and repeated matches, no matches, cleared/cancelled queries, narrow/wide layouts, and resize. Find phrases and path substrings even when wrapping splits them across lines. Match text rather than terminal style sequences; the active match stays distinguishable and continuation actions remain reachable.
- Exercise the emitted terminal control sequences as well as the model's text layout. Unsupported spacing optimizations must not shift pane boundaries or overlap text on supported terminals.
- Record automated rendering checks separately from a real-terminal visual check and from the human recognition tasks below. A passing text-content test or a browser mockup is not evidence of terminal color fidelity.

### Primary UX validation

Use a permitted corpus of recent work familiar to the participant, with several projects, overlapping purposes, multiple sessions per project, and different worktrees. Establish the intended session and directory for each task in advance. Give the user no session IDs or exact titles:

1. Recognize a session from yesterday's work across projects.
2. Recognize a session from three to seven days ago from its purpose and directory.
3. Distinguish two sessions with similar purposes in different projects.
4. Identify the intended worktree among sessions in the same repository.
5. Recognize which recent session in a project contains the context the user wants to continue.

The provisional target is correctly identifying session, purpose, and directory within 15 seconds in at least four of five tasks using the list/overview without transcript reading or content search. Record confusing labels, path ambiguity, navigation effort, and whether the user needed deeper inspection. A dataset with technically valid but unrecognizable excerpts does not pass the UX target.

For a chosen session, ask the participant to explain resume, fork, and clean-session behavior, identify the target directory, and prepare an available choice without executing it. Assess clarity of the choice; do not prescribe a single correct continuation strategy for every task. Repeat core flows at 80×24 and in a wide terminal. Compare with direct alternatives on the same tasks; record setup friction separately. These are proposed validation targets, not measured results.

### Secondary transcript/search validation

Verify that the user can open a session's transcript, search a remembered phrase within it, navigate matches, and return to the same overview context. Verify an explicit cross-session content search, broadened project/time scope, and copying an exchange with provenance. Use a provisional 30-second target to reach a known passage, measured separately from primary recognition tasks.

### Performance targets

Initial targets below are provisional until the first implementation is measured. Record any revisions with their rationale rather than presenting unmeasured goals as results.

- Declare a sanitized or synthetic Linux benchmark corpus with at least 10,000 sessions and 1 GiB of source logs across both MVP providers, including at least one 50 MiB transcript. Record corpus composition, event counts, hardware, storage, OS, and build version.
- With an existing index, show an interactive session list within 2 seconds; a background refresh must not block it.
- Selection changes must update a cached overview within 100 ms p95 over at least 100 representative selection changes. Full-transcript parsing/rendering must not block list navigation.
- Warm search latency must be at most 200 ms at p95 from query submission to visible results over at least 100 representative queries. Record index size and search scope.
- Show the initial viewport or selected match of the 50 MiB transcript within 1 second with an existing index; avoid rendering the entire transcript first.
- Measure and report cold indexing duration and peak memory separately. First-run indexing must show progress and allow cancellation and browsing of already indexed sessions. Set a numeric memory budget after the initial measurements.

## 10. Settled scope and open decisions

**Settled for the first release:** Linux; Claude Code and Codex; a seven-day recent-work TUI; purpose/project-first overview; resume/fork/clean-session decision support and validated command preparation; secondary transcript reading/search and explicit context copying; read-only native sources; no network requests. Native command execution and a separate CLI command suite are deferred. The repository uses the [MIT license](LICENSE).

Remaining decisions:

1. Exact native log locations, supported provider versions, versioned parser/extraction contracts, and resume/fork/clean-session command capability per adapter.
2. Index storage format/location and concrete configuration syntax for roots and exclusions.
3. Installation channels, public release scope, and final project name; later macOS/Windows support.
4. Performance target calibration and memory budget after the first measured implementation.
5. Whether recent-work recognition and continuation-choice validation support the proposed differentiation, and which later enhancement to prioritize.
