# Provider contracts (Phase 1)

Verified on 2026-10-15 against the local stores and CLI help output.
Anything not listed as validated is unavailable in Trailmark rather
than simulated.

## Claude Code

- **Log location:** `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl`
- **Record shape:** JSONL records with `type` (`user`, `assistant`,
  `attachment`, `system`, `summary`), shared `sessionId`, `cwd`,
  `timestamp`, `uuid`, and `parentUuid` fields. Message content is a
  string or an array of `text`, `tool_use`, and `tool_result` blocks.
- **Native title:** a `summary` record's `summary` field when present;
  otherwise the first substantive user request is used as a labeled
  request excerpt. No native title was present in the sampled local
  corpus, so titles are treated as optional.
- **Relationships:** `isSidechain` marks sidechain sessions. No
  session-level fork or parent field was observed, so no relationship
  is inferred from message-level `parentUuid` values.
- **Continuation (validated from `claude --help`):**
  - Resume: `claude --resume {id}`
  - Fork: `claude --resume {id} --fork-session`
  - Clean session: `claude` (launched in the recorded cwd)
- **Unavailable:** launching or executing any of these from Trailmark
  (MVP prepares and copies commands only, in a later phase).

## Codex

- **Log location:** `~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<id>.jsonl`
- **Record shape:** first record `session_meta` with `id`, `timestamp`,
  `cwd`, `originator`, `cli_version`, and optional `git`,
  `forked_from_id`, and `parent_thread_id`; later `response_item`
  records carry `message`, `function_call`, and
  `function_call_output` payloads; `turn_context` carries a fallback
  `cwd`.
- **Native title:** `thread_name` from `~/.codex/session_index.jsonl`
  keyed by session `id`; otherwise a labeled request excerpt.
- **Relationships:** `forked_from_id` and `parent_thread_id` from
  `session_meta` are preserved verbatim. Nothing is inferred from
  timing or text.
- **Continuation (validated from `codex resume --help` and
  `codex fork --help`):**
  - Resume: `codex resume {id}`
  - Fork: `codex fork {id}`
  - Clean session: `codex` (launched in the recorded cwd)
- **Unavailable:** launching or executing any of these from Trailmark.

## Shared guarantees

- Source logs are opened read-only and never written.
- Parsers tolerate malformed lines, truncated final records, and
  unknown fields: bad records are counted, the session is marked
  partial, and other sessions still index.
- Indexing and browsing make no network requests.
