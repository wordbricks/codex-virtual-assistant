# Codex Virtual Assistant (CVA) [/ˈsiːvə/]

Codex Virtual Assistant is a WTL GAN-policy based web personal virtual assistant for browser-centric office work. The product source of truth is [PRD.md](/Users/dev/git/CodexVirtualAssistant/PRD.md); code under `specs/` remains reference material only.

This repository now includes the first runnable product foundation outside `specs/`:

- `cmd/assistantd` boots a local HTTP server.
- `internal/assistant` defines shared run, task, attempt, evaluation, artifact, evidence, and wait-state types, plus `TaskSpec` normalization defaults.
- `internal/assistantapp` owns background run creation, status lookup, resume/input, and cancel orchestration over the engine and store.
- `internal/prompting` defines the planner and evaluator JSON contracts plus strict output decoding.
- `internal/store` owns the local SQLite schema and repository methods for runs, events, attempts, artifacts, evidence, evaluations, tool calls, web steps, and wait requests.
- `internal/wtl` now contains the core run engine plus a concrete Codex-oriented runtime that maps agent-browser style execution traces into persisted tool calls, browser steps, evidence, and screenshot artifacts.
- `internal/api` now exposes the run lifecycle routes and an SSE event stream backed by persisted run events and a live event broker.
- `internal/config`, `internal/app`, and `internal/policy/gan` establish the product bootstrap and lifecycle policy package layout.
- `web/` now contains the embedded operator UI shell served by the app.

## Agent Bootstrap

Copy and paste this instruction to your favorite coding agent to start.

```text
Clone this repository, install prerequisites, build it, and run it locally.

1. Clone the repository and enter it:
   - Repository URL: https://github.com/siisee11/CodexVirtualAssistant
   - git clone git@github.com:siisee11/CodexVirtualAssistant.git
   - cd CodexVirtualAssistant

2. Install prerequisites:
   - Install Go 1.26 or newer.
   - Install sqlite3 and make sure it is on PATH.
   - Install `ffmpeg` and make sure it is on PATH. CVA uses it to turn captured browser frames into the report video replay.
   - Install the codex CLI and authenticate it so `codex app-server` can run.
   - Install the `agent-browser` skill at project scope:
     - npx skills add https://github.com/vercel-labs/agent-browser --skill agent-browser

3. Build the server binary:
   - mkdir -p dist
   - go build -o dist/assistantd ./cmd/assistantd

4. Start the app:
   - ./dist/assistantd
   - or run with full Codex filesystem access: ./dist/assistantd --yolo

5. Open the UI:
   - http://127.0.0.1:8080
```

## Run locally

```bash
go run ./cmd/assistantd
```

Then open `http://127.0.0.1:8080`.

To force the Codex app server to run with `danger-full-access`, start the server with:

```bash
go run ./cmd/assistantd --yolo
```

The server now uses `codex app-server` as the default execution runtime for planner/generator/evaluator phases. Make sure the `codex` CLI is installed, authenticated, and able to run `codex app-server` on your machine before you start a run from the UI.

If you want browser activity to appear as an embedded replay video in the final supervisor report, `ffmpeg` must also be installed and available on `PATH`.

Environment variables:

- `ASSISTANT_HTTP_ADDR`: override the listen address, default `127.0.0.1:8080`
- `ASSISTANT_DATA_DIR`: root directory for local state, default `./workspace`
- `ASSISTANT_PROJECTS_DIR`: project workspace root, default `<data dir>/projects`
- `ASSISTANT_DATABASE_PATH`: SQLite database path, default `<data dir>/assistant.db`
- `ASSISTANT_ARTIFACT_DIR`: directory for generated artifacts, default `<data dir>/artifacts`
- `ASSISTANT_MAX_GENERATION_ATTEMPTS`: default generator retry budget, default `3`
- `ASSISTANT_CODEX_BIN`: Codex CLI path, default `codex`
- `ASSISTANT_CODEX_CWD`: working directory given to Codex app server, default current repo root
- `ASSISTANT_CODEX_APPROVAL_POLICY`: Codex approval policy, default `never`
- `ASSISTANT_CODEX_SANDBOX`: Codex sandbox mode, default `workspace-write`
- `ASSISTANT_CODEX_NETWORK_ACCESS`: outbound network access for Codex workspace-write turns, default `true`

## Current scope

Current completed foundation:

- local config loading and validation
- data directory and SQLite-path bootstrapping
- TaskSpec normalization and planner JSON contracts
- SQLite-backed persistence for the core run records
- WTL lifecycle orchestration for planner, generator, evaluator, retry, waiting, resume, and cancel flows
- Codex-style runtime mapping for browser-oriented evidence, tool activity, and screenshot artifacts
- run lifecycle HTTP routes and SSE event streaming
- a usable web UI skeleton for starting tasks, following live progress, reviewing evidence and artifacts, and handling waiting states

## Using the web UI

Open `http://127.0.0.1:8080`, describe the task in plain language, and start the run from the top form.

The page then keeps a single run view updated through the run status API and SSE stream:

- current task summary and status
- live progress timeline
- recent browser and tool activity
- collected evidence and generated artifacts
- waiting cards for approvals, clarification, or authentication

## Runtime behavior

Current local runtime behavior:

- the app boots a single-user local server with embedded static assets
- run state is stored in SQLite and created automatically on startup
- the default shipped executor starts `codex app-server` and runs each phase through a real Codex turn
- the product does not call `agent-browser` itself; Codex chooses and invokes available tools during the turn, and the app maps resulting tool, command, and browser-like events back into persisted evidence
- runs move through `queued`, `planning`, `generating`, `evaluating`, `waiting`, and terminal states while persisting attempts, events, evidence, tool calls, browser steps, evaluations, and artifacts
- app-server approval or user-input requests are converted into product-level `waiting` states until the operator resumes the run with approval or missing context

Local state locations:

- SQLite database: `data/assistant.db` by default
- generated artifacts directory: `data/artifacts` by default

## Validation

Useful local verification commands:

```bash
GOCACHE=/tmp/cva-go-build go test ./cmd/assistantd ./internal/... ./web
node --check web/static/app.js
```

## HTTP API

- `POST /api/v1/runs`
- `GET /api/v1/runs/:id`
- `GET /api/v1/runs/:id/events`
- `POST /api/v1/runs/:id/input`
- `POST /api/v1/runs/:id/resume`
- `POST /api/v1/runs/:id/cancel`

## Local requirements

- Go 1.26+
- `codex` CLI installed and authenticated; the default runtime shells out to `codex app-server`
- `sqlite3` available on the local machine; the current repository layer uses the system SQLite CLI for local persistence in this sandboxed environment
- `ffmpeg` available on the local machine; browser-session report videos are generated only when `ffmpeg` is present on `PATH`
- Node.js is optional, but useful for checking the browser script during local development
