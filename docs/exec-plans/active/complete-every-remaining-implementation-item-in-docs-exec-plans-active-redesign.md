# Complete Remaining Redesign Implementation Items

## Goal / scope

Implement every remaining item in [docs/exec-plans/active/redesign-cva-web-project-first.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/exec-plans/active/redesign-cva-web-project-first.md), covering all backend and frontend phases for the project-first CVA web redesign.

Scope includes:

- Backend project-scoped run queries and APIs.
- Backend wiki page listing/tree API and project-scoped run creation.
- Frontend API/type split, project-first routing shell, and page-level UI (home, overview, wiki, runs, run detail).
- Legacy chat compatibility during migration.
- Test/build verification for modified backend and frontend surfaces.

Out of scope remains the same as the active redesign plan (wiki editing, multi-user permissions, store/runtime replacement, and semantic wiki search).

## Background

The active redesign plan defines a full project-first information architecture and implementation order, but execution work is still pending. The repository already has core primitives in API handlers, wiki services, store records, and a monolithic `webapp/src/App.tsx`; remaining work is integrating these into project-centered backend endpoints and frontend routes.

`ARCHITECTURE.md` is not present in this worktree, so architecture context is taken from [README.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/README.md), [PRD.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/PRD.md), and the active redesign execution plan.

## Milestones

- [ ] Milestone 1: Implement backend project-scoped run data access and service plumbing (`ListRunsByProjectSlug`, filtering, pagination/status handling, and store/service tests).
- [ ] Milestone 2: Implement backend project APIs (`GET /api/v1/projects/:slug`, `GET /api/v1/projects/:slug/runs`, `GET /api/v1/projects/:slug/wiki/pages`) plus handler/service coverage.
- [ ] Milestone 3: Implement project-scoped run creation support in `POST /api/v1/runs` (`project_slug` request field, validation/policy behavior, and regression tests).
- [ ] Milestone 4: Refactor frontend foundations by extracting API client/types from `App.tsx`, aligning status models, and introducing project-first routes/shell.
- [ ] Milestone 5: Build project-first frontend views (projects home, project overview, wiki reader with tree/metadata/internal link navigation).
- [ ] Milestone 6: Build runs experience (kanban board, run detail route/drawer, scheduled/waiting visibility), preserve legacy chat route compatibility, and finish verification (targeted tests + build).

## Current progress

- Not started.
- Plan created for full remaining implementation execution.

## Key decisions

- Use the existing redesign plan as source of truth for feature scope and sequence.
- Treat Project as top-level navigation object and Run as project work unit.
- Keep legacy chat routes available while project/runs surfaces become primary.
- Prioritize minimal-risk backend additions first, then frontend IA/routing, then page assembly.
- Require tests/build checks before considering milestones complete.

## Remaining issues / open questions

- Whether project-scoped run creation should hard-pin project assignment or remain a strong hint when planner output conflicts.
- Whether wiki pages API should return a flat list with metadata only or explicit hierarchical grouping from backend.
- Whether changed wiki pages should be inferred from ingest artifacts/events or persisted explicitly for run cards/detail.

## Links to related documents

- [AGENTS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/AGENTS.md)
- [README.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/README.md)
- [PRD.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/PRD.md)
- [docs/PLANS.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/PLANS.md)
- [docs/exec-plans/active/redesign-cva-web-project-first.md](/Users/dev/git/codex-virtual-assistant/.worktrees/redesign-cva-web-project-first/docs/exec-plans/active/redesign-cva-web-project-first.md)
