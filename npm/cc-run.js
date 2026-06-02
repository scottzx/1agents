#!/usr/bin/env node

"use strict";

const { execFileSync, execSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const packageDir = __dirname;

function getPlatformDir() {
  const platform = process.platform;
  const arch = process.arch;

  if (platform === "darwin" && arch === "arm64") {
    return "darwin-arm64";
  }
  if (platform === "linux") {
    if (arch === "x64") return "linux-amd64";
    if (arch === "arm64") return "linux-arm64";
  }
  throw new Error(`Unsupported platform: ${platform}/${arch}. Only macOS (arm64) and Linux (amd64/arm64) are supported.`);
}

let platformDir;
try {
  platformDir = getPlatformDir();
} catch (err) {
  console.error(`❌ [1agent] ${err.message}`);
  process.exit(1);
}

const myBinDir = path.join(packageDir, "bin", platformDir);
const ext = process.platform === "win32" ? ".exe" : "";
let ccPath = path.join(myBinDir, "cc-connect" + ext);

// ── Smart Compatibility: Delegate to standalone global cc-connect if it is newer/installed ──
try {
  const globalPrefix = execSync("npm config get prefix", { encoding: "utf8" }).trim();
  const standaloneCCPath = process.platform === "win32"
    ? path.join(globalPrefix, "node_modules", "cc-connect", "bin", "cc-connect.exe")
    : path.join(globalPrefix, "lib", "node_modules", "cc-connect", "bin", "cc-connect");

  if (fs.existsSync(standaloneCCPath)) {
    // If standalone global cc-connect exists, use it to ensure absolute consistency and latest version
    ccPath = standaloneCCPath;
  }
} catch (e) {
  // Silent fallback to our bundled binary if detection fails or throws
}

// ── Fallback Execution ──
if (!fs.existsSync(ccPath)) {
  console.error(`❌ [1agent] cc-connect CLI binary is missing in ${myBinDir}. Please make sure the package was installed correctly.`);
  process.exit(1);
}

if (process.platform !== "win32" && ccPath.startsWith(myBinDir)) {
  try {
    fs.chmodSync(ccPath, 0o755);
  } catch (e) {}
}

try {
  execFileSync(ccPath, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  process.exit(err.status || 1);
}
