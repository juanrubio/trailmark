# README showcase

`showcase.tape` records the actual Trailmark TUI: recent-work navigation,
Claude Code filtering, transcript search, resume-command preparation, and
the quit confirmation. The green theme and terminal GIF format match the
visual tour used in Boardwalk's README.

## Recreate

Install Go (the version required by `go.mod`), Python 3,
[VHS](https://github.com/charmbracelet/vhs), `ttyd`, and `ffmpeg`, and make
them available on PATH. The tape uses DejaVu Sans Mono. VHS also needs its
supported Chromium browser setup. The initial recording used VHS 0.10.0.

From the repository root:

```sh
./tapes/record-showcase.sh
```

The script builds the current source into a temporary directory, generates
six fictional native JSONL sessions, validates the tape, and writes
`docs/media/showcase.gif`. It removes the temporary executable, logs,
configuration, and database when finished. Run the wrapper rather than the
tape directly: the tape expects the isolated launcher supplied by the script.

The recorded process has a temporary HOME and explicit Trailmark config/data
directories. Default discovery roots are disabled; only the generated Codex
and Claude Code roots are scanned. Titles, paths, requests, replies, models,
usage counters, and test results are synthetic. Dates are relative to the
recording time so the seven-day overview remains populated on future runs.
No application-specific demo flag or alternate renderer is used.

The tour prepares a resume command but does not copy or execute it. It does
not invoke an agent or access the recorder's real session index. VHS itself
retains its normal environment for browser/cache access.

## Review before committing

Open the generated GIF and inspect the overview, filtered list, transcript
match, continuation sheet, and quit dialog. Check that text is legible,
there is no clipping or shell-setup flash, the opening frame contains data,
and the final overview gives the loop a natural return point. The recording
uses 1440×900 pixels at 12 fps; keep the media small enough for the README.

The fixture generator and tape are recording assets, not benchmark or live
provider-compatibility evidence. Synthetic agent completion claims in the
animation do not describe work actually performed on those fictional projects.
