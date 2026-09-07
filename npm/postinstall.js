#!/usr/bin/env node
// Downloads the figctl release binary for this platform, verifies it against
// the release checksums.txt, and unpacks it next to this script.
//
// No dependencies: node:https, node:fs, node:zlib, and node:crypto only.
//
// Environment overrides:
//   FIGCTL_SKIP_DOWNLOAD=1   skip entirely (the shim then needs figctl on PATH)
//   FIGCTL_BINARY=/path      use an existing binary instead of downloading
//   FIGCTL_DOWNLOAD_BASE=URL download from somewhere other than GitHub releases

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const zlib = require("node:zlib");
const https = require("node:https");
const crypto = require("node:crypto");

const pkg = require("./package.json");

// Every GoReleaser target in .goreleaser.yml. Adding a target there means
// adding it here; a Go test asserts the two agree.
const PLATFORMS = {
  "darwin-x64": { os: "darwin", arch: "amd64", ext: "tar.gz", bin: "figctl" },
  "darwin-arm64": { os: "darwin", arch: "arm64", ext: "tar.gz", bin: "figctl" },
  "linux-x64": { os: "linux", arch: "amd64", ext: "tar.gz", bin: "figctl" },
  "linux-arm64": { os: "linux", arch: "arm64", ext: "tar.gz", bin: "figctl" },
  "win32-x64": { os: "windows", arch: "amd64", ext: "zip", bin: "figctl.exe" },
  "win32-arm64": { os: "windows", arch: "arm64", ext: "zip", bin: "figctl.exe" },
};

const REPO = "tiaanduplessis/figctl";
const VERSION = pkg.version;
const TAG = "v" + VERSION;
const BASE =
  process.env.FIGCTL_DOWNLOAD_BASE ||
  `https://github.com/${REPO}/releases/download/${TAG}`;

function fail(message, detail) {
  const lines = ["", "figctl: " + message];
  if (detail) {
    lines.push("");
    lines.push(detail);
  }
  lines.push("");
  lines.push("Install it another way instead:");
  lines.push("  go install github.com/tiaanduplessis/figctl/cmd/figctl@latest");
  lines.push("  brew install tiaanduplessis/tap/figctl");
  lines.push(`  https://github.com/${REPO}/releases/tag/${TAG}`);
  lines.push("");
  process.stderr.write(lines.join("\n") + "\n");
  process.exit(1);
}

function platformKey() {
  return `${process.platform}-${process.arch}`;
}

function target() {
  const key = platformKey();
  const t = PLATFORMS[key];
  if (!t) {
    fail(
      `no prebuilt binary for ${process.platform} ${process.arch}`,
      "Supported platforms: " + Object.keys(PLATFORMS).sort().join(", ")
    );
  }
  return t;
}

// get follows redirects and resolves with the response body as a Buffer.
function get(url, redirects = 0) {
  return new Promise((resolve, reject) => {
    if (redirects > 10) {
      reject(new Error("too many redirects for " + url));
      return;
    }
    const req = https.get(
      url,
      { headers: { "user-agent": `figctl-npm/${VERSION}` } },
      (res) => {
        const status = res.statusCode || 0;
        if (status >= 300 && status < 400 && res.headers.location) {
          res.resume();
          resolve(get(new URL(res.headers.location, url).toString(), redirects + 1));
          return;
        }
        if (status !== 200) {
          res.resume();
          reject(new Error(`HTTP ${status} for ${url}`));
          return;
        }
        const chunks = [];
        res.on("data", (c) => chunks.push(c));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      }
    );
    req.on("error", reject);
    req.setTimeout(120000, () => {
      req.destroy(new Error("timed out after 120s downloading " + url));
    });
  });
}

// verify checks the archive against the sha256 line for it in checksums.txt.
function verify(checksums, assetName, archive) {
  let want = null;
  for (const line of checksums.toString("utf8").split("\n")) {
    const m = line.trim().match(/^([0-9a-f]{64})\s+\*?(.+)$/);
    if (m && m[2] === assetName) {
      want = m[1];
      break;
    }
  }
  if (!want) {
    fail(
      `${assetName} is not listed in checksums.txt`,
      "The release assets look incomplete. This may be a partially published release."
    );
  }
  const got = crypto.createHash("sha256").update(archive).digest("hex");
  if (got !== want) {
    fail(
      "checksum mismatch for " + assetName,
      `expected sha256 ${want}\nreceived sha256 ${got}\n\nThe download was corrupted or tampered with. Nothing was installed.`
    );
  }
}

