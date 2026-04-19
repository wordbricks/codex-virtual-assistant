# Automation Safety Policy

This document describes the conditional automation safety layer used by CVA for browser runs.

## Scope

The policy is designed to reduce repetitive and high-risk browser mutation patterns.

In scope:

- Structured automation policy in `TaskSpec`
- Config-file defaults and project overrides
- Planner/contract/generator/evaluator/scheduler integration
- Browser action logging and recent-activity metrics
- Deterministic hard-limit enforcement for high-risk engagement profiles

Out of scope:

- Account or credential rotation
- Site-specific stealth/evasion logic
- Claims of undetectable automation

## TaskSpec Policy Model

Policy is attached at `TaskSpec.AutomationSafety`.

Profiles:

- `none`
- `browser_read_only`
- `browser_mutating`
- `browser_high_risk_engagement`

Enforcement modes:

- `advisory`
- `evaluator_enforced`
- `engine_blocking`

Primary fields:

- `mode_policy`
- `rate_limits`
- `pattern_rules`
- `text_reuse_policy`
- `cooldown_policy`

High-risk defaults are normalized to:

- `max_account_changing_actions_per_run=2`
- `max_replies_per_24h=12`
- `min_spacing_minutes=20`
- `allow_no_action_success=true`
- `require_no_action_evidence=true`

## Config Overrides

Config is read from the normal CVA JSON config path using `automation_safety`.

Merge precedence:

1. Engine defaults
2. Global profile defaults (`automation_safety.defaults`)
3. Project override (`automation_safety.projects.<slug>`)
4. Planner-provided policy values

Example:

```json
{
  "automation_safety": {
    "defaults": {
      "browser_mutating": {
        "enforcement": "evaluator_enforced"
      },
      "browser_high_risk_engagement": {
        "enforcement": "engine_blocking",
        "rate_limits": {
          "max_account_changing_actions_per_run": 2,
          "max_replies_per_24h": 12,
          "min_spacing_minutes": 20
        }
      }
    },
    "projects": {
      "community-outreach": {
        "profile_override": "browser_high_risk_engagement",
        "rate_limits": {
          "max_account_changing_actions_per_run": 1
        }
      }
    }
  }
}
```

## Prompt Integration

The policy is threaded into:

- Planner output contract (`automation_safety` field)
- Contract prompt context
- Generator prompt context
- Evaluator prompt context
- Scheduler prompt context

## Browser Action Metrics

Browser action records are persisted in `browser_actions` with run/attempt/project correlation.

Recorded attributes include:

- Action type and action name
- Target/source context
- Source URL
- `account_state_changed`
- Text fingerprint (where applicable)
- Occurred-at timestamp

Recent-activity metrics include:

- Mutation density
- Reply count
- Source path concentration
- Repeated action-sequence score
- Text reuse risk score

## Evaluator and Scheduler Enforcement

`browser_mutating`:

- Violations are evaluator-soft
- Evaluations are marked failed with actionable retry guidance
- No hard engine block is applied

`browser_high_risk_engagement` with `engine_blocking`:

- Deterministic violations hard-fail the run
- Scheduler blocks unsafe follow-up creation before persisting scheduled runs

Deterministic checks include:

- Per-run account-changing action cap
- Rolling 24-hour reply cap
- Minimum spacing between mutating actions
- Disallowed default mutating action-trio pattern
- Required no-action evidence when no-action success is claimed
- Disallowed fixed short follow-up loops in scheduler output

## Key Code Paths

- `internal/assistant/types.go`
- `internal/assistant/taskspec.go`
- `internal/config/config.go`
- `internal/prompting/prompts.go`
- `internal/safety/metrics.go`
- `internal/wtl/engine.go`
- `internal/store/repository.go`
- `internal/store/schema.go`
