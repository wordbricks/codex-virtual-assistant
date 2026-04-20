# Goal / scope

Add support for randomized follow-up scheduling so that the next deferred run can be assigned a start time sampled from a non-normal distribution instead of a fixed timestamp or fixed relative delay.

The immediate goal is narrow:

1. Allow scheduled follow-up work to express a randomized future start window in a structured `scheduled_for` format.
2. Materialize that randomized window into a concrete `scheduled_runs.scheduled_for` timestamp at schedule creation time.
3. Make the scheduler prompt aware of the new syntax so follow-up runs can intentionally avoid predictable fixed intervals.
4. Preserve all existing fixed-time schedule behavior for projects that do not need randomized follow-up timing.

Out of scope for the first pass:

- Replacing cron scheduling with randomized cron-like recurrence.
- Retrofitting all existing schedule entries in the database.
- Applying randomization to all browser work or all projects by default.
- Adding a full policy engine for anti-bot-safe automation in this change.

## Background

Today CVA supports three `scheduled_for` styles in [internal/assistant/schedule.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/schedule.go):

- absolute RFC3339 timestamps
- relative durations such as `+30m`
- same-day or next-day clock times such as `13:00`

Those values are parsed by `assistant.ParseScheduledFor()` and are used in both:

- the WTL scheduler phase when deferred `SchedulePlan` entries are materialized into `ScheduledRun` records
- the user-facing `cva schedule create --at ...` and `cva schedule update --at ...` paths via `RunService.resolveScheduledFor()`

That means a single parser change can support both automatic and manual scheduling paths without splitting behavior.

The current scheduler prompt in [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go) strongly prefers fixed RFC3339 timestamps whenever possible. This leads to predictable exact follow-up times such as `+30m` or a specific clock time. For work that benefits from less predictable pacing, CVA needs a way to represent "schedule this sometime in a safe window" while still saving a concrete timestamp into `scheduled_runs`.

## Proposed design

Introduce a new `scheduled_for` expression:

`randexp(min,max)`

Examples:

- `randexp(45m,3h)`
- `randexp(90m,6h)`

Semantics:

- `min` and `max` are positive relative durations measured from schedule creation time.
- A uniform random sample is transformed through a non-normal truncated exponential curve.
- The resulting concrete timestamp is sampled once when the schedule entry is materialized.
- The saved `ScheduledRun.ScheduledFor` remains a normal UTC timestamp in the database.

Why truncated exponential:

- It is explicitly non-normal.
- It is simple to implement and reason about.
- It biases execution earlier in the allowed window while still preserving irregular timing.
- It avoids the symmetry and center clustering of a normal distribution.

## Expected behavior

When a planner or scheduler wants a non-fixed follow-up:

1. The schedule entry carries `scheduled_for = "randexp(45m,3h)"`.
2. The WTL scheduler phase calls `assistant.ParseScheduledFor(...)`.
3. The parser resolves the expression into a concrete UTC timestamp between `now+45m` and `now+3h`.
4. The created `scheduled_runs` row stores only the resulting concrete timestamp.

Manual CLI behavior should remain aligned:

- `cva schedule create --run <id> --at 'randexp(45m,3h)' "..."` should work through the same parser.
- `cva schedule update <scheduled_run_id> --at 'randexp(45m,3h)'` should also work through the same parser.

## Key decisions

- The randomization should happen at schedule creation time, not at trigger time.
  This keeps scheduler polling simple and keeps each scheduled run deterministic once created.

- The randomized syntax should be opt-in.
  Existing `+30m`, `13:00`, and RFC3339 schedules should keep their current semantics.

- The parser should own the feature.
  The WTL scheduler phase, CLI scheduling paths, and API scheduling paths already converge through `ParseScheduledFor`, so the feature belongs there.

- The first implementation should use a single non-normal distribution only.
  One well-defined expression is easier to document and test than a family of random schedule functions.

## Milestones

