---
name: ralph-loop-run
version: 1
description: Safely run the full Ralph loop with schema discovery, dry-run preflight, and machine-readable output.
---

# Ralph Loop Run

1. Start with `./ralph-loop schema main --output json`.
2. If the request mutates state, run `./ralph-loop --json - --dry-run --output json`.
3. Execute with `--output json` or `--output ndjson`.
4. Use the returned `plan_path`, `worktree_path`, and `log_path` for follow-up inspection.
