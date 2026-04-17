# Complete Every Remaining Redesign Implementation Item

## Goal / scope

Complete all remaining implementation work defined in [docs/exec-plans/active/redesign-cva-web-project-first.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/exec-plans/active/redesign-cva-web-project-first.md), including backend and frontend phases.

In scope:

- Backend project-scoped run queries and project aggregate APIs.
- Flat project wiki pages API and project-scoped run creation hard-bound by `project_slug`.
- Frontend migration to TanStack Router + TanStack Query.
- Full project-first UX surfaces (projects home, project overview, wiki reader, runs board, run detail drawer).
- Removal of legacy chat UI, `/chats/:chatId` route, and report overlay.
- Verification with relevant tests/build checks.

Out of scope remains unchanged from the active redesign plan.

## Background

The active redesign plan already records product direction, API targets, and frontend IA. This branch contains updated decisions that must be treated as fixed implementation constraints: TanStack Router, TanStack Query, hard-bound `project_slug` run creation, flat wiki pages API, Linear-style side drawer for run detail, and removal of report overlay plus legacy chat route/UI.

`ARCHITECTURE.md` is not present in this worktree, so implementation context is taken from `README.md`, `docs/PLANS.md`, and the active redesign execution plan.

## Milestones

- [x] Milestone 1: Finish backend project-scoped run data access and filtering (`ListRunsByProjectSlug` path, service wiring, pagination/status behavior, and store/service tests).
- [x] Milestone 2: Finish backend project APIs for project-first pages (`GET /api/v1/projects/:slug`, `GET /api/v1/projects/:slug/runs`) with handler/service coverage.
- [ ] Milestone 3: Finish backend flat wiki pages API and hard-bound run creation (`GET /api/v1/projects/:slug/wiki/pages`, `POST /api/v1/runs` with explicit `project_slug` override and selector bypass) with tests.
- [ ] Milestone 4: Replace frontend app shell with TanStack Router + TanStack Query and split API client/types out of legacy `App.tsx`.
- [ ] Milestone 5: Implement project-first pages (projects home, project overview, Notion-style wiki reader with tree, breadcrumbs, metadata row, and internal link navigation).
- [ ] Milestone 6: Implement Linear-style runs board with side drawer detail, remove report overlay and all legacy chat UI including `/chats/:chatId`, then run verification builds/tests.

## Current progress

- Milestone 1 completed:
- Added `SQLiteRepository.ListRunsByProjectSlug(ctx, slug)` that filters hydrated runs by `run.Project.Slug`.
- Added `RunService.ListRunsByProjectSlug(ctx, slug, query)` with status filtering, page/page_size normalization, and pagination metadata (`total`, `total_pages`).
- Added focused tests:
- `TestSQLiteRepositoryListRunsByProjectSlug`
- `TestRunServiceListRunsByProjectSlugFiltersStatusAndPaginates`
- `TestRunServiceListRunsByProjectSlugRejectsInvalidStatus`
- Verification run: `go test ./internal/store ./internal/assistantapp`
- Milestone 2 completed:
- Added `GET /api/v1/projects/:slug` project aggregate payload with:
- `project` summary
- `stats` (`active_runs`, `waiting_runs`, `scheduled_runs`, `completed_runs`, `stopped_runs`, `wiki_page_count`)
- `recent_runs` (latest 5 by `updated_at`)
- `latest_log_entries`
- Added `GET /api/v1/projects/:slug/runs` with query support for:
- `status`, `page`, `page_size`, `include_details`
- Added API response pagination metadata and optional `run_records` expansion when `include_details=true`.
- Added service wiring for aggregate path: `RunService.ListAllRunsByProjectSlug(ctx, slug)`.
- Added focused coverage:
- `TestRunServiceListAllRunsByProjectSlug`
- `TestProjectsAPIProjectDetailAndRunsEndpoints`
- Verification run: `go test ./internal/assistantapp ./internal/api ./internal/store`

## Key decisions

- Project is the primary navigation object.
- TanStack Router is required for routing.
- TanStack Query is required for server-state management.
- `project_slug` is hard-bound for project-scoped run creation; project selector is skipped when provided.
- Wiki pages API remains flat; frontend builds tree/grouping by page path.
- Run detail is a Linear-style side drawer over the runs board.
- Report overlay is removed.
- Legacy chat UI is removed, including `/chats/:chatId`.
- Implementation order is backend-first, then frontend shell, then page surfaces.
- Project run list pagination defaults: `page=1`, `page_size=20`, capped at `page_size=200`.
- Project run status filtering validates against `assistant.AllRunStatuses()` and rejects unknown statuses.
- `/api/v1/projects/:slug/runs?include_details=true` returns paginated run summaries plus `run_records`; default remains summary-only payload.

## Remaining issues / open questions

- Confirm whether changed wiki page summaries for run cards should be inferred from ingest artifacts/events or persisted in a dedicated field.
- Confirm the desired fallback UX when project wiki metadata fields are absent on older pages.

## Links to related documents

- [AGENTS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/AGENTS.md)
- [README.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/README.md)
- [docs/PLANS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/PLANS.md)
- [docs/exec-plans/active/redesign-cva-web-project-first.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/exec-plans/active/redesign-cva-web-project-first.md)
