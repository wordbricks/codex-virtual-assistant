#!/usr/bin/env bash
set -euo pipefail

readonly PROFILE_NAME="devnam_growth"
readonly DEFAULT_PORT="9222"
readonly CHROME_BIN="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
readonly PROFILE_ROOT="${HOME}/.cache/codex-virtual-assistant/chrome-profiles"

PORT="${1:-$DEFAULT_PORT}"
PROFILE_DIR="${PROFILE_ROOT}/${PROFILE_NAME}"

if [[ ! -x "$CHROME_BIN" ]]; then
  echo "Chrome binary not found: $CHROME_BIN" >&2
  exit 1
fi

mkdir -p "$PROFILE_DIR"

echo "Launching Chrome profile '${PROFILE_NAME}' on CDP port ${PORT}" >&2
echo "User data dir: ${PROFILE_DIR}" >&2

exec "$CHROME_BIN" \
  --user-data-dir="$PROFILE_DIR" \
  --remote-debugging-port="$PORT" \
  --no-first-run \
  --no-default-browser-check \
  --new-window \
  about:blank
