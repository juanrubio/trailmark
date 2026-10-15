# Trailmark (working title)

A local, keyboard-first TUI for remembering recent coding-agent work and deciding how to continue it.

See what you worked on over the last few days, recognize each session's purpose and project path, and decide whether to resume it, fork it, or start a clean session in that area.

**Status:** Phase 3 implemented — continuation sheets (resume, fork, clean session) prepare and copy validated commands, and session context copies with provenance. Transcript reading and content search follow in later phases. The project name has not been cleared.

## Core workflows

- **Remember recent work:** scan a seven-day overview across projects and agents without having to enter a search query.
- **Choose how to continue:** compare a session's purpose, directory, and latest context before choosing resume, fork, or a clean session.
- **Inspect when needed:** open the transcript or search its contents to resolve a question. This is a secondary MVP workflow.

## First release

- Linux, with native Claude Code and Codex session logs.
- Launch `trailmark` into the last seven days, grouped by day and ordered by last session activity.
- Use the time selector to show Today, 3 days, 7 days, 30 days, or All time. The seven-day default does not limit retained history.
- Keep the launch interface to `trailmark`, `--help`, and `--version`; normal operations live inside the TUI.
- Scan compact session cards that give purpose and project/path visual priority, with an adjacent overview where space permits.
- Switch between all projects and the current project while keeping repository and worktree context visible.
- Filter by harness with `h`: All harnesses, Codex, or Claude Code. Combine it with time and project filters; the same scope applies to transcript-content search.
- Show a purpose label from the native title or a clearly labeled request excerpt, plus the latest substantive exchange. An agent's completion claim is not proof of success.
- Present resume, fork, and clean-session choices with their history behavior and exact target directory. The first release prepares/copies validated native commands where supported; it does not launch agents.
- Read transcripts, search their contents with matching snippets and passage navigation, and explicitly copy selected context with its source reference.
- Keep source logs read-only and session data on-device, with exclusion controls, cache clearing, and visible indexing status.

Next enhancements include bookmarks, personal labels, previewable Markdown context export, and related-session navigation. Semantic search, generated summaries, and additional adapters are deferred until the recent-work overview is validated. Future adapters may include GitHub Copilot CLI, Muse Code, Pi, and OpenCode.

## Requirements

- [Market Requirements Document](MRD.md)
- [Product Requirements Document](PRD.md)
- [TUI experience and wireframes](UX.md)
- [Interactive TUI concept](docs/mockups/index.html) — open locally in a browser; fictional sessions, with no connection to real logs or agents.

## Technology direction

Go, using the Charm TUI stack described in the PRD: Bubble Tea v2, Bubbles, and Lip Gloss v2. This follows the stack already used by the local gh-boardwalk project. Cobra may handle the minimal launch interface.

## Development

`make check` runs vet and tests; `make build` builds `./trailmark`. Provider contracts are recorded in [docs/providers.md](docs/providers.md), and configuration and data locations in [docs/configuration.md](docs/configuration.md).

## License

[MIT](LICENSE).
