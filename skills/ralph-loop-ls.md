---
name: ralph-loop-ls
version: 1
description: Discover active Ralph loop sessions and their worktree/log metadata.
---

# Ralph Loop LS

1. Use `./ralph-loop ls --output json` for automation.
2. Add `--fields worktree_id,work_branch,log_path` to shrink responses.
3. Use the returned selector fields as input to `./ralph-loop tail`.
