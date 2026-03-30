# Goal / scope

Implement the first usable version of the WTL GAN-policy based web personal virtual assistant described in [PRD.md](/Users/dev/git/CodexVirtualAssistant/PRD.md). The implementation should live outside `specs/` and include the core run lifecycle, planner/generator/evaluator loop, SQLite-backed persistence, HTTP/SSE API, and a usable web UI skeleton for single-user local operation. Add focused tests and documentation needed to make the product runnable and understandable.

## Background

The repository currently contains the PRD, Ralph loop guidance, and the vendored `specs/what-the-loop` reference materials, but no product implementation outside `specs/`. The PRD defines a WTL-based assistant that executes real browser-centric work, persists runs and attempts, loops generator/evaluator phases until completion or exhaustion, and exposes progress through a non-technical web UI. `specs/what-the-loop` should be treated as behavioral reference material rather than product code. `ARCHITECTURE.md` was requested in the task instructions but is not present in the repository, so the plan is based on `PRD.md`, [AGENTS.md](/Users/dev/git/CodexVirtualAssistant/AGENTS.md), [docs/PLANS.md](/Users/dev/git/CodexVirtualAssistant/docs/PLANS.md), and the WTL reference docs.

## Milestones

- [x] Milestone 1: Establish the product skeleton outside `specs/`, including the root Go module, `cmd/assistantd`, internal package layout, shared domain types for runs/tasks/attempts/evaluations/artifacts, configuration, and local app bootstrapping.
- [x] Milestone 2: Implement planning and persistence foundations: TaskSpec normalization, planner prompt/input-output contracts, SQLite schema and repository layer for runs/events/attempts/artifacts/evaluations/tool calls/wait requests, plus unit tests for stateful storage operations.
- [x] Milestone 3: Implement the WTL GAN-policy execution core: run state machine, planner/generator/evaluator phase orchestration, attempt accounting, critique-driven retry flow, wait/resume/cancel handling, observer event emission, and deterministic engine/policy tests for lifecycle invariants.
- [x] Milestone 4: Implement the execution runtime layer for real work evidence collection, starting with a Codex-oriented runtime abstraction and an `agent-browser` oriented action/evidence model that records tool activity, visited steps, artifacts, and evaluator-visible evidence without depending on `specs/` code.
- [x] Milestone 5: Build the product API surface: `POST /runs`, `GET /runs/:id`, `GET /runs/:id/events`, `POST /runs/:id/input`, `POST /runs/:id/cancel`, and `POST /runs/:id/resume`, with background run execution, SSE streaming, API tests, and run bootstrap wiring from the web UI.
- [x] Milestone 6: Build a usable web UI skeleton for non-technical users, including a new-run form, live run detail view, recent actions/evidence/artifacts panels, waiting and approval states, and concise user-facing copy that hides internal model details while exposing progress and next actions.
- [x] Milestone 7: Finish integration hardening: end-to-end smoke coverage across planning to completion/waiting flows, developer documentation for local setup and runtime behavior, and cleanup needed to make the repo coherent for the next coding-loop iterations.

## Current progress

- Milestone 1 is complete. The repository now has a root Go module, a runnable `assistantd` entrypoint, config/bootstrap wiring, the initial `internal/...` package tree, shared assistant domain types, a static web shell, and smoke tests covering config and HTTP bootstrap behavior.
- Milestone 2 is complete. `TaskSpec` defaults and normalization now produce evaluator-ready structures from sparse planner output, the planner prompt declares a strict JSON contract, and the repository layer persists runs, events, attempts, artifacts, evaluations, tool calls, and wait requests in SQLite.
- Milestone 3 is complete. The WTL engine now drives planner, generator, and evaluator attempts against persisted run state, records phase changes through the observer pipeline, retries generator work from evaluator critique, and supports waiting, resume, and cancel semantics with deterministic tests.
- Milestone 4 is complete. The store now persists first-class evidence and web-step records, the engine saves them from runtime responses, and a concrete `CodexRuntime` translates browser-oriented phase execution traces into tool calls, screenshot artifacts, web-step summaries, and evaluator-visible evidence.
- Milestone 5 is complete. The app now exposes the run lifecycle HTTP routes, replays and streams run events over SSE, starts and resumes runs in the background through an application service, and boots with a local heuristic phase executor so the product server can exercise the full route surface.
- Milestone 6 is complete. The embedded web shell now lets an operator start a task, follow a single run through a live detail view, review recent activity/evidence/artifacts, and respond to waiting states with approval or input without leaving the page.
- Milestone 7 is complete. The app now has top-level smoke coverage for the real shipped HTTP surface using the local heuristic runtime, and the README documents local setup, runtime behavior, persisted state, and verification commands.
- All planned milestones are complete for the first usable local product cut described in the PRD.

## Key decisions

