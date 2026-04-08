const os = require("node:os");
const path = require("node:path");

function packageRoot() {
  return path.resolve(__dirname, "..");
}

function readPackageMetadata() {
  // eslint-disable-next-line global-require, import/no-dynamic-require
  return require(path.join(packageRoot(), "package.json"));
}

function currentTarget() {
  const platform = os.platform();
  const arch = os.arch();

  const supported = new Set([
    "linux:x64",
    "linux:arm64",
    "darwin:x64",
    "darwin:arm64",
    "win32:x64",
    "win32:arm64"
  ]);

  if (!supported.has(`${platform}:${arch}`)) {
    throw new Error(`Unsupported platform for cva: ${platform}/${arch}`);
  }

  return { platform, arch };
}

function executableName(platform) {
  return platform === "win32" ? "cva.exe" : "cva";
}

function assetName(target) {
  const metadata = readPackageMetadata();
  const prefix = metadata.cva && metadata.cva.assetPrefix ? metadata.cva.assetPrefix : "cva";
  return `${prefix}-${target.platform}-${target.arch}${target.platform === "win32" ? ".exe" : ""}`;
}

function binaryPath(target = currentTarget()) {
  return path.join(packageRoot(), "vendor", target.platform, target.arch, executableName(target.platform));
}

function releaseTag(metadata = readPackageMetadata()) {
  return `v${metadata.version}`;
}

function downloadURL(target = currentTarget(), metadata = readPackageMetadata()) {
  const owner = metadata.cva && metadata.cva.owner ? metadata.cva.owner : "wordbricks";
  const repo = metadata.cva && metadata.cva.repo ? metadata.cva.repo : "codex-virtual-assistant";
  return `https://github.com/${owner}/${repo}/releases/download/${releaseTag(metadata)}/${assetName(target)}`;
}

module.exports = {
  assetName,
  binaryPath,
  currentTarget,
  downloadURL,
  executableName,
  packageRoot,
  readPackageMetadata,
  releaseTag
};
