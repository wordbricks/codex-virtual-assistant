const fs = require("node:fs");
const path = require("node:path");
const https = require("node:https");
const { pipeline } = require("node:stream/promises");

const {
  MANAGED_BINARY,
  agentBrowserBinaryPath,
  agentBrowserDownloadURL,
  binaryPath,
  currentTarget,
  downloadURL,
  packageRoot
} = require("./platform");

function unsupportedPlatformInstallError(error) {
  if (!(error instanceof Error) || !error.message.startsWith("Unsupported platform for cva:")) {
    return error;
  }

  return new Error(
    `${error.message}. The @wordbricks/cva npm package supports only darwin/x64 and darwin/arm64.`
  );
}

function defaultManagedBinaries(target) {
  return [
    {
      kind: MANAGED_BINARY.CVA,
      label: "cva",
      destination: binaryPath(target),
      url: downloadURL(target)
    },
    {
      kind: MANAGED_BINARY.AGENT_BROWSER,
      label: "agent-browser",
      destination: agentBrowserBinaryPath(target),
      url: agentBrowserDownloadURL(target)
    }
  ];
}

async function downloadFile(url, destination) {
  const tempFile = `${destination}.tmp`;
  await new Promise((resolve, reject) => {
    const request = https.get(url, (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        response.resume();
        resolve(downloadFile(response.headers.location, destination));
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        reject(new Error(`Download failed with HTTP ${response.statusCode}: ${url}`));
        return;
      }

      const output = fs.createWriteStream(tempFile, { mode: 0o755 });
      pipeline(response, output)
        .then(() => {
          fs.renameSync(tempFile, destination);
          resolve();
        })
        .catch(reject);
    });
    request.on("error", reject);
  });
}

async function ensureManagedBinaries(options = {}) {
  const {
    force = false,
    quiet = false,
    target: explicitTarget,
    managedBinaries,
    download = downloadFile
  } = options;

  if (process.env.CVA_SKIP_DOWNLOAD === "1") {
    if (!quiet) {
      console.warn("Skipping managed cva binary downloads because CVA_SKIP_DOWNLOAD=1.");
    }
    return null;
  }

  let target = explicitTarget;
  if (!target) {
    try {
      target = currentTarget();
    } catch (error) {
      throw unsupportedPlatformInstallError(error);
    }
  }

  const binaries = managedBinaries || defaultManagedBinaries(target);
  const installed = {};

  for (const binary of binaries) {
    installed[binary.kind] = binary.destination;

    if (!force && fs.existsSync(binary.destination)) {
      continue;
    }

    fs.mkdirSync(path.dirname(binary.destination), { recursive: true });
    if (!quiet) {
      console.log(`Downloading managed ${binary.label} binary from ${binary.url}`);
    }
    await download(binary.url, binary.destination);
    if (target.platform !== "win32") {
      fs.chmodSync(binary.destination, 0o755);
    }
  }

  return installed;
}

async function ensureBinary(options = {}) {
  const installed = await ensureManagedBinaries(options);
  if (!installed) {
    return null;
  }
  return installed[MANAGED_BINARY.CVA] || null;
}

if (require.main === module) {
  ensureManagedBinaries()
    .catch((error) => {
      const relativeRoot = packageRoot();
      console.error(`Failed to install managed cva binaries into ${relativeRoot}`);
      console.error(error.message);
      process.exit(1);
    });
}

module.exports = {
  downloadFile,
  ensureBinary,
  ensureManagedBinaries,
  unsupportedPlatformInstallError
};
