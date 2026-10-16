# Trailmark — Market Requirements Document

- **Working name:** Trailmark (not name-cleared)
- **Status:** Draft
- **Date:** 2026-10-14
- **Visual requirements updated:** 2026-10-16

## 1. Opportunity

Developers increasingly work across several AI coding agents. Each agent keeps its own session history, format, and project picker. After several days across different projects, it is easy to forget what each session was for, where the work happened, and which conversation would be useful to continue.

Trailmark is a local overview of recent coding-agent work. It helps users recognize a session's purpose and project/path at a glance, recall where they left off, and decide whether to resume, fork, or start clean. It reads existing native logs and opens directly onto the last seven days, expanding to 30 days or All time when needed to show available work. Transcript reading and content search support this decision and remain secondary MVP capabilities.

## 2. Target users

**Primary:** developers returning to work spread across multiple projects and coding-agent sessions over the past few days or week. They need recognition from a recent-work overview, without having to remember a query or session ID. Claude Code and Codex users are the initial audience; using both is not a prerequisite for value.

**Secondary:** developers who need a quick history of agent-assisted work for handoffs, debugging, or reviewing decisions.

### Core workflows

1. **Remember recent work:** open Trailmark and scan recent sessions by day, with purpose and project/path visible before inspecting any transcript.
2. **Choose how to continue:** inspect the selected session's overview and decide between resuming its history, forking from it where supported, or starting with clean conversation history in the same project/directory.
3. **Inspect when needed:** read or search a transcript to clarify what happened, and optionally copy useful context with a source reference. This supports the first two workflows.

## 3. Market needs

Users need to:

1. See the last seven days of work from supported agents together, grouped by day and ordered by last session activity, with older work reachable.
2. Identify session purpose and project/path at a glance; agent and recency provide supporting context.
3. Recall where work left off without first reading a long transcript, while distinguishing reported outcomes from recorded tool results.
4. Understand resume, fork, and clean-session choices, their use of prior history, and their target directory.
5. Narrow the overview by project, harness (such as Codex or Claude Code), time, or visible labels; combine those filters and retain their scope when searching transcript content.
6. Keep private code and transcripts on the local machine by default.
7. Trust that scanning does not modify the source logs and tolerates incomplete or changing log files.
8. Understand discovery and indexing coverage, exclude private projects, and control the lifetime of derived local data.

## 4. Market requirements

- A provider-adapter model must normalize different native log formats into a common session view.
- The product must work with existing sessions, not only sessions created by Trailmark or an ACP wrapper.
- The first release must prioritize Claude Code and Codex; additional providers must be addable without changing the core UI.
- The default screen must work without a query: prefer a seven-day recent-session overview with a stable selection and purpose/project-first visual hierarchy. When the active project/harness has only older work, automatically expand to 30 days or All time; respect a time window the user explicitly chose. An inactive project should not appear to have no history just because the default window is too short.
- Session purpose must use a native title or a clearly labeled request excerpt. The overview must also show the latest substantive exchange, with source navigation and explicit unavailable states. Preview quality is a release criterion.
- Any outcome shown must distinguish an agent's completion claim from a recorded tool result; neither establishes the current state of the repository.
- Support continuation decisions with recorded model and branch context, token consumption, and latest-turn timing/outcome. Provide an expandable view of recorded file and command activity. Keep this evidence secondary to purpose and project; missing measurements must not appear as zero or imply success, and historical Git metadata must not imply the current checkout state.
- The primary experience must be usable entirely by keyboard, with a clear selection, contextual key hints, restrained styling, and usable wide and narrow layouts. See the [UX specification](UX.md).
- The implemented TUI must preserve the mockups' visual hierarchy: distinct session cards and day groups, recognizable project paths, an unmistakable selected session, and clear separation between browsing, detail, and continuation actions. Color and spacing must support recognition in dark and light terminals, with selection and grouping still understandable without color. Offer the original Green appearance plus Blue, Amber, and Monokai alternatives with a remembered choice. Monokai uses a pure black or white base to match dark or light terminals. Exact palettes and terminal adaptations belong in the [UX visual specification](UX.md#8-visual-and-state-rules).
- Resume, fork, and clean-session choices must explain their history behavior and target directory. Native support must be explicit; unsupported forking must not be simulated by relabeling a clean session.
- Search must return highlighted excerpts and open at the matching message. Users must be able to move between matches without losing their search context.
- Users must be able to copy selected context with provenance. Project grouping must retain the original working directory and distinguish worktrees.
- The MVP index and excerpts must remain on-device. Exclusions, cache clearing, source availability, and scan freshness must be visible and understandable.

## 5. Competitive and adjacent landscape

Documentation review dated 2026-10-14. These are published capabilities, not independently tested runtime quality or an exhaustive market/trademark review.

