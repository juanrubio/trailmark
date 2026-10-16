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

## Recorded Overview metadata (2026-10-16)

The Overview keeps these details after the purpose and conversation excerpts.
They describe source records, not the current repository or a running process.
Missing fields remain unavailable; a recorded zero is retained. The original
session list stays purpose/project-first.

| Detail | Codex | Claude Code |
| --- | --- | --- |
| Models and reasoning efforts | Distinct `turn_context.model` and effort values | Distinct assistant `message.model` and recorded effort values |
| Git context | `session_meta.git` branch, commit hash, repository URL | Recorded `gitBranch`; no inferred commit or remote |
| Token usage | Latest cumulative `event_msg.token_count.info.total_token_usage` or `token_usage_record.thread_token_usage` | Usage aggregated across unique assistant response identities; repeated snapshots replace/merge fields rather than adding another response |
| Latest-turn outcome | Recorded task start, completion, interruption, or error; supplied `duration_ms` | Assistant stop reasons and explicit error/duration records when available; tool-use stops are not completed turns |
| Recorded work | Deduplicated completed item events: successful file-change paths, command count, failure count and latest command/exit status | Unavailable when equivalent structured evidence is absent |

Codex custom-tool calls use `input`, whereas function calls use `arguments`.
Both inputs are retained as transcript text and become searchable after
reindexing. This fixes tool entries that previously contained only a name.

Input tokens are normalized to include cache reads and cache writes. Codex
supplies cumulative input directly; Claude supplies uncached input plus cache
read and creation counters. If those components are incomplete, preserve the
known components without inventing a total. Reasoning/thinking tokens, when
reported, are an output subset. Never sum repeated cumulative snapshots,
add cache/reasoning subsets twice, or combine parent and child totals into a
supposed exclusive session measurement. Usage scope text qualifies inherited
history and incomplete data. These are recorded token counts, not calculated
billing amounts.

A new turn start invalidates a previous completion label. A token-limit stop
is interrupted, and a start/tool handoff without a later completion is labeled
as having no completion recorded. A completed turn is explicitly distinguished
from successful completion of the user's task. Missing duration does not fall
back to wall-clock time between the first and last log entries.

Recorded work is collapsed until `e` is pressed with Overview focused. File
paths come from successful structured change events, not prose mentions;
commands come from structured completion events, not duplicate transcript
outputs. The summary retains up to 100 distinct file paths and 1,000 characters
of the latest command, with a truncation indicator. Failure totals remain
unknown if an observed command lacks a usable outcome. Recorded work is not
an exhaustive repository diff or proof that those files still exist.

Metadata lives in the derived SQLite cache through an additive migration.
Parser version 3 forces unchanged native logs to be reindexed at the next
scan; no source files are rewritten and no manual cache deletion is required.
