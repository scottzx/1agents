"use strict";
const { spawnSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const cliDir = path.join(__dirname, "..", "happy-cli");
if (!fs.existsSync(path.join(cliDir, "package.json"))) {
  console.warn("[@1agents/happy] happy-cli missing; skip");
  process.exit(0);
}
if (fs.existsSync(path.join(cliDir, "node_modules"))) {
  process.exit(0);
}
const hasLock = fs.existsSync(path.join(cliDir, "package-lock.json"));
const args = hasLock
  ? ["ci", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund"]
  : ["install", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund"];
console.log(`[@1agents/happy] npm ${args[0]} in happy-cli`);
const r = spawnSync("npm", args, {
  cwd: cliDir,
  stdio: "inherit",
  shell: process.platform === "win32",
});
if (r.status !== 0) {
  console.warn("[@1agents/happy] dependency install failed (optional)");
  process.exit(0);
}
const nm = path.join(cliDir, "node_modules", "@anthropic-ai");
if (fs.existsSync(nm)) {
  for (const ent of fs.readdirSync(nm)) {
    if (ent.startsWith("claude-agent-sdk-")) {
      fs.rmSync(path.join(nm, ent), { recursive: true, force: true });
    }
  }
}