// extractTarGz returns the contents of one file from a gzipped tar archive.
// Only the ustar fields figctl's archives use are read.
function extractTarGz(buf, name) {
  const tar = zlib.gunzipSync(buf);
  let offset = 0;
  while (offset + 512 <= tar.length) {
    const header = tar.subarray(offset, offset + 512);
    if (header.every((b) => b === 0)) {
      break;
    }
    const entry = header.subarray(0, 100).toString("utf8").replace(/\0.*$/, "");
    const sizeField = header.subarray(124, 136).toString("utf8").replace(/\0.*$/, "").trim();
    const size = parseInt(sizeField, 8) || 0;
    const typeFlag = String.fromCharCode(header[156]);
    const start = offset + 512;
    if ((typeFlag === "0" || typeFlag === "\0") && path.posix.basename(entry) === name) {
      return tar.subarray(start, start + size);
    }
    offset = start + Math.ceil(size / 512) * 512;
  }
  return null;
}

// extractZip returns the contents of one file from a zip archive, reading the
// end-of-central-directory record and inflating the entry it points at.
function extractZip(buf, name) {
  let eocd = -1;
  for (let i = buf.length - 22; i >= 0 && i >= buf.length - 65557; i--) {
    if (buf.readUInt32LE(i) === 0x06054b50) {
      eocd = i;
      break;
    }
  }
  if (eocd < 0) {
    return null;
  }
  const count = buf.readUInt16LE(eocd + 10);
  let p = buf.readUInt32LE(eocd + 16);
  for (let i = 0; i < count; i++) {
    if (buf.readUInt32LE(p) !== 0x02014b50) {
      return null;
    }
    const method = buf.readUInt16LE(p + 10);
    const compressedSize = buf.readUInt32LE(p + 20);
    const nameLen = buf.readUInt16LE(p + 28);
    const extraLen = buf.readUInt16LE(p + 30);
    const commentLen = buf.readUInt16LE(p + 32);
    const localOffset = buf.readUInt32LE(p + 42);
    const entry = buf.subarray(p + 46, p + 46 + nameLen).toString("utf8");
    if (path.posix.basename(entry) === name) {
      const localNameLen = buf.readUInt16LE(localOffset + 26);
      const localExtraLen = buf.readUInt16LE(localOffset + 28);
      const start = localOffset + 30 + localNameLen + localExtraLen;
      const data = buf.subarray(start, start + compressedSize);
      if (method === 0) {
        return data;
      }
      if (method === 8) {
        return zlib.inflateRawSync(data);
      }
      return null;
    }
    p += 46 + nameLen + extraLen + commentLen;
  }
  return null;
}

async function main() {
  if (process.env.FIGCTL_SKIP_DOWNLOAD) {
    process.stderr.write(
      "figctl: FIGCTL_SKIP_DOWNLOAD is set; figctl must already be on PATH\n"
    );
    return;
  }

  const t = target();
  const dest = path.join(__dirname, "bin", t.bin);

  if (process.env.FIGCTL_BINARY) {
    fs.copyFileSync(process.env.FIGCTL_BINARY, dest);
    fs.chmodSync(dest, 0o755);
    return;
  }

  const assetName = `figctl_${VERSION}_${t.os}_${t.arch}.${t.ext}`;
  let archive;
  let checksums;
  try {
    [archive, checksums] = await Promise.all([
      get(`${BASE}/${assetName}`),
      get(`${BASE}/checksums.txt`),
    ]);
  } catch (err) {
    fail("could not download " + assetName, String(err && err.message ? err.message : err));
  }

  verify(checksums, assetName, archive);

  let binary;
  try {
    binary = t.ext === "zip" ? extractZip(archive, t.bin) : extractTarGz(archive, t.bin);
  } catch (err) {
    fail("could not unpack " + assetName, String(err && err.message ? err.message : err));
  }
  if (!binary || binary.length === 0) {
    fail(`${t.bin} was not found inside ${assetName}`);
  }

  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.writeFileSync(dest, binary, { mode: 0o755 });
  fs.chmodSync(dest, 0o755);
}

main().catch((err) => {
  fail("installation failed", String(err && err.stack ? err.stack : err));
});
