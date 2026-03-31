# Goal / scope

Implement a gate-first run flow that routes each new run to either an answer path or the existing workflow path, while preserving workflow phase order after gate as `selecting_project -> planning -> contracting -> generating -> evaluating`. Add follow-up run support using `parent_run_id` so follow-up questions create a new run that can reference the parent run's artifacts, evidence, and summary, and ensure gate/answer execute via the existing Codex app-server-backed phase runtime (no local shortcut path).

## Background

The current system already has a persisted run/attempt/evidence/artifact model, phase executor integration through Codex app server, and waiting/resume semantics for in-progress runs. Existing workflow phases include `selecting_project`, `planning`, `contracting`, `generating`, and `evaluating`. This task extends the lifecycle with gate + answer routing, introduces parent-linked follow-up runs, and requires API/UI updates so completed runs are not resumed for follow-up. `ARCHITECTURE.md` is not present in this repository, so this plan is based on `AGENTS.md`, `README.md`, `PRD.md`, and `docs/PLANS.md`.

## Milestones

- [x] Milestone 1: Extend core data contracts and persistence for follow-up runs and gate/answer metadata.
- [x] Milestone 2: Add gate and answer prompt/runtime integration through the existing Codex app-server phase executor stack.
- [x] Milestone 3: Update the run engine/state machine to execute gate first, branch to answer or workflow, preserve post-gate workflow order, and keep waiting/resume behavior for in-progress runs.
- [x] Milestone 4: Add parent-run-based follow-up creation semantics in app/API layers so follow-ups always create a new run (never resume a completed run).
- [ ] Milestone 5: Update UI run creation/follow-up flows and run detail rendering for gate-routed answer runs and parent-linked context.
- [ ] Milestone 6: Add and pass tests for gate routing, answer-run lifecycle, parent_run_id follow-up creation, and waiting/resume regression coverage.

## Current progress

- Milestone 1 completed:
  - Added run-level follow-up linking metadata: `parent_run_id`.
  - Added gate routing metadata on runs: `gate_route`, `gate_reason`, `gate_decided_at`.
  - Added lifecycle contract enums for gate/answer states (`gating`, `answering`) and attempt roles (`gate`, `answer`) in shared assistant types.
  - Extended SQLite `runs` schema + repository read/write plumbing to persist and hydrate the new fields.
  - Added backward-compatible run-table migration logic that adds missing columns on existing databases.
  - Added store/assistant tests covering validation, round-trip persistence, and legacy schema migration.
- Milestone 2 completed:
  - Added gate prompt contract and decoder (`route`, `reason`, `summary`) in `internal/prompting`.
  - Added answer prompt contract and decoder (`summary`, `output`, wait fields) in `internal/prompting`.
  - Added explicit parent-run context prompt payload shape (`run_id`, request, summary, artifact/evidence highlights with caps) for gate/answer prompting.
  - Extended Codex app-server phase schema wiring for new roles (`gate`, `answer`) in `phaseOutputSchema`.
  - Extended app-server result parsing so answer-phase structured outputs produce persisted report artifacts and wait requests through the same runtime path as other phases.
  - Added runtime tool-hint routing for gate/answer to read-oriented stored-context tool sets.
  - Added role-to-phase mapping for gate/answer (`gating`, `answering`) and test coverage for prompts/runtime/schema mappings.
- Milestone 3 completed:
  - Run engine now starts every run at `gate` instead of `selecting_project`.
  - Added gate phase execution in engine (`executeGate`) using runtime attempts and strict gate-output decoding.
  - Added answer phase execution in engine (`executeAnswer`) using runtime attempts, read-oriented prompting, wait propagation, and terminal completion.
  - Added parent-run context hydration in engine for gate/answer (`parent_run_id` -> parent run record summary/artifacts/evidence).
  - Added gate-route persistence during execution (`gate_route`, `gate_reason`, `gate_decided_at`) and branching:
    - `answer` -> `answering` -> complete/wait
    - `workflow` -> preserved order `selecting_project -> planning -> contracting -> generating -> evaluating`
  - Kept waiting/resume semantics by resuming from the last attempt role; default resume role updated to `gate` when no attempts exist.
  - Updated engine + API sequence tests to include gate-first execution, added direct answer-route engine coverage, and adjusted polling deadlines for added phase latency.
- Milestone 4 completed:
  - Added optional `parent_run_id` to `POST /api/v1/runs` request contract.
  - Extended app-layer run creation to accept `parent_run_id`, validate that the parent run exists, and persist parent linkage on the new run.
  - Preserved run creation semantics as asynchronous engine start with normal visibility wait.
  - Tightened resume/input semantics in app layer so only `waiting` runs can be resumed; non-waiting runs now return a follow-up guidance error recommending `parent_run_id`.
  - Updated API error mapping:
    - missing parent run on create -> `404`
    - resume/input on non-waiting run -> `409`
  - Added API tests for:
    - creating a follow-up run linked to a completed parent (`parent_run_id` round-trip),
    - rejecting follow-up creation when parent does not exist,
    - rejecting `/input` on completed runs with explicit follow-up guidance.

## Key decisions

- Gate and answer phases must run via the same Codex app-server-backed phase executor/runtime used by existing workflow phases.
- Follow-up questions for completed runs will always create a new run linked by `parent_run_id`.
- Answer runs are read-oriented and may consume parent run context (artifacts/evidence/summary) without entering the full generation/evaluation workflow unless gate routes there.
- Gate routing will be stored directly on `runs` (`gate_route`, `gate_reason`, `gate_decided_at`) for auditability and API/UI visibility without reconstructing from events.
- `parent_run_id` remains nullable and migration-safe, with schema evolution handled in-repo via additive `ALTER TABLE` checks.
- Gate output contract is fixed to strict JSON `{route, reason, summary}` with `route ∈ {"answer","workflow"}`.
- Answer output contract is fixed to strict JSON `{summary, output, needs_user_input, wait_kind, wait_title, wait_prompt, wait_risk_summary}` to preserve wait/resume compatibility.
- Parent context prompt payload now includes a bounded summary block: parent run id/request/summary plus most-recent artifacts (max 5) and evidence highlights (max 8).
- Engine-level branching decision is source-of-truth from gate attempt output; no local heuristic shortcut exists in engine path.
- Answer runs are terminal after a successful answer phase (unless answer requests wait), and do not enter planning/contracting/generating/evaluating.
- Follow-up creation API shape is now `POST /api/v1/runs` with optional `parent_run_id`; follow-ups are new runs, not resumes.
- Resume/input endpoints remain dedicated to `waiting` runs only and intentionally reject completed runs.

## Remaining issues / open questions

- Confirm UI wording for when the operator sends a follow-up from a completed run versus resuming a waiting run.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `README.md`
- `PRD.md`
- `docs/exec-plans/completed/implement-the-product-described-in-users-dev-git-codexvirtualassistant-prd-md-tr.md`
