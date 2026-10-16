# Plan: read-only memory browsing in Trailmark

**Status: Implemented on 2026-10-16.**

Recorded on 2026-10-16 against Trailmark v0.6.0 and implemented the same
day: a read-only Memories view inside Trailmark, initially supporting
Claude Code and Codex. The implementation lives in `internal/memory`
(shared catalog, no terminal dependencies) and `internal/tui/memories*.go`.

Decisions taken where this plan left questions open:

- Memory roots derive from the effective session roots (Claude project
  stores as-is; each Codex session root maps to its sibling `memories/`
  directory), from `CLAUDE_CONFIG_DIR` / `CODEX_HOME`, and from Claude's
  `autoMemoryDirectory` setting (only that key is read). Extra locations
  use a separate `memory_roots` configuration key, managed in Settings as
  well; derived roots are listed but not removable.
- Rendered Markdown uses Glamour (`charm.land/glamour/v2`, the
  lipgloss-v2-native major of the `github.com/charmbracelet/glamour`
  library; the v0/v1 line does not compile against this repository's
  Bubble Tea v2 dependency stack). `v` toggles rendered/raw.
- `m` toggles between Sessions and Memories; refresh is explicit only
  (`R`); the default categories are notes and consolidated sections.
- A project is never guessed: entries without reliable evidence are
  labelled Unknown (or global), with the evidence shown.
- Files are read read-only with a 2 MiB per-file cap and a visible
  truncated state. Section identity is canonical path + heading path +
  occurrence index, so repeated headings stay distinct.
- Terminal escape sequences in source text are sanitized before
  rendering. Repository fixtures are fictional only.

## 1. Purpose and product boundary

Help users answer:

- What has each harness saved about this project or my preferences?
- Where is a particular remembered instruction, decision, or lesson stored?
- What supporting note or session does a memory refer to?
- Which memory sources are present, missing, or unavailable on this machine?

Keep recent sessions as Trailmark's opening screen. Memories is a distinct
view reached from the existing application, sharing its visual language,
project/harness controls, and navigation conventions. The feature must not
require an agent session, a model call, or a network connection.

Build the memory reader as an independent module so another interface or
a standalone application could reuse it later. This work does not depend
on the separately deferred [browser interface plan](web-interface-plan.md).

## 2. Initial scope

Include:

- Discovery of supported local memory stores and explicitly added roots.
- Browsing Claude Code memory indexes and topic notes.
- Browsing Codex consolidated memory, summaries, and referenced supporting
  notes, including sections inside larger documents.
- Harness, project, document-category, and text filters.
- Full-text search with snippets, match navigation, and source locations.
- Readable Markdown and raw-source views, with line and page scrolling.
- Source metadata, explicit refresh, coverage/errors, and exclusions.
- Navigation to an already indexed session when a recorded reference can
  be resolved unambiguously.
- Explicit copying of selected text with its source reference.

Exclude from the initial release:

- Editing, deleting, merging, importing, or synchronizing provider memories.
- Asking a model to create, summarize, classify, or repair memory content.
- Claiming to know which memories a running agent actually loaded or used.
- Automatic contradiction detection, semantic search, or quality scoring.
- Cross-machine aggregation or cloud storage.
- General browsing of credentials, settings, caches, or arbitrary files
  inside a harness's private directory.

Instruction files such as `CLAUDE.md` and `AGENTS.md`, and executable skills,
are adjacent concepts. Label links to them when relevant, but defer a full
instruction/skill browser. Do not silently treat them as learned memories.

## 3. Source evidence and compatibility

The feasibility inspection found 32 Markdown files in Claude project memory
directories across 13 projects, and 27 Markdown files in the local Codex
memory directory, including supporting summaries and skills. Together this
was approximately 370 KiB of Markdown. These are observations from one
installation, not provider limits or a performance benchmark.

### Claude Code adapter

The documented default store is
`~/.claude/projects/<project>/memory/`, containing a `MEMORY.md` index and
topic files. Support the configured Claude home and explicit custom memory
roots, including the documented `autoMemoryDirectory` setting when its
location is known. Inspect only the configuration fields needed for source
discovery; do not ingest entire settings files into the memory collection.

