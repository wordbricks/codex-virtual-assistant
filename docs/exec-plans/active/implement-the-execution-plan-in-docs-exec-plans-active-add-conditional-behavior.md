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
- [x] Milestone 3: Integrate policy into planner prompt/context and plan decoding so profile inference + override application produce normalized policy.
- [x] Milestone 4: Thread normalized policy into contract and generator prompts, including no-action handling and required evidence semantics.
- [x] Milestone 5: Implement browser action logging plus recent-activity metrics needed by behavioral checks.
- [ ] Milestone 6: Integrate policy checks into evaluator and scheduler flows/prompts, with hard-limit enforcement only for `browser_high_risk_engagement`.
- [ ] Milestone 7: Add/update tests and documentation for policy fields, config overrides, prompt integration, metrics, and enforcement behavior.

## Current progress

- Milestone 1 complete.
- Milestone 2 complete.
- Milestone 3 complete.
- Milestone 4 completed in the previous iteration.
- Milestone 5 completed in this iteration.
- Planner prompt contract now includes `automation_safety` in strict JSON planner output schema and adds explicit profile/enforcement guidance for browser mutating and high-risk engagement classification.
- Planner user context now includes automation-safety config context (global defaults + project override visibility) to expose structured config policy inputs during planning.
- Planner decode path now:
  - infers baseline automation-safety profile from planner tools/risk/user-request semantics,
  - resolves final policy with config merge precedence via config resolver (`engine defaults` -> `global defaults` -> `project override`),
  - normalizes resolved policy into `TaskSpec.AutomationSafety`.
- Engine/app wiring now injects automation-safety config into planner prompt and decode flow.
- App-server planner output JSON schema and local heuristic planner output were updated to include `automation_safety` so runtime contracts remain aligned.
- Added/updated planner tests for prompt contract/context and decode inference + config-override behavior.
- Contract prompt user context now includes normalized automation-safety policy details and contract system guidance now requires policy-derived acceptance criteria/evidence, including no-action semantics when configured.
- Generator prompt now includes normalized automation-safety policy context and explicit instructions for policy-gated no-action terminal paths and required no-action evidence details.
- Generator prompt now explicitly requires preserving browser action detail needed for later safety metrics calculations (action type, target/source context, state-change, timing/spacing context).
- Browser action metadata now flows through runtime web-step mapping (`action_name`, `target/ref`, `value`, `session`) so generated browser steps carry structured action context.
- Added first-class browser action domain models:
  - `BrowserActionRecord` with normalized action type, context, state-change marker, and optional text fingerprint.
  - `BrowserRecentActivityMetrics` with mutation density, source concentration, repeated sequence score, and text-reuse score.
- Added safety metrics package (`internal/safety`) that:
  - extracts browser action records from web steps via deterministic classification,
  - derives source context from URL host/path,
  - computes payload fingerprints for mutating text-bearing actions,
  - computes rolling-window recent-activity metrics for policy checks.
- Added persistent browser-action storage:
  - new `browser_actions` SQLite table + indexes,
  - repository APIs to add run actions and query project-window activity,
  - run-record hydration includes stored browser actions.
- Engine execution persistence now writes a browser-action record for each persisted web step with run/attempt/project correlation.
- Evaluator and scheduler prompt inputs now accept recent-activity metrics; prompts include a structured metrics context block when available.
- Added/updated tests covering:
  - action extraction/classification + metrics calculations (`internal/safety`),
  - repository persistence/query paths for browser actions (`internal/store`),
  - runtime action metadata propagation (`internal/wtl/runtime_codex_test.go`),
  - evaluator/scheduler prompt metric context rendering (`internal/prompting/prompts_test.go`).
- Full test suite passed: `go test ./...`.

## Key decisions

- Follow `add-conditional-behavior-safety-layer-for-browser-automation.md` as the implementation source of truth.
- Keep the first pass scoped to structured policy modeling and enforcement integration; do not add account/credential rotation or stealth/evasion features.
- Treat `browser_high_risk_engagement` as the only profile with engine-blocking hard-limit enforcement in the first pass.
- Use structured config-file overrides (global defaults + project overrides) as the policy override mechanism.
- Keep automation safety optional on `TaskSpec` (`omitempty`) so existing runs/specs remain backward compatible when no policy is present.
- Keep policy merge logic in config for deterministic reuse by downstream planner integration.
- Inference should avoid downscoping stronger planner-emitted profiles; decode uses max-risk ranking when planner-provided profile and inferred profile differ.
- Browser action metrics are computed from persisted structured step/action data (not prompt prose parsing) to keep evaluator/scheduler checks deterministic.

## Remaining issues / open questions

- Milestone 6: integrate evaluator/scheduler policy checks and limit hard blocking to `browser_high_risk_engagement`.
- Milestone 7: complete docs/test coverage updates for prompt propagation, metrics, and enforcement outcomes.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `docs/exec-plans/active/add-conditional-behavior-safety-layer-for-browser-automation.md`
- `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md`
