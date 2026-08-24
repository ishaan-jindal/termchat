#!/usr/bin/env node
"use strict";

const { spawnSync } = require("child_process");
const path = require("path");

// npm platform/arch -> Go target name used in binary filenames.
const targets = {
  "linux-x64": "linux-amd64",
  "linux-arm64": "linux-arm64",
  "linux-ia32": "linux-386",
  "darwin-x64": "darwin-amd64",
  "darwin-arm64": "darwin-arm64",
  "win32-x64": "windows-amd64",
  "win32-arm64": "windows-arm64",
};

const key = `${process.platform}-${process.arch}`;
const pkg = `@sacredcat/termchat-${key}`;
const exe = process.platform === "win32" ? "termchat.exe" : "termchat";

let bin;
try {
  bin = require.resolve(path.join(pkg, exe));
} catch {
  console.error(`termchat: no build available for ${key}.`);
  console.error(
    "Grab a binary directly from https://github.com/ishaan-jindal/termchat/releases"
  );
  process.exit(1);
}

if (require.main !== module) {
  module.exports = bin;
  return;
}

const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  console.error(`termchat: failed to launch ${bin}: ${result.error.message}`);
  process.exit(1);
}

if (result.signal) {
  // Mirror the child's death signal so shell callers see the same status.
  process.kill(process.pid, result.signal);
  process.exit(1);
}

process.exit(result.status ?? 1);
