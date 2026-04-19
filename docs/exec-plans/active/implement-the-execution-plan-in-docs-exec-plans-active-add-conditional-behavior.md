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
- [ ] Milestone 2: Extend config loading/validation to support automation-safety defaults and per-project overrides, including merge precedence.
- [ ] Milestone 3: Integrate policy into planner prompt/context and plan decoding so profile inference + override application produce normalized policy.
- [ ] Milestone 4: Thread normalized policy into contract and generator prompts, including no-action handling and required evidence semantics.
- [ ] Milestone 5: Implement browser action logging plus recent-activity metrics needed by behavioral checks.
- [ ] Milestone 6: Integrate policy checks into evaluator and scheduler flows/prompts, with hard-limit enforcement only for `browser_high_risk_engagement`.
- [ ] Milestone 7: Add/update tests and documentation for policy fields, config overrides, prompt integration, metrics, and enforcement behavior.

## Current progress

- Milestone 1 completed in this iteration.
- Added first-class `TaskSpec.AutomationSafety` policy support with profile and enforcement enums, mode/rate/pattern/text/cooldown substructures, and JSON compatibility via optional fields.
- Added automation-safety validation in assistant types, including:
  - allowed profile and enforcement values,
  - restriction that `engine_blocking` enforcement is only valid for `browser_high_risk_engagement`,
  - non-negative rate-limit checks,
  - required high-risk hard-limit fields,
  - required no-action evidence list when `require_no_action_evidence` is enabled.
- Added normalization support in `NormalizeTaskSpec` for optional automation safety policy, including high-risk defaults:
  - `max_account_changing_actions_per_run = 2`,
  - `max_replies_per_24h = 12`,
  - `min_spacing_minutes = 20`,
  - default no-action evidence requirements for high-risk profiles.
- Added assistant unit tests covering high-risk default normalization and policy validation success/failure paths.
- Full test suite passed: `go test ./...`.

## Key decisions

- Follow `add-conditional-behavior-safety-layer-for-browser-automation.md` as the implementation source of truth.
- Keep the first pass scoped to structured policy modeling and enforcement integration; do not add account/credential rotation or stealth/evasion features.
- Treat `browser_high_risk_engagement` as the only profile with engine-blocking hard-limit enforcement in the first pass.
- Use structured config-file overrides (global defaults + project overrides) as the policy override mechanism.
- Keep automation safety optional on `TaskSpec` (`omitempty`) so existing runs/specs remain backward compatible when no policy is present.

## Remaining issues / open questions

- Milestone 2: wire `automation_safety` defaults and project-level overrides into config loading and validation, and finalize merge precedence against engine defaults.
- Milestone 3: update planner prompt and decoding contract to emit and normalize policy profiles consistently.
- Confirm exact project-key lookup in config for per-project policy overrides.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `docs/exec-plans/active/add-conditional-behavior-safety-layer-for-browser-automation.md`
- `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md`