Parse optional Markdown frontmatter, preserving recorded fields such as
name, description, type, and source-session identifiers. Treat missing or
unrecognized fields as optional metadata, not parse failures. Local files
already include both notes with frontmatter and plain Markdown notes.

Map projects using reliable metadata or an unambiguous existing session
association. Do not reconstruct paths by blindly replacing separators in
encoded directory names. When association is uncertain, retain the store
label and show the project as unknown.

Use `MEMORY.md` as an index/navigation document. If it points to an existing
topic file, make that relationship navigable instead of presenting the
index entry and topic file as two unrelated memories.

### Codex adapter

The inspected installation contains `memory_summary.md`, `MEMORY.md`,
`raw_memories.md`, `rollout_summaries/`, `extensions/`, and `skills/` under
`~/.codex/memories/`. Treat this as an observed layout, with explicit
capability detection and fallback behavior. Do not assume every Codex
version or installation produces all of these artifacts.

Respect the selected Codex home and allow explicit additional roots.
Begin with the recognized summary and consolidated-memory documents, plus
supporting Markdown summaries. Raw memory and local extension notes should
be available under clearly labeled supporting categories, not mixed into
the default list as equally authoritative current memories. Skills are
separate artifacts and remain outside the initial memory collection.

Split consolidated documents into navigable sections using their heading
structure. Preserve parent headings, source lines, recorded project scope,
and explicit references. A global document may describe many projects;
its filesystem location alone must not assign it to a single project.

Missing stores are normal: memory may be disabled, not yet generated, or
stored elsewhere. Show source status and an Add root action rather than
assuming a parser failure.

### Compatibility principles

- Keep provider-specific discovery and interpretation inside adapters.
- Preserve original text and unknown metadata; never rewrite provider files
  to fit Trailmark's model.
- For recognized files with unfamiliar structure, offer a document-level
  fallback and explain that structured extraction is unavailable.
- Distinguish unsupported layout, unreadable source, malformed metadata,
  empty store, and excluded content in source status.
- Check documented defaults and configurable paths against the installed
  versions before implementation; internal layouts can change.

References consulted during feasibility assessment:

