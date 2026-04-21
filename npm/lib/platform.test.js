const test = require("node:test");
const assert = require("node:assert/strict");
const os = require("node:os");
const path = require("node:path");

const {
  agentBrowserAssetName,
  agentBrowserBinaryPath,
  agentBrowserDownloadURL,
  binaryPath,
  currentTarget,
  downloadURL
} = require("./platform");

function withPatchedTarget(platform, arch, fn) {
  const originalPlatform = os.platform;
  const originalArch = os.arch;
  os.platform = () => platform;
  os.arch = () => arch;
  try {
    return fn();
  } finally {
    os.platform = originalPlatform;
    os.arch = originalArch;
  }
}

test("currentTarget supports only darwin x64 and darwin arm64", () => {
  assert.deepEqual(
    withPatchedTarget("darwin", "x64", () => currentTarget()),
    { platform: "darwin", arch: "x64" }
  );
  assert.deepEqual(
    withPatchedTarget("darwin", "arm64", () => currentTarget()),
    { platform: "darwin", arch: "arm64" }
  );

  assert.throws(
    () => withPatchedTarget("linux", "x64", () => currentTarget()),
    /Unsupported platform for cva: linux\/x64/
  );
});

test("managed cva and agent-browser asset names can be resolved independently", () => {
  const metadata = {
    version: "1.2.3",
    cva: {
      owner: "example",
      repo: "repo",
      assetPrefix: "cva-custom",
      agentBrowserAssetPrefix: "agent-browser-custom"
    }
  };
  const target = { platform: "darwin", arch: "arm64" };

  assert.equal(binaryPath(target), path.join(path.resolve(__dirname, ".."), "vendor", "darwin", "arm64", "cva"));
  assert.equal(
    agentBrowserBinaryPath(target),
    path.join(path.resolve(__dirname, ".."), "vendor", "darwin", "arm64", "agent-browser")
  );

  assert.equal(
    downloadURL(target, metadata),
    "https://github.com/example/repo/releases/download/v1.2.3/cva-custom-darwin-arm64"
  );
  assert.equal(
    agentBrowserDownloadURL(target, metadata),
    "https://github.com/example/repo/releases/download/v1.2.3/agent-browser-custom-darwin-arm64"
  );
  assert.equal(agentBrowserAssetName(target, metadata), "agent-browser-custom-darwin-arm64");
});
