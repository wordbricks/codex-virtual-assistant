# Goal / scope

Implement the macOS npm-install-only managed `agent-browser` plan for CVA.

In scope:

- Support npm-installed macOS users by downloading both native `cva` and patched `agent-browser` binaries from the same GitHub Release.
- Pass the downloaded managed `agent-browser` path from the Node wrapper into native CVA.
- Update native CVA resolver priority for `agent-browser` to: `ASSISTANT_AGENT_BROWSER_BIN`, then `CVA_AGENT_BROWSER_BIN`, then PATH `agent-browser` fallback.
- Keep `AGENT_BROWSER_EXECUTABLE_PATH` semantics as Chrome executable path only.
- Update release automation to publish only darwin x64/arm64 CVA assets and darwin x64/arm64 `agent-browser` assets from `ref/agent-browser`.
- Update npm platform/install/bin logic and tests.
- Update repository docs and npm docs for macOS-only npm support and bundled managed `agent-browser` behavior.

Out of scope:

- Supporting GitHub Release raw CVA binary users for this change.
- Continuing Windows/Linux support in CVA release/npm build targets.

## Background

Current release/npm flow downloads a single native `cva` binary and includes cross-platform targets (Linux, macOS, Windows). The runtime currently resolves `agent-browser` from environment/PATH and also sets `AGENT_BROWSER_EXECUTABLE_PATH` for browser execution context. This task switches npm delivery to a managed macOS bundle model where npm users receive both binaries from a single release and CVA is explicitly told which `agent-browser` binary to use, while preserving `AGENT_BROWSER_EXECUTABLE_PATH` as Chrome path only.

Repository references show this work spans:

- release asset matrix and upload logic,
- npm platform support and installer/download behavior,
- npm command wrapper environment forwarding,
- native resolver behavior in CVA runtime,
- tests and docs.

## Milestones

- [ ] Milestone 1: Narrow release workflow targets to darwin `amd64`/`arm64` for CVA and add darwin `amd64`/`arm64` patched `agent-browser` asset build/upload from `ref/agent-browser`.
- [ ] Milestone 2: Update npm platform metadata/resolution so only macOS (`darwin:x64`, `darwin:arm64`) is supported, and ensure asset naming/URL resolution includes both managed `cva` and managed `agent-browser` binaries.
- [ ] Milestone 3: Update npm install flow to download/install both binaries for supported macOS targets and keep install error handling/messages coherent for unsupported platforms.
- [ ] Milestone 4: Update npm bin wrapper to pass the downloaded managed `agent-browser` path to native CVA via `ASSISTANT_AGENT_BROWSER_BIN` (and any required compatibility wiring), without changing `AGENT_BROWSER_EXECUTABLE_PATH` meaning.
- [ ] Milestone 5: Update native CVA `agent-browser` resolver logic and tests to enforce priority order: `ASSISTANT_AGENT_BROWSER_BIN` -> `CVA_AGENT_BROWSER_BIN` -> PATH fallback; keep Chrome executable handling separate.
- [ ] Milestone 6: Update docs (`README.md`, `npm/README.md`) and run focused Go/Node tests covering release/npm/runtime path-resolution changes.

## Current progress

- Not started.

## Key decisions

- npm-installed macOS users are the supported path for this change; GitHub Release raw-binary users are intentionally not covered.
- CVA release/npm support drops Linux and Windows targets in this scope.
- Managed `agent-browser` resolution priority in CVA is explicit: `ASSISTANT_AGENT_BROWSER_BIN`, then `CVA_AGENT_BROWSER_BIN`, then PATH fallback.
- `AGENT_BROWSER_EXECUTABLE_PATH` remains reserved for Chrome executable path semantics.
- Release assets for `cva` and patched `agent-browser` must come from the same GitHub Release tag for npm install consistency.

## Remaining issues / open questions

- Confirm final release asset naming convention for managed `agent-browser` darwin artifacts to keep npm installer logic stable.
- Confirm whether `CVA_AGENT_BROWSER_BIN` should be documented as compatibility-only or first-class override for non-wrapper invocations.
- Confirm minimum focused test set expected by reviewers beyond unit coverage in Go (`internal/wtl`) and Node (`npm/lib`).

## Links to related documents

- `AGENTS.md`
- `docs/PLANS.md`
- `README.md`
- `npm/README.md`
- `.github/workflows/release.yml`
- `npm/lib/platform.js`
- `npm/lib/install.js`
- `npm/bin/cva.js`
- `internal/wtl/executor_codex_appserver.go`
