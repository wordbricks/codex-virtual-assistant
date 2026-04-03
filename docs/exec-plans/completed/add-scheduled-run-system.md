# Goal / scope

Add a scheduling system to CVA so that user requests can be decomposed into immediate tasks and future tasks, where future tasks are registered as scheduled runs and automatically triggered at their designated times.

Today CVA executes all work immediately upon receiving a request. A real virtual assistant needs the ability to plan work across time — for example, "find 5 hospitals that treat disease A, then contact one every 30 minutes starting at 1pm." The research runs immediately, and the five phone calls are each scheduled as separate runs triggered at 13:00, 13:30, 14:00, 14:30, and 15:00.

The implementation adds:

1. A `ScheduledRun` domain entity persisted in SQLite for deferred work items.
2. A background `Scheduler` service that polls for due scheduled runs and triggers them via the existing `CreateRun()` path.
3. A new `scheduling` WTL engine phase where the planner's schedule plan is materialized into concrete `ScheduledRun` records using generator results.
4. HTTP API endpoints and CLI commands for managing scheduled runs.
5. Hook-driven agent-message notifications for schedule lifecycle events.

## Background

The current system processes user requests through a WTL (What The Loop) state machine: gate → (answer | workflow) → reporter → completed. Runs are created via `RunService.CreateRun()` which immediately starts execution in a background goroutine. There is no concept of deferred execution — every run begins the moment it is created.

The planner phase produces a `TaskSpec` containing goal, deliverables, constraints, and acceptance criteria. This plan currently assumes all work happens in a single uninterrupted pass. To support scheduling, the planner output must be extended with an optional `SchedulePlan` that describes which sub-tasks should execute at future times.

The scheduler service operates independently from the WTL engine. It polls the database for pending scheduled runs whose trigger time has arrived and creates standard runs through the existing `CreateRun()` entry point. This keeps the execution path identical for scheduled and immediate runs.

This plan is based on `AGENTS.md`, `README.md`, `PRD.md`, `docs/PLANS.md`, and the current `internal/assistant`, `internal/api`, `internal/app`, `internal/prompting`, `internal/wtl`, and `internal/store` code paths.

## Milestones

- [ ] Milestone 1: Add `ScheduledRun` domain type and database schema.
- [ ] Milestone 2: Add background `Scheduler` service that polls and triggers due runs.
- [ ] Milestone 3: Extend planner output with `SchedulePlan` and add the `scheduling` WTL engine phase.
- [ ] Milestone 4: Add HTTP API endpoints for scheduled run management.
- [ ] Milestone 5: Add CLI commands for scheduled run management.
- [ ] Milestone 6: Add hook-driven agent-message notifications for schedule lifecycle events.
- [ ] Milestone 7: Add tests for scheduling system.

## Current progress

- Not started.

## Key decisions

- Scheduled runs are independent `Run` entities created through the same `CreateRun()` path as user-initiated runs. They inherit the `ChatID` and use `ParentRunID` of the orchestrating run.
- The `Scheduler` service uses a polling model (default 30-second interval) rather than a timer-per-run model. Polling is simpler, crash-recoverable, and sufficient for the expected volume.
- The planner phase is responsible for deciding whether scheduling is needed. If the `TaskSpec.SchedulePlan` field is nil, the scheduling phase is a no-op pass-through.
- The scheduling phase runs a Codex call to finalize each entry's prompt using generator results (e.g., actual hospital names and phone numbers from research). The planner only produces template-level prompts; the scheduler phase concretizes them.
- Time parsing supports both absolute ISO 8601 timestamps and relative expressions like "13:00" (interpreted as today or next occurrence) and "+30m" (relative to the scheduling phase execution time).
- Scheduled runs can be cancelled individually before their trigger time via API or CLI.

## Implementation plan

### Milestone 1: ScheduledRun domain type and database schema

Add the domain type and persistence layer for scheduled runs.

Planned changes:

- Add `ScheduledRunStatus` enum: `pending`, `triggered`, `cancelled`, `failed`.
- Add `ScheduledRun` struct with fields: `ID`, `ChatID`, `ParentRunID`, `UserRequestRaw`, `MaxGenerationAttempts`, `ScheduledFor` (time.Time), `Status`, `RunID` (populated after trigger), `ErrorMessage`, `CreatedAt`, `TriggeredAt`.
- Add `ScheduleEntry` struct: `ScheduledFor` (string, raw time expression), `Prompt` (string).
- Add `SchedulePlan` struct: `Entries []ScheduleEntry`.
- Add `SchedulePlan *SchedulePlan` optional field to `TaskSpec`.
- Add `scheduled_runs` table to the SQLite schema with appropriate indexes on `(status, scheduled_for)`.
- Add repository methods: `SaveScheduledRun`, `ListPendingScheduledRuns(before time.Time)`, `ListScheduledRunsByParent(parentRunID)`, `ListScheduledRunsByChat(chatID)`, `UpdateScheduledRunTriggered(id, runID)`, `UpdateScheduledRunStatus(id, status, errorMsg)`.