- [Claude Code memory documentation](https://code.claude.com/docs/en/memory)
- [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)

The Codex reference documents controls for memory generation and use; the
specific file layout above was verified locally, not established as a
universal storage contract by that reference.

## 4. Shared model and architecture

Proposed package boundary: `internal/memory`, with separate Claude Code
and Codex adapters. Keep discovery, parsing, filtering, and search free of
Bubble Tea or terminal-rendering dependencies.

```text
Allowed memory roots
        |
Claude Code / Codex adapters
        |
Memory catalog: documents, sections, metadata, references
        |
Filtering, search, refresh, and optional session resolution
        |
Trailmark Memories view
```

Represent a source document separately from its browsable entries:

| Record | Required information |
| --- | --- |
| Source document | Harness, root, canonical path, document category, modification time, read/parse status |
| Memory entry | Title, original text, document identity, heading path or source line span, optional recorded metadata |
| Project association | Recorded or inferred association, its evidence, or an explicit unknown/global state |
| Reference | Original link or identifier, reference kind, resolution status, and optional target |

Use optional values for missing metadata. Keep recorded dates separate
from filesystem modification time. Do not display modification time as
last recalled, last used, creation time, or proof that the fact is current.

Use source paths and available native identifiers for document identity.
Section identities must handle repeated headings and changes between
refreshes. Reconcile using recorded identifiers or heading/text context
where possible; otherwise select the nearest surviving entry and explain
the change. Do not promise that a line number is a permanent identifier.

## 5. TUI experience

### Entry and return behavior

Propose `m` from the session list to open Memories, subject to the final
shortcut conflict audit. Carry the current project and harness into the
memory view, but do not apply the session list's seven-day date window:
older memories may still be relevant. Show the active scope prominently
and offer All projects and All harnesses.

Returning to Sessions restores its selection, filters, expansion, and
scroll positions. Opening a related session from a memory is an explicit
navigation action, with a route back to the same memory and search match.

### Layout and visual treatment

- Wide terminals: an entry list on the left and the selected memory on the
  right. Lead with title/topic and project; show harness, category, and
  modification time as supporting metadata.
- Narrow terminals: list first, Enter opens the detail view, Esc returns.
- Retain Trailmark's outer margins, item separators, selection marker,
  semantic colors, and four light/dark theme pairs.
- Show focus clearly. Provide line-by-line and Page Up/Page Down scrolling
  in detail, with discoverable hints and no horizontal scrolling required
  for ordinary prose or paths.
- Offer rendered Markdown and raw source. Preserve code blocks, source
  line locators, and a readable fallback for malformed Markdown.

Default browsing includes primary memory notes and consolidated sections.
Make index documents, overviews, and raw/supporting material available
through category controls. Clearly label their roles rather than implying
that all copies of a statement are separate independent facts.

Illustrative layout, using fictional content:

```text
Memories     All harnesses · atlas · Notes and consolidated memory
-----------------------------------------------------------------
> Preserve migration ordering   | Preserve migration ordering
  atlas · Claude Code           | Source: .../memory/migrations.md
                                | Category: project (recorded)
  Deployment recovery notes     |
  atlas · Codex                 | Apply schema changes before...
                                |
  Search result caching         | References
  atlas · Claude Code           | Related session: available
-----------------------------------------------------------------
Enter Open   / Filter   s Search   Tab Focus   Esc Sessions
```

This is an interaction sketch, not a final layout or shortcut contract.

## 6. Search and references

Separate list filtering from content search, following Trailmark's existing
conventions. Start with literal, case-insensitive search over titles,
descriptions, and original Markdown text. Keep the query scoped to the
selected harness, project, and categories, with an obvious way to broaden it.

Show snippets and match counts; opening a result reveals the matching
passage. Support next/previous matches and an explicit no-results state.
Search source text rather than terminal styling, and map highlights back
to rendered lines without losing raw-source navigation.

Group matches by document/section to avoid presenting an index pointer and
its target as duplicate independent memories. Do not merge unrelated notes
from different harnesses simply because their text is similar.

Resolve local note links only within approved roots. Display external or
unresolved references as text with their status. Do not automatically open
URLs or traverse arbitrary paths mentioned in a memory.

Resolve session references through the existing index when the harness and
session identity are unambiguous. A missing or expired transcript stays
marked unavailable; do not fabricate a link based only on title or date,
and do not launch a broad transcript scan merely to resolve a reference.

## 7. Discovery, refresh, and performance

Use direct reads and an in-memory catalog initially, with a small derived
cache only if measurement justifies it. Avoid adding memory content to the
large transcript search index by default. This collection has different
size, identity, and refresh needs.

Discovery should enumerate known memory locations and explicit roots,
deduplicate canonical paths, and exclude `.git`, credential stores, session
JSONL, and unrelated caches. Memory coverage is separate from the current
7/30-day session-indexing scope.

Open the view without waiting for transcript indexing. Perform discovery,
parsing, and search off the TUI update path, with cancellation and bounded
file sizes/results. Define and document actual limits during implementation;
show a visible truncated/too-large state instead of silently dropping text.

Provide explicit refresh. Compare file metadata and content as needed,
read changed files safely, and retain selection when possible. If a file
changes during reading, retry within a bound or report that it is changing;
do not replace a usable catalog with a misleading empty result. Apply
confirmed removals and exclusions, but distinguish those from temporary
root failures.

Initial performance goals, subject to measurement:

- Show usable entries within one second for a collection comparable to the
  inspected local collection.
- Keep navigation immediate and typing responsive while work is pending.
- Measure larger synthetic collections and long individual documents;
  record startup, search latency, and peak memory before selecting limits.

These are acceptance goals, not measured results or guarantees for all
stores. No hours-long initial transcript indexing should be required to
use Memories.

## 8. Read-only and privacy behavior

Open provider sources for reading only. Do not update their indexes,
frontmatter, timestamps deliberately, Git state, or configuration. Provider
files are untrusted content to display, not instructions for Trailmark to
execute. Escape terminal control sequences before rendering.

Any settings, catalog, or cache writes belong exclusively to Trailmark's
own directories. Never copy source text into routine diagnostics. Use
fictional fixtures for repository tests, screenshots, and demos.

Honor source-path exclusions and known project exclusions. When a global
document contains multiple project sections, omit confidently associated
excluded sections. Do not claim that an ambiguous global document has been
fully redacted: document the limit and provide a whole-file/root exclusion
when that is the user's required privacy boundary.

Symlinks must not silently expand the set of approved roots. Surface an
outside-root link and require that location to be explicitly included
before traversing it. Explicit text copying should include provenance;
simply selecting an entry must not copy anything to the clipboard.

## 9. Implementation stages and acceptance

### Stage 1: source contracts and fictional fixtures

Confirm supported layouts, configuration overrides, and project association
rules. Define the shared records and adapter contract. Add fictional
examples for both providers, including plain Markdown, optional frontmatter,
large consolidated files, missing metadata, and linked notes.

Acceptance: discovery identifies the intended documents without traversing
unrelated private directories, and every entry retains its exact source.

### Stage 2: catalog and basic browsing

Implement adapters, the catalog, filtering, refresh, and the dedicated TUI
view. Support wide/narrow layouts, Markdown/raw reading, explicit scope,
source status, and stable return to Sessions.

Acceptance: a user can browse both harnesses, locate a project memory, read
its source, and return to the original session without changing provider data.

### Stage 3: search and source navigation

Add content search, snippets, match navigation, copying, local note links,
and explicit session-reference resolution.

Acceptance: matches remain reachable through wrapping and resize; missing
links are explained; duplicate headings and index references do not point
to the wrong passage.

### Stage 4: robustness, documentation, and release readiness

Validate malformed/unknown fields, empty stores, permission failures,
concurrent writes, file removal, large files, symlink boundaries, exclusions,
and escaped terminal controls. Check repeated refresh and cancellation.

Exercise all four themes, light/dark, no-color/ASCII, 80×24, wide layouts,
and the existing 40×12 minimum. Verify focus, line/page scrolling, search,
and Sessions-to-Memories-to-Sessions navigation. Record real-terminal
inspection separately from automated geometry checks.

Verify read-only behavior against fixtures using content hashes and an
inventory before/after the workflow. Do not use filesystem access time as
evidence of a write: reading can update it depending on mount settings.

Update README, PRD, UX, configuration guidance, and provider documentation
once implementation is authorized and the actual behavior is established.
Record compatibility and performance limits without claiming universal
memory-format support. (Done: see the Memories sections in those
documents.)

## 10. Effort, maintenance, and remaining decisions

Preliminary estimate: a few days for a useful read-only prototype, roughly
one to two weeks for a polished initial feature. Provider variation,
source association, large-document handling, and TUI refinements may extend
that estimate. It is not a delivery commitment.

The main ongoing maintenance burden is provider file-layout evolution.
Keep adapter fixtures and fallback document browsing so an unfamiliar
structure does not hide readable content. Reuse Trailmark's visual and
navigation components without duplicating memory parsing in renderers.

Before implementation, settle:

1. The final entry shortcut and navigation/focus contract.
2. Which discovered custom roots are included automatically and how the
   user reviews coverage and exclusions.
3. The exact default categories and treatment of ambiguous project scope.
4. File/result limits and the policy for incomplete or changing documents.
5. Whether automatic refresh is needed initially; explicit refresh is the
   proposed baseline.

**Outcome:** all four stages are implemented and covered by tests,
including robustness cases (malformed frontmatter, empty and unreadable
stores, dangling symlinks, CRLF/BOM, duplicate headings, repeated
refresh, cancellation) and a synthetic performance smoke test (200 Codex
sections plus 100 Claude notes load and search well under the 5-second
test bound). This document remains the design record; behavior is
documented in the README, PRD, UX, configuration, and provider docs.
