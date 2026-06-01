#!/usr/bin/env node

"use strict";

const { execFileSync, execSync, spawn } = require("child_process");
const path = require("path");
const fs = require("fs");
const os = require("os");

const PACKAGE = require("./package.json");
const NAME = "1agents";
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
  console.log(`[1agent] Binaries missing, running installer...`);
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

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function main() {
  const userArgs = process.argv.slice(2);
  const command = userArgs[0];

  const daemonDir = path.join(os.homedir(), ".1agents");
  const daemonJson = path.join(daemonDir, "daemon.json");
  const logFile = path.join(daemonDir, "1agents.log");

  const isDaemonCommand = ["start", "stop", "status", "logs"].includes(command);

  if (isDaemonCommand) {
    if (command === "start") {
      let isRunning = false;
      let existingPid = null;
      let existingAddr = "";
      try {
        if (fs.existsSync(daemonJson)) {
          const info = JSON.parse(fs.readFileSync(daemonJson, "utf8"));
          existingPid = info.pid;
          existingAddr = info.listen_addr;
          if (existingPid) {
            process.kill(existingPid, 0);
            isRunning = true;
          }
        }
      } catch (e) {
        isRunning = false;
      }

      if (isRunning) {
        console.log(`⚠️ 1agents is already running in the background (PID: ${existingPid}) on ${existingAddr}.`);
        process.exit(0);
      }

      // Prepare arguments
      const finalArgs = [];
      if (!userArgs.some(arg => arg.startsWith("-ttyd-bin"))) {
        finalArgs.push("-ttyd-bin", ttydPath);
      }
      if (!userArgs.some(arg => arg.startsWith("-static"))) {
        const staticPath = path.join(packageDir, "dist");
        finalArgs.push("-static", staticPath);
      }
      // Add all original arguments except 'start'
      finalArgs.push(...userArgs.slice(1));

      if (!fs.existsSync(daemonDir)) {
        fs.mkdirSync(daemonDir, { recursive: true });
      }

      console.log("Starting 1agents in the background...");
      const logStream = fs.openSync(logFile, "a");
      const child = spawn(agentPath, finalArgs, {
        detached: true,
        stdio: ["ignore", logStream, logStream]
      });
      child.unref();

      // Wait for the Go binary to boot and write its daemon.json
      let started = false;
      let pid = child.pid;
      let listenAddr = "";

      for (let i = 0; i < 20; i++) {
        await sleep(200);
        try {
          process.kill(child.pid, 0);
        } catch (e) {
          break; // Exited early
        }

        try {
          if (fs.existsSync(daemonJson)) {
            const info = JSON.parse(fs.readFileSync(daemonJson, "utf8"));
            if (info.pid === child.pid || process.platform === "win32") {
              pid = info.pid || child.pid;
              listenAddr = info.listen_addr;
              started = true;
              break;
            }
          }
        } catch (e) {}
      }

      if (!started) {
        console.error("❌ Failed to start 1agents in the background.");
        try {
          if (fs.existsSync(logFile)) {
            const logs = fs.readFileSync(logFile, "utf8").split("\n").slice(-15).join("\n");
            console.error("\nLast 15 lines of log:\n" + logs);
          }
        } catch (e) {}
        process.exit(1);
      }

      console.log("\n==================================================================");
      console.log("🚀 1agents started successfully in the background!");
      console.log(`● PID         : ${pid}`);
      console.log(`● Local Port  : ${listenAddr}`);
      console.log(`● Log File    : ${logFile}`);
      console.log("==================================================================");
      console.log("Commands to manage the daemon:");
      console.log("  1agents status   - Check running status");
      console.log("  1agents logs -f  - Follow log output");
      console.log("  1agents stop     - Stop the background server\n");
      process.exit(0);
    }

    else if (command === "stop") {
      let pid = null;
      try {
        if (fs.existsSync(daemonJson)) {
          const info = JSON.parse(fs.readFileSync(daemonJson, "utf8"));
          pid = info.pid;
        }
      } catch (e) {}

      if (!pid) {
        console.log("1agents is not running (no active daemon found).");
        process.exit(0);
      }

      let isAlive = false;
      try {
        process.kill(pid, 0);
        isAlive = true;
      } catch (e) {}

      if (!isAlive) {
        console.log("1agents is not running (PID not active). Cleaning up...");
        try { fs.unlinkSync(daemonJson); } catch (e) {}
        process.exit(0);
      }

      console.log(`Stopping 1agents (PID: ${pid})...`);
      try {
        process.kill(pid, "SIGTERM");
      } catch (e) {
        console.error(`Failed to send SIGTERM: ${e.message}`);
      }

      let stopped = false;
      for (let i = 0; i < 25; i++) {
        await sleep(200);
        try {
          process.kill(pid, 0);
        } catch (e) {
          stopped = true;
          break;
        }
      }

      if (!stopped) {
        console.log("Process did not stop gracefully. Sending SIGKILL...");
        try {
          process.kill(pid, "SIGKILL");
        } catch (e) {}
      }

      try { fs.unlinkSync(daemonJson); } catch (e) {}
      console.log("❇️ 1agents stopped successfully.");
      process.exit(0);
    }

    else if (command === "status") {
      let isRunning = false;
      let pid = null;
      let listenAddr = "";
      let mtime = null;
      try {
        if (fs.existsSync(daemonJson)) {
          const stats = fs.statSync(daemonJson);
          mtime = stats.mtime;
          const info = JSON.parse(fs.readFileSync(daemonJson, "utf8"));
          pid = info.pid;
          listenAddr = info.listen_addr;
          if (pid) {
            process.kill(pid, 0);
            isRunning = true;
          }
        }
      } catch (e) {
        isRunning = false;
      }

      if (isRunning) {
        console.log("\n1agents daemon status:");
        console.log(`● Active     : running (since ${mtime ? mtime.toLocaleString() : "unknown"})`);
        console.log(`● PID        : ${pid}`);
        console.log(`● Address    : ${listenAddr}`);
        console.log(`● Log File   : ${logFile}`);

        console.log();
      } else {
        if (fs.existsSync(daemonJson)) {
          console.log("● Status     : stopped (stale PID file found)");
          try {
            fs.unlinkSync(daemonJson);
          } catch (e) {}
        } else {
          console.log("● Status     : stopped");
        }
      }
      process.exit(0);
    }

    else if (command === "logs") {
      const follow = userArgs.includes("-f") || userArgs.includes("--follow");
      let numLines = 50;

      const nIndex = userArgs.findIndex(arg => arg === "-n");
      if (nIndex !== -1 && nIndex + 1 < userArgs.length) {
        const val = parseInt(userArgs[nIndex + 1], 10);
        if (!isNaN(val) && val > 0) {
          numLines = val;
        }
      }

      if (!fs.existsSync(logFile)) {
        console.log("No log file found.");
        process.exit(0);
      }

      const data = fs.readFileSync(logFile, "utf8");
      const lines = data.split("\n");
      const lastLines = lines.slice(-numLines).join("\n");
      console.log(lastLines);

      if (follow) {
        fs.watchFile(logFile, { interval: 250 }, (curr, prev) => {
          if (curr.mtime <= prev.mtime) return;
          if (curr.size <= prev.size) return;
          try {
            const fd = fs.openSync(logFile, "r");
            const buffer = Buffer.alloc(curr.size - prev.size);
            fs.readSync(fd, buffer, 0, buffer.length, prev.size);
            fs.closeSync(fd);
            process.stdout.write(buffer.toString());
          } catch (e) {}
        });

        await new Promise(() => {}); // Keep alive
      }
      process.exit(0);
    }
  } else {
    // Foreground run (default)
    const finalArgs = [];
    if (!userArgs.some(arg => arg.startsWith("-ttyd-bin"))) {
      finalArgs.push("-ttyd-bin", ttydPath);
    }
    if (!userArgs.some(arg => arg.startsWith("-static"))) {
      const staticPath = path.join(packageDir, "dist");
      finalArgs.push("-static", staticPath);
    }
    finalArgs.push(...userArgs);

    try {
      execFileSync(agentPath, finalArgs, { stdio: "inherit" });
    } catch (err) {
      process.exit(err.status || 1);
    }
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
