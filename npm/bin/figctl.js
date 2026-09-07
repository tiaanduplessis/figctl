#!/usr/bin/env node
// Thin shim: run the figctl binary that postinstall.js downloaded next to
// this file, forwarding arguments, stdio, signals, and the exit code.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const binary = path.join(
  __dirname,
  process.platform === "win32" ? "figctl.exe" : "figctl"
);

if (!fs.existsSync(binary)) {
  process.stderr.write(
    [
      "",
      "figctl: the binary was not downloaded during install.",
      "",
      "Re-run the install, or get figctl another way:",
      "  npm rebuild figctl",
      "  go install github.com/tiaanduplessis/figctl/cmd/figctl@latest",
      "  brew install tiaanduplessis/tap/figctl",
      "  https://github.com/tiaanduplessis/figctl/releases",
      "",
    ].join("\n") + "\n"
  );
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  process.stderr.write("figctl: " + result.error.message + "\n");
  process.exit(1);
}
if (result.signal) {
  process.kill(process.pid, result.signal);
}
process.exit(result.status === null ? 1 : result.status);
