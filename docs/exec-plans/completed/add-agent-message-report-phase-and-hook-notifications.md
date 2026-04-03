# Goal / scope

Add `agent-message`-based user communication to CVA in two layers at once:

1. Add a new Codex-backed `report` phase that runs immediately before completion and sends the substantive final user-facing result through `agent-message` using `json_render`.
2. Add hook-driven lifecycle notifications that also use `agent-message` to notify the user when major run state changes occur.

The implementation should assume the local `agent-message` CLI and server configuration are already fully set up. CVA should create and use one dedicated `agent-message` account per chat, derived from the chat id, and use that account when sending messages for that chat.

## Background

The current system persists runs, attempts, artifacts, evidence, evaluations, and wait requests, and executes phases through the Codex app-server-backed runtime. Today, successful answer runs and workflow runs terminate directly at `completeRun()` after answer or evaluator success. There is no dedicated reporting phase, and there is no outbound messaging channel from CVA to the user.

The new design intentionally adds both:

- a product-owned notification layer for lifecycle visibility, and
- a model-authored report layer for richer final delivery.

Those two layers serve different purposes. Hook notifications provide concise state-change awareness. The report phase provides the actual user-facing result package, authored by Codex after inspecting the live `agent-message` component catalog via `agent-message catalog prompt`.

This plan is based on `AGENTS.md`, `README.md`, `PRD.md`, `docs/PLANS.md`, and the current `internal/assistant`, `internal/api`, `internal/app`, `internal/prompting`, and `internal/wtl` code paths.

## Milestones

- [x] Milestone 1: Extend shared lifecycle contracts to add a `report` phase and corresponding runtime role.
- [x] Milestone 2: Add chat-scoped `agent-message` account management and a small CLI adapter used by both hook notifications and pre-report setup.
- [x] Milestone 3: Add hook-driven `agent-message` lifecycle notifications for run started, waiting, completed, exhausted, and failed transitions.
- [x] Milestone 4: Add a Codex-backed `report` phase that reads `agent-message catalog prompt`, composes a valid `json_render` payload, and sends it before run completion.
- [x] Milestone 5: Update engine sequencing so answer and workflow success paths pass through `report` before `completed`, while preserving wait/retry/fail behavior.
- [x] Milestone 6: Add tests for lifecycle sequencing, chat-account provisioning, hook notifications, and report-phase delivery behavior.

## Current progress

- Completed and shipped on `main`.

## Key decisions

- `agent-message` environment configuration is treated as preconfigured infrastructure. This change does not introduce new CVA env vars for CLI path, username, PIN, recipient, or server URL.
- CVA will use one `agent-message` account per chat rather than one global agent identity.
- The chat-scoped account naming rule is conceptually `cva-<chatId>`, but implementation must still satisfy the CLI username constraints. If the persisted `chat_id` format remains longer than the CLI allows, the actual registered username must be a deterministic shortened derivative of `chatId` while preserving one-to-one mapping.
- Hook notifications and report delivery both remain enabled:
  - hook notifications send short lifecycle cards,
  - the report phase sends the substantive result card.
- The report phase runs through the existing Codex app-server runtime, not through a local helper shortcut.
- `agent-message catalog prompt` is the source of truth for what `json_render` payloads the report phase is allowed to emit.
- Completion happens only after a successful report phase on successful answer/workflow paths.

## Implementation plan

### Milestone 1: Add report lifecycle contracts

Update shared types in `internal/assistant` so the lifecycle explicitly models reporting.

Planned changes:

- Add `RunStatusReporting`.
- Add `RunPhaseReporting`.
- Add `AttemptRoleReporter`.
- Update helpers such as `AllRunStatuses()`, `AllRunPhases()`, validation, and any role/phase rendering logic.
- Update runtime mappings that currently assume evaluator is the last non-terminal phase.

Primary files:

- `internal/assistant/types.go`
- `internal/wtl/runtime_codex.go`
- `internal/wtl/executor_codex_appserver.go`
- `internal/wtl/executor_local.go`

### Milestone 2: Add chat-scoped agent-message account support

Introduce a small `agent-message` adapter owned by CVA. Its responsibilities are operational, not generative.

Planned responsibilities:

- derive the deterministic chat account name from `chatId`,
- ensure the account exists,
- login or switch to that chat account before any send,
- expose simple operations such as:
  - `EnsureChatAccount(chatID string)`,
  - `CatalogPrompt(chatID string)`,
  - `SendJSONRender(chatID string, payload string)`.

The adapter should invoke the CLI with explicit argv arrays rather than shell-string interpolation.

Because the user requested no new config surface, the adapter should assume:

- `agent-message` is already on `PATH`,
- server connectivity is already configured,
- authentication material or profile behavior needed for account creation is already available in the environment or default CLI config.

