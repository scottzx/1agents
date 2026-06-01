#!/usr/bin/env node

"use strict";

const { execFileSync, execSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const packageDir = __dirname;
const myBinDir = path.join(packageDir, "bin");
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
  console.log(`[1agent] cc-connect CLI binary missing, running installer...`);
  try {
    execSync("node " + JSON.stringify(path.join(packageDir, "install.js")), {
      stdio: "inherit",
      cwd: packageDir,
    });
  } catch (err) {
    console.error("[1agent] Auto-install failed. Please run manually: npm rebuild");
    process.exit(1);
  }
}

try {
  execFileSync(ccPath, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  process.exit(err.status || 1);
}
