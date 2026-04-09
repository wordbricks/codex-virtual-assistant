# Goal / scope

Improve the `cva` CLI experience by replacing the current line-oriented `cva run` output with a full-screen Bubble Tea TUI that feels closer to the web experience.

The primary scope is `cva run` in interactive terminal sessions:

1. Render as a full-screen TUI in the alternate screen buffer.
2. Show the current run phase prominently, similar to the web phase/status treatment.
3. Clearly separate:
   - a phase/status area,
   - a scrolling log/activity area,
   - a chat input/composer area.
4. Preserve existing `cva run --json` and non-interactive automation behavior.
5. Keep the existing server and SSE protocol intact unless a small protocol addition becomes clearly necessary.

Out of scope for the first pass:

- Reworking `cva status`, `cva watch`, `cva list`, or `cva chat` into full-screen TUI commands.
- Rebuilding the full web UI feature set in the terminal.
- Introducing terminal-only domain logic that diverges from the current run/chat model.

## Background

Today `cva run` creates a run and prints a summary plus streaming formatted events line-by-line to stdout. That implementation is simple and automation-friendly, but it does not provide strong visual separation between run status, live activity, and user input. It also makes phase transitions easy to miss in long runs.

Relevant current behavior:

- [cmd/cva/main.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/main.go) creates the run, opens the SSE stream, and prints each event as plain text.
- [cmd/cva/format.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/format.go) formats individual events and run summaries for line output.
- [cmd/cva/sse.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/sse.go) parses the SSE event stream.
- [webapp/src/App.tsx](/Users/dev/git/codex-virtual-assistant/webapp/src/App.tsx) already defines a phase vocabulary, status labels, and status messages that can guide the terminal UX.

The desired CLI experience is closer to a persistent chat surface:

- phase is always visible,
- logs remain readable and scrollable,
- the input area is visually distinct and available without losing context.

## Milestones

- [ ] Milestone 1: Define the TUI interaction model and command boundaries for `cva run`.
- [ ] Milestone 2: Add a Bubble Tea-based terminal app shell with clear layout regions for phase, logs, and composer.
- [ ] Milestone 3: Adapt run creation and SSE streaming into a state-driven event model consumable by the TUI.
- [ ] Milestone 4: Implement a phase/status header that mirrors the web run lifecycle and stays visible during scrolling.
- [ ] Milestone 5: Implement a log/activity viewport with filtering, truncation, and scroll behavior suitable for long-running sessions.
- [ ] Milestone 6: Implement a chat input/composer panel that can submit the initial request and support the expected follow-up flow.
- [ ] Milestone 7: Preserve compatibility for non-TTY and `--json` usage, and add focused tests for the new interactive path.

## Current progress

- Not started

## Key decisions

- `cva run` should switch to TUI only when stdout/stdin are interactive terminals and `--json` is not set.
- Non-interactive use must keep the current plain streaming behavior so scripts and automation do not break.
- Bubble Tea should be the top-level TUI framework. `bubbles` components such as `viewport`, `textarea`, `help`, and `spinner` are likely appropriate, with `lipgloss` used only for layout and styling.
- The TUI should remain a thin presentation layer over the current HTTP + SSE client path, not a second execution engine.
- The first implementation should prioritize clarity and resilience over terminal visual flourish.
- The phase header should use the same run status/phase language as the web app to avoid product drift.
- The bottom composer must have a clearly defined submit target:
  - initial request creation before the first run exists,
  - follow-up submission after completion,
  - possible resume/wait input when a run enters `waiting`.

## Implementation plan

### Milestone 1: Define the interaction model

Decide exactly how `cva run` behaves in TUI mode.

Planned decisions and deliverables:

- Document when TUI mode activates:
  - interactive TTY + not `--json`,
  - otherwise fall back to the current stdout streaming path.
- Define the screen regions:
  - top status/phase strip,
  - center log viewport,
  - bottom composer/help strip.
- Define the keyboard model:
  - submit composer,
  - scroll logs,
  - quit,
  - optional clear/focus shortcuts.
- Define how the composer behaves across run states:
  - before run creation: submit initial prompt,
  - while run is active: likely disabled or converted to a read-only hint,
  - while waiting: optionally maps to `cva resume`,
  - after completion: optionally sends a follow-up run in the same chat.

Primary files:

- `cmd/cva/main.go`
- new TUI package under `cmd/cva` or a small internal package such as `internal/cliui`

### Milestone 2: Build the Bubble Tea shell

Introduce the full-screen app container and region layout.

Planned work:

- Add Bubble Tea dependencies to `go.mod`.
- Create a TUI model that owns:
  - terminal dimensions,
  - current run/chat metadata,
  - phase/status summary,
  - event list / rendered log lines,
  - composer state,
  - network/stream lifecycle state,
  - error state.
- Enter alternate screen and restore terminal state cleanly on exit.
- Use `lipgloss` layout helpers to create visually distinct panes without making the UI noisy.