- `PRD.md` is the product source of truth; `specs/what-the-loop` is reference material for WTL behavior only.
- The first implementation will target a single-user local deployment and optimize for end-to-end run correctness before breadth of SaaS integrations.
- Product code will be created entirely outside `specs/`, following the PRD’s suggested Go-oriented package layout (`cmd/assistantd`, `internal/...`, `web`).
- The initial web experience should favor a thin, dependable UI skeleton over a framework-heavy frontend, so backend and streaming lifecycle work remain the critical path.
- Persistence and observability are part of the product core, not optional add-ons; run history, attempts, evaluations, tool activity, and waiting reasons must be stored from the first implementation slice.
- The root Go module uses the repository remote path `github.com/siisee11/CodexVirtualAssistant`, keeping imports aligned with the repo that will eventually host the product code.
- The first bootstrap cut serves a narrow HTTP surface (`/`, `/healthz`, `/api/v1/bootstrap`) and only prepares the SQLite file path plus artifact directories; the actual schema and repository logic are intentionally deferred to Milestone 2.
- The current planner contract is strict-JSON-first: planner responses are decoded into a typed `PlannerOutput` and normalized into a valid `TaskSpec`, with default tools, done definitions, evidence requirements, and lightweight risk-flag detection applied centrally.
- The SQLite persistence layer is implemented around the local `sqlite3` CLI rather than a third-party Go driver so the repository remains runnable in this sandbox without fetching extra modules; schema migration happens during app bootstrap.
- The run engine is synchronous for now: `Start` and `Resume` drive execution until the run completes, waits, fails, exhausts, or is cancelled. Background execution and SSE delivery remain Milestone 5 work.
- Waiting resumes into the same role that requested external input, determined from the latest persisted attempt. Historical wait requests remain stored for audit, but `run.waiting_for` is only hydrated when the current run status is actually `waiting`.
- Evaluator outputs are treated as structured JSON and the GAN policy only consumes generator-attempt counts when deciding between retry, completion, and exhaustion. Planner attempts do not burn the generation-attempt budget.
- Runtime execution is split into two layers: `RunEngine` orchestrates persisted lifecycle state, while `CodexRuntime` adapts a narrower phase executor interface into phase responses containing artifacts, evidence, tool calls, web steps, and optional wait requests.
- Browser activity is represented explicitly through `AgentBrowserStep` and `AgentBrowserAction`; screenshot paths emitted by the runtime are converted into screenshot artifacts, and browser/tool traces also produce persisted evidence records for evaluator consumption.
- The HTTP layer is intentionally thin: `internal/assistantapp` owns background run orchestration, `internal/api` owns request parsing plus SSE serialization, and the event broker replays persisted events before streaming new observer events to subscribers.
- The default app wiring currently uses a local heuristic phase executor behind `CodexRuntime` so the server can exercise the API surface without a live Codex app server. The production-facing executor remains pluggable through the narrower `CodexPhaseExecutor` interface.
- CLI-backed SQLite access can experience transient lock contention under concurrent read/write paths, so the repository now retries `database is locked` / `database is busy` failures with a short backoff.
- The web experience remains a single static page on purpose: it uses hash-based run selection, the existing run/status endpoints, and SSE refreshes instead of adding a heavier frontend framework or expanding backend scope during the UI milestone.
- Final integration coverage is anchored at the app boundary: smoke tests now boot the real app wiring and verify both completion and wait/resume flows through the HTTP API using the default heuristic runtime, complementing the lower-level engine and API tests already in the repo.

## Remaining issues / open questions

- `ARCHITECTURE.md` is absent, so any architectural assumptions not covered by the PRD will need to be made explicitly during implementation.
- The exact shape of the Codex app server integration and how much can be stubbed versus executed locally in tests will need to be resolved when implementing the runtime layer.
- Approval policy boundaries for risky actions are called out by the PRD but not fully specified; the first cut should define a minimal safe policy that still supports waiting and resume flows.
- The UI skeleton should be usable immediately, but visual/system design details beyond the PRD remain open.
- Go commands need a writable `GOCACHE` inside the sandbox (for example `/tmp/cva-go-build`) during local verification in this environment.
- The concrete Codex app server integration is still abstracted behind the `CodexPhaseExecutor` interface; Milestone 5 can wire a local/background executor without changing the engine/runtime contracts.
- Future work beyond this plan is mostly productionization: replacing the heuristic executor with the real Codex app server path, refining approval policies, and expanding the operator UI beyond the current single-run local workflow.

## Links to related documents

- [AGENTS.md](/Users/dev/git/CodexVirtualAssistant/AGENTS.md)
- [docs/PLANS.md](/Users/dev/git/CodexVirtualAssistant/docs/PLANS.md)
- [PRD.md](/Users/dev/git/CodexVirtualAssistant/PRD.md)
- [specs/what-the-loop/README.md](/Users/dev/git/CodexVirtualAssistant/specs/what-the-loop/README.md)
- [specs/what-the-loop/SPEC.md](/Users/dev/git/CodexVirtualAssistant/specs/what-the-loop/SPEC.md)
- [specs/what-the-loop/PROBLEMS.md](/Users/dev/git/CodexVirtualAssistant/specs/what-the-loop/PROBLEMS.md)
