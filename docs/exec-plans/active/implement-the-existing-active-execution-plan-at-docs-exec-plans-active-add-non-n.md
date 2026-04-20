# Goal / scope

Implement the active execution plan in `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md` end-to-end as the source of truth.

In scope:

1. Add non-normal randomized follow-up scheduling support via `scheduled_for` expressions.
2. Keep existing fixed scheduling behavior fully compatible.
3. Add parser/unit/integration coverage and update user/developer documentation.
4. Complete milestone-by-milestone delivery with commits, pushes, PR creation, and auto-merge enablement.
5. Move the completed active plan into `docs/exec-plans/completed`.

Out of scope:

- New schedule distribution families beyond the scoped randomized expression.
- Broader recurrence/policy engines beyond the checked-in active plan.

## Background

The active plan defines a new randomized schedule syntax (`randexp(min,max)`) that should resolve into a concrete timestamp at schedule creation time while preserving existing RFC3339, relative duration, and clock-time scheduling behavior.

This execution plan is the implementation tracker that breaks that active plan into small coding-loop milestones and records progress and decisions during delivery.

## Milestones

- [x] Milestone 1: Implement parser support in `internal/assistant/schedule.go` for randomized `randexp(min,max)` expressions with strict validation and concrete timestamp materialization.
- [x] Milestone 2: Add/extend tests for randomized parsing and compatibility with existing fixed scheduling semantics.
- [x] Milestone 3: Update scheduler prompt guidance in `internal/prompting/prompts.go` so randomized scheduling can be intentionally emitted for irregular cooldown windows.
- [ ] Milestone 4: Update CLI/help and docs to expose randomized `--at` syntax for manual scheduling flows.
- [ ] Milestone 5: Run full verification, complete plan bookkeeping (including moving completed active plan file), then commit/push final milestone updates and open PR with auto-merge enabled.

## Current progress

- Milestone 1 completed: `ParseScheduledFor` now recognizes `randexp(min,max)` and materializes a concrete UTC timestamp at parse time.
- Added strict parser validation for randomized expressions: exact two args, parseable durations, positive durations, and `max > min`.
- Added truncated exponential inverse-CDF sampling helper with fixed `lambda=2.0` bias and sampler output guardrails (`[0,1)`).
- Milestone 2 completed: extended `internal/assistant/types_test.go` with randomized schedule coverage and fixed-format compatibility checks.
- Added randomized parser tests for: public `ParseScheduledFor` bounds checking, deterministic helper sampling boundaries, and invalid `randexp(...)` window validation cases.
- Added explicit RFC3339 parse compatibility test to keep fixed scheduling semantics covered alongside existing relative and clock-time tests.
- Milestone 3 completed: updated scheduler prompt instructions to keep RFC3339 as the preferred precise format while explicitly allowing `randexp(min,max)` for irregular cooldown windows.
- Added explicit prompt constraints for randomized syntax validity (`min`/`max` positive durations, `max > min`) to align generated schedules with parser validation behavior.
- Verification runs: `go test ./internal/assistant ./internal/assistantapp ./internal/wtl` and `go test ./internal/prompting ./internal/wtl ./internal/assistantapp` passed.

## Key decisions

- Use `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md` as the implementation source of truth.
- Keep randomization opt-in and parser-centric so all scheduling entry points remain consistent.
- Preserve backward compatibility for existing fixed scheduling inputs.
- Use a single fixed truncated-exponential shape parameter (`lambda=2.0`) for Milestone 1 and defer tuning to follow-up iterations if needed.
- Keep an internal sampler-injection helper (`parseRandExpScheduledForWithSampler`) to support deterministic tests in Milestone 2.
- Exercise both public and helper parsing paths in tests so production behavior and boundary math remain coupled to one parser implementation.
- Keep prompt wording preference-ordered: precise RFC3339 first, randomized windows as opt-in for irregular cooldown cases.
- Require test-backed validation before each milestone commit.

## Remaining issues / open questions

- Milestone 4 pending: document randomized `--at` syntax for CLI users and examples.
- Milestone 5 pending: run final verification, move completed active plan into `docs/exec-plans/completed`, and finish final bookkeeping.
- `ARCHITECTURE.md` was not present in this worktree; `README.md` is used as architecture/context reference.

## Links to related documents

- `AGENTS.md`
- `README.md`
- `docs/PLANS.md`
- `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md`
- `docs/automation-safety-policy.md`
