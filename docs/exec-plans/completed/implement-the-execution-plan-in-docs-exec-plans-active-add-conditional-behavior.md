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
- [x] Milestone 6: Integrate policy checks into evaluator and scheduler flows/prompts, with hard-limit enforcement only for `browser_high_risk_engagement`.
- [x] Milestone 7: Add/update tests and documentation for policy fields, config overrides, prompt integration, metrics, and enforcement behavior.

## Current progress

- Milestones 1 through 7 are complete.
- Planner prompt contract includes `automation_safety` in strict JSON planner output schema and explicit profile/enforcement guidance for browser mutating and high-risk engagement classification.
- Planner user context includes automation-safety config context (global defaults + project override visibility) to expose structured config policy inputs during planning.
- Planner decode path now:
  - infers baseline automation-safety profile from planner tools/risk/user-request semantics,
  - resolves final policy with config merge precedence via config resolver (`engine defaults` -> `global defaults` -> `project override`),
  - normalizes resolved policy into `TaskSpec.AutomationSafety`.
- Engine/app wiring injects automation-safety config into planner prompt and decode flow.
- App-server planner output JSON schema and local heuristic planner output include `automation_safety` so runtime contracts remain aligned.
- Contract prompt user context includes normalized automation-safety policy details and contract system guidance requires policy-derived acceptance criteria/evidence, including no-action semantics when configured.
- Generator prompt includes normalized automation-safety policy context and explicit instructions for policy-gated no-action terminal paths and required no-action evidence details.
- Generator prompt explicitly requires preserving browser action detail needed for later safety metrics calculations (action type, target/source context, state-change, timing/spacing context).
- Browser action metadata flows through runtime web-step mapping (`action_name`, `target/ref`, `value`, `session`) so generated browser steps carry structured action context.
- Browser action domain model and metrics are implemented:
  - `BrowserActionRecord` with normalized action type, context, state-change marker, and optional text fingerprint.
  - `BrowserRecentActivityMetrics` with mutation density, source concentration, repeated sequence score, and text-reuse score.
- Safety metrics package (`internal/safety`) now:
  - extracts browser action records from web steps via deterministic classification,
  - derives source context from URL host/path,
  - computes payload fingerprints for mutating text-bearing actions,
  - computes rolling-window recent-activity metrics for policy checks.
- Persistent browser-action storage added:
  - `browser_actions` SQLite table + indexes,
  - repository APIs to add run actions and query project-window activity,
  - run-record hydration includes stored browser actions.
- Engine execution persistence writes a browser-action record for each persisted web step with run/attempt/project correlation.
- Evaluator and scheduler prompt inputs include automation-safety context and recent-activity metrics context.
- Evaluator flow now performs policy checks before final pass/fail routing:
  - merges automation-safety findings into evaluator output as missing requirements/next-action guidance,
  - keeps violations soft for `browser_mutating` (`evaluator_enforced`) by forcing retry-oriented failed evaluations,
  - hard-fails `browser_high_risk_engagement` (`engine_blocking`) on deterministic hard-limit violations.
- Deterministic high-risk hard-limit checks include:
  - per-run account-changing action cap,
  - rolling 24h reply cap,
  - minimum spacing between mutating actions,
  - disallowed default action-trio pattern,
  - required no-action evidence when no-action success is claimed.
- Scheduler flow enforces high-risk deterministic safety constraints before saving scheduled runs:
  - blocks scheduling when reply cap is already reached,
  - blocks follow-ups that violate minimum spacing,
  - blocks fixed short follow-up loops when disallowed by policy.
- Tests now cover policy fields, config override merge behavior, prompt integration, browser metrics, evaluator soft/hard enforcement, scheduler hard enforcement, and no-action-evidence validation checks.
- Documentation was added to describe policy model, config schema, metric model, enforcement mapping, and code touchpoints.
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
- Hard engine blocks remain limited to deterministic checks under `browser_high_risk_engagement`; ordinary `browser_mutating` violations stay evaluator-soft.

## Remaining issues / open questions

- None for the scoped first-pass implementation.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `docs/automation-safety-policy.md`
- `docs/exec-plans/active/add-conditional-behavior-safety-layer-for-browser-automation.md`
- `docs/exec-plans/active/add-non-normal-randomized-follow-up-scheduling.md`
