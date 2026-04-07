# Add Project Wiki Memory Layer

## Goal / scope

Add a project-scoped wiki memory layer to CVA so each named project maintains a persistent, LLM-managed markdown knowledge base alongside the existing run/evidence/artifact store.

Initial scope:

- create a standard wiki scaffold for each project
- teach project selection, planning, and answer phases to read project wiki context
- add a post-run wiki ingest flow that updates markdown pages from run outputs
- add a wiki lint flow for contradictions, stale claims, orphan pages, and missing references
- keep SQLite run records as the source of provenance while markdown remains the main human-readable memory layer

Out of scope for the first implementation:

- embeddings or vector infrastructure
- multi-user wiki permissions
- automatic conflict resolution beyond explicit conflict notes
- polished wiki UI beyond basic API and project visibility hooks

## Background

CVA already has most of the underlying primitives needed for project memory:

- project selection and project workspace creation, including `PROJECT.md`, via `internal/project`
- a phase-driven run engine with gate, answer, project selection, planner, contractor, generator, evaluator, scheduler, and reporter stages
- persisted runs, attempts, events, artifacts, evidence, evaluations, tool calls, and web steps in SQLite
- follow-up runs through `parent_run_id` and chat history

What is missing is a durable project knowledge layer that sits between raw execution history and future reasoning. Today CVA can store what happened in a run, but it does not actively maintain a project memory artifact that improves with each completed task. The wiki layer should make CVA project-aware over time rather than forcing every new task to rediscover context from scratch.

This should follow a strict separation of concerns:

- SQLite run records remain immutable provenance and audit history
- project wiki markdown remains the synthesized, human-readable memory layer
- project schema files define how the wiki is structured and maintained

## Milestones

- [ ] Milestone 1: Define project wiki scaffold and lifecycle contracts
- [ ] Milestone 2: Add wiki-aware read path for project selection, planning, and answer flows
- [ ] Milestone 3: Add wiki ingest after all completed runs
- [ ] Milestone 4: Add wiki lint and maintenance workflows
- [ ] Milestone 5: Expose basic API and operator visibility for project wiki state

## Current progress

- Not started

## Key decisions

- Project wiki is per named project under the project workspace, not global.
- SQLite remains the system of record for raw execution provenance.
- Markdown wiki is the primary human-facing memory artifact.
- The first iteration should avoid embeddings and rely on file/index based search.
- The first iteration should prioritize named projects and keep `no_project` lightweight.
- `no_project` does not get a wiki. Only named projects have wiki scaffolds.
- Wiki ingest triggers on all completed runs, including failures and errors. Failures can contain learnings worth recording.
- Wiki ingest failure does not affect run status. The run remains successful; ingest failure is recorded separately.
- Wiki ingest uses a hybrid approach: `log.md` and `index.md` updates are deterministic, page content creation/updates are LLM-driven.
- Entity vs topic distinction: entities are proper nouns (specific products, people, companies, APIs), topics are abstract concepts (authentication, caching, performance).
- `wiki/sources/` contains metadata pages about sources (URL, summary, confidence). `raw/imports/` stores original source files.
- Wiki page filenames use kebab-case slugs (e.g., `topics/rate-limiting.md`, `entities/openai-api.md`).
- Initial wiki context in prompts is limited to `overview.md` + `index.md` only (~500-1500 tokens).
- Answer mode freshness/sufficiency is delegated to LLM judgment via a `NEEDS_WORKFLOW` tag rather than rule-based thresholds.
- Wiki lint in Milestone 4 includes both manual trigger and automatic scheduling via the existing scheduled run system.
- Wiki ingest phase is placed between Scheduler and Reporter in the engine flow (both answer and workflow modes).
- Answer mode also runs wiki ingest. Useful synthesized answers are recorded in project memory.
- Existing named projects without wiki/ are auto-migrated: EnsureProject creates the wiki scaffold if missing.
- Wiki ingest uses a single LLM call that returns both the update plan and all page contents together.
- Page updates use full rewrite: LLM receives existing page + run results, outputs complete updated page.
- `log.md` uses markdown block entries with heading per run (date, run ID, status, summary, changed pages list).
- `index.md` uses section-per-type format with links and one-line descriptions, grouped by page type.
- Conflict notes use blockquote + tag pattern: `> **⚠️ CONFLICT** (run-id, date)` with previous/new/source/resolution fields.
- NEEDS_WORKFLOW fallback in answer mode notifies the user and waits for confirmation before launching a new workflow run.
- Wiki lint auto-schedule runs daily (fixed 1-day cycle for all projects).

