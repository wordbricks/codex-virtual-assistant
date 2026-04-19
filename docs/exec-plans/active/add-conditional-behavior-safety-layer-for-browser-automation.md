# Goal / scope

Add a conditional behavior-safety layer to CVA so that browser automation runs with external side effects can follow stricter anti-detection operating rules without imposing the same restrictions on unrelated coding, research, or read-only browser tasks.

The purpose of this change is not to "guarantee undetectability." The purpose is to reduce obviously repetitive, high-risk automation patterns by making CVA plan, execute, evaluate, and schedule mutating browser work more conservatively.

This plan focuses on the product and engine design needed to support that behavior.

In scope:

1. Introduce a structured automation-safety policy model in the run planning layer.
2. Activate that policy only for relevant runs, especially browser automation with external side effects.
3. Apply the same policy across planner, contract, generator, evaluator, and scheduler phases.
4. Support project-specific operating rules while keeping the safety layer reusable across projects.

Out of scope for the first pass:

- Building a site-specific stealth system.
- Promising that automated behavior cannot be detected by third-party platforms.
- Applying heavy restrictions to all runs by default.
- Replacing project playbooks or project documentation with engine-only rules.
- Adding account or credential rotation.

## Background

Today CVA already has the right high-level execution shape for a reusable safety layer:

- `TaskSpec` is the central normalized plan object.
- The planner defines the work shape.
- The contract phase freezes the acceptance criteria.
- The generator performs the work.
- The evaluator decides whether the result should pass, fail, or retry.
- The scheduler decides deferred follow-up behavior.

Relevant current code paths:

- [internal/assistant/types.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/types.go)
- [internal/assistant/taskspec.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/taskspec.go)
- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)
- [internal/wtl/engine.go](/Users/dev/git/codex-virtual-assistant/internal/wtl/engine.go)

Today the system already captures coarse risk through `risk_flags`, for example `external-side-effect`, but it does not yet model behavioral safety explicitly. That means mutating browser work can still be planned and evaluated with the same "complete the task" bias used for normal work.

For low-risk tasks this is fine. For account-growth, outreach, community-engagement, marketplace, messaging, and other externally visible automation, that is too weak.

The desired outcome is a conditional policy layer that:

- stays off for ordinary runs,
- applies lightly for read-only browser work,
- applies strongly for mutating browser automation,
- can be tuned per domain or project.

## Problem statement

Different project types need different levels of behavioral control.

Examples:

- A coding project that edits Go files should not be slowed down by anti-bot pacing rules.
- A browser-based QA run that only reads pages and takes screenshots should not inherit high-friction mutation rules.
- A social-growth or outreach workflow that follows, replies, sends messages, or otherwise changes external account state needs strong pacing, diversity, and no-action fallback rules.

Without an explicit safety layer, those distinctions live only in project prose and prompt wording. That makes behavior inconsistent and difficult to reuse across projects.

## Design principles

1. Conditional activation

The safety layer should only activate when the run profile justifies it.

2. Shared policy across phases

The planner, generator, evaluator, and scheduler must all use the same policy object. It is not enough to warn only the generator.

3. No-action is a valid result

For high-risk mutating automation, a run should be allowed to stop with observation-only or no-action results when behavioral risk is too high.

4. Browser automation is not one thing

Read-only browser work and mutating browser work should be treated differently.

5. Project-specific rules still matter

The engine should support shared structure while letting projects override or tighten behavior.

## User-confirmed decisions

The first concrete implementation should follow these decisions:

- Default activation starts at `browser_mutating`. Read-only browser work can still be classified as `browser_read_only`, but it should not receive mutation-specific restrictions unless a config override explicitly tightens it.
- High-risk classification should use the recommended conservative catalog: social growth, outbound messaging, public comments/replies, follows or connection requests, marketplace inquiries, community engagement, recruiting or networking messages, and other account-reputation-affecting browser actions.
- Enforcement should be hard only for `browser_high_risk_engagement`. Normal `browser_mutating` work receives structured policy guidance, contract criteria, and evaluator checks, but the engine should not hard-block it in the first pass.
- Default high-risk limits are two account-changing actions per run and twelve replies per rolling 24-hour window. Project config can tighten or loosen these values.
- No-action success should follow the recommended path: allowed by default for high-risk runs, and allowed for other mutating browser runs only when the policy explicitly enables it or safety cannot be established. No-action outcomes must include evidence explaining what was observed, what action was skipped, why it was skipped, and what a safe next step would be.
- Project-specific overrides should come from a structured config file, not from unstructured prose extraction in the first pass.
- Recent-activity metrics should include new browser action logging, not only existing run, evidence, scheduled-run, and tool-call records.
- Account and credential rotation are explicitly out of scope.

