# Goal / scope

Implement a gate-first run flow that routes each new run to either an answer path or the existing workflow path, while preserving workflow phase order after gate as `selecting_project -> planning -> contracting -> generating -> evaluating`. Add follow-up run support using `parent_run_id` so follow-up questions create a new run that can reference the parent run's artifacts, evidence, and summary, and ensure gate/answer execute via the existing Codex app-server-backed phase runtime (no local shortcut path).

## Background

The current system already has a persisted run/attempt/evidence/artifact model, phase executor integration through Codex app server, and waiting/resume semantics for in-progress runs. Existing workflow phases include `selecting_project`, `planning`, `contracting`, `generating`, and `evaluating`. This task extends the lifecycle with gate + answer routing, introduces parent-linked follow-up runs, and requires API/UI updates so completed runs are not resumed for follow-up. `ARCHITECTURE.md` is not present in this repository, so this plan is based on `AGENTS.md`, `README.md`, `PRD.md`, and `docs/PLANS.md`.

## Milestones

- [x] Milestone 1: Extend core data contracts and persistence for follow-up runs and gate/answer metadata.
- [ ] Milestone 2: Add gate and answer prompt/runtime integration through the existing Codex app-server phase executor stack.
- [ ] Milestone 3: Update the run engine/state machine to execute gate first, branch to answer or workflow, preserve post-gate workflow order, and keep waiting/resume behavior for in-progress runs.
- [ ] Milestone 4: Add parent-run-based follow-up creation semantics in app/API layers so follow-ups always create a new run (never resume a completed run).
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

## Key decisions

- Gate and answer phases must run via the same Codex app-server-backed phase executor/runtime used by existing workflow phases.
- Follow-up questions for completed runs will always create a new run linked by `parent_run_id`.
- Answer runs are read-oriented and may consume parent run context (artifacts/evidence/summary) without entering the full generation/evaluation workflow unless gate routes there.
- Gate routing will be stored directly on `runs` (`gate_route`, `gate_reason`, `gate_decided_at`) for auditability and API/UI visibility without reconstructing from events.
- `parent_run_id` remains nullable and migration-safe, with schema evolution handled in-repo via additive `ALTER TABLE` checks.

## Remaining issues / open questions

- Define the precise gate output contract that determines `answer` vs `workflow` routing and captures rationale for auditability.
- Define the exact parent-run context payload shape/limits passed into gate and answer prompts.
- Confirm UI wording and API request shape for creating follow-up runs from a completed run versus resuming waiting runs.

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `README.md`
- `PRD.md`
- `docs/exec-plans/completed/implement-the-product-described-in-users-dev-git-codexvirtualassistant-prd-md-tr.md`