## Proposed design

### 1. Project workspace layout

Standardize each named project directory as:

```text
workspace/projects/<slug>/
  PROJECT.md
  wiki/
    index.md
    log.md
    overview.md
    open-questions.md
    entities/
    topics/
    decisions/
    reports/
    sources/
    playbooks/
  raw/
    imports/
    attachments/
  .browser-profile/
```

Notes:

- `PROJECT.md` remains the high-level project charter and routing hint.
- `wiki/` is LLM-maintained synthesized memory.
- `raw/` stores user-imported source material and downloaded attachments.
- `.browser-profile/` remains unchanged for browser continuity.

### 2. Wiki document conventions

All wiki pages should use lightweight YAML frontmatter with at least:

- `title`
- `page_type`
- `updated_at`
- `status`
- `confidence`
- `source_refs`
- `related`

Allowed initial page types:

- `overview`
- `topic`
- `entity`
- `decision`
- `report`
- `source`
- `playbook`
- `question`

Provenance rule:

- every material claim added to the wiki must be attributable to at least one run, artifact, evidence item, or source URL
- when confidence is low or evidence is incomplete, the wiki should mark that explicitly rather than writing an unqualified claim
- when new evidence conflicts with existing content, the wiki should append a revision or conflict note instead of silently overwriting history

### 3. Read path integration

#### Project selection

Extend project selection so the selector can inspect not only `projects/*/PROJECT.md` but also the project wiki summary files, especially:

- `wiki/overview.md`
- `wiki/index.md`

Selection should consider:

- fit with the user request
- continuity with existing project context
- whether the request belongs in a long-lived project or in `no_project`

#### Planning

Planner input should include a project wiki context summary with:

- project overview summary
- relevant pages
- recent log entries
- open questions
- known decisions or constraints

This should help planning avoid re-deriving known facts and should make the resulting `TaskSpec` more project-aware.

#### Answer

Answer mode should become wiki-first for named projects:

- search relevant wiki pages before answering
- use parent run context as a supplement rather than the only memory source
- fall back from answer mode to workflow mode when the wiki lacks sufficient evidence or freshness

### 4. Wiki ingest flow

Add a post-run wiki ingest flow for all completed runs (including failures and errors).

Candidate placement:

- a new `wiki_ingesting` phase before reporter, or
- a post-reporter hook that runs before marking the run fully complete

Preferred direction:

- insert an explicit wiki ingest step before final completion so the system guarantees that successful work has already been reflected in project memory when the run is reported as complete

The ingest step should:

- read artifacts, evidence, evaluations, and relevant attempt summaries
- decide which wiki pages should be created or updated
- append a chronological entry to `wiki/log.md`
- update `wiki/index.md`
- update `wiki/overview.md` when project state materially changes
- create or update pages in `topics/`, `reports/`, `decisions/`, and other relevant sections

The first implementation should bias toward:

- `overview.md`
- `index.md`
- `log.md`
- `reports/`
- `topics/`

Entity extraction and deeper page graph maintenance can come later.

### 5. Wiki lint flow

Add a lint workflow for project wiki health.

Checks should include:

- stale claims superseded by newer evidence
- contradictions between pages
- orphan pages missing inbound references
- pages with weak or missing provenance
- concepts repeatedly mentioned but lacking their own page
- unresolved open questions that should trigger more research

Outputs should be written to:

- `wiki/reports/wiki-health-<date>.md`

And should also update:

- `wiki/open-questions.md`

This lint supports both manual trigger and automatic scheduling via the existing scheduled run system from the start.

## Implementation plan

### Milestone 1: Wiki scaffold and service interfaces

Create a new `internal/wiki` package with the initial responsibilities:

- project wiki scaffold creation
- basic page read/write helpers
- index and log helpers
- wiki context loading for prompts

Likely changes:

- update `internal/project/manager.go` so `EnsureProject` creates the wiki scaffold for named projects
- add reusable templates for `PROJECT.md` and initial wiki files
- define the internal types needed for wiki context and wiki update planning

