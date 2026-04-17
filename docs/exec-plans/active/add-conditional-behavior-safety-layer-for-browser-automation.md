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

## Proposed model

Add a structured automation safety policy to the normalized run plan.

Proposed conceptual shape:

```json
{
  "automation_safety": {
    "profile": "none | browser_read_only | browser_mutating | browser_high_risk_engagement",
    "mode_policy": {
      "allowed_session_modes": ["read_only", "like_only", "reply_only"],
      "allow_no_action_success": true
    },
    "rate_limits": {
      "max_account_changing_actions_per_hour": 1,
      "max_replies_per_24h": 3,
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

- account-changing action caps
- cooldown expectations
- explicit no-action or skip paths where appropriate

### Profile `browser_high_risk_engagement`

Use when:

- the run mutates external state
- and the domain has stronger behavioral-detection sensitivity

Examples:

- social growth workflows
- outbound engagement systems
- message-heavy networking automation
- public-facing community interaction workflows

Behavior:

- stronger session-mode control
- text reuse controls
- source-diversity expectations
- schedule randomness and cooldown logic
- explicit evaluator checks for repetitive behavior

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
- project wiki/playbook policy

4. Request semantics:
- keywords implying mutation such as `post`, `reply`, `follow`, `connect`, `send`, `submit`, `publish`, `apply`, `message`, `purchase`

5. Project overrides:
- a project may explicitly request a stronger profile when domain risk is known

## Phase-by-phase behavior

### Planner

The planner should:

- determine the safety profile
- emit the structured safety policy in the normalized task spec
- define whether no-action is acceptable
- declare rate limits and disallowed action patterns for high-risk runs

This is the right place to establish intent, because downstream phases should consume the same normalized policy instead of rediscovering it independently.

### Contract

The contract should turn the safety policy into explicit acceptance criteria.

Examples:

- "At most one account-changing action is allowed in this run."
- "A no-action finish counts as acceptable if safety cannot be proven."
- "Do not perform the default follow-like-reply trio."

This makes safety evaluator-visible and not merely generator guidance.

### Generator

The generator should:

- obey the selected profile
- treat no-action as a valid terminal path where the policy allows it
- avoid defaulting toward mutation when risk is high
- honor pacing, source-diversity, and pattern restrictions

The generator should not need to invent these rules ad hoc. It should act inside the policy selected earlier.

### Evaluator

The evaluator should judge both:

- task completion quality
- behavioral safety compliance

For high-risk browser automation, the evaluator should be able to fail or force retry when the run violates the safety contract even if the nominal task outcome succeeded.

Examples of future evaluator checks:

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

## Proposed implementation architecture

### Milestone 1: Add structured safety fields to normalized planning

Primary files:

- [internal/assistant/types.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/types.go)
- [internal/assistant/taskspec.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/taskspec.go)

Planned work:

- add a structured safety policy object to `TaskSpec`
- keep backward compatibility for existing runs
- define validation rules for optional policy fields

### Milestone 2: Extend planner prompt and decoding

Primary file:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)

Planned work:

- teach the planner to emit the safety profile and policy when browser automation is relevant
- keep the output minimal or empty when the safety profile is `none`
- update planner prompt tests accordingly

### Milestone 3: Carry safety policy into contract and generator prompts

Primary file:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)

Planned work:

- include policy context in contract prompt construction
- include policy context in generator prompt construction
- explicitly allow no-action success paths for matching profiles

### Milestone 4: Add behavioral safety checks to evaluator logic

Primary files:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)
- [internal/wtl/engine.go](/Users/dev/git/codex-virtual-assistant/internal/wtl/engine.go)

Planned work:

- extend evaluator instructions so policy compliance is part of the pass/fail decision
- keep those checks conditional on the selected profile

### Milestone 5: Feed policy into scheduler decisions

Primary files:

- [internal/prompting/prompts.go](/Users/dev/git/codex-virtual-assistant/internal/prompting/prompts.go)
- [internal/wtl/engine.go](/Users/dev/git/codex-virtual-assistant/internal/wtl/engine.go)

Planned work:

- use safety policy to shape deferred work
- prevent obviously repetitive short follow-up scheduling in high-risk profiles

### Milestone 6: Add reusable recent-activity safety metrics

Primary area:

- new package, likely under `internal/safety`

Planned work:

- compute recent behavioral signals from stored runs, evidence, tool calls, and scheduled runs
- make those signals available to evaluator and scheduler

Examples:

- recent mutation density
- repeated source-path concentration
- repeated action-sequence score
- text reuse risk score

This milestone can follow after the policy shape is in place.

## Project integration strategy

Project docs and playbooks should not be replaced. They should feed into the structured safety layer.

Recommended split:

- project docs define domain-specific operating rules
- CVA engine defines the reusable safety structure
- planner maps project guidance into normalized safety policy
- evaluator and scheduler enforce the normalized policy consistently

This preserves project flexibility while avoiding repeated one-off prompt rules across projects.

## Rollout strategy

Recommended rollout order:

1. Add the policy shape to `TaskSpec`
2. Add planner support
3. Add generator + contract support
4. Add evaluator policy checks
5. Add scheduler policy handling
6. Add dynamic recent-activity metrics

This keeps the first pass simple and minimizes breakage.

## Risks / open questions

- Overreach:
  If the safety layer activates too broadly, it may degrade normal browser QA or research workflows.

- Under-specification:
  If the policy object is too vague, the generator and evaluator will still behave inconsistently.

- Policy drift:
  Project playbooks and engine rules could diverge unless the planner reliably translates project policy into normalized fields.

- Data availability:
  Strong evaluator checks will need recent run metrics; some of that can be added later.

- Domain generality:
  Some rules are generic, while others are domain-specific. The engine should support both without hard-coding one site's rules into every project.

## Current progress

- Planned only.
- No code changes should be made as part of this document-only step.
