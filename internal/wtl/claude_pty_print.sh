#!/usr/bin/env bash
set -euo pipefail

real_claude="${CLAUDE_REAL_BIN:-$(command -v claude)}"

if [[ ! -x "$real_claude" ]]; then
  echo "claude-pty-print: could not find executable claude binary" >&2
  exit 1
fi

print_mode=false
output_format="text"
declare -a pass_args=()
declare -a prompt_parts=()

while (($#)); do
  case "$1" in
    -p|--print)
      print_mode=true
      shift
      ;;
    --output-format)
      if (($# < 2)); then
        echo "claude-pty-print: --output-format requires a value" >&2
        exit 2
      fi
      output_format="$2"
      shift 2
      ;;
    --output-format=*)
      output_format="${1#*=}"
      shift
      ;;
    --)
      shift
      while (($#)); do
        prompt_parts+=("$1")
        shift
      done
      ;;
    -*)
      pass_args+=("$1")
      shift
      ;;
    *)
      prompt_parts+=("$1")
      shift
      ;;
  esac
done

if [[ "$print_mode" != true ]]; then
  exec "$real_claude" "${pass_args[@]}" "${prompt_parts[@]}"
fi

if [[ "$output_format" != "text" ]]; then
  echo "claude-pty-print: only text output is supported for wrapped -p mode" >&2
  exit 2
fi

prompt_text="${prompt_parts[*]-}"
if [[ -z "$prompt_text" ]]; then
  echo "claude-pty-print: wrapped -p mode requires a prompt argument" >&2
  exit 2
fi

raw_file="$(mktemp -t claude-pty-print.raw.XXXXXX)"
clean_file="$(mktemp -t claude-pty-print.clean.XXXXXX)"
spawn_script="$(mktemp -t claude-pty-print.spawn.XXXXXX)"
trap 'rm -f "$raw_file" "$clean_file" "$spawn_script"' EXIT

{
  printf '%q ' "$real_claude"
  if ((${#pass_args[@]})); then
    printf '%q ' "${pass_args[@]}"
  fi
} >"$spawn_script"
spawn_cmd="$(cat "$spawn_script")"

CLAUDE_WRAP_PROMPT="$prompt_text" \
CLAUDE_WRAP_COMMAND="$spawn_cmd" \
CLAUDE_WRAP_RAW="$raw_file" \
/usr/bin/python3 - <<'PY'
import os
import pty
import select
import signal
import subprocess
import sys
import time

prompt = os.environ["CLAUDE_WRAP_PROMPT"]
cmd = os.environ["CLAUDE_WRAP_COMMAND"]
raw_path = os.environ["CLAUDE_WRAP_RAW"]

master_fd, slave_fd = pty.openpty()
proc = subprocess.Popen(
    ["/bin/zsh", "-lc", cmd],
    stdin=slave_fd,
    stdout=slave_fd,
    stderr=slave_fd,
    close_fds=True,
)
os.close(slave_fd)

deadline = time.time() + 180
prompt_marker = "❯".encode("utf-8")
answer_marker = "⏺".encode("utf-8")
sent_prompt = False
seen_answer = False
prompt_count = 0

with open(raw_path, "wb") as raw:
    while time.time() < deadline:
        ready, _, _ = select.select([master_fd], [], [], 0.5)
        if master_fd in ready:
            try:
                chunk = os.read(master_fd, 65536)
            except OSError:
                chunk = b""
            if chunk:
                raw.write(chunk)
                raw.flush()
                prompt_count += chunk.count(prompt_marker)
                if answer_marker in chunk:
                    seen_answer = True
                if not sent_prompt and prompt_count >= 1:
                    os.write(master_fd, prompt.encode("utf-8") + b"\r")
                    sent_prompt = True
                    continue
                if sent_prompt and seen_answer and prompt_count >= 2:
                    break
            elif proc.poll() is not None:
                break
        elif proc.poll() is not None:
            break

    try:
        os.write(master_fd, b"\x03")
    except OSError:
        pass

    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.send_signal(signal.SIGTERM)
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait(timeout=2)

os.close(master_fd)

if not sent_prompt:
    sys.stderr.write("claude-pty-print: timed out waiting for initial prompt\n")
    sys.exit(124)
if not seen_answer:
    sys.stderr.write("claude-pty-print: no assistant response detected before timeout\n")
    sys.exit(124)
PY

/usr/bin/perl -CSDA -pe '
  s/\e\[([0-9]+)C/" " x $1/ge;
  s/\e\[[0-9;?]*[ -\/]*[@-~]//g;
  s/\e\][^\a]*(?:\a|\e\\)//g;
  s/\r//g;
' "$raw_file" >"$clean_file"

/usr/bin/python3 - "$clean_file" <<'PY'
from pathlib import Path
import re
import sys

text = Path(sys.argv[1]).read_text(errors="ignore")
text = text.replace("\u00a0", " ")
text = re.sub(r"[ \t]+\n", "\n", text)

match = None
for m in re.finditer(r"⏺\s*(.+?)(?:\n\s*✻\s+Worked for|\n\s*❯\s|\Z)", text, re.S):
    match = m

if not match:
    tail = "\n".join(text.splitlines()[-80:])
    sys.stderr.write("claude-pty-print: could not extract assistant response\n")
    sys.stderr.write(tail + "\n")
    sys.exit(1)

answer = match.group(1)
answer = answer.split("❯", 1)[0]
answer = re.sub(r"\n{3,}", "\n\n", answer)
answer = "\n".join(line.rstrip() for line in answer.splitlines()).strip()
answer = re.sub(r"(?<!\n)\n(?!\n)", " ", answer)
print(answer)
PY
