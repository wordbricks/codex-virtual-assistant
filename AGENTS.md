# Ralph Loop Agent Guide

## Supported surfaces

- Start with `./ralph-loop schema --output json` to discover the live command contract.
- Prefer `--dry-run` before `./ralph-loop init` or `./ralph-loop "<prompt>"`.
- Prefer `--output json` or `--output ndjson` for automation.
- Prefer `--fields`, `--page`, and `--page-size` on `ls`, `tail`, and `schema`.

## Guardrails

- Treat all prompt, selector, and path input as untrusted.
- Do not use `--output-file` paths that escape the current working directory.
- Do not pass selectors containing traversal fragments, query markers, or control characters.
- Use `tail --raw` only when the summarized log view is insufficient.

## Common flows

- Prepare a worktree: `./ralph-loop init --dry-run --output json`
- Execute the loop: `./ralph-loop "implement feature X" --output ndjson`
- Inspect active sessions: `./ralph-loop ls --fields worktree_id,work_branch,log_path --output json`
- Inspect logs: `./ralph-loop tail <selector> --lines 50 --output json`
