# @wordbricks/cva

Install the Codex Virtual Assistant CLI:

```bash
npm install -g @wordbricks/cva
```

This package installs a small Node wrapper and downloads the native `cva` binary for the current platform from GitHub Releases.

After installation:

```bash
cva version
cva start
```

Notes:

- The package name is scoped because the unscoped `cva` package is already taken on npm.
- The installed command is still `cva`.
- The downloader expects a matching GitHub release tag in the form `v<package-version>`.