## Proposed model

Add a structured automation safety policy to the normalized run plan.

Proposed conceptual shape:

```json
{
  "automation_safety": {
    "profile": "none | browser_read_only | browser_mutating | browser_high_risk_engagement",
    "enforcement": "advisory | evaluator_enforced | engine_blocking",
    "mode_policy": {
      "allowed_session_modes": ["read_only", "single_action", "reply_only"],
      "allow_no_action_success": true,
      "require_no_action_evidence": true
    },
    "rate_limits": {
      "max_account_changing_actions_per_run": 2,
      "max_replies_per_24h": 12,
      "min_spacing_minutes": 20
    },
    "pattern_rules": {
      "disallow_default_action_trios": true,
      "disallow_fixed_short_followups": true,
      "require_source_diversity": true
    },
    "text_reuse_policy": {
      "reject_high_similarity": true,
      "avoid_repeated_self_intro": true
    },
    "cooldown_policy": {
      "force_read_only_after_dense_activity": true,
      "prefer_longer_cooldown_after_blocked_runs": true
    }
  }
}
```

The exact field names can be finalized during implementation, but the key point is that the safety policy becomes a first-class part of the normalized run plan instead of remaining implicit in project prose.

The default enforcement mapping should be:

- `none`: no policy emitted.
- `browser_read_only`: `advisory`, with no mutation-specific limits.
- `browser_mutating`: `evaluator_enforced`, meaning planner, contract, generator, and evaluator all see the policy, but the engine does not hard-block actions in the first pass.
- `browser_high_risk_engagement`: `engine_blocking` for hard limits that can be checked deterministically, especially per-run account-changing action caps, rolling reply limits, disallowed action bundles, and unsafe short follow-up scheduling.

## Config override model

Project-specific policy overrides should be read from a structured config file. The first implementation should extend the existing JSON config path rather than extracting policy from project prose.

Proposed config shape:

```json
{
  "automation_safety": {
    "defaults": {
      "browser_mutating": {
        "enforcement": "evaluator_enforced",
        "allow_no_action_success": false
      },
      "browser_high_risk_engagement": {
        "enforcement": "engine_blocking",
        "max_account_changing_actions_per_run": 2,
        "max_replies_per_24h": 12,
        "min_spacing_minutes": 20,
        "allow_no_action_success": true
      }
    },
    "projects": {
      "example-project-slug": {
        "profile_override": "browser_high_risk_engagement",
        "max_account_changing_actions_per_run": 1
      }
    }
  }
}
```

The merge order should be:

1. engine defaults,
2. global config defaults,
3. project-specific config entry,
4. explicit run or `TaskSpec` override, if one is added later.

For this first pass, config-file support is required; unstructured project wiki or playbook extraction is intentionally deferred.

## Activation model

The safety layer should activate by profile, not globally.

### Profile `none`

Use when:

- the run does not involve browser automation
- the run has no meaningful external side-effect risk

Examples:

- local coding
- documentation updates
- repository review
- offline data transforms

### Profile `browser_read_only`

Use when:

- the run uses `agent-browser`
- but is limited to reading, scraping, screenshotting, QA, or observation

Examples:

- checking a dashboard
- collecting website pricing
- taking screenshots of a web app

Behavior:

- light pacing or realism guidance
- no high-friction account-growth rules
- no mutation-specific rate limits
- not part of the default activation scope unless a config override requests stricter treatment

### Profile `browser_mutating`

Use when:

- the run uses browser automation
- and can change external site state

Examples:

- submitting forms
- sending messages
- posting content
- saving preferences
- interacting with user accounts

Behavior:

- account-changing action caps as policy criteria, but no first-pass engine hard block unless escalated to high-risk
- cooldown expectations
- explicit no-action or skip paths where appropriate
- evaluator-enforced compliance checks

### Profile `browser_high_risk_engagement`

Use when:

- the run mutates external state
- and the domain has stronger behavioral-detection sensitivity

Examples:

- social growth workflows
- outbound engagement systems
- message-heavy networking automation
- public-facing community interaction workflows
- public comments or replies
- follows, likes, connection requests, endorsements, or other reputation-affecting reactions
- marketplace, recruiting, sales, or networking outreach
- repeated inquiry or application submission workflows

Behavior:

