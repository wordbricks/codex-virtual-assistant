---
name: ralph-loop-tail
version: 1
description: Inspect Ralph loop logs with narrow selectors, field masks, and NDJSON follow mode.
---

# Ralph Loop Tail

1. Narrow the target with a selector before reading large logs.
2. Prefer `--output json` for snapshots and `--follow --output ndjson` for streaming.
3. Use `--fields` to reduce payload size when consuming structured lines.
