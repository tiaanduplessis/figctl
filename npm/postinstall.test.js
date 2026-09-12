"use strict";

const assert = require("node:assert/strict");
const { test } = require("node:test");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");
const zlib = require("node:zlib");
const { spawnSync } = require("node:child_process");
const { verify, extractTarGz, extractZip } = require("./postinstall");

function tar(name, data) {
  const header = Buffer.alloc(512);
  header.write(name);
  header.write(data.length.toString(8).padStart(11, "0"), 124);
  header[156] = 48;
  return zlib.gzipSync(Buffer.concat([header, data, Buffer.alloc(512)]));
}

function zip(name, data, method = 8) {
  const compressed = method === 8 ? zlib.deflateRawSync(data) : data;
  const local = Buffer.alloc(30);
  local.writeUInt32LE(0x04034b50);
  local.writeUInt16LE(name.length, 26);
  const entry = Buffer.concat([local, Buffer.from(name), compressed]);
  const central = Buffer.alloc(46);
  central.writeUInt32LE(0x02014b50);
  central.writeUInt16LE(method, 10);
  central.writeUInt32LE(compressed.length, 20);
  central.writeUInt16LE(name.length, 28);
  const directory = Buffer.concat([central, Buffer.from(name)]);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50);
  end.writeUInt16LE(1, 10);
  end.writeUInt32LE(entry.length, 16);
  return Buffer.concat([entry, directory, end]);
}

function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "figctl-npm-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(path.join(dir, "bin"));
  for (const file of ["postinstall.js", "package.json", "bin/figctl.js"]) {
    fs.copyFileSync(path.join(__dirname, file), path.join(dir, file));
  }
  return dir;
}

function run(dir, script, env = {}, args = []) {
  const cleanEnv = { ...process.env };
  for (const key of ["FIGCTL_BINARY", "FIGCTL_SKIP_DOWNLOAD", "FIGCTL_DOWNLOAD_BASE"]) {
    delete cleanEnv[key];
  }
  return spawnSync(process.execPath, [path.join(dir, script), ...args], {
    env: { ...cleanEnv, ...env }, encoding: "utf8",
  });
}

test("checksum verification rejects missing and corrupted archives", () => {
  const archive = Buffer.from("release archive");
  const checksums = Buffer.from(crypto.createHash("sha256").update(archive).digest("hex") + "  archive.tar.gz\n");
  verify(checksums, "archive.tar.gz", archive);
  assert.throws(() => verify(checksums, "missing.tar.gz", archive), /not listed/);
  assert.throws(() => verify(checksums, "archive.tar.gz", Buffer.from("corrupt")), /checksum mismatch/);
});

test("tar extraction accepts regular binaries and rejects truncated entries", () => {
  const data = Buffer.from("executable");
  assert.deepEqual(extractTarGz(tar("figctl", data), "figctl"), data);
  assert.equal(extractTarGz(tar("README.md", data), "figctl"), null);
  const malformed = zlib.gunzipSync(tar("figctl", data));
  malformed.write("00000077777", 124);
  assert.throws(() => extractTarGz(zlib.gzipSync(malformed), "figctl"), /truncated/);
  assert.throws(() => extractTarGz(Buffer.from("invalid"), "figctl"));
});

test("zip extraction handles stored and deflated binaries and rejects malformed data", () => {
  const data = Buffer.from("windows executable");
  for (const method of [0, 8]) {
    assert.deepEqual(extractZip(zip("figctl.exe", data, method), "figctl.exe"), data);
  }
  assert.equal(extractZip(zip("README.md", data), "figctl.exe"), null);
  assert.equal(extractZip(Buffer.from("invalid"), "figctl.exe"), null);
  const malformed = zip("figctl.exe", data);
  malformed.writeUInt32LE(0, 0);
  assert.throws(() => extractZip(malformed, "figctl.exe"), /invalid zip entry/);
});

