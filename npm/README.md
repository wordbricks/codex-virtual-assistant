# @wordbricks/cva

Install the Codex Virtual Assistant CLI:

```bash
npm install -g @wordbricks/cva
```

This package supports macOS x64 and macOS arm64. It installs a small Node wrapper and downloads the managed native `cva` and `agent-browser` binaries for the current macOS architecture from the matching GitHub Release.

After installation:

```bash
cva version
cva upgrade
cva start
```

Notes:

- The package name is scoped because the unscoped `cva` package is already taken on npm.
- The installed command is still `cva`.
- The downloader expects a matching GitHub release tag in the form `v<package-version>`.
- Release assets must include both `cva-darwin-<arch>` and `agent-browser-darwin-<arch>`.
- The wrapper passes the downloaded `agent-browser` path to native CVA through `ASSISTANT_AGENT_BROWSER_BIN`. `AGENT_BROWSER_EXECUTABLE_PATH` remains reserved for a browser executable path such as Chrome.
