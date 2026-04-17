# Redesign CVA Web Around Projects

## Goal / scope

Redesign the CVA web UI from a chat-first assistant surface into a project-first operating console.

CVA's primary object is a project. A virtual agent works inside that project to make progress toward goals, writes durable project wiki pages, and leaves auditable run history behind. The web UI should make that structure obvious:

1. Users choose a project first.
2. Users read the project wiki like a note app.
3. Users inspect runs for that project.
4. Users understand how each run ended, what it changed, and what still needs attention.
5. Users can start a new run in the selected project without falling back into a generic chat list.

Out of scope for the first pass:

- Full wiki editing and collaborative note editing.
- Multi-user permissions.
- Replacing the existing run engine, SQLite store, or wiki ingest flow.
- Removing legacy chat APIs before a compatibility route exists.
- Building embeddings or semantic wiki search.

## Background

The current web app is centered on chats. The sidebar lists chat history, the main panel renders an assistant thread, and the final report is shown as an overlay. That structure makes individual interactions readable, but it hides the larger product model:

- projects are long-lived workspaces,
- project wiki pages are the durable memory layer,
- runs are work units that move the project forward,
- chat is only one interaction surface around those runs.

The repository already has most of the backend primitives needed for a project-first UI:

- [internal/api/handler.go](/Users/dev/git/codex-virtual-assistant/internal/api/handler.go) exposes `/api/v1/projects`, project wiki read APIs, run APIs, scheduled run APIs, and SSE event streams.
- [internal/wiki/service.go](/Users/dev/git/codex-virtual-assistant/internal/wiki/service.go) lists projects, reads wiki pages, scaffolds project wiki files, ingests runs, and lints wiki health.
- [internal/store/schema.go](/Users/dev/git/codex-virtual-assistant/internal/store/schema.go) stores runs, run events, attempts, artifacts, evidence, evaluations, tool calls, web steps, wait requests, and scheduled runs.
- [internal/store/repository.go](/Users/dev/git/codex-virtual-assistant/internal/store/repository.go) already returns detailed `RunRecord` and `ChatRecord` views.
- [webapp/src/App.tsx](/Users/dev/git/codex-virtual-assistant/webapp/src/App.tsx) currently owns nearly all frontend state and renders the chat-first interface.

The main missing pieces are project-centered API aggregates, a wiki page tree API, project-scoped run listing, and a frontend information architecture that treats `Project` as the top-level route.

## Product direction

The web app should move from this mental model:

```text
Chat -> Thread -> Latest run/report
```

To this model:

```text
Project -> Overview / Wiki / Runs / Activity / Settings
```

Chat should remain available as legacy context and run provenance, but it should no longer be the main navigation object.

## Proposed information architecture

### Projects home

The root page should show all projects.

Each project row/card should include:

- project name and slug,
- description,
- latest update time,
- wiki page count,
- active run count,
- waiting run count,
- completed run count,
- failed/stopped run count,
- latest run summary.

`no_project` should be shown separately as an Inbox or Unsorted area because it is not a durable named project and does not have a wiki.

### Project overview

The project landing page should summarize the current project state.

Recommended content:

- project title, slug, description, and workspace path,
- rendered `wiki/overview.md`,
- recent entries from `wiki/log.md`,
- open questions from `wiki/open-questions.md`,
- latest 5 runs,
- a project-scoped new run composer.

### Project wiki

The wiki view should feel closer to a note app than a report drawer.

Recommended layout:

```text
+------------------+--------------------+-------------+
| Wiki tree        | Markdown document  | Page meta   |
| overview.md      |                    | status      |
| topics/...       |                    | confidence  |
| reports/...      |                    | source refs |
+------------------+--------------------+-------------+
```

The first implementation should be read-only and should support:

- page tree grouped by page type and folder,
- markdown rendering,
- frontmatter metadata display,
- internal wiki link navigation,
- source refs and related links,
- quick switching between `overview.md`, `index.md`, `log.md`, `open-questions.md`, `topics/*`, and `reports/*`.