- stronger session-mode control
- engine-blocking enforcement for deterministic hard limits
- text reuse controls
- source-diversity expectations
- schedule randomness and cooldown logic
- explicit evaluator checks for repetitive behavior
- default cap of two account-changing actions per run
- default cap of twelve replies per rolling 24-hour window
- no-action success allowed when the run records enough evidence to justify skipping mutation

## Detection logic

The profile should be selected automatically where possible, with room for project overrides.

Inputs for profile inference:

1. Tools:
- `TaskSpec.ToolsAllowed`
- `TaskSpec.ToolsRequired`

2. Risk flags:
- existing `external-side-effect`
- future browser-specific risk flags

3. Project context:
- browser profile / CDP port present
- project-specific automation-safety config

4. Request semantics:
- keywords implying mutation such as `post`, `reply`, `follow`, `connect`, `send`, `submit`, `publish`, `apply`, `message`, `purchase`
- high-risk keywords and intents such as `growth`, `engage`, `outreach`, `comment`, `DM`, `invite`, `connection request`, `marketplace inquiry`, `recruiting message`, `networking`, `like`, `endorse`, `review`, or `community reply`

5. Project overrides:
- a project config entry may explicitly request a stronger profile when domain risk is known

The planner should prefer false positives over false negatives only when the candidate profile is `browser_high_risk_engagement`. For ordinary `browser_mutating` work such as a single form submission or preference save, the planner should keep the profile at `browser_mutating` unless the request is public-facing, reputation-affecting, message-heavy, or config-upgraded.

## Phase-by-phase behavior

### Planner

The planner should:

- determine the safety profile
- emit the structured safety policy in the normalized task spec
- define whether no-action is acceptable
- declare rate limits and disallowed action patterns for high-risk runs
- mark `browser_high_risk_engagement` policies as engine-blocking for deterministic limits
- include the source of policy decisions, such as inferred request semantics or config override

This is the right place to establish intent, because downstream phases should consume the same normalized policy instead of rediscovering it independently.

### Contract

The contract should turn the safety policy into explicit acceptance criteria.

Examples:

- "At most two account-changing actions are allowed in this run."
- "At most twelve replies are allowed in the rolling 24-hour window."
- "A no-action finish counts as acceptable if safety cannot be proven."
- "Do not perform the default follow-like-reply trio."
- "If no action is taken, record the observed context, skipped action, reason, and suggested safe next step."

This makes safety evaluator-visible and not merely generator guidance.

### Generator

The generator should:

- obey the selected profile
- treat no-action as a valid terminal path where the policy allows it
- avoid defaulting toward mutation when risk is high
- honor pacing, source-diversity, and pattern restrictions
- emit structured browser action records for relevant browser interactions so later phases can compute recent-activity metrics
- stop before mutating when a high-risk hard limit has already been reached

The generator should not need to invent these rules ad hoc. It should act inside the policy selected earlier.

### Evaluator

The evaluator should judge both:

- task completion quality
- behavioral safety compliance

For high-risk browser automation, the evaluator should be able to fail or force retry when the run violates the safety contract even if the nominal task outcome succeeded.

Examples of future evaluator checks:

- account-changing action count exceeds two in a high-risk run
- reply count would exceed twelve in the rolling 24-hour window
- repeated action sequence
- too many account-changing actions in the recent window
- repeated source path concentration
- overly similar text outputs
- schedule cadence that is too fixed or too dense

### Scheduler

The scheduler should incorporate behavior policy into deferred follow-up planning.

Examples:

- avoid short fixed follow-up loops
- widen cooldown windows after dense activity
- force the next run into `read_only` after repeated mutation success
- avoid scheduling another mutating run when recent similarity or source concentration is too high
- prefer randomized follow-up windows for high-risk engagement when scheduling is appropriate

## Proposed implementation architecture

### Milestone 1: Add structured safety fields to normalized planning

Primary files:

- [internal/assistant/types.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/types.go)
- [internal/assistant/taskspec.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/taskspec.go)

Planned work:

- add a structured safety policy object to `TaskSpec`
- keep backward compatibility for existing runs
- define validation rules for optional policy fields
- add profile and enforcement enums
- represent default high-risk limits: two account-changing actions per run, twelve replies per rolling 24-hour window, and minimum spacing guidance
- represent no-action evidence requirements

### Milestone 2: Add config-file policy overrides

Primary files:

- [internal/config/config.go](/Users/dev/git/codex-virtual-assistant/internal/config/config.go)
- [internal/config/config_test.go](/Users/dev/git/codex-virtual-assistant/internal/config/config_test.go)

Planned work:

