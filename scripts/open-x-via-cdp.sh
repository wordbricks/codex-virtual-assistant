#!/usr/bin/env bash
set -euo pipefail

readonly DEFAULT_HOST="localhost"
readonly DEFAULT_PORT="9222"
readonly DEFAULT_URL="https://x.com"

HOST="${1:-$DEFAULT_HOST}"
PORT="${2:-$DEFAULT_PORT}"
TARGET_URL="${3:-$DEFAULT_URL}"
if ! command -v agent-browser >/dev/null 2>&1; then
  echo "agent-browser is not installed" >&2
  exit 1
fi

readonly AGENT_BROWSER_CDP_ARGS=(--cdp "$PORT")

agent-browser "${AGENT_BROWSER_CDP_ARGS[@]}" open "$TARGET_URL" >/dev/null
agent-browser "${AGENT_BROWSER_CDP_ARGS[@]}" wait 3000 >/dev/null

CURRENT_URL="$(agent-browser "${AGENT_BROWSER_CDP_ARGS[@]}" get url)"
CURRENT_TITLE="$(agent-browser "${AGENT_BROWSER_CDP_ARGS[@]}" get title 2>/dev/null || true)"

echo "Opened ${TARGET_URL} via agent-browser on ${HOST}:${PORT}" >&2
echo "Current URL: ${CURRENT_URL}" >&2
if [[ -n "${CURRENT_TITLE}" ]]; then
  echo "Current title: ${CURRENT_TITLE}" >&2
fi
