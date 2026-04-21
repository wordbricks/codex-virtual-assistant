const os = require("node:os");
const path = require("node:path");

const MANAGED_BINARY = Object.freeze({
  CVA: "cva",
  AGENT_BROWSER: "agent-browser"
});

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

  const supported = new Set(["darwin:x64", "darwin:arm64"]);

  if (!supported.has(`${platform}:${arch}`)) {
    throw new Error(`Unsupported platform for cva: ${platform}/${arch}`);
  }

  return { platform, arch };
}

function executableName(platform) {
  return platform === "win32" ? "cva.exe" : "cva";
}

function agentBrowserExecutableName(platform) {
  return platform === "win32" ? "agent-browser.exe" : "agent-browser";
}

function assetName(target, metadata = readPackageMetadata()) {
  const prefix = metadata.cva && metadata.cva.assetPrefix ? metadata.cva.assetPrefix : "cva";
  return `${prefix}-${target.platform}-${target.arch}${target.platform === "win32" ? ".exe" : ""}`;
}

function agentBrowserAssetName(target, metadata = readPackageMetadata()) {
  const prefix =
    metadata.cva && metadata.cva.agentBrowserAssetPrefix
      ? metadata.cva.agentBrowserAssetPrefix
      : "agent-browser";
  return `${prefix}-${target.platform}-${target.arch}${target.platform === "win32" ? ".exe" : ""}`;
}

function binaryPath(target = currentTarget()) {
  return path.join(packageRoot(), "vendor", target.platform, target.arch, executableName(target.platform));
}

function agentBrowserBinaryPath(target = currentTarget()) {
  return path.join(
    packageRoot(),
    "vendor",
    target.platform,
    target.arch,
    agentBrowserExecutableName(target.platform)
  );
}

function releaseTag(metadata = readPackageMetadata()) {
  return `v${metadata.version}`;
}

function downloadURL(target = currentTarget(), metadata = readPackageMetadata()) {
  const owner = metadata.cva && metadata.cva.owner ? metadata.cva.owner : "wordbricks";
  const repo = metadata.cva && metadata.cva.repo ? metadata.cva.repo : "codex-virtual-assistant";
  return `https://github.com/${owner}/${repo}/releases/download/${releaseTag(metadata)}/${assetName(target, metadata)}`;
}

function agentBrowserDownloadURL(target = currentTarget(), metadata = readPackageMetadata()) {
  const owner = metadata.cva && metadata.cva.owner ? metadata.cva.owner : "wordbricks";
  const repo = metadata.cva && metadata.cva.repo ? metadata.cva.repo : "codex-virtual-assistant";
  return `https://github.com/${owner}/${repo}/releases/download/${releaseTag(metadata)}/${agentBrowserAssetName(target, metadata)}`;
}

module.exports = {
  MANAGED_BINARY,
  agentBrowserAssetName,
  agentBrowserBinaryPath,
  agentBrowserDownloadURL,
  agentBrowserExecutableName,
  assetName,
  binaryPath,
  currentTarget,
  downloadURL,
  executableName,
  packageRoot,
  readPackageMetadata,
  releaseTag
};
