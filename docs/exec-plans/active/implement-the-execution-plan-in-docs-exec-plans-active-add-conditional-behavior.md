# Goal / scope

Implement the execution plan in `docs/exec-plans/active/add-conditional-behavior-safety-layer-for-browser-automation.md` as the source of truth, delivering a conditional automation safety policy for browser mutating and high-risk engagement workflows.

In scope:

1. Add `TaskSpec` automation-safety policy fields and validation.
2. Add config-file defaults and per-project overrides for automation safety.
3. Integrate policy into planner, contract, generator, evaluator, and scheduler prompts/flows.
4. Add browser action logging and recent-activity metrics used by evaluator/scheduler policy checks.
5. Add/adjust tests and docs to cover behavior.

Out of scope:

- Account or credential rotation.
- Site-specific stealth/evasion behavior.
- Any behavior not described by the checked-in execution plan.

## Background

CVA currently has risk flags and multi-phase run processing, but it lacks a first-class, structured automation-safety policy that conditionally applies stricter rules for external-state-mutating browser workflows.

The active plan requires a reusable policy model that remains off for ordinary runs, is lighter for read-only browser runs, and becomes strict for high-risk engagement automation. Implementation must preserve project flexibility through structured config overrides and avoid prose-based extraction in the first pass.

## Milestones

- [x] Milestone 1: Add automation safety policy model to `TaskSpec`/assistant types (profiles, enforcement, limits, no-action evidence requirements) with compatibility and validation.
- [x] Milestone 2: Extend config loading/validation to support automation-safety defaults and per-project overrides, including merge precedence.
- [ ] Milestone 3: Integrate policy into planner prompt/context and plan decoding so profile inference + override application produce normalized policy.
- [ ] Milestone 4: Thread normalized policy into contract and generator prompts, including no-action handling and required evidence semantics.
- [ ] Milestone 5: Implement browser action logging plus recent-activity metrics needed by behavioral checks.
- [ ] Milestone 6: Integrate policy checks into evaluator and scheduler flows/prompts, with hard-limit enforcement only for `browser_high_risk_engagement`.
- [ ] Milestone 7: Add/update tests and documentation for policy fields, config overrides, prompt integration, metrics, and enforcement behavior.

## Current progress

- Milestone 1 complete.
- Milestone 2 completed in this iteration.
- Extended `internal/config` file schema with structured `automation_safety` support:
  - global profile defaults (`automation_safety.defaults`),
  - per-project overrides (`automation_safety.projects`),
  - project-level `profile_override`, and policy override fields for enforcement/mode/rate/pattern/text-reuse/cooldown sections.
- Added config-layer validation for automation-safety values:
  - rejects unknown profiles,
  - rejects invalid enforcement modes,
  - rejects negative numeric limits,
  - enforces `engine_blocking` only when profile context is `browser_high_risk_engagement`.
- Added merge-precedence resolver in config:
  - `engine defaults` -> `global defaults for effective profile` -> `project override`.
- Added config unit tests for invalid profile/enforcement rejection and precedence merge behavior.
- Full test suite passed: `go test ./...`.

## Key decisions

- Follow `add-conditional-behavior-safety-layer-for-browser-automation.md` as the implementation source of truth.
- Keep the first pass scoped to structured policy modeling and enforcement integration; do not add account/credential rotation or stealth/evasion features.
- Treat `browser_high_risk_engagement` as the only profile with engine-blocking hard-limit enforcement in the first pass.
- Use structured config-file overrides (global defaults + project overrides) as the policy override mechanism.
- Keep automation safety optional on `TaskSpec` (`omitempty`) so existing runs/specs remain backward compatible when no policy is present.
- Keep policy merge logic in config for deterministic reuse by downstream planner integration.

## Remaining issues / open questions

- Milestone 3: thread config policy into planner prompt/context and planner output decoding so inferred profile + config overrides produce normalized `TaskSpec.AutomationSafety`.
- Confirm final planner-side profile inference behavior for read-only vs mutating vs high-risk engagement classification.
- Milestones 4-7 remain pending for prompt threading, logging/metrics, evaluator/scheduler enforcement, and docs.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `docs/exec-plans/active/add-conditional-behavior-safety-layer-for-browser-automation.md`
- `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md`
