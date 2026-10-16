#!/usr/bin/env bash
# Record the shipped application against fictional native logs only.
set -euo pipefail

repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
for dependency in go python3 vhs ttyd ffmpeg; do
  command -v "$dependency" >/dev/null || { echo "Missing dependency: $dependency" >&2; exit 1; }
done

demo_dir=$(mktemp -d /tmp/trailmark-demo.XXXXXX)
trap 'rm -rf -- "$demo_dir"' EXIT
mkdir -p "$demo_dir/bin" "$repo_dir/docs/media"
cd "$repo_dir"
go build -o "$demo_dir/bin/trailmark" ./cmd/trailmark
python3 tapes/seed-showcase.py "$demo_dir"

# Only the recorded child process receives the isolated HOME. VHS can still
# use its existing browser installation and cache. No provider is launched.
cat > "$demo_dir/launch" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
demo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
export HOME="$demo_dir"
export TRAILMARK_CONFIG_DIR="$demo_dir/config"
export TRAILMARK_DATA_DIR="$demo_dir/data"
export TERM=xterm-256color COLORTERM=truecolor
unset NO_COLOR
cd "$HOME"
exec "$demo_dir/bin/trailmark"
SH
chmod +x "$demo_dir/launch"
export TRAILMARK_DEMO_BIN="$demo_dir/launch"
cd "$repo_dir/tapes"
vhs validate showcase.tape
vhs showcase.tape
