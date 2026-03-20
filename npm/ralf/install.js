#!/usr/bin/env node
"use strict";

const { existsSync, mkdirSync, copyFileSync, chmodSync } = require("fs");
const { join, dirname } = require("path");

const PLATFORMS = {
  "darwin-arm64": "ralf-darwin-arm64",
  "darwin-x64": "ralf-darwin-x64",
  "linux-x64": "ralf-linux-x64",
  "linux-arm64": "ralf-linux-arm64",
};

const key = `${process.platform}-${process.arch}`;
const pkg = PLATFORMS[key];

if (!pkg) {
  console.error(
    `ralf: unsupported platform ${key}. ` +
    `Download manually from https://github.com/Hideart/ralf/releases`
  );
  process.exit(0); // Don't fail install for unsupported platforms
}

// Resolve the platform binary from node_modules.
const candidates = [
  join(__dirname, "node_modules", pkg, "bin", "ralf"),
  join(dirname(__dirname), pkg, "bin", "ralf"),
];

let src = null;
for (const c of candidates) {
  if (existsSync(c)) {
    src = c;
    break;
  }
}

if (!src) {
  console.error(
    `ralf: platform package ${pkg} not found. ` +
    `Try: npm install ${pkg}`
  );
  process.exit(0);
}

const dest = join(__dirname, "bin", "ralf");
mkdirSync(dirname(dest), { recursive: true });
copyFileSync(src, dest);
chmodSync(dest, 0o755);