test("skip-download installs no binary and tells users how to supply one", (t) => {
  const dir = fixture(t);
  const result = run(dir, "postinstall.js", { FIGCTL_SKIP_DOWNLOAD: "1" });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stderr, /FIGCTL_BINARY/);
  const shim = run(dir, "bin/figctl.js");
  assert.equal(shim.status, 1);
  assert.match(shim.stderr, /FIGCTL_BINARY/);
});

test("binary override installs an executable and shim forwards arguments and exit status", (t) => {
  const dir = fixture(t);
  const install = run(dir, "postinstall.js", { FIGCTL_BINARY: process.execPath });
  assert.equal(install.status, 0, install.stderr);
  const shim = run(dir, "bin/figctl.js", {}, ["-e", "console.log(process.argv[1]); process.exit(7)", "forwarded"]);
  assert.equal(shim.status, 7, shim.stderr);
  assert.equal(shim.stdout.trim(), "forwarded");
});

test("runtime override works after skipping download and errors on an invalid path", (t) => {
  const dir = fixture(t);
  const shim = run(dir, "bin/figctl.js", { FIGCTL_BINARY: process.execPath }, ["--version"]);
  assert.equal(shim.status, 0, shim.stderr);
  assert.equal(shim.stdout.trim(), process.version);
  const invalid = run(dir, "postinstall.js", { FIGCTL_BINARY: path.join(dir, "missing") });
  assert.equal(invalid.status, 1);
  assert.match(invalid.stderr, /installation failed/);
});

test("download installation verifies archives before writing an executable", (t) => {
  for (const scenario of ["valid", "corrupt", "missing-binary"]) {
    const dir = fixture(t);
    const windows = process.platform === "win32";
    const binaryName = windows ? "figctl.exe" : "figctl";
    const entryName = scenario === "missing-binary" ? "README.md" : binaryName;
    const archive = windows ? zip(entryName, Buffer.from("binary")) : tar(entryName, Buffer.from("binary"));
    const releaseOS = windows ? "windows" : process.platform;
    const arch = process.arch === "x64" ? "amd64" : process.arch;
    const asset = `figctl_${require("./package.json").version}_${releaseOS}_${arch}.${windows ? "zip" : "tar.gz"}`;
    const digest = scenario === "corrupt" ? "0".repeat(64) : crypto.createHash("sha256").update(archive).digest("hex");
    fs.writeFileSync(path.join(dir, "archive"), archive);
    fs.writeFileSync(path.join(dir, "checksums.txt"), `${digest}  ${asset}\n`);
    // Replace HTTPS only inside the child process; no network or global state.
    fs.writeFileSync(path.join(dir, "fake-https.cjs"), `
      const { EventEmitter } = require("node:events");
      const fs = require("node:fs");
      const path = require("node:path");
      require("node:https").get = (url, options, callback) => {
        const request = new EventEmitter();
        request.setTimeout = () => {};
        process.nextTick(() => {
          const response = new EventEmitter();
          response.statusCode = 200;
          response.headers = {};
          callback(response);
          const file = url.endsWith("checksums.txt") ? "checksums.txt" : "archive";
          response.emit("data", fs.readFileSync(path.join(__dirname, file)));
          response.emit("end");
        });
        return request;
      };
    `);
    const env = { ...process.env };
    delete env.FIGCTL_SKIP_DOWNLOAD;
    delete env.FIGCTL_BINARY;
    const result = spawnSync(process.execPath, ["--require", path.join(dir, "fake-https.cjs"), path.join(dir, "postinstall.js")], { env, encoding: "utf8" });
    const dest = path.join(dir, "bin", binaryName);
    if (scenario === "valid") {
      assert.equal(result.status, 0, result.stderr);
      assert.equal(fs.readFileSync(dest, "utf8"), "binary");
    } else {
      assert.equal(result.status, 1);
      assert.equal(fs.existsSync(dest), false);
      assert.match(result.stderr, scenario === "corrupt" ? /checksum mismatch/ : /not found inside/);
    }
  }
});
