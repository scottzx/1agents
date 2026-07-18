#!/usr/bin/env node
"use strict";

const { execFileSync, execSync } = require("child_process");
const fs = require("fs");
const path = require("path");

function resolveCcConnect() {
  // Prefer @1agents/cc-connect meta package resolver
  try {
    const { resolveBinary } = require("@1agents/cc-connect");
    return resolveBinary();
  } catch (err) {
    // Fall through to standalone global cc-connect if present
  }

  try {
    const globalPrefix = execSync("npm config get prefix", {
      encoding: "utf8",
    }).trim();
    const standalone =
      process.platform === "win32"
        ? path.join(globalPrefix, "node_modules", "cc-connect", "bin", "cc-connect.exe")
        : path.join(
            globalPrefix,
            "lib",
            "node_modules",
            "cc-connect",
            "bin",
            "cc-connect"
          );
    if (fs.existsSync(standalone)) return standalone;
  } catch (_) {}

  throw new Error(
    "cc-connect binary not found. Ensure @1agents/cc-connect (and its platform package) are installed from the npm registry."
  );
}

let ccPath;
try {
  ccPath = resolveCcConnect();
} catch (err) {
  console.error(`❌ [1agents] ${err.message}`);
  process.exit(1);
}

if (process.platform !== "win32") {
  try {
    fs.chmodSync(ccPath, 0o755);
  } catch (_) {}
}

try {
  execFileSync(ccPath, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  process.exit(err.status || 1);
}
