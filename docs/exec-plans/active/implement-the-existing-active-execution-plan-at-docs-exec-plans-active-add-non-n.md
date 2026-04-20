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

- [ ] Milestone 1: Implement parser support in `internal/assistant/schedule.go` for randomized `randexp(min,max)` expressions with strict validation and concrete timestamp materialization.
- [ ] Milestone 2: Add/extend tests for randomized parsing and compatibility with existing fixed scheduling semantics.
- [ ] Milestone 3: Update scheduler prompt guidance in `internal/prompting/prompts.go` so randomized scheduling can be intentionally emitted for irregular cooldown windows.
- [ ] Milestone 4: Update CLI/help and docs to expose randomized `--at` syntax for manual scheduling flows.
- [ ] Milestone 5: Run full verification, complete plan bookkeeping (including moving completed active plan file), then commit/push final milestone updates and open PR with auto-merge enabled.

## Current progress

- Not started.

## Key decisions

- Use `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md` as the implementation source of truth.
- Keep randomization opt-in and parser-centric so all scheduling entry points remain consistent.
- Preserve backward compatibility for existing fixed scheduling inputs.
- Require test-backed validation before each milestone commit.

## Remaining issues / open questions

- No blocker identified at planning start.
- `ARCHITECTURE.md` was not present in this worktree; `README.md` is used as architecture/context reference.

## Links to related documents

- `AGENTS.md`
- `README.md`
- `docs/PLANS.md`
- `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md`
- `docs/automation-safety-policy.md`
