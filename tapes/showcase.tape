# Run via ./tapes/record-showcase.sh: it builds the real application and
# supplies an isolated launcher with fictional Codex and Claude Code logs.
Output ../docs/media/showcase.gif
Set Shell bash
Set Width 1440
Set Height 900
Set FontSize 18
Set FontFamily "DejaVu Sans Mono"
Set Padding 16
Set Framerate 12
Set CursorBlink false
Set Theme { "background": "#191e20", "foreground": "#e5e9e4", "cursor": "#ade6c2" }

# Start on a populated overview, without shell setup or indexing flicker.
Hide
Type `"$TRAILMARK_DEMO_BIN"`
Enter
Wait+Screen /Make search feel instant/
Sleep 1s
Show
Sleep 3s

# Recognize recent work across projects and harnesses.
Type "j"
Sleep 2s
Type "j"
Sleep 2s
Type "kk"
Sleep 1s
Type "h"
Sleep 1500ms
Down 2
Enter
Sleep 2s
Type "h"
Up 2
Enter
Sleep 1s

# Inspect the underlying conversation and find a passage.
Type "t"
Sleep 2s
Type "/"
Type@80ms "shortcuts"
Enter
Sleep 2s
Escape
Sleep 700ms

# Prepare a continuation command; never copy or execute it.
Type "r"
Sleep 3s
Escape
Sleep 1s
Type "q"
Sleep 1500ms
Escape
Sleep 2s
Hide
Type "qy"
Sleep 500ms
