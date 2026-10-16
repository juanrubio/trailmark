# Plan: a browser interface served by Trailmark

**Status: Deferred — discussion only; implementation is not authorized.**

Recorded on 2026-10-16 against Trailmark v0.6.0 (`41f949b`). This proposal
does not change the current release requirements or add a web command.

## Objective

Offer a browser interface with the look and feel of the
[original HTML mockup](mockups/index.html): its colors, spacing, item
separators, session cards, adjacent overview, dialogs, and keyboard
interactions. Trailmark itself would serve the pages, retaining the
single-binary installation and the existing TUI.

Both interfaces should share session behavior and application logic.
Browser scrolling, clipboard access, focus, and navigation would use
browser-appropriate interactions rather than reproducing terminal mechanics.

## Feasibility and existing building blocks

This is a moderate extension rather than a second implementation of the
whole application. Provider adapters, the normalized session model, the
SQLite index, transcript retrieval, search, and continuation-command
preparation already live outside the TUI and can be reused.

The mockup is working HTML, CSS, and JavaScript, but uses fictional data
and illustrative continuation choices. It supplies a visual starting point,
not a connected frontend. Its outer scene controls are presentation aids
and should not become application navigation.

Some reusable behavior currently lives in `internal/tui`: filtering and
grouping sessions, automatic time-window selection, settings operations,
and scan coordination. Extract that behavior before connecting a browser
interface, preserving the current TUI behavior.

## Proposed architecture

```text
Native session logs
        |
Shared adapters, scanner, and SQLite index
        |
Shared application services
sessions · overview · transcripts · search · settings · scan progress
        |                                    |
       TUI                               HTTP server
                                             |
                                      Browser interface
```

- Embed the HTML, CSS, and JavaScript in the Go executable. Users would
  not need a separate web server or Node.js runtime.
- Consider `trailmark serve` as the explicit launch command. Its name,
  port, browser-opening behavior, and lifecycle remain design decisions.
- Expose shared application operations through a small HTTP API. Keep
  rules such as filtering, scope expansion, and continuation capabilities
  out of duplicated frontend implementations.
- Keep selection, open panels, and display filters independent per browser
  tab. Indexing scope and source configuration remain application settings.
- Use one scan coordinator within a running server. Define ownership and
  locking before allowing a separate TUI process and server to scan or
  change the same index/configuration concurrently.
- Share semantic theme values, such as selection, paths, warnings, and
  separators, while retaining separate terminal and CSS renderers.

The exact frontend structure and progress transport are intentionally
undecided. Prefer a small implementation that builds on the mockup and
fits the existing distribution model.

## Proposed delivery stages

### 1. Extract shared application behavior

Move reusable behavior out of the TUI into an application layer with
explicit operations and structured results. Keep terminal rendering and
keyboard state in the TUI. Establish common behavior checks for filtering,
grouping, time-window expansion, continuation preparation, and settings.

### 2. Build a read-only browser experience

Serve embedded assets and connect the mockup's session list, filters,
overview, and transcript reader to real indexed data. Preserve purpose and
project/path as the main visual hierarchy, with narrow and wide layouts.
Page transcript data instead of loading an entire large conversation.

### 3. Add the remaining main workflows

Add transcript/content search, continuation-command preparation and browser
copying, four themes, source settings, indexing scope, progress, and scan
cancellation. Define confirmation behavior for cache clearing and other
settings changes. Commands should retain the current prepare-and-copy
behavior; native agent execution is outside this proposal.

### 4. Validate both interfaces and remote access

Check shared behavior through the common application layer, then exercise
each interface's rendering and interactions separately. Cover long logs,
empty/error states, keyboard focus, narrow layouts, multiple tabs, scan
cancellation, and settings changes during scans. Record browser visual
comparison with the mockup separately from terminal acceptance.

## Performance and concurrency

A browser interface will inherit the current indexing and search costs;
it is not a performance fix. See [the benchmark record](benchmarks.md).

The current index uses one database connection. Review how reads and long
indexing transactions interact before promising responsive concurrent HTTP
requests. Requests need bounded results and cancellation, and progress
updates should not depend on querying a busy database. Shared scan state
must survive a browser tab closing without leaving ownership ambiguous.

## Local and Tailnet access

A server would show sessions from the machine and account running it.
Connecting to Galactus from another machine would show Galactus's indexed
logs; it would not combine logs from the three deployed machines.

Default to localhost. Remote access should be explicit, with a decided
access-control mechanism and safe handling of settings-changing requests.
Render transcript content as untrusted text and protect against script
execution in browser pages. Tailnet exposure, authentication, and clipboard
behavior over the chosen connection need separate design and validation.

Cross-machine aggregation, public hosting, multiple user accounts, and
remote agent execution are outside the initial scope.

## Maintaining the TUI and browser interface

Parser fixes, metadata additions, indexing improvements, and application
rules should be implemented once. New user-facing features will generally
need controls in both interfaces, and layout, focus, scrolling, and copying
will require separate checks.

The main maintenance risk is duplicating behavior in Go TUI code and browser
JavaScript. Shared services, theme values, and common behavior checks reduce
that risk. Explicitly track any temporary feature differences rather than
claiming full parity while one interface lacks a workflow.

## Planning estimate

Based on the v0.6.0 code inspection, a basic browser for sessions, overview,
and transcripts could take a few days of focused work. A polished browser
version covering the main workflows could take roughly one to two weeks.
These are preliminary engineering estimates, not commitments. Shared-layer
extraction, concurrency changes, remote access, and full parity testing may
increase the effort.

## Decisions required before implementation

- Confirm the first browser release's feature scope and parity expectations.
- Decide whether the TUI and server may run concurrently against one index,
  and how indexing/configuration ownership is coordinated.
- Choose localhost-only launch initially or include authenticated Tailnet
  access in the first browser release.
- Define server startup/shutdown, port selection, tab disconnect behavior,
  and the browser equivalent of quitting the application.
- Choose the frontend structure and progress-update transport after the
  shared application boundary is established.
- Update the PRD and UX specifications when implementation is authorized:
  the existing TUI-first launch and network boundaries do not yet describe
  a served browser interface.

**Next step when resumed:** confirm these decisions and begin with the
shared application layer. No implementation work is scheduled by this plan.
