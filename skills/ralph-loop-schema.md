---
name: ralph-loop-schema
version: 1
description: Introspect the live CLI schema before constructing raw JSON payloads.
---

# Ralph Loop Schema

1. Start with `./ralph-loop schema --output json`.
2. Use `./ralph-loop schema <command> --output json` before building `--json` payloads.
3. Prefer the runtime schema over hard-coded assumptions about flags or defaults.