Primary files:

- `go.mod`
- new files under `cmd/cva/` or `internal/cliui/`

### Milestone 3: Bridge existing run/SSE code into TUI state

Refactor the current `cmdRun` flow so the interactive path feeds structured messages into the Bubble Tea update loop instead of printing directly.

Planned work:

- Extract reusable run creation logic from the current `cmdRun` implementation.
- Extract reusable SSE subscription logic so events can be pushed into Bubble Tea messages.
- Define typed UI messages for:
  - run created,
  - event received,
  - waiting state entered,
  - terminal state reached,
  - stream error,
  - resize and input updates.
- Reuse existing `Client.CreateRun` and `Client.StreamEvents` instead of inventing a separate transport layer.

Primary files:

- [cmd/cva/main.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/main.go)
- [cmd/cva/client.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/client.go)
- [cmd/cva/sse.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/sse.go)

### Milestone 4: Add the persistent phase header

Make the active phase easy to read at all times.

Planned work:

- Reuse the current run phase vocabulary from `internal/assistant` and the existing web labels/messages.
- Display:
  - run id,
  - chat id or short chat context,
  - current status,
  - current phase,
  - attempt count,
  - waiting prompt summary when relevant.
- Keep the phase header pinned while the log viewport scrolls.
- Highlight transitions when new `phase_changed` events arrive.

Primary files:

- [internal/assistant/types.go](/Users/dev/git/codex-virtual-assistant/internal/assistant/types.go)
- [webapp/src/App.tsx](/Users/dev/git/codex-virtual-assistant/webapp/src/App.tsx)
- new TUI presenter files

### Milestone 5: Add the log/activity viewport

Represent SSE events in a way that is readable during long-running sessions.

Planned work:

- Convert existing `formatEvent` concepts into richer log rows for the viewport.
- Preserve event order and timestamps.
- Support scrolling without losing the latest event behavior.
- Auto-follow the bottom unless the user has manually scrolled up.
- Handle large tool outputs and reasoning summaries safely:
  - truncate in the viewport,
  - preserve the important leading context,
  - avoid breaking layout on long lines.
- Distinguish event classes visually:
  - phase changes,
  - tool start/end,
  - reasoning,
  - evaluations,
  - waiting,
  - terminal completion/failure.

Primary files:

- [cmd/cva/format.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/format.go)
- new TUI rendering files

### Milestone 6: Add the composer/chat input panel

Introduce a bottom input region that behaves like a terminal-native chat composer.

Planned work:

- Use a `textarea` or equivalent input component for multi-word prompts.
- Make submit behavior explicit in the UI:
  - `Enter` or `Cmd/Ctrl+Enter`,
  - help text in the footer.
- Decide and implement the initial conversation model:
  - minimum viable behavior: composer is used for the first prompt only, then shows wait/follow-up affordances,
  - preferred behavior: the screen remains open after completion so the user can send a follow-up in the same chat context.
- If the run enters `waiting`, map the composer to the resume path where reasonable, or clearly show why free-form input is not enough.

Primary files:

- [cmd/cva/main.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/main.go)
- new TUI input/controller files

### Milestone 7: Preserve compatibility and add tests

Make sure the TUI does not damage existing CLI behavior.

Planned work:

- Keep `cva run --json` exactly machine-readable.
- Keep plain output fallback for non-TTY sessions and redirected stdout.
- Add tests for:
  - TTY mode selection,
  - non-TTY fallback,
  - phase header state transitions,
  - log viewport event ingestion,
  - composer submission state changes,
  - waiting / completed state behavior.
- Add at least one lightweight integration-style test for the run flow state machine independent of actual terminal drawing.

Primary files:

- `cmd/cva/*_test.go`
- new TUI model tests

## Remaining issues / open questions

- Should `cva run` become a persistent chat session after the first run completes, or should the first version keep the composer mostly for the initial request and waiting/resume states?
- Should waiting-state resume support be free-form only, or does it need structured key/value prompts for parity with `cva resume <run_id> key=value`?
- Should `cva watch` eventually reuse the same TUI shell for read-only monitoring, or should that remain separate?
- How much of the web status copy should be shared directly versus duplicated in CLI-specific helpers?
- Do we want an explicit flag such as `--plain` or `--no-tui` for users who are on a TTY but prefer the old streaming output?

## Links to related documents

- [AGENTS.md](/Users/dev/git/codex-virtual-assistant/AGENTS.md)
- [docs/PLANS.md](/Users/dev/git/codex-virtual-assistant/docs/PLANS.md)
- [cmd/cva/main.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/main.go)
- [cmd/cva/format.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/format.go)
- [cmd/cva/sse.go](/Users/dev/git/codex-virtual-assistant/cmd/cva/sse.go)
- [webapp/src/App.tsx](/Users/dev/git/codex-virtual-assistant/webapp/src/App.tsx)
