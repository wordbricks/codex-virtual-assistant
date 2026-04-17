# Complete Remaining Redesign Implementation Items

## Goal / scope

Implement every remaining item in [docs/exec-plans/active/redesign-cva-web-project-first.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/exec-plans/active/redesign-cva-web-project-first.md), covering all backend and frontend phases for the project-first CVA web redesign.

Scope includes:

- Backend project-scoped run queries and APIs.
- Backend flat wiki pages API and hard-bound project-scoped run creation.
- Frontend API/type split, TanStack Router/Query project shell, and page-level UI (home, overview, Notion-style wiki, Linear-style runs board and run drawer).
- Removal of the legacy chat-first UI, legacy `/chats/:chatId` route, and old report overlay in favor of project/run surfaces.
- Test/build verification for modified backend and frontend surfaces.

Out of scope remains the same as the active redesign plan (wiki editing, multi-user permissions, store/runtime replacement, and semantic wiki search).

## Background

The active redesign plan defines a full project-first information architecture and implementation order, but execution work is still pending. The repository already has core primitives in API handlers, wiki services, store records, and a monolithic `webapp/src/App.tsx`; remaining work is integrating these into project-centered backend endpoints and frontend routes.

`ARCHITECTURE.md` is not present in this worktree, so architecture context is taken from [README.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/README.md), [PRD.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/PRD.md), and the active redesign execution plan.

## Milestones

- [ ] Milestone 1: Implement backend project-scoped run data access and service plumbing (`ListRunsByProjectSlug`, filtering, pagination/status handling, and store/service tests).
- [ ] Milestone 2: Implement backend project APIs (`GET /api/v1/projects/:slug`, `GET /api/v1/projects/:slug/runs`, `GET /api/v1/projects/:slug/wiki/pages`) plus handler/service coverage.
- [ ] Milestone 3: Implement hard-bound project-scoped run creation support in `POST /api/v1/runs` (`project_slug` request field, explicit project validation, skip project selector when provided, and regression tests).
- [ ] Milestone 4: Refactor frontend foundations by installing TanStack Router and TanStack Query, extracting API client/types from `App.tsx`, aligning status models, and introducing project-first routes/shell.
- [ ] Milestone 5: Build project-first frontend views (projects home, project overview, Notion-style wiki reader with collapsible tree, rich markdown blocks, metadata property row, breadcrumbs, and internal link navigation).
- [ ] Milestone 6: Build the Linear-style runs experience (kanban columns, compact cards, filters, scheduled/waiting visibility, smooth refresh behavior, run detail side drawer), remove the legacy chat route/report overlay, and finish verification (targeted tests + build).

## Current progress

- Not started.
- Plan created for full remaining implementation execution.

## Key decisions

- Use the existing redesign plan as source of truth for feature scope and sequence.
- Treat Project as top-level navigation object and Run as project work unit.
- Use TanStack Router for durable, shareable project routes.
- Use TanStack Query for server-state caching, background refresh, and SSE integration.
- Hard-bind project-scoped run creation: explicit `project_slug` wins and skips the project selector.
- Represent run detail as a Linear-style side drawer over the kanban board, not a full-page route.
- Return wiki pages as a flat API response and build the tree in the frontend.
- Replace the old report overlay with the new run detail drawer.
- Drop `/chats/:chatId` entirely; do not preserve a backward-compatible chat route.
- Prioritize minimal-risk backend additions first, then frontend IA/routing, then page assembly.
- Require tests/build checks before considering milestones complete.

## Remaining issues / open questions

- Whether changed wiki pages should be inferred from ingest artifacts/events or persisted explicitly for run cards/detail.

## Links to related documents

- [AGENTS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/AGENTS.md)
- [README.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/README.md)
- [PRD.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/PRD.md)
- [docs/PLANS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/PLANS.md)
- [docs/exec-plans/active/redesign-cva-web-project-first.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/exec-plans/active/redesign-cva-web-project-first.md)