### Project runs board

Runs should be shown as a kanban-style board because users need to understand project progress at a glance.

Recommended columns:

- `Queued`
- `Working`
- `Waiting`
- `Scheduled`
- `Completed`
- `Stopped`

Status grouping:

- `Queued`: `queued`
- `Working`: `gating`, `answering`, `selecting_project`, `planning`, `contracting`, `generating`, `evaluating`, `scheduling`, `wiki_ingesting`, `reporting`
- `Waiting`: `waiting`
- `Scheduled`: pending scheduled runs
- `Completed`: `completed`
- `Stopped`: `failed`, `exhausted`, `cancelled`

Each run card should show:

- goal or user request,
- status,
- phase,
- project slug,
- created/updated/completed time,
- latest evaluation score and summary when available,
- whether the run is waiting for input,
- artifact count,
- changed wiki pages when available,
- short outcome summary.

### Run detail

Opening a run card should show a detail drawer or route.

The detail view should answer:

- What was the goal?
- How did the run end?
- What did CVA produce?
- What evidence was checked?
- What artifacts are available?
- What wiki pages were changed?
- Did it fail, wait, or schedule follow-up work?

Recommended sections:

- run title, status, phase, timestamps,
- user request,
- final outcome summary,
- evaluation score and missing requirements,
- artifacts,
- wiki ingest summary and changed pages,
- attempts,
- timeline of events,
- evidence,
- tool calls,
- web steps,
- wait requests,
- scheduled follow-ups,
- raw event list for debugging.

## Backend plan

### Milestone 1: Add project-centered run queries

Add repository/service support for project-scoped run listing.

Initial implementation can filter decoded `run.Project.Slug` in Go using existing `ListRuns`, because this keeps the migration small. If performance becomes an issue, add a persisted `project_slug` column and index.

Likely additions:

- `SQLiteRepository.ListRunsByProjectSlug(ctx, slug)`
- `RunService.ListRunsByProjectSlug(ctx, slug)`
- focused store tests

### Milestone 2: Add project detail aggregate API

Add:

```text
GET /api/v1/projects/:slug
```

Suggested response:

```json
{
  "project": {},
  "stats": {
    "active_runs": 1,
    "waiting_runs": 0,
    "scheduled_runs": 2,
    "completed_runs": 12,
    "stopped_runs": 2,
    "wiki_page_count": 18
  },
  "recent_runs": [],
  "latest_log_entries": []
}
```

This endpoint should be optimized for the project overview page and projects home.

### Milestone 3: Add project runs API

Add:

```text
GET /api/v1/projects/:slug/runs
```

Supported query parameters:

- `status`
- `page`
- `page_size`
- `include_details=false`

The default response should return run summaries suitable for board cards. Detailed run data should continue to come from:

```text
GET /api/v1/runs/:run_id
```

### Milestone 4: Add wiki pages/tree API

Add:

```text
GET /api/v1/projects/:slug/wiki/pages
```

Suggested response:

```json
{
  "pages": [
    {
      "path": "overview.md",
      "title": "Project Overview",
      "page_type": "overview",
      "updated_at": "2026-04-16T00:00:00Z",
      "status": "active",
      "confidence": "medium",
      "source_refs": ["PROJECT.md"],
      "related": ["index.md", "open-questions.md"]
    }
  ]
}
```

The frontend can build a tree/grouped navigation from this flat list.

### Milestone 5: Add project-scoped run creation

Today `POST /api/v1/runs` accepts a raw user request and lets the project selector choose a project.

For the project UI, add a way to explicitly request a project:

```json
{
  "user_request_raw": "continue competitor pricing research",
  "project_slug": "competitor-pricing",
  "max_generation_attempts": 3
}
```

The engine/policy should still validate that the project exists. If explicit project selection conflicts with planner output, the explicit project should win unless the user chooses an escape hatch.

## Frontend plan

### Milestone 1: Split frontend types and API client

Move shared API types and fetch helpers out of `App.tsx`.

Proposed structure:

