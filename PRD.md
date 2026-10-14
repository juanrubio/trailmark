# Trailmark — Product Requirements Document

- **Working name:** Trailmark (not name-cleared)
- **Status:** Draft
- **Date:** 2026-10-14
- **Related:** [MRD](MRD.md)

## 1. Summary

Trailmark is a local Go TUI for discovering, summarizing, searching, and reading native coding-agent session logs. It gives the user one keyboard-driven place to answer: “What was I working on, which agent session was it, and in which repo or directory?”

Trailmark indexes existing native logs. It is not an agent runner, session orchestrator, or replacement for the providers’ own tools.

## 2. Problem and goals

Session history is fragmented across agent-specific directories and formats. A user who returns to work after a day or two may remember the topic but not the CLI, session title, or working directory needed to find it.

### Goals

- Provide one local catalog across supported agents.
- Make the agent, title/preview, repo or working directory, dates, and concise summary visible in a scannable session card.
- Let the user search/filter, open a session, and read its transcript without leaving the TUI.
- Keep provider parsing isolated so new log formats can be added incrementally.
- Leave original log files untouched and keep transcript data on-device by default.

### Non-goals for MVP

- Starting, steering, or terminating coding agents. Whether opening a native session for resume/attach belongs in the MVP remains open.
- Requiring sessions to be created through Trailmark or ACP.
- Cloud sync, shared team history, telemetry, or a hosted account.
- Writing to, migrating, or cleaning up native agent logs.
- Remote LLM summarization by default.

## 3. Users and core stories

- As a developer, I can see recent Claude Code and Codex sessions together, sorted by last activity.
- As a developer, I can recognize a session from its title or prompt preview, project path, and summary.
- As a developer, I can search by words I remember or filter by agent, date, and repository.
- As a developer, I can open a session and scroll through the conversation and available tool events.
- As a developer, I can refresh/rescan and still browse if one log is malformed or currently being written.

## 4. Scope

### MVP harnesses

- Claude Code native session logs.
- Codex CLI native session logs.

### Later adapters

- GitHub Copilot CLI.
- Muse Code.
- Pi.
- OpenCode, if its log format and access are practical.

### MVP capabilities

1. **Discovery and indexing**
   - Detect known Claude Code and Codex log locations on Linux, with configurable additional roots.
   - Read source logs without modifying them; tolerate partial writes, truncated records, and unknown fields.
   - Normalize available session IDs, title/first prompt, agent, timestamps, cwd, repository/root, and source path.
   - Maintain an incremental local index/cache so repeated launches do not need to reparse every transcript. Storage format and exact location are implementation decisions.
2. **Session list and cards**
   - Sort by most recently active by default; make the sort visible.
   - Show agent, title or prompt preview, repo/working directory, last activity, and concise summary.
   - Clearly mark missing or inferred metadata rather than presenting guesses as facts.
3. **Search and filters**
   - Search titles, prompt previews, summaries, paths, and indexed transcript text.
   - Filter by agent, repository/path, and time range.
   - Keep search useful when a provider log cannot be fully parsed; show partial-index status.
4. **Session detail**
   - Open a selected card to read the available transcript in chronological order.
   - Show roles and timestamps where present; tool activity may be collapsed by default and expandable.
   - Keep the session's source path and working directory easy to inspect/copy.
5. **Summaries**
   - Present a short summary of the session goal and most important known outcome; include changed-file hints only when present in the source log.
   - Default MVP behavior is local and deterministic/extractive, with a clear “summary unavailable” fallback. Optional model-generated summaries require a separate privacy decision and must never upload logs silently.

## 5. TUI interaction

- Keyboard-first, with arrows and j/k for navigation, Enter to open a session, Esc to return, q to quit, and / to search.
- The list view is the main screen; cards should remain compact, scannable, and usable at common terminal sizes.
- The detail view should support scrolling without losing the selected session context.
- Show contextual key hints so the user does not need to memorize shortcuts.
- No action in the MVP may mutate or delete provider logs.

## 6. Proposed CLI surface

Names are provisional and should be validated while implementing:

~~~text
trailmark                 # launch the TUI
trailmark tui             # launch explicitly
trailmark list --agent codex
trailmark search "phrase I remember"
trailmark show <session-id>
trailmark scan             # refresh the local index
~~~

Provide machine-readable output for list/search where practical; the TUI remains the primary product surface.

## 7. Technical direction

- **Language:** Go.
- **TUI and CLI stack:** reuse gh-boardwalk's Go stack: Cobra for command structure; Bubble Tea v2 for the TUI; Bubbles for reusable widgets; Lip Gloss v2 for layout and styling. Use Huh v2 for interactive prompts if needed, Glamour v2 for Markdown transcript rendering, and teatest for TUI tests where appropriate.
- **Reference:** gh-boardwalk's ADR 0007, “Go stack: Cobra + Charm,” and its current Go module versions. Trailmark should choose compatible versions when implementation begins rather than copying a stale lock blindly.
- **Architecture:** provider-specific read-only adapters feed a shared normalized session model; indexing, summaries, search, and TUI operate on that model.
- **Portability:** develop and test on Linux first; keep paths and adapters isolated so other operating systems can be added.

## 8. Privacy, safety, and reliability

- Treat transcripts as sensitive: do not transmit session contents, prompt text, paths, or summaries off-device by default.
- Keep the source logs read-only. Store only derived index/cache data in Trailmark's own application-data location.
- Do not log transcript bodies or secrets to diagnostic output.
- Handle missing directories, permission errors, malformed JSONL, partial final lines, and provider schema changes without hiding other valid sessions.
- Identify each session by provider plus native session ID and source identity; avoid duplicate cards when a log is rediscovered through multiple roots.

## 9. MVP acceptance criteria

- Claude Code and Codex sessions from fixture datasets appear in one list with correct provider, ID, path, and timestamps when available.
- A user can navigate cards using only the keyboard, search for a remembered phrase, open a result, read its transcript, and return to the list.
- Summaries show the session goal and known outcome or an explicit unavailable state; they do not claim unsupported facts.
- Incremental rescans correctly notice new/changed logs and do not rewrite source files.
- A malformed or actively-written log does not prevent other sessions from appearing; the UI reports the affected source.
- Automated parser tests use representative, sanitized fixtures. TUI tests cover list, search, detail, empty state, and partial/error states.

## 10. Open decisions

1. Exact native log locations and versioned parser contracts for Claude Code and Codex.
2. Whether summary generation stays extractive or gains an explicit, opt-in model backend.
3. Whether the TUI opens directly by default or only through a subcommand.
4. Whether session resume/attach is a must-have for the MVP or a later enhancement.
5. Supported OS matrix, release/distribution model, and final project name.
