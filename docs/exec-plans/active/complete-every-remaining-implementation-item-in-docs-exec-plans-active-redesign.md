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
- [x] Milestone 3: Finish backend flat wiki pages API and hard-bound run creation (`GET /api/v1/projects/:slug/wiki/pages`, `POST /api/v1/runs` with explicit `project_slug` override and selector bypass) with tests.
- [x] Milestone 4: Replace frontend app shell with TanStack Router + TanStack Query and split API client/types out of legacy `App.tsx`.
- [x] Milestone 5: Implement project-first pages (projects home, project overview, Notion-style wiki reader with tree, breadcrumbs, metadata row, and internal link navigation).
- [x] Milestone 6: Implement Linear-style runs board with side drawer detail, remove report overlay and all legacy chat UI including `/chats/:chatId`, then run verification builds/tests.

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
- Milestone 3 completed:
- Added flat wiki pages endpoint `GET /api/v1/projects/:slug/wiki/pages` returning `{"pages":[...]}` metadata entries.
- Added wiki service read API `ListPages(slug)` for flat page metadata listing.
- Added explicit project-bound run creation in API `POST /api/v1/runs` via `project_slug`.
- Added project existence validation for explicit `project_slug` before run creation (missing project returns `404`).
- Added `RunService.CreateRunWithProject(...)` and handler wiring to pass explicit project slug.
- Updated run engine to:
- respect explicit project context at start (`EnsureProject` for pre-bound project),
- skip `AttemptRoleProjectSelector` when workflow run already has a bound project slug.
- Added focused coverage:
- `TestListPagesReturnsFlatSortedMetadata`
- `TestRunsAPICreateRunWithProjectSlugSkipsProjectSelector`
- `TestRunServiceCreateRunWithProjectBindsProjectSlug`
- Verification run: `go test ./internal/assistantapp ./internal/wiki ./internal/wtl ./internal/api ./internal/store`
- Milestone 4 completed:
- Added TanStack Router app shell with route tree and router provider wiring.
- Added TanStack Query provider and bootstrap/projects query usage in the new shell/routes.
- Replaced `main.tsx` root render path with `QueryClientProvider` + `RouterProvider`.
- Split API domain types from legacy chat UI into `webapp/src/api/types.ts`.
- Split shared fetch client into `webapp/src/api/client.ts`.
- Moved legacy chat-heavy UI from `App.tsx` to `legacy/LegacyChatPage.tsx` and mounted it behind `/legacy`.
- Added route scaffolding for:
- `/`
- `/projects/:slug`
- `/projects/:slug/wiki`
- `/projects/:slug/wiki/*`
- `/projects/:slug/runs`
- Explicitly did not add `/chats/:chatId` route in new router shell.
- Verification run: `npm run build` (webapp)
- Milestone 5 completed:
- Added project-first page implementations in `webapp/src/routes/placeholders.tsx`:
- projects home cards with per-project stats and latest-run summary (`/api/v1/projects` + per-project aggregate queries).
- project overview with stats, rendered overview/open-questions wiki sections, recent logs/runs, and project-scoped run composer posting `project_slug`.
- Notion-style wiki reader with page-type grouped sidebar tree, folder branches, breadcrumb navigation, frontmatter metadata pills, and inline internal link navigation through TanStack Router routes.
- Added wiki/project API client coverage in `webapp/src/api/client.ts` and corresponding types in `webapp/src/api/types.ts`.
- Added `react-markdown` dependency for rich wiki document rendering.
- Added styling for project home/overview/wiki layouts and typography in `webapp/src/styles.css`.
- Verification run: `npm run build` (webapp)
- Milestone 6 completed:
- Implemented a Linear-style project runs board in `webapp/src/routes/placeholders.tsx` with:
- horizontal status columns (`Queued`, `Working`, `Waiting`, `Scheduled`, `Completed`, `Stopped`),
- compact run cards with status badges, relative time, evaluation/artifact/wiki-change indicators,
- filter bar for status column and date window,
- scheduled-runs column backed by `/api/v1/scheduled` and project-parent run resolution.
- Implemented a run detail side drawer (no full-page run route) containing:
- run metadata and request/outcome summary,
- evaluations (including missing requirements),
- artifacts, wiki changed pages, attempts, timeline events, evidence, tool calls, web steps,
- wait requests, scheduled follow-ups, and raw event payload view.
- Added frontend API types/client support for project runs, run record retrieval, and scheduled runs in:
- `webapp/src/api/types.ts`
- `webapp/src/api/client.ts`
- Removed report overlay and legacy chat UI surface:
- removed `/legacy` route from TanStack Router and sidebar nav link,
- deleted `webapp/src/legacy/LegacyChatPage.tsx`,
- deleted legacy `webapp/src/components/assistant-ui/*` components,
- removed `@assistant-ui/react` and `@assistant-ui/react-markdown` dependencies.
- Updated `webapp/src/styles.css` to add runs board + drawer styles and remove legacy report overlay styles.
- Verification run: `npm run build` (webapp)

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
- `POST /api/v1/runs` now accepts optional `project_slug`; when provided, the run is hard-bound to that project and workflow execution bypasses project selector phase.
- Frontend shell now uses TanStack Router + TanStack Query with project-first pages, runs board, and run detail side drawer.

## Remaining issues / open questions

- Confirm whether changed wiki page summaries for run cards should be inferred from ingest artifacts/events or persisted in a dedicated field.
- Confirm the desired fallback UX when project wiki metadata fields are absent on older pages.

## Links to related documents

- [AGENTS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/AGENTS.md)
- [README.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/README.md)
- [docs/PLANS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/PLANS.md)
- [docs/exec-plans/active/redesign-cva-web-project-first.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/exec-plans/active/redesign-cva-web-project-first.md)
