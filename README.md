# Trailmark (working title)

A local, keyboard-first TUI for finding, summarizing, and reading native coding-agent session logs across repositories.

**Status:** requirements drafted; implementation not started. The project name has not been cleared.

## MVP

- Index native Claude Code and Codex session logs.
- Browse keyboard-navigable session cards with a concise summary and working-directory context.
- Search and open a session to read its transcript.
- Keep source logs read-only and session data local by default.

Later adapters may include GitHub Copilot CLI, Muse Code, Pi, and OpenCode.

## Requirements

- [Market Requirements Document](MRD.md)
- [Product Requirements Document](PRD.md)

## Technology direction

Go, using the Cobra + Charm TUI stack described in the PRD: Bubble Tea v2, Bubbles, and Lip Gloss v2. This follows the stack already used by the local gh-boardwalk project.

## Development

The repository is currently docs-first. The Makefile targets assume implementation files will be added under cmd/trailmark.
