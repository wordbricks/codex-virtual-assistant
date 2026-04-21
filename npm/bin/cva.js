#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");

const {
  MANAGED_BINARY,
  agentBrowserBinaryPath,
  binaryPath,
  currentTarget
} = require("../lib/platform");
const { ensureManagedBinaries } = require("../lib/install");

function createLaunchEnv(baseEnv, managedAgentBrowser) {
  const launchEnv = {
    ...baseEnv,
    ASSISTANT_AGENT_BROWSER_BIN: managedAgentBrowser
  };
  if (!launchEnv.CVA_AGENT_BROWSER_BIN) {
    launchEnv.CVA_AGENT_BROWSER_BIN = managedAgentBrowser;
  }
  return launchEnv;
}

async function resolveManagedPaths(options = {}) {
  const {
    target = currentTarget(),
    existsSync = fs.existsSync,
    ensureManagedBinariesFn = ensureManagedBinaries,
    quiet = false
  } = options;
  let cvaExecutable = binaryPath(target);
  let managedAgentBrowser = agentBrowserBinaryPath(target);

  if (!existsSync(cvaExecutable) || !existsSync(managedAgentBrowser)) {
    const installed = await ensureManagedBinariesFn({ quiet, target });
    if (installed) {
      cvaExecutable = installed[MANAGED_BINARY.CVA] || cvaExecutable;
      managedAgentBrowser = installed[MANAGED_BINARY.AGENT_BROWSER] || managedAgentBrowser;
    }
  }

  return { cvaExecutable, managedAgentBrowser };
}

async function main() {
  const { cvaExecutable, managedAgentBrowser } = await resolveManagedPaths({ quiet: false });
  if (!cvaExecutable || !managedAgentBrowser || !fs.existsSync(cvaExecutable) || !fs.existsSync(managedAgentBrowser)) {
    console.error("The managed cva and agent-browser binaries are not installed.");
    console.error("Try reinstalling the package or run with CVA_SKIP_DOWNLOAD unset.");
    process.exit(1);
  }

  const result = spawnSync(cvaExecutable, process.argv.slice(2), {
    stdio: "inherit",
    env: createLaunchEnv(process.env, managedAgentBrowser)
  });
  if (result.error) {
    console.error(result.error.message);
    process.exit(1);
  }
  process.exit(result.status === null ? 1 : result.status);
}

if (require.main === module) {
  main().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
}

module.exports = {
  createLaunchEnv,
  main,
  resolveManagedPaths
};
