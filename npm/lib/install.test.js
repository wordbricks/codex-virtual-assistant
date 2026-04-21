const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { PassThrough } = require("node:stream");
const https = require("node:https");

const {
  downloadFile,
  ensureBinary,
  ensureManagedBinaries
} = require("./install");
const { MANAGED_BINARY } = require("./platform");

test("downloadFile follows redirects without double-renaming the temp file", async () => {
  const originalGet = https.get;
  const calls = [];

  https.get = (url, callback) => {
    calls.push(url);
    const response = new PassThrough();
    if (calls.length === 1) {
      response.statusCode = 302;
      response.headers = { location: "https://example.com/final" };
      process.nextTick(() => {
        callback(response);
        response.end();
      });
    } else {
      response.statusCode = 200;
      response.headers = {};
      process.nextTick(() => {
        callback(response);
        response.end("binary-data");
      });
    }
    return {
      on() {
        return this;
      }
    };
  };

  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "cva-install-test-"));
  const destination = path.join(tempDir, "cva");

  try {
    await downloadFile("https://example.com/redirect", destination);
    assert.equal(fs.readFileSync(destination, "utf8"), "binary-data");
    assert.deepEqual(calls, [
      "https://example.com/redirect",
      "https://example.com/final"
    ]);
  } finally {
    https.get = originalGet;
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
});

test("ensureManagedBinaries downloads managed cva and agent-browser binaries", async () => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "cva-install-test-"));
  const cvaDestination = path.join(tempDir, "vendor", "darwin", "arm64", "cva");
  const agentBrowserDestination = path.join(
    tempDir,
    "vendor",
    "darwin",
    "arm64",
    "agent-browser"
  );
  const calls = [];

  try {
    const installed = await ensureManagedBinaries({
      quiet: true,
      target: { platform: "darwin", arch: "arm64" },
      managedBinaries: [
        {
          kind: MANAGED_BINARY.CVA,
          label: "cva",
          destination: cvaDestination,
          url: "https://example.com/cva"
        },
        {
          kind: MANAGED_BINARY.AGENT_BROWSER,
          label: "agent-browser",
          destination: agentBrowserDestination,
          url: "https://example.com/agent-browser"
        }
      ],
      download: async (url, destination) => {
        calls.push({ url, destination });
        fs.writeFileSync(destination, "binary");
      }
    });

    assert.deepEqual(calls, [
      { url: "https://example.com/cva", destination: cvaDestination },
      {
        url: "https://example.com/agent-browser",
        destination: agentBrowserDestination
      }
    ]);
    assert.equal(installed[MANAGED_BINARY.CVA], cvaDestination);
    assert.equal(installed[MANAGED_BINARY.AGENT_BROWSER], agentBrowserDestination);
    assert.ok((fs.statSync(cvaDestination).mode & 0o111) !== 0);
    assert.ok((fs.statSync(agentBrowserDestination).mode & 0o111) !== 0);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
});

test("ensureManagedBinaries skips existing binaries unless force is true", async () => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "cva-install-test-"));
  const cvaDestination = path.join(tempDir, "vendor", "darwin", "x64", "cva");
  const agentBrowserDestination = path.join(tempDir, "vendor", "darwin", "x64", "agent-browser");
  fs.mkdirSync(path.dirname(cvaDestination), { recursive: true });
  fs.writeFileSync(cvaDestination, "existing");
  fs.mkdirSync(path.dirname(agentBrowserDestination), { recursive: true });
  fs.writeFileSync(agentBrowserDestination, "existing");

  const calls = [];

  try {
    await ensureManagedBinaries({
      quiet: true,
      target: { platform: "darwin", arch: "x64" },
      managedBinaries: [
        {
          kind: MANAGED_BINARY.CVA,
          label: "cva",
          destination: cvaDestination,
          url: "https://example.com/cva"
        },
        {
          kind: MANAGED_BINARY.AGENT_BROWSER,
          label: "agent-browser",
          destination: agentBrowserDestination,
          url: "https://example.com/agent-browser"
        }
      ],
      download: async (url) => {
        calls.push(url);
      }
    });
    assert.equal(calls.length, 0);

    await ensureManagedBinaries({
      force: true,
      quiet: true,
      target: { platform: "darwin", arch: "x64" },
      managedBinaries: [
        {
          kind: MANAGED_BINARY.CVA,
          label: "cva",
          destination: cvaDestination,
          url: "https://example.com/cva"
        },
        {
          kind: MANAGED_BINARY.AGENT_BROWSER,
          label: "agent-browser",
          destination: agentBrowserDestination,
          url: "https://example.com/agent-browser"
        }
      ],
      download: async (url, destination) => {
        calls.push(url);
        fs.writeFileSync(destination, "forced");
      }
    });

    assert.deepEqual(calls, [
      "https://example.com/cva",
      "https://example.com/agent-browser"
    ]);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
});

test("ensureBinary returns managed cva binary path", async () => {
  const installed = await ensureBinary({
    quiet: true,
    target: { platform: "darwin", arch: "arm64" },
    managedBinaries: [
      {
        kind: MANAGED_BINARY.CVA,
        label: "cva",
        destination: "/tmp/cva",
        url: "https://example.com/cva"
      },
      {
        kind: MANAGED_BINARY.AGENT_BROWSER,
        label: "agent-browser",
        destination: "/tmp/agent-browser",
        url: "https://example.com/agent-browser"
      }
    ],
    download: async (_url, destination) => {
      fs.mkdirSync(path.dirname(destination), { recursive: true });
      fs.writeFileSync(destination, "binary");
    }
  });

  assert.equal(installed, "/tmp/cva");
});

test("ensureManagedBinaries reports unsupported platform with macOS-only guidance", async () => {
  const originalPlatform = os.platform;
  const originalArch = os.arch;
  os.platform = () => "linux";
  os.arch = () => "x64";

  try {
    await assert.rejects(
      () => ensureManagedBinaries({ quiet: true }),
      /supports only darwin\/x64 and darwin\/arm64/
    );
  } finally {
    os.platform = originalPlatform;
    os.arch = originalArch;
  }
});
