const test = require("node:test");
const assert = require("node:assert/strict");

const { createLaunchEnv, resolveManagedPaths } = require("./cva");

test("createLaunchEnv sets ASSISTANT_AGENT_BROWSER_BIN and keeps AGENT_BROWSER_EXECUTABLE_PATH unchanged", () => {
  const launchEnv = createLaunchEnv(
    {
      AGENT_BROWSER_EXECUTABLE_PATH: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
    },
    "/managed/agent-browser"
  );

  assert.equal(launchEnv.ASSISTANT_AGENT_BROWSER_BIN, "/managed/agent-browser");
  assert.equal(launchEnv.CVA_AGENT_BROWSER_BIN, "/managed/agent-browser");
  assert.equal(
    launchEnv.AGENT_BROWSER_EXECUTABLE_PATH,
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  );
});

test("createLaunchEnv preserves existing CVA_AGENT_BROWSER_BIN override", () => {
  const launchEnv = createLaunchEnv(
    {
      CVA_AGENT_BROWSER_BIN: "/custom/agent-browser"
    },
    "/managed/agent-browser"
  );

  assert.equal(launchEnv.ASSISTANT_AGENT_BROWSER_BIN, "/managed/agent-browser");
  assert.equal(launchEnv.CVA_AGENT_BROWSER_BIN, "/custom/agent-browser");
});

test("resolveManagedPaths does not trigger install when both binaries exist", async () => {
  let installerCalls = 0;
  const paths = await resolveManagedPaths({
    target: { platform: "darwin", arch: "arm64" },
    existsSync: () => true,
    ensureManagedBinariesFn: async () => {
      installerCalls += 1;
      return null;
    },
    quiet: true
  });

  assert.equal(installerCalls, 0);
  assert.match(paths.cvaExecutable, /vendor\/darwin\/arm64\/cva$/);
  assert.match(paths.managedAgentBrowser, /vendor\/darwin\/arm64\/agent-browser$/);
});

test("resolveManagedPaths uses installed managed binary paths when install runs", async () => {
  const paths = await resolveManagedPaths({
    target: { platform: "darwin", arch: "x64" },
    existsSync: () => false,
    ensureManagedBinariesFn: async () => ({
      cva: "/tmp/installed/cva",
      "agent-browser": "/tmp/installed/agent-browser"
    }),
    quiet: true
  });

  assert.equal(paths.cvaExecutable, "/tmp/installed/cva");
  assert.equal(paths.managedAgentBrowser, "/tmp/installed/agent-browser");
});