Suggested initial types:

- `WikiContext`
- `WikiPageMeta`
- `WikiSearchResult`
- `WikiUpdatePlan`
- `WikiPagePatch`
- `ClaimRef`

### Milestone 2: Wiki-aware prompt inputs

Extend prompt input builders and engine orchestration so these phases can consume wiki context:

- project selector
- planner
- answer

Required changes:

- add wiki loading to the engine after project selection and before planner execution
- add wiki loading to answer mode for named projects
- update prompt builders to accept summarized wiki context

This should remain summary-based rather than dumping entire wiki files into prompts.

### Milestone 3: Wiki ingest phase

Add a new engine step that runs after all completed runs (including failures). Ingest failure does not affect run status.

Responsibilities:

- summarize what the run established
- identify pages to create or edit
- persist markdown updates safely inside the project wiki
- append a log entry with run id and changed pages

Possible engine changes:

- add a new attempt role and phase for wiki ingest, or
- implement a deterministic internal post-processing step that does not require a separate external attempt

Tradeoff:

- a dedicated phase gives better observability and retry semantics
- an internal step is simpler but less transparent

Preferred first pass:

- explicit `wiki_ingesting` phase for observability

### Milestone 4: Wiki lint flow

Introduce a lint entrypoint that can run against an existing project wiki without requiring a normal end-user task.

The lint path should:

- scan wiki files
- compare recent wiki claims against recent run provenance
- surface missing source refs and structural gaps
- emit a report page and optionally create scheduled follow-up runs

### Milestone 5: API and UI visibility

Expose enough visibility so operators can inspect the wiki state without opening files directly.

Initial API candidates:

- `GET /api/v1/projects`
- `GET /api/v1/projects/:slug/wiki/index`
- `GET /api/v1/projects/:slug/wiki/page`
- `POST /api/v1/projects/:slug/wiki/lint`

UI can stay minimal in the first pass:

- show project wiki summary on the project detail surface
- show wiki pages updated by the latest run
- link a run to the wiki pages it changed

## Data model considerations

The first version can operate directly on files under `wiki/` and use existing SQLite records as provenance lookup.

Optional second-step persistence:

- `wiki_pages`
- `wiki_page_refs`

Those tables would improve:

- page lookup
- provenance backreferences
- operator UI responsiveness

But they should not block the initial implementation.

## Validation plan

Manual validation:

1. Create a new named project and confirm the wiki scaffold is created.
2. Run a workflow task inside that project and confirm the wiki is updated.
3. Ask a follow-up question and confirm answer mode can use the wiki without redoing the full workflow when appropriate.
4. Inspect `wiki/log.md` and `wiki/index.md` for consistency.
5. Run wiki lint and confirm it generates a health report with actionable findings.

Automated validation targets:

- project manager tests for scaffold creation
- wiki service tests for page creation, index updates, and log appends
- engine tests covering wiki-aware answer and wiki ingest paths
- prompt tests for wiki context injection
- API tests for wiki index/page/lint routes when added

## Remaining issues / open questions

- No unresolved questions at this time.

### Resolved

- ~~Should `no_project` get a minimal wiki scaffold or no wiki at all?~~ → No wiki for `no_project`.
- ~~Should wiki ingest be modeled as an LLM phase, a deterministic service, or a hybrid?~~ → Hybrid: deterministic for log/index, LLM for page content.
- ~~How should the system cap wiki prompt context as projects become large?~~ → Initial version uses only `overview.md` + `index.md`.
- ~~Should wiki lint create scheduled runs automatically or only recommend them first?~~ → Both manual trigger and automatic scheduling in M4.
- ~~Should answer mode be allowed to update the wiki when a useful synthesized answer is generated, or should that be limited to workflow completion for the first version?~~ → Answer mode also runs wiki ingest.
- ~~When conflicting claims exist, what exact markdown pattern should be used for conflict notes and superseded claims?~~ → Blockquote + tag: `> **⚠️ CONFLICT** (run-id, date)` with previous/new/source/resolution fields.

## Links to related documents

- `AGENTS.md`
- `README.md`
- `PRD.md`
- `docs/PLANS.md`
- `internal/project/manager.go`
- `internal/wtl/engine.go`
- `internal/prompting/prompts.go`
- `internal/store/schema.go`