- extend the JSON config model with `automation_safety`
- support global defaults and per-project overrides
- merge engine defaults with config-file overrides before planner policy construction
- validate config values and reject unknown profiles, enforcement modes, or negative limits
- keep unstructured project wiki/playbook extraction out of the first pass

### Milestone 3: Extend planner prompt and decoding

Primary file:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)

Planned work:

- teach the planner to emit the safety profile and policy when browser automation is relevant
- keep the output minimal or empty when the safety profile is `none`
- default activation to `browser_mutating` and `browser_high_risk_engagement`
- classify social growth, outbound messaging, public comments/replies, follows/connections, marketplace inquiries, community engagement, recruiting, and networking messages as high-risk
- carry config-file profile overrides into the planner context
- update planner prompt tests accordingly

### Milestone 4: Carry safety policy into contract and generator prompts

Primary file:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)

Planned work:

- include policy context in contract prompt construction
- include policy context in generator prompt construction
- explicitly allow no-action success paths for matching profiles
- require no-action evidence when a high-risk run skips mutation
- require generator output to preserve enough action detail for later metric calculation

### Milestone 5: Add browser action logging and recent-activity metrics

Primary areas:

- new package, likely under `internal/safety`
- repository/store types that currently persist run, tool-call, evidence, and web-step data

Planned work:

- add structured browser action records for actions relevant to safety calculations
- include action type, target/source context, timestamp, whether external account state changed, text payload fingerprint where applicable, and run/project identifiers
- compute recent mutation density, reply counts, source-path concentration, repeated action sequence score, and text reuse risk score
- make those metrics available to evaluator and scheduler

### Milestone 6: Add behavioral safety checks to evaluator logic

Primary files:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)
- [internal/wtl/engine.go](/Users/dev/git/codex-virtual-assistant/internal/wtl/engine.go)

Planned work:

- extend evaluator instructions so policy compliance is part of the pass/fail decision
- keep those checks conditional on the selected profile
- hard-fail high-risk runs that exceed deterministic hard limits
- use soft evaluator findings for ordinary `browser_mutating` policy violations
- verify no-action evidence when no-action success is claimed

### Milestone 7: Feed policy into scheduler decisions

Primary files:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)
- [internal/wtl/engine.go](/Users/dev/git/codex-virtual-assistant/internal/wtl/engine.go)

Planned work:

- use safety policy to shape deferred work
- prevent obviously repetitive short follow-up scheduling in high-risk profiles
- use recent-activity metrics and reply/action counts before creating a new mutating follow-up
- prefer randomized follow-up windows over fixed short delays for high-risk engagement
- force read-only follow-up when high-risk limits or density thresholds are reached

## Project integration strategy

Project docs and playbooks should not be replaced, but the first implementation should not rely on prose extraction for policy. Project-specific operating rules should be entered into a structured config file so the planner receives deterministic policy inputs.

Recommended split:

- project docs explain domain-specific operating rules for humans
- config file defines machine-readable domain-specific safety policy
- CVA engine defines the reusable safety structure
- planner maps config and request semantics into normalized safety policy
- evaluator and scheduler enforce the normalized policy consistently

This preserves project flexibility while avoiding repeated one-off prompt rules across projects.

## Rollout strategy

Recommended rollout order:

1. Add the policy shape to `TaskSpec`
2. Add config-file overrides
3. Add planner support
4. Add generator + contract support
5. Add browser action logging and recent-activity metrics
6. Add evaluator policy checks
7. Add scheduler policy handling

This keeps the first pass simple and minimizes breakage.

## Risks / open questions

- Overreach:
  If the safety layer activates too broadly, it may degrade normal browser QA or research workflows.

- Under-specification:
  If the policy object is too vague, the generator and evaluator will still behave inconsistently.

- Policy drift:
  Project playbooks and config rules could diverge. For the first pass, config wins because it is machine-readable; prose extraction remains out of scope.

- Data availability:
  Strong evaluator checks need reliable action logs. The plan now includes explicit browser action logging before hard evaluator and scheduler enforcement.

- Domain generality:
  Some rules are generic, while others are domain-specific. The engine should support both without hard-coding one site's rules into every project.

- False positive hard blocks:
  Hard blocking is limited to `browser_high_risk_engagement` because a mistaken block in ordinary browser mutation could prevent legitimate QA, admin, or form-submission workflows.

- Remaining ambiguity:
  The exact config file path and project-key format should be finalized during implementation after reviewing how project slugs and the existing CVA config are loaded.

## Current progress

- Planned only.
- User questionnaire answers have been incorporated into the plan.
- No product code changes have been made as part of this document-only step.
