#!/usr/bin/env node

"use strict";

const { execFileSync, execSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const PACKAGE = require("./package.json");
const NAME = "remote-agents";
const packageDir = __dirname;
const binDir = path.join(packageDir, "bin");
const ext = process.platform === "win32" ? ".exe" : "";
const agentPath = path.join(binDir, NAME + ext);
const ttydPath = path.join(binDir, "ttyd" + ext);

function needsInstall() {
  if (!fs.existsSync(agentPath)) return true;
  if (!fs.existsSync(ttydPath)) return true;
  return false;
}

if (needsInstall()) {
  console.log(`[remote-agent] Binaries missing, running installer...`);
  try {
    execSync("node " + JSON.stringify(path.join(packageDir, "install.js")), {
      stdio: "inherit",
      cwd: packageDir,
    });
  } catch (err) {
    console.error("[remote-agent] Auto-install failed. Please run manually: npm rebuild");
    process.exit(1);
  }
}

// Prepare execution arguments
const userArgs = process.argv.slice(2);
const finalArgs = [];

// Inject absolute path to ttyd binary if not explicitly provided by the user
if (!userArgs.some(arg => arg.startsWith("-ttyd-bin"))) {
  finalArgs.push("-ttyd-bin", ttydPath);
}

// Inject absolute path to frontend dist directory if not explicitly provided by the user
if (!userArgs.some(arg => arg.startsWith("-static"))) {
  const staticPath = path.join(packageDir, "dist");
  finalArgs.push("-static", staticPath);
}

// Append all original user arguments
finalArgs.push(...userArgs);

// Execute the Go remote-agent binary
try {
  execFileSync(agentPath, finalArgs, { stdio: "inherit" });
} catch (err) {
  process.exit(err.status || 1);
}