Primary files:

- `internal/assistant/types.go`
- `internal/store/schema.go`
- `internal/store/repository.go`

### Milestone 2: Background Scheduler service

Add a standalone service that runs as a background goroutine and triggers due scheduled runs.

Planned design:

```
Scheduler {
    repo     SchedulerRepository   // subset of Repository for scheduled runs
    runs     RunCreator            // delegates to RunService.CreateRun
    interval time.Duration         // polling interval, default 30s
    now      func() time.Time      // injectable clock for testing
}
```

Behavior:

- `Scheduler.Run(ctx)` starts a ticker loop.
- Each tick calls `ListPendingScheduledRuns(ctx, now())`.
- For each due run:
  - Calls `CreateRun(ctx, sr.UserRequestRaw, sr.MaxGenerationAttempts, sr.ParentRunID)`.
  - On success: updates status to `triggered`, sets `run_id` and `triggered_at`.
  - On failure: updates status to `failed`, sets `error_message`.
- The created run executes through the standard WTL engine — no special handling needed.

Integration with App bootstrap:

- `App` struct gains a `scheduler` field.
- `App.Run()` launches `go a.scheduler.Run(ctx)` alongside the HTTP server.
- The scheduler shares the same `context.Context` so it shuts down cleanly with the app.

Primary files:

- `internal/scheduler/scheduler.go` (new package)
- `internal/app/app.go`

### Milestone 3: Scheduling WTL engine phase

Extend the planner to recognize scheduling needs and add a new engine phase that materializes scheduled runs from generator results.

#### 3a. Planner prompt extension

Update `BuildPlannerPrompt()` to instruct the model:

- "If the user request contains time-distributed sub-tasks, populate `schedule_plan.entries` with each sub-task's execution time and a template prompt."
- "Immediate work goes into the standard TaskSpec fields. Future work goes exclusively into `schedule_plan`."
- "Leave `schedule_plan` null if all work should execute immediately."

Update `DecodePlannerOutput()` to parse the optional `schedule_plan` field from the JSON output.

#### 3b. New engine phase and lifecycle constants

Add to `internal/assistant/types.go`:

- `RunStatusScheduling RunStatus = "scheduling"`
- `RunPhaseScheduling RunPhase = "scheduling"`
- `AttemptRoleScheduler AttemptRole = "scheduler"`

Update `AllRunStatuses()`, `AllRunPhases()` and any validation helpers.

#### 3c. Engine flow modification

Change the `continueRun()` state machine in `internal/wtl/engine.go`:

Current workflow route:
```
planner → contractor → generator → evaluator → reporter → completed
```

New workflow route:
```
planner → contractor → generator → evaluator → scheduler → reporter → completed
```

Current answer route:
```
answer → reporter → completed
```

New answer route:
```
answer → scheduler → reporter → completed
```

Transition rules:

- When evaluator returns `DirectiveComplete`, next role becomes `AttemptRoleScheduler` (instead of `AttemptRoleReporter`).
- When answer phase completes, next role becomes `AttemptRoleScheduler` (instead of `AttemptRoleReporter`).
- Scheduler phase always advances to `AttemptRoleReporter`.

#### 3d. Scheduler phase executor

Add `executeScheduler()` to `internal/wtl/engine.go`:

1. Check `record.Run.TaskSpec.SchedulePlan` — if nil, skip directly to reporter (no-op).
2. If present:
   a. Build a scheduler prompt containing: original request, schedule plan entries, generator artifacts/evidence summary.
   b. Call Codex via the standard `executeAttempt()` path.
   c. Codex returns finalized `[{scheduled_for, prompt}]` array with concrete details from research results.
   d. Parse each entry's time expression into `time.Time` (UTC).
   e. For each entry, call `repo.SaveScheduledRun()` with the run's `ChatID`, `ParentRunID = run.ID`.
   f. Publish `EventTypeScheduleCreated` event.
3. Advance to reporter phase.

Add `BuildSchedulerPrompt()` to `internal/prompting/prompts.go`:

- Input: original request, SchedulePlan, latest generator summary, key artifacts and evidence.
- Instruction: "Finalize each schedule entry's prompt by incorporating specific information discovered during execution. Replace template placeholders with actual names, numbers, and details."
- Output schema: `{"entries": [{"scheduled_for": "2026-04-03T13:00:00Z", "prompt": "...concrete prompt..."}]}`

Add scheduler role mapping in `internal/wtl/runtime_codex.go`.

Primary files:

- `internal/assistant/types.go`
- `internal/wtl/engine.go`
- `internal/prompting/prompts.go`
- `internal/wtl/runtime_codex.go`
- `internal/wtl/executor_codex_appserver.go`
- `internal/wtl/executor_local.go`

### Milestone 4: HTTP API endpoints

