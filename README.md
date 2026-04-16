# Codex Virtual Assistant (CVA) [/ˈsiːvə/]

Codex Virtual Assistant is a WTL GAN-policy based web personal virtual assistant for browser-centric office work. The product source of truth is [PRD.md](/Users/dev/git/CodexVirtualAssistant/PRD.md); code under `specs/` remains reference material only.

This repository now includes the first runnable product foundation outside `specs/`:

- `cmd/cva` provides the main CLI, including `cva start` for the local HTTP server.
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
   - Install Node.js 20.19.0 or newer (or Node.js 22.12.0+) for the webapp toolchain.
   - Install sqlite3 and make sure it is on PATH.
   - Install `ffmpeg` and make sure it is on PATH. CVA uses it to turn captured browser frames into the report video replay.
   - Install the codex CLI and authenticate it so `codex app-server` can run.
   - Install the `agent-browser` CLI and browser runtime:
     - npm install -g agent-browser
     - agent-browser install

3. Build the CLI binary:
   - mkdir -p dist
   - go build -o dist/cva ./cmd/cva

4. Start the app:
   - ./dist/cva start
   - or run with full Codex filesystem access: ./dist/cva start --yolo

5. Open the UI:
   - http://127.0.0.1:8080
```

You can also install the CLI from npm:

```bash
npm install -g @wordbricks/cva
cva version
```

The npm package name is scoped because the unscoped `cva` package is already taken on npm. The installed command is still `cva`, and installation downloads the native binary for the current platform from GitHub Releases.

## Install and Use

Install from npm:

```bash
npm install -g @wordbricks/cva
```

Or build from source:

```bash
git clone git@github.com:wordbricks/codex-virtual-assistant.git
cd codex-virtual-assistant
go build -o dist/cva ./cmd/cva
./dist/cva version
```

Start the local server:

```bash
cva start
```

Start it as a background daemon:

```bash
cva start --daemon
```

Start with full Codex filesystem access:

```bash
cva start --yolo
```

Then open `http://127.0.0.1:8080`.

Daemon logs are written to `<CVA home>/workspace/logs/cva.log` by default, and the PID file is stored at `<CVA home>/workspace/cva.pid`.

Basic CLI usage:

```bash
cva version
cva upgrade
cva start --daemon
cva logs
cva logs --follow
cva stop
cva run "Summarize today's work"
cva status <run_id>
cva watch <run_id>
cva list
cva chat <chat_id>
cva cancel <run_id>
cva resume <run_id> key=value
```

For scheduled work:

```bash
cva schedule list
cva schedule show <scheduled_run_id>
cva schedule cancel <scheduled_run_id>
```

## Run locally

```bash
go run ./cmd/cva start
```

Then open `http://127.0.0.1:8080`.

To run the server in the background during development:

```bash
go run ./cmd/cva start --daemon
go run ./cmd/cva logs --follow
go run ./cmd/cva stop
```

To force the Codex app server to run with `danger-full-access`, start the server with:

```bash
go run ./cmd/cva start --yolo
```

The server now uses `codex app-server` as the default execution runtime for planner/generator/evaluator phases. Make sure the `codex` CLI is installed, authenticated, and able to run `codex app-server` on your machine before you start a run from the UI.

Install `agent-browser` on the same machine so Codex can execute browser work through the current CLI:

```bash
npm install -g agent-browser
agent-browser install
```

If you want browser activity to appear as an embedded replay video in the final supervisor report, `ffmpeg` must also be installed and available on `PATH`.

Environment variables:

- `ASSISTANT_HTTP_ADDR`: override the listen address, default `127.0.0.1:8080`
- `ASSISTANT_DATA_DIR`: root directory for local state, default `<CVA home>/workspace`
- `ASSISTANT_PROJECTS_DIR`: project workspace root, default `<data dir>/projects`
- `ASSISTANT_DATABASE_PATH`: SQLite database path, default `<data dir>/assistant.db`
- `ASSISTANT_ARTIFACT_DIR`: directory for generated artifacts, default `<data dir>/artifacts`
- `ASSISTANT_MAX_GENERATION_ATTEMPTS`: default generator retry budget, default `3`
- `ASSISTANT_CODEX_BIN`: Codex CLI path, default `codex`
- `ASSISTANT_CODEX_CWD`: working directory given to Codex app server, default `<CVA home>`
- `ASSISTANT_CODEX_APPROVAL_POLICY`: Codex approval policy, default `never`
- `ASSISTANT_CODEX_SANDBOX`: Codex sandbox mode, default `workspace-write`
- `ASSISTANT_CODEX_NETWORK_ACCESS`: outbound network access for Codex workspace-write turns, default `true`

`<CVA home>` is the app-owned directory under the current user's OS config directory, so `cva` uses the same default workspace no matter where you launch it. Relative path overrides are resolved from that directory.

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

- SQLite database: `<CVA home>/workspace/assistant.db` by default
- generated artifacts directory: `<CVA home>/workspace/artifacts` by default

## Validation

Useful local verification commands:

```bash
GOCACHE=/tmp/cva-go-build go test ./cmd/cva ./internal/... ./web
node --check web/static/app.js
```

## npm release

The repository now includes an npm wrapper package in [`npm/`](/Users/dev/git/codex-virtual-assistant/npm) for `@wordbricks/cva`.

Release flow:

1. Configure npm Trusted Publishing for this GitHub repository once.
2. Push a semver tag like `v0.1.0`.
3. The GitHub Actions workflow at [release.yml](/Users/dev/git/codex-virtual-assistant/.github/workflows/release.yml) will:
   - run verification
   - build native `cva` binaries for supported platforms
   - upload them to the GitHub release for that tag
   - publish `@wordbricks/cva` to npm with provenance

The npm package installs the `cva` command and downloads the matching native binary asset from the GitHub release for the package version.

For the first manual CLI-driven publish:

```bash
npm login
make release VERSION=0.1.0
```

If you want the individual steps instead:

```bash
make release-manual VERSION=0.1.0
make release-tag VERSION=0.1.0
make release-gh VERSION=0.1.0
make release-npm
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
- Node.js `^20.19.0 || >=22.12.0` for the `webapp/` Vite toolchain
- `codex` CLI installed and authenticated; the default runtime shells out to `codex app-server`
- `agent-browser` CLI installed and initialized with `agent-browser install`
- `sqlite3` available on the local machine; the current repository layer uses the system SQLite CLI for local persistence in this sandboxed environment
- `ffmpeg` available on the local machine; browser-session report videos are generated only when `ffmpeg` is present on `PATH`
- The npm CLI wrapper in `npm/` remains compatible with Node.js 18+, but repository development now assumes the webapp runtime above
