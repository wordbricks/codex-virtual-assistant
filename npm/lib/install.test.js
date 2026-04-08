const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { PassThrough } = require("node:stream");
const https = require("node:https");

const { downloadFile } = require("./install");

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
