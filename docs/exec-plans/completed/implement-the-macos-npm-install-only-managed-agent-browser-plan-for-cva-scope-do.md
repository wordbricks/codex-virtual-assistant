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

- [x] Milestone 1: Narrow release workflow targets to darwin `amd64`/`arm64` for CVA and add darwin `amd64`/`arm64` patched `agent-browser` asset build/upload from `ref/agent-browser`.
- [x] Milestone 2: Update npm platform metadata/resolution so only macOS (`darwin:x64`, `darwin:arm64`) is supported, and ensure asset naming/URL resolution includes both managed `cva` and managed `agent-browser` binaries.
- [x] Milestone 3: Update npm install flow to download/install both binaries for supported macOS targets and keep install error handling/messages coherent for unsupported platforms.
- [x] Milestone 4: Update npm bin wrapper to pass the downloaded managed `agent-browser` path to native CVA via `ASSISTANT_AGENT_BROWSER_BIN` (and any required compatibility wiring), without changing `AGENT_BROWSER_EXECUTABLE_PATH` meaning.
- [x] Milestone 5: Update native CVA `agent-browser` resolver logic and tests to enforce priority order: `ASSISTANT_AGENT_BROWSER_BIN` -> `CVA_AGENT_BROWSER_BIN` -> PATH fallback; keep Chrome executable handling separate.
- [x] Milestone 6: Update docs (`README.md`, `npm/README.md`) and run focused Go/Node tests covering release/npm/runtime path-resolution changes.

## Current progress

- Milestone 1 completed in `.github/workflows/release.yml`.
- Release build jobs are now split into `build-cva-binaries` and `build-agent-browser-binaries`.
- CVA release artifacts now build only darwin targets (`cva-darwin-x64`, `cva-darwin-arm64`).
- Added darwin-only managed `agent-browser` artifacts built from checkout ref `ref/agent-browser` as `agent-browser-darwin-x64` and `agent-browser-darwin-arm64`.
- `create-release` now depends on both binary build jobs so both components publish into the same GitHub Release.
- Milestone 2 completed across npm metadata and platform resolution:
  - `npm/package.json` now declares `os: ["darwin"]` and `cpu: ["x64", "arm64"]`.
  - `npm/lib/platform.js` now enforces darwin-only targets in `currentTarget()`.
  - Added explicit managed `agent-browser` resolution helpers in `npm/lib/platform.js`:
    - `agentBrowserExecutableName`
    - `agentBrowserAssetName`
    - `agentBrowserBinaryPath`
    - `agentBrowserDownloadURL`
  - Added npm metadata field `cva.agentBrowserAssetPrefix` (default `agent-browser`) for release asset naming.
  - Added focused Node tests in `npm/lib/platform.test.js`.
- Milestone 3 completed in npm installer flow:
  - `npm/lib/install.js` now installs both managed binaries (`cva` + `agent-browser`) via `ensureManagedBinaries`.
  - `ensureBinary` is retained as a compatibility wrapper and now returns the installed `cva` path while still ensuring both binaries are present.
  - Unsupported-platform install errors are now rewritten with explicit macOS-only npm guidance (`darwin/x64`, `darwin/arm64`).
  - Installer CLI failure text now reflects managed dual-binary install failure context.
  - `npm/lib/install.test.js` now covers dual download behavior, force/skip behavior, compatibility `ensureBinary` return path, and unsupported-platform messaging.
  - Focused tests run: `node --test npm/lib/platform.test.js npm/lib/install.test.js`.
- Milestone 4 completed in npm wrapper flow:
  - `npm/bin/cva.js` now resolves both managed paths before spawning native CVA.
  - If either managed binary is missing, the wrapper runs the managed dual-binary installer before launching.
  - The spawned native CVA process receives `ASSISTANT_AGENT_BROWSER_BIN=<managed agent-browser path>`.
  - The wrapper mirrors the same path into `CVA_AGENT_BROWSER_BIN` only when that compatibility env var is not already set.
  - `AGENT_BROWSER_EXECUTABLE_PATH` is not modified by the wrapper and remains a browser executable path.
  - `npm/bin/cva.test.js` verifies wrapper env behavior and managed path resolution.
- Milestone 5 completed in native runtime resolver:
  - `detectAgentBrowserCLIPath()` now resolves `ASSISTANT_AGENT_BROWSER_BIN`, then `CVA_AGENT_BROWSER_BIN`, then PATH `agent-browser`.
  - Go tests verify both managed env priority paths.
- Milestone 6 completed in docs and verification:
  - Root README and npm README now describe macOS-only npm support and the bundled managed `agent-browser` behavior.
  - Release verification now runs npm wrapper, installer, and platform tests.
  - `npm/package.json` `files` is narrowed so npm tarballs include runtime files only, not local tests.
  - Focused tests run: `node --check npm/bin/cva.js npm/lib/install.js npm/lib/platform.js`.
  - Focused tests run: `node --test npm/bin/cva.test.js npm/lib/install.test.js npm/lib/platform.test.js`.
  - Focused tests run: `go test ./internal/wtl`.
  - Package verification run: `npm pack --dry-run`.

## Key decisions

- npm-installed macOS users are the supported path for this change; GitHub Release raw-binary users are intentionally not covered.
- CVA release/npm support drops Linux and Windows targets in this scope.
- Managed `agent-browser` resolution priority in CVA is explicit: `ASSISTANT_AGENT_BROWSER_BIN`, then `CVA_AGENT_BROWSER_BIN`, then PATH fallback.
- `AGENT_BROWSER_EXECUTABLE_PATH` remains reserved for Chrome executable path semantics.
- Release assets for `cva` and patched `agent-browser` must come from the same GitHub Release tag for npm install consistency.
- Managed `agent-browser` release asset names are `agent-browser-darwin-x64` and `agent-browser-darwin-arm64` to mirror CVA darwin naming style.
- npm platform metadata now hard-gates package installation to macOS x64/arm64 via `os`/`cpu` fields in `npm/package.json`.
- npm install flow now treats `cva` and `agent-browser` as a managed pair for download/install; `ensureBinary` remains for wrapper compatibility.
- Release CI builds `agent-browser` on `macos-latest` because the source is a Rust/Node CLI targeting `x86_64-apple-darwin` and `aarch64-apple-darwin`.
- The release workflow checks out the pinned `vercel-labs/agent-browser` source ref used by local `ref/agent-browser` (`57405f93614fae46e5c955ce662b4785283e1301`) into `ref/agent-browser` during CI.

## Remaining issues / open questions

- If the patched `agent-browser` source moves to a project-owned fork, update `AGENT_BROWSER_REPOSITORY` and `AGENT_BROWSER_REF` in `.github/workflows/release.yml`.
- `CVA_AGENT_BROWSER_BIN` is treated as compatibility-only; `ASSISTANT_AGENT_BROWSER_BIN` is the first-class managed path.

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
