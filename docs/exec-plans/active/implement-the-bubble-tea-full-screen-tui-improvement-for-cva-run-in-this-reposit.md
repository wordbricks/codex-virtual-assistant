# Goal / scope

Implement a full-screen Bubble Tea TUI for `cva run` when both stdin/stdout are TTYs and `--json` is not set. The TUI must present a persistent phase/status header, a scrollable live activity viewport, and a chat composer/input area while reusing the existing HTTP + SSE client flow.

Out of scope:

- Replacing non-interactive or `--json` run output behavior.
- Reworking other commands (`cva watch`, `cva list`, `cva status`, `cva chat`) into full-screen mode.

## Background

The current `cva run` UX streams line-oriented output that is automation-friendly but weak for long interactive sessions. The repository already has an approved improvement direction in `docs/exec-plans/active/improve-cva-run-with-bubble-tea-fullscreen-tui.md`, and this execution plan focuses on implementation steps in the current worktree/branch.

Key constraints:

- Preserve existing machine-readable/plain streaming behavior for non-TTY usage and `--json`.
- Reuse existing run creation and SSE transport/client code paths.
- Add or update tests for mode selection and core TUI state behavior.

## Milestones

- [x] Milestone 1: Confirm `cva run` mode-selection boundaries and add explicit gating for interactive TUI vs plain/json streaming.
- [x] Milestone 2: Introduce Bubble Tea app model/layout with three persistent regions (header, scrollable activity viewport, composer).
- [ ] Milestone 3: Wire existing HTTP run creation + SSE event stream into TUI state messages without creating a separate execution engine.
- [ ] Milestone 4: Implement phase/status header behavior and viewport ingestion/scroll-follow behavior for live events.
- [ ] Milestone 5: Implement composer behaviors and state transitions (initial prompt, waiting/completed behavior) and integrate submit flow.
- [ ] Milestone 6: Add/update tests for mode selection and core TUI state transitions; verify non-TTY and `--json` behavior remains unchanged.

## Current progress

- Milestone 1 completed.
- Added explicit run output mode selection in `cmd/cva/main.go` via `selectRunOutputMode(jsonMode, stdinTTY, stdoutTTY)` with modes `json`, `tui`, and `plain`.
- `cva run` now routes through mode gating:
  - `--json` => JSON response path,
  - interactive stdin/stdout TTY => TUI path hook (`streamRunTUI`),
  - otherwise => existing plain streaming output.
- Introduced `streamRunPlain` (existing behavior) and `streamRunTUI` (now launches Bubble Tea full-screen shell).
- Added tests in `cmd/cva/run_mode_test.go` for mode selection and terminal detection guard behavior.
- Milestone 2 completed:
  - Added Bubble Tea dependencies (`bubbletea`, `bubbles`, `lipgloss`) to `go.mod`/`go.sum`.
  - Added `cmd/cva/run_tui.go` with a dedicated run TUI model:
    - persistent header panel (run/chat/status/phase/request),
    - scrollable activity viewport panel,
    - composer/input panel.
  - Implemented full-screen alternate-screen entry through `runRunTUI(ctx, run)` and wired `streamRunTUI` to it.
  - Added layout synchronization on terminal resize and basic key handling for quit (`q`, `Ctrl+C`) and viewport navigation (`PgUp`, `PgDn`, `Home`, `End`).

## Key decisions

- Keep TUI activation strict: only when stdin/stdout are TTYs and `--json` is false.
- Keep non-TTY and `--json` output path unchanged for automation compatibility.
- Use Bubble Tea as presentation/state layer only; backend flow remains existing HTTP + SSE client path.
- Prioritize clear state behavior and testability over visual complexity.
- Land a no-regression TUI hook in Milestone 1 (`streamRunTUI`) that currently delegates to plain streaming so Milestone 2 can focus on UI construction without changing mode boundaries again.
- For Milestone 2, prioritize a stable full-screen layout shell first; keep live event ingestion and run lifecycle updates for Milestone 3+ to avoid conflating UI structure with transport wiring.

## Remaining issues / open questions

- Determine whether composer should accept follow-up prompts immediately after terminal completion in v1 or remain focused on initial/wait states.
- Confirm whether any additional explicit CLI flag (`--plain` / `--no-tui`) is needed now or deferred.
- Milestone 3 needs to connect existing `CreateRun` + `StreamEvents` flow into Bubble Tea update messages so the viewport/header reflect real-time run events.
- Composer submit behavior is currently placeholder text/input only and must be wired to real submit/resume flow in Milestone 5.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `docs/exec-plans/active/improve-cva-run-with-bubble-tea-fullscreen-tui.md`
- `cmd/cva/main.go`
- `cmd/cva/format.go`
- `cmd/cva/sse.go`