- [x] Milestone 1: Extend `ParseScheduledFor()` with `randexp(min,max)` support.
- [x] Milestone 2: Add unit tests covering valid windows, invalid windows, and boundary handling.
- [x] Milestone 3: Update the scheduler prompt so deferred follow-up work can intentionally choose the randomized syntax.
- [x] Milestone 4: Update CLI/help text or docs so manual schedule management users know the syntax exists.
- [x] Milestone 5: Verify that WTL scheduling and direct schedule creation both flow through the same parser path without additional branching.

## Implementation plan

### Milestone 1: Parser support

Primary file:

- [internal/assistant/schedule.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/schedule.go)

Planned changes:

- Add a parser branch before the existing relative-duration handling.
- Recognize the form `randexp(min,max)`.
- Parse `min` and `max` as positive durations.
- Validate `max > min`.
- Sample a uniform random value and transform it through a truncated exponential inverse CDF.
- Convert the sampled normalized value into an offset inside `[min, max)`.
- Return a concrete UTC timestamp.

Planned helper structure:

- keep `ParseScheduledFor(raw, now)` as the public entry point
- add a small internal helper for randomized expressions so tests can inject a deterministic sampler if needed

### Milestone 2: Tests

Primary file:

- [internal/assistant/types_test.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/types_test.go)

Planned test coverage:

- existing relative and clock-time behavior still passes unchanged
- randomized expression returns a time within the declared bounds
- invalid randomized windows reject:
  - missing arguments
  - non-duration arguments
  - non-positive durations
  - `max <= min`

If deterministic testing is needed, add an unexported helper that accepts an injected uniform sampler.

### Milestone 3: Scheduler prompt guidance

Primary file:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)

Planned prompt change:

- Keep the current preference for RFC3339 when a fixed precise time is truly intended.
- Add explicit guidance that when the desired behavior is "next follow-up after an irregular cooldown window", the scheduler may emit `randexp(min,max)` instead of a fixed timestamp.

This keeps exact scheduling available while enabling less predictable follow-up timing.

### Milestone 4: User-facing documentation

Primary files:

- [cmd/cva/main.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/main.go)
- optionally `README.md` if the schedule feature docs should mention the new syntax

Planned updates:

- Mention that `--at` supports randomized scheduling expressions.
- Add one short example for `randexp(45m,3h)`.

### Milestone 5: End-to-end path verification

Relevant files:

- [internal/wtl/engine.go](/Users/dev/git/codex-virtual-assistant/internal/wtl/engine.go)
- [internal/assistantapp/service.go](/Users/dev/git/codex-virtual-assistant/internal/assistantapp/service.go)

Verification goals:

- WTL-generated scheduled follow-ups and manual `cva schedule create/update` commands both resolve through the same parser.
- No new scheduler polling behavior is required.
- Saved scheduled run rows continue to store concrete timestamps, not raw random expressions.

## Risks / open questions

- Distribution shape:
  The first implementation should pick a fixed exponential shape parameter. If the bias is too aggressive or too weak, a follow-up change can tune it.

- Reproducibility:
  The parser should sample with a real random source in production, but tests may need deterministic injection.

- Prompt reliability:
  Even after prompt updates, the model may still emit exact timestamps often. That is acceptable; the feature only needs to make randomized scheduling possible, not mandatory.

- Documentation clarity:
  The syntax should be simple enough that a human operator can use it directly in CLI scheduling commands.

## Current progress

- Completed. `randexp(min,max)` parsing is implemented and materialized to concrete UTC timestamps at schedule creation time.
- Coverage added for randomized parsing validity/bounds and compatibility with existing fixed scheduling inputs.
- Scheduler prompt guidance updated to allow randomized windows when irregular cooldown spacing is appropriate.
- CLI/help and README docs updated to expose manual `--at` support for `randexp(min,max)`.
- Verified parser-path convergence across WTL scheduling and manual/API scheduling flows; full test suite passed before completion.
