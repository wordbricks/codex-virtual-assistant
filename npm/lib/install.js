const fs = require("node:fs");
const path = require("node:path");
const https = require("node:https");
const { pipeline } = require("node:stream/promises");

const {
  binaryPath,
  currentTarget,
  downloadURL,
  packageRoot
} = require("./platform");

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

async function ensureBinary(options = {}) {
  const { force = false, quiet = false } = options;
  if (process.env.CVA_SKIP_DOWNLOAD === "1") {
    if (!quiet) {
      console.warn("Skipping cva binary download because CVA_SKIP_DOWNLOAD=1.");
    }
    return null;
  }

  const target = currentTarget();
  const destination = binaryPath(target);
  if (!force && fs.existsSync(destination)) {
    return destination;
  }

  fs.mkdirSync(path.dirname(destination), { recursive: true });
  const url = downloadURL(target);
  if (!quiet) {
    console.log(`Downloading cva binary from ${url}`);
  }
  await downloadFile(url, destination);
  if (target.platform !== "win32") {
    fs.chmodSync(destination, 0o755);
  }
  return destination;
}

if (require.main === module) {
  ensureBinary()
    .catch((error) => {
      const relativeRoot = packageRoot();
      console.error(`Failed to install the native cva binary into ${relativeRoot}`);
      console.error(error.message);
      process.exit(1);
    });
}

module.exports = {
  downloadFile,
  ensureBinary
};
