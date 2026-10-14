# Trailmark — Market Requirements Document

- **Working name:** Trailmark (not name-cleared)
- **Status:** Draft
- **Date:** 2026-10-14

## 1. Opportunity

Developers increasingly work across several AI coding agents. Each agent keeps its own session history, format, and project picker. When work spans repos and days, it is easy to forget which conversation contains a decision, what an agent changed, or which directory a session belongs to.

Trailmark is a local, keyboard-first catalog for finding and understanding those existing sessions. It reads native agent logs rather than requiring the user to start every conversation through a new orchestrator.

## 2. Target users

**Primary:** developers who use more than one coding-agent CLI across multiple repositories and want to revisit prior work without remembering the agent, session title, or working directory.

**Secondary:** developers who need a quick history of agent-assisted work for handoffs, debugging, or reviewing decisions.

## 3. Market needs

Users need to:

1. See sessions from different coding agents in one chronological view.
2. Identify a session at a glance by agent, title or prompt, repository/directory, date, and recent activity.
3. Understand the goal and outcome without first reading a long transcript.
4. Search and filter by agent, project/path, time, or text.
5. Open a session and read its available conversation and tool activity.
6. Keep private code and transcripts on the local machine by default.
7. Trust that scanning does not modify the source logs and tolerates incomplete or changing log files.

## 4. Market requirements

- A provider-adapter model must normalize different native log formats into a common session view.
- The product must work with existing sessions, not only sessions created by Trailmark or an ACP wrapper.
- The first release must prioritize Claude Code and Codex; additional providers must be addable without changing the core UI.
- Summaries must expose the session goal and most important known outcome, with a clear fallback when the log does not contain enough information.
- The primary experience must be usable entirely by keyboard and must support both rapid scanning and deeper transcript reading.
- The index and summaries must be local by default. Sending transcript content to a service must not happen implicitly.

## 5. Competitive and adjacent landscape

- **Built-in agent history:** useful within each provider, but fragmented across tools and commonly tied to the current project/directory.
- **[Codex Session Scout](https://github.com/strato-space/codex-session-scout):** a close adjacent tool for inspecting and searching local Codex logs; its published scope is Codex-focused.
- **[acpx](https://github.com/openclaw/acpx):** maintains persistent named ACP sessions. It is useful for sessions started through acpx, but is not a passive index of every agent's native logs.
- **[Atlas](https://github.com/pacifio/atlas):** an adjacent multi-agent coding/source-control product that links work to sessions; broader than a native-log browser.

These are examples, not an exhaustive market or trademark review. Trailmark's proposed distinction is a small, local-first browser over the session files users already have, with provider-neutral search and a focused TUI.

## 6. Positioning

**For developers who lose track of AI coding conversations, Trailmark is a local session-history explorer that makes past work findable across agents and repositories—without replacing the agents or moving their logs into a hosted service.**

## 7. Initial success measures

- A user can find a session from the previous few days by repository, agent, or a remembered phrase without knowing its session ID.
- Every indexed session clearly identifies its source agent and working directory when those fields exist in the source log.
- The summary/detail flow lets a user decide quickly whether a transcript is the one they need.
- Scanning and browsing leaves source logs unchanged; no transcript content leaves the machine by default.

## 8. Assumptions and open questions

- The initial user is a local developer with logs from several CLI agents; team sharing and hosted collaboration are not assumed.
- The MVP harnesses are Claude Code and Codex. GitHub Copilot CLI, Muse Code, Pi, and possibly OpenCode are future adapters.
- A local extractive summary is the safe MVP default. Whether to offer optional model-generated summaries is an open privacy and product decision.
- Linux is the first development/acceptance environment. macOS and Windows support, installation channels, licensing, and public release scope remain open.
- Trailmark is a working name only; name/domain/trademark availability has not been checked.