```text
webapp/src/api/client.ts
webapp/src/api/types.ts
```

Also align frontend run statuses with backend statuses, including `wiki_ingesting`.

### Milestone 2: Introduce project routes and shell

Adopt a route structure centered on projects.

Recommended URLs:

```text
/                         -> projects home
/projects/:slug           -> project overview
/projects/:slug/wiki      -> wiki index
/projects/:slug/wiki/*    -> wiki page
/projects/:slug/runs      -> runs board
/runs/:runId              -> run detail
/chats/:chatId            -> legacy chat view or redirect
```

Using `react-router` is appropriate for this redesign because the app now has durable, shareable pages instead of a single hash-based chat state.

### Milestone 3: Build projects home

Implement the project list from `/api/v1/projects` plus the new project aggregate data when available.

The page should make recent project activity and project health visible without entering a chat.

### Milestone 4: Build project overview

Implement:

- project header,
- overview markdown,
- recent log entries,
- open questions snapshot,
- recent runs,
- project-scoped new run composer.

### Milestone 5: Build wiki reader

Implement:

- wiki page list/tree,
- markdown page renderer,
- metadata panel,
- internal wiki link navigation,
- loading/error/empty states.

The first pass should stay read-only.

### Milestone 6: Build runs kanban board

Implement:

- status grouping,
- run cards,
- scheduled run column data,
- active run polling/SSE refresh,
- run detail drawer or route.

The board should make terminal outcomes visually obvious.

### Milestone 7: Preserve and demote chat behavior

Keep the existing thread experience available enough to avoid breaking old links, but move it behind project and run views.

Options:

- keep `/chats/:chatId` as a legacy route,
- redirect a chat to its latest run detail,
- expose chat transcript inside run detail when useful.

## Current progress

- Not started.
- Planning document created on 2026-04-16.

## Key decisions

- Project is the top-level navigation object.
- Run is a project work unit, not primarily a chat message.
- Wiki is the human-readable project memory surface.
- SQLite remains the raw provenance and audit store.
- Wiki editing is deferred; first pass is read-only.
- `no_project` should be treated as Inbox/Unsorted, not as a normal project.
- Existing chat APIs should remain during migration.
- Kanban board status columns should group noisy internal phases into user-comprehensible workflow states.
- Project-scoped new runs should explicitly bind to the selected project.

## Remaining issues / open questions

- Should project-scoped run creation bypass project selection entirely, or should it provide the selected project as a strong hint while still allowing the selector to reject it?
- Should the wiki page list API return full folder tree structure or a flat list with enough metadata for the frontend to group it?
- Should scheduled runs be stored with explicit project slug, or should they continue inheriting project context from their parent run?
- Should changed wiki pages be persisted in a dedicated table/event payload, or inferred from wiki ingest artifacts/events?
- Should the run detail page replace the current supervisor report overlay entirely, or should the overlay remain as a compact summary?

## Suggested implementation order

1. Add project-scoped backend aggregates and tests.
2. Split frontend API/types from `App.tsx`.
3. Add project routes and project shell.
4. Build projects home and project overview.
5. Build wiki reader.
6. Build project runs board.
7. Add run detail drawer/page.
8. Add project-scoped run creation.
9. Keep legacy chat route working until the new project-first UI is stable.

## Links to related documents

- [AGENTS.md](/Users/dev/git/codex-virtual-assistant/AGENTS.md)
- [docs/exec-plans/completed/add-project-wiki-memory-layer.md](/Users/dev/git/codex-virtual-assistant/docs/exec-plans/completed/add-project-wiki-memory-layer.md)
- [webapp/src/App.tsx](/Users/dev/git/codex-virtual-assistant/webapp/src/App.tsx)
- [internal/api/handler.go](/Users/dev/git/codex-virtual-assistant/internal/api/handler.go)
- [internal/wiki/service.go](/Users/dev/git/codex-virtual-assistant/internal/wiki/service.go)
- [internal/store/schema.go](/Users/dev/git/codex-virtual-assistant/internal/store/schema.go)
