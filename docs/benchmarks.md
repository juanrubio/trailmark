# Benchmarks (PRD section 9)

Measured 2026-10-15 on the development machine: Intel Xeon
E5-1620 v2 @ 3.70 GHz (8 cores), 39 GiB RAM, Linux
7.0.0-38-generic x86_64, Go 1.27.2, Trailmark at Phase 4 commit
`e42ba02` plus the uncommitted Phase 5 tree. The corpus and index
live outside the repository (`~/bench-trailmark`) and are never
committed.

## What users should expect

A large all-history first scan can take hours. The recorded cold scan
below consumed approximately **2 hours 18 minutes of CPU time** for
**10,000 sessions / 1.19 GiB**. CPU time is not wall-clock duration, and
session count alone is not a useful predictor: transcript sizes and message
counts vary widely. Do not extrapolate a guaranteed rate or finish time
from this corpus. Peak indexing memory was not captured.

First-run scope choices now let users defer unindexed files older than
7 or 30 days, based on file modification time, or choose All history.
This reduces work when the collection contains many older files; it does
not accelerate included files. A recently modified long transcript can
still take substantial time. The progress indicator reports activity and
counts, not a percentage or ETA. Esc cancels while retaining completed
index entries. Expand coverage later in Sources and settings.

Cached browsing is a separate operation: opening the measured 10,000-session
list took **103 ms**, without waiting for a rescan. Unchanged indexed logs
are skipped on subsequent scans. These historical results have not been
re-measured for the progress/scope changes, and no scoped-indexing duration
or reliable per-100/per-1,000-session estimate is available.

## Corpus

Generated with `go run ./cmd/trailmark-bench -dir ~/bench-trailmark
-sessions 10000 -gib 1.0`: 10,000 synthetic sessions split across
the Claude Code and Codex layouts, 1.19 GiB of JSONL total, 24
messages per ordinary session, and one 50 MiB transcript
(`rollout-bench-big.jsonl`, 52,439,097 bytes, 2,382 messages).
Message text is synthetic filler from a 256-word vocabulary, which
makes common query terms match a far larger share of messages than
in real logs; search numbers below are therefore worst-case for
match density, on top of the design costs noted there.

## Results

| Metric | Measured | Provisional target | Verdict |
| --- | --- | --- | --- |
| Cold indexing, 10,000 sessions / 1.19 GiB | ≈ 2 h 18 min CPU (single-threaded; process CPU time when the scan phase completed) | report only | Recorded; very slow, see below |
| Peak RSS during cold indexing | not recorded (the combined generate+measure run was superseded before printing) | report only | Missing; re-measure with a future run |
| Derived index size | 2.9 GiB for a 1.19 GiB corpus | report only | Full message text is stored twice (messages table plus FTS copy) |
| Cached list opening (10,000 sessions) | 103 ms | 2 s | Pass |
| Selection-to-overview p95 (100 changes) | 0.89 ms | 100 ms | Pass |
| Warm search p95 (100 queries) | not obtained; per-query latency is minutes (see below) | 200 ms | **Missed by orders of magnitude** |
| 50 MiB transcript first viewport (200 messages) | 8.8 ms | 1 s | Pass |

## Warm search: what happened

Two full 100-query measurement loops were started and aborted
(after roughly 3.5 and 2 CPU-hours respectively) because
individual scoped queries over the 2.9 GiB index take on the
order of minutes with the CGo-free `modernc.org/sqlite` driver.
A subsequent bounded probe was also stopped once the conclusion
was unambiguous. No honest p95 exists, so none is reported.

Contributing factors, in estimated order:

1. Exact per-session match counts (a Phase 4 correctness
   requirement) force posting-list scans for every matching
   session, chunked across all 10,000 scoped paths.
2. The pure-Go SQLite driver is far slower than C SQLite for
   FTS5 insertion and bm25 scoring; cold indexing averaged
   roughly 0.15 MB/s.
3. The synthetic vocabulary inflates match density far beyond
   real logs.

## Target disposition

- List (2 s), overview (100 ms p95), and viewport (1 s) targets
  are met with wide margins at the full corpus scale and are
  adopted as regression thresholds.
- The 200 ms warm-search target is **not met** and is not
  recalibrated to the measured minutes: that would ratify an
  unusable interaction. It stays as the design target, with the
  gap recorded here. Likely remediation, in order: compute
  match counts only for displayed groups; narrow the default
  search scope to the active window; evaluate the CGO SQLite
  driver or an external-content FTS layout to halve index size.
  None of that is implemented in Phase 5.
- Cold indexing has no target; the recorded duration and the
  2.9 GiB derived size are the baseline any driver or storage
  change must beat.

## Reproducing

```bash
go run ./cmd/trailmark-bench -dir ~/bench-trailmark -sessions 10000 -gib 1.0
```

Generation and measurement re-execute as separate processes so
peak RSS reflects indexing only. `-skip-generate` reuses a
corpus; `-skip-index` reuses an index for warm metrics only.
Expect cold indexing to take hours with the current driver.