Primary files:

- `internal/agentmessage/client.go` or equivalent new package
- `internal/app/app.go`
- tests near the new package and app bootstrap paths

### Milestone 3: Add hook-driven lifecycle notifications

Use the existing hook system in `internal/api/events.go` so CVA emits concise lifecycle messages through `agent-message`.

Hook coverage:

- `HookOnRunStarted`
- `HookOnWaitEntered`
- `HookOnRunCompleted`
- `HookOnRunExhausted`
- failure path via phase-changed snapshot logic or a new explicit hook if needed

Message behavior:

- Each hook ensures the chat account exists for `record.Run.ChatID`.
- Each hook sends a small `json_render` card through the chat-scoped account.
- Cards should stay short and status-oriented:
  - started: request accepted, run id, current phase
  - waiting: what input/approval is needed
  - completed: run finished; substantive result has already been or will be delivered by report
  - exhausted/failed: run could not complete; include next suggested action

To avoid confusing duplication:

- hook notifications must not attempt to restate the entire final report,
- the completed hook should be framed as state confirmation, not as the full deliverable.

Primary files:

- `internal/api/events.go`
- `internal/app/app.go`
- new `internal/agentmessage/render.go` helper for hook card specs

### Milestone 4: Add Codex-backed report phase

Add a new prompting contract and engine phase whose sole job is to deliver the final user-facing report through `agent-message`.

Planned report flow:

1. Ensure the chat-scoped `agent-message` account is active.
2. Run the report phase through Codex app server.
3. In that phase, Codex must call `agent-message catalog prompt`.
4. Codex uses that catalog output to construct a valid `json_render` message.
5. Codex sends the message with `agent-message send '<json>' --kind json_render`.
6. Codex returns a strict structured result describing whether delivery succeeded.

Prompt contract requirements:

- include original request,
- include accepted contract context where applicable,
- include latest evaluation summary,
- include most relevant artifacts and evidence,
- include the chat id and derived chat account username,
- instruct Codex to send exactly one final report message unless it must wait or fails safely.

Suggested report phase output schema:

- `summary`
- `delivery_status`
- `message_preview`
- `needs_user_input`
- `wait_kind`
- `wait_title`
- `wait_prompt`
- `wait_risk_summary`

Persistence expectations:

- store the final report payload as an artifact,
- store catalog prompt output and send result as evidence/tool calls,
- preserve wait behavior if reporting cannot continue safely.

Primary files:

- `internal/prompting/prompts.go`
- `internal/wtl/engine.go`
- `internal/wtl/runtime_codex.go`
- `internal/wtl/executor_codex_appserver.go`
- `internal/wtl/executor_local.go`

### Milestone 5: Update engine sequencing

Change the state machine so successful runs route through `report`.

Planned target flows:

- answer route:
  - `queued -> gating -> answering -> reporting -> completed`
- workflow route:
  - `queued -> gating -> selecting_project -> planning -> contracting -> generating -> evaluating -> reporting -> completed`

Behavioral rules:

- successful answer phase no longer calls `completeRun()` directly,
- evaluator pass no longer calls `completeRun()` directly,
- both instead transition to `report`,
- if report returns a wait request, the run enters `waiting`,
- if report fails, the run should fail rather than silently complete,
- retry semantics remain owned by evaluator only; report is a terminal delivery step, not a new critique loop.

Primary files:

- `internal/wtl/engine.go`
- `internal/wtl/contracts.go`

### Milestone 6: Tests and regression coverage

Add focused coverage for both communication paths and the new phase.

Required test areas:

- shared enum/validation updates for `reporting` and `reporter`,
- engine flow:
  - answer success goes to `report` before `completed`,
  - evaluator success goes to `report` before `completed`,
  - report wait transitions to `waiting`,
  - report failure prevents terminal completion,
- heuristic/local executor support for report role,
- hook notification emission:
  - started, waiting, completed, exhausted, failed,
- chat-account provisioning:
  - deterministic mapping from chat id to CLI username,
  - repeated sends reuse the same chat account,
- report-phase persistence:
  - final report payload artifact,
  - evidence/tool-call capture from catalog and send actions.

Primary files:

- `internal/wtl/engine_test.go`
- `internal/wtl/runtime_codex_test.go`
- `internal/app/app_test.go`
- new tests for `internal/agentmessage`

## Remaining issues / open questions

- None for this change set. The implementation uses a deterministic shortened username derived from `chatId`, treats hook delivery as best-effort, and keeps report delivery required for successful completion.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `README.md`
- `PRD.md`
- `docs/exec-plans/completed/implement-gate-and-answer-phases-in-codex-virtual-assistant-while-preserving-the.md`
