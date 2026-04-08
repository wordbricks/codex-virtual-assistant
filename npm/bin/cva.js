#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");

const { binaryPath, currentTarget } = require("../lib/platform");
const { ensureBinary } = require("../lib/install");

async function main() {
  const target = currentTarget();
  let executable = binaryPath(target);
  if (!fs.existsSync(executable)) {
    executable = await ensureBinary({ quiet: false });
  }
  if (!executable || !fs.existsSync(executable)) {
    console.error("The native cva binary is not installed.");
    console.error("Try reinstalling the package or run with CVA_SKIP_DOWNLOAD unset.");
    process.exit(1);
  }

  const result = spawnSync(executable, process.argv.slice(2), {
    stdio: "inherit",
    env: process.env
  });
  if (result.error) {
    console.error(result.error.message);
    process.exit(1);
  }
  process.exit(result.status === null ? 1 : result.status);
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