### Direct alternatives

| Product | Published scope | Implication for Trailmark |
| --- | --- | --- |
| [cass](https://github.com/Dicklesworthstone/coding_agent_session_search) | Unified TUI and CLI for indexing and searching local session history across multiple agents, including Claude Code and Codex. | Multi-provider local search and a TUI already exist. |
| [ccfullsearch](https://github.com/materkey/ccfullsearch) | Terminal search, inspection, and resume across Claude Code, Codex, and other session sources. | Compare the ease of recognizing recent work and choosing how to continue. |
| [agent-session-manager](https://github.com/hxy91819/agent-session-manager) | Local TUI for finding, inspecting, and resuming coding-agent sessions across projects. | Cross-project browsing and resume alone do not establish a distinction. |

### Adjacent alternatives

- **Built-in agent history:** useful within each provider, but fragmented across tools and commonly tied to the current project/directory.
- **[Codex Session Scout](https://github.com/strato-space/codex-session-scout):** a close adjacent tool for inspecting and searching local Codex logs; its published scope is Codex-focused.
- **[acpx](https://github.com/openclaw/acpx):** maintains persistent named ACP sessions. It is useful for sessions started through acpx, but is not a passive index of every agent's native logs.
- **[Atlas](https://github.com/pacifio/atlas):** an adjacent multi-agent coding/source-control product that links work to sessions; broader than a native-log browser.

### Differentiation hypothesis and validation

Trailmark's hypothesis is that a calm recent-work overview, recognizable purpose labels, prominent project paths, and clear continuation choices make it easier to pick work back up after several days. A simple installation and a focused interaction model may also matter. These are hypotheses to validate, not established advantages over the alternatives.

Before expanding providers or adding generated summaries, compare Trailmark with direct alternatives on the recent-work tasks described in the PRD. Record time to recognize the right session and directory, whether a transcript/search was necessary, and whether the user understands the continuation choices. Evaluate transcript search separately as a supporting task. Use permitted local data or sanitized fixtures; do not publish private transcripts. If no useful advantage emerges, reconsider scope or reuse an existing tool before broadening implementation.

## 6. Positioning

**For developers returning to recent agent-assisted work, Trailmark is a local TUI that reminds them what each session was for, where the work happened, and helps them choose whether to resume, fork, or start clean.**

## 7. Initial success measures

- A user can identify the intended session, its purpose, and its project/directory within 15 seconds in at least four of five recent-work tasks, using the overview without transcript reading or content search. This is an initial validation target, not a measured result.
- Every indexed session clearly identifies its source agent and working directory when those fields exist in the source log.
- A user can distinguish resume, fork, and clean-session behavior and prepare an available choice for the intended directory without accidental execution.
- The cached overview opens within a provisional 2-second target, and selection/preview changes respond within 100 ms p95 on the declared benchmark corpus and machine. Secondary search targets are specified in the PRD.
- Scanning and browsing leave source logs unchanged; the MVP makes no network requests and sends no transcript content off-device.
- Missing, excluded, stale, and partially indexed sources are distinguishable from a genuine search with no matches.
- Visual acceptance includes comparison with the supplied mockups in wide and narrow layouts. Functional correctness alone does not establish that selection, grouping, paths, and continuation choices are visually clear; record rendering evidence separately from human recognition results.

## 8. Release decisions and later scope

- The initial user is a local developer with recent sessions across projects; team sharing and hosted collaboration are not assumed.
- The first release supports Linux, Claude Code, and Codex. `trailmark` launches the TUI by default.
- The launch interface is limited to `trailmark`, `--help`, and `--version`. Filters, search, refresh, and source management live inside the TUI; a CLI command suite and machine-readable output are deferred.
- The MVP prioritizes the seven-day overview, recognizable purpose/project context, and continuation decisions. Transcript reading, full-text search with passage navigation, and explicit context copying remain supporting MVP capabilities.
- Continuation choices prepare/copy validated native commands where supported. Trailmark does not execute resume, fork, or clean-session commands in the MVP; a later native launch handoff needs a separate interaction contract.
- The repository is licensed under [MIT](LICENSE). Installation channels, public release scope, and macOS/Windows support remain open.
- Next enhancements are bookmarks, personal labels stored by Trailmark, previewable Markdown context export, and related-session navigation.
- Semantic search, model-generated summaries, and additional providers are deferred until the recent-work overview is validated. If deterministic purpose labels fail the recognition target, revisit their design before broadening the feature set. Any future model backend needs an explicit privacy and quality contract. GitHub Copilot CLI, Muse Code, Pi, and possibly OpenCode remain adapter candidates.
- Trailmark is a working name only; name/domain/trademark availability has not been checked.
