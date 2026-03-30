---
name: ralph-loop-init
version: 1
description: Prepare a worktree safely with dry-run, JSON output, and no interactive prompts.
---

# Ralph Loop Init

1. Discover the live contract with `./ralph-loop schema init --output json`.
2. Prefer `./ralph-loop init --dry-run --output json` before the real run.
3. Use the returned `worktree_id`, `worktree_path`, and `runtime_root` for later commands.