Add endpoints for managing scheduled runs.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/scheduled` | List all scheduled runs. Supports `?chat_id=X` and `?status=pending` filters. |
| GET | `/api/v1/scheduled/{id}` | Get a single scheduled run by ID. |
| POST | `/api/v1/scheduled/{id}/cancel` | Cancel a pending scheduled run. Returns 409 if already triggered. |
| GET | `/api/v1/runs/{id}/scheduled` | List scheduled runs created by a specific run. |

Extend `store.RunRecord` to include `ScheduledRuns []assistant.ScheduledRun` so that when fetching a run record, its child scheduled runs are also returned.

Add new SSE event types:

- `EventTypeScheduleCreated` — emitted when scheduling phase creates scheduled runs.
- `EventTypeScheduleTriggered` — emitted by the scheduler service when a scheduled run fires.

Primary files:

- `internal/api/handler.go`
- `internal/api/events.go`
- `internal/store/repository.go` (RunRecord extension)
- `internal/assistant/types.go` (event type constants)

### Milestone 5: CLI commands

Add `schedule` subcommand group to the CLI client.

```
cva schedule list [--chat <chat_id>] [--status <status>]    # List scheduled runs
cva schedule show <id>                                       # Show scheduled run details
cva schedule cancel <id>                                     # Cancel a pending scheduled run
```

Also extend `cva status <run_id>` output to show associated scheduled runs if any exist.

Primary files:

- `cmd/cva/main.go`
- `cmd/cva/client.go`
- `cmd/cva/format.go` (output formatting for scheduled runs)

### Milestone 6: Hook-driven notifications

Add agent-message lifecycle notifications for scheduling events.

New hook names:

- `HookOnScheduleCreated` — fires when the scheduling phase creates scheduled runs. Card shows: number of items scheduled, time range, brief description of each.
- `HookOnScheduleTriggered` — fires when the scheduler triggers a run. Card shows: which scheduled item fired, the created run ID, remaining scheduled items count.
- `HookOnScheduleFailed` — fires when the scheduler fails to trigger a run. Card shows: error details, the failed scheduled run info.

Add card rendering functions to `internal/agentmessage/render.go`:

- `ScheduleCreatedCard(run, scheduledRuns []ScheduledRun)`
- `ScheduleTriggeredCard(scheduledRun, createdRun)`
- `ScheduleFailedCard(scheduledRun, errorMsg)`

Register hooks in `registerAgentMessageHooks()` in `internal/app/app.go`.

Primary files:

- `internal/api/events.go`
- `internal/app/app.go`
- `internal/agentmessage/render.go`

### Milestone 7: Tests

Required test coverage:

- **Domain types**: ScheduledRun validation, SchedulePlan serialization/deserialization, time expression parsing.
- **Repository**: SaveScheduledRun, ListPendingScheduledRuns (verify time filtering), UpdateScheduledRunTriggered, UpdateScheduledRunStatus.
- **Scheduler service**: Mock repository and RunCreator to verify:
  - Due runs are triggered.
  - Already-triggered runs are not re-triggered.
  - Failed triggers update status correctly.
  - Cancelled runs are skipped.
- **Engine flow**:
  - Workflow with SchedulePlan: evaluator pass → scheduling phase → reporter → completed.
  - Workflow without SchedulePlan: evaluator pass → scheduling phase (no-op) → reporter → completed.
  - Answer with SchedulePlan: answer → scheduling → reporter → completed.
  - Scheduling phase creates correct ScheduledRun records.
- **API**: Schedule listing, cancellation, status filtering.
- **Hook notifications**: Verify cards are rendered and sent for created/triggered/failed events.

Primary files:

- `internal/assistant/types_test.go`
- `internal/store/repository_test.go`
- `internal/scheduler/scheduler_test.go` (new)
- `internal/wtl/engine_test.go`
- `internal/api/handler_test.go`

## Remaining issues / open questions

- The planner's ability to produce a good `SchedulePlan` depends on the model correctly interpreting time-based instructions in natural language. The planner prompt needs careful design and testing with various Korean and English time expressions (relative, absolute, recurring).
- The current 30-second polling interval means scheduled runs may trigger up to 30 seconds late. This is acceptable for most use cases but could be made configurable via `ASSISTANT_SCHEDULER_INTERVAL` if needed.
- If the CVA server restarts, the scheduler service will resume polling and trigger any overdue pending runs. This is the desired crash-recovery behavior but means a run scheduled for 13:00 that was missed due to downtime will trigger immediately on restart rather than being skipped.
- The scheduling phase calls Codex to finalize prompts, which consumes an additional LLM call per run. For requests with many scheduled items, this cost should be weighed against the benefit of concrete, context-aware prompts.
- Recurring schedules (e.g., "every day at 9am") are out of scope for this implementation. Only one-shot scheduled runs are supported.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `README.md`
- `PRD.md`
- `docs/exec-plans/active/add-agent-message-report-phase-and-hook-notifications.md`
