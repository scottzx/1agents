#!/usr/bin/env node
"use strict";

/**
 * @1agents/1agents entry — resolves binaries from installed @1agents/* packages
 * on the local npm tree. Does NOT download GitHub Release archives.
 *
 * Subcommands handled in this JS shim:
 *   install [all|module] [--check]  — module runtime bootstrap (venv, happy deps, …)
 *   start|stop|status|logs          — daemon lifecycle
 * Everything else is forwarded to the core `1agents` binary.
 */

const { execFileSync, spawn } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

const {
  mapPlatform,
  resolveCoreBin,
  resolveWebDist,
  resolveSkillsRoot,
} = require("./lib/platform");
const { runInstall } = require("./lib/install");

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function resolvePaths() {
  try {
    mapPlatform();
  } catch (err) {
    console.error(`❌ [1agents] ${err.message}`);
    process.exit(1);
  }

  let agentPath;
  let ttydPath;
  let staticPath;
  try {
    agentPath = resolveCoreBin("1agents");
    ttydPath = resolveCoreBin("ttyd");
    staticPath = resolveWebDist();
  } catch (err) {
    console.error(`❌ [1agents] ${err.message}`);
    console.error(
      "   Core and web packages must be installed from the npm registry " +
        "(e.g. optionalDependency @1agents/core-<plat> and dependency @1agents/web)."
    );
    process.exit(1);
  }

  // Soft-fail: core can run without skills; supervisor logs and skips the service.
  let skillsPath = null;
  try {
    skillsPath = resolveSkillsRoot();
  } catch (err) {
    console.warn(`⚠️  [1agents] @1agents/skills not found: ${err.message}`);
    console.warn(
      "   Skills service will be unavailable. Install with: npm i -g @1agents/skills"
    );
  }

  if (process.platform !== "win32") {
    try {
      fs.chmodSync(agentPath, 0o755);
      fs.chmodSync(ttydPath, 0o755);
    } catch (_) {}
  }

  return { agentPath, ttydPath, staticPath, skillsPath };
}

/** Inject default binary/static/skills paths unless the user already set them. */
function buildCoreArgs(userArgs, { ttydPath, staticPath, skillsPath }) {
  const finalArgs = [];
  if (!userArgs.some((a) => a.startsWith("-ttyd-bin"))) {
    finalArgs.push("-ttyd-bin", ttydPath);
  }
  if (!userArgs.some((a) => a.startsWith("-static"))) {
    finalArgs.push("-static", staticPath);
  }
  if (skillsPath && !userArgs.some((a) => a.startsWith("-skills-dir"))) {
    finalArgs.push("-skills-dir", skillsPath);
  }
  return finalArgs;
}

async function main() {
  const userArgs = process.argv.slice(2);
  const command = userArgs[0];

  // `1agents install …` does not need core binaries resolved first.
  if (command === "install") {
    process.exit(runInstall(userArgs.slice(1)));
  }

  const paths = resolvePaths();
  const { agentPath } = paths;

  const daemonDir = path.join(os.homedir(), ".1agents");
  const daemonJson = path.join(daemonDir, "daemon.json");
  const logFile = path.join(daemonDir, "1agents.log");
  const isDaemon = ["start", "stop", "status", "logs"].includes(command);

  if (!isDaemon) {
    const finalArgs = buildCoreArgs(userArgs, paths);
    finalArgs.push(...userArgs);
    try {
      execFileSync(agentPath, finalArgs, { stdio: "inherit" });
    } catch (err) {
      process.exit(err.status || 1);
    }
    return;
  }

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
    } catch (_) {
      isRunning = false;
    }
    if (isRunning) {
      console.log(
        `⚠️ 1agents is already running (PID: ${existingPid}) on ${existingAddr}.`
      );
      process.exit(0);
    }

    const finalArgs = buildCoreArgs(userArgs, paths);
    finalArgs.push(...userArgs.slice(1));

    fs.mkdirSync(daemonDir, { recursive: true });
    console.log("Starting 1agents in the background...");
    const logStream = fs.openSync(logFile, "a");
    const child = spawn(agentPath, finalArgs, {
      detached: true,
      stdio: ["ignore", logStream, logStream],
    });
    child.unref();

    let started = false;
    let pid = child.pid;
    let listenAddr = "";
    for (let i = 0; i < 20; i++) {
      await sleep(200);
      try {
        process.kill(child.pid, 0);
      } catch (_) {
        break;
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
      } catch (_) {}
    }
    if (!started) {
      console.error("❌ Failed to start 1agents in the background.");
      try {
        if (fs.existsSync(logFile)) {
          console.error(
            "\nLast log lines:\n" +
              fs.readFileSync(logFile, "utf8").split("\n").slice(-15).join("\n")
          );
        }
      } catch (_) {}
      process.exit(1);
    }
    console.log("🚀 1agents started");
    console.log(`● PID     : ${pid}`);
    console.log(`● Address : ${listenAddr}`);
    console.log(`● Log     : ${logFile}`);
    process.exit(0);
  }

  if (command === "stop") {
    let pid = null;
    try {
      if (fs.existsSync(daemonJson)) {
        pid = JSON.parse(fs.readFileSync(daemonJson, "utf8")).pid;
      }
    } catch (_) {}
    if (!pid) {
      console.log("1agents is not running.");
      process.exit(0);
    }
    try {
      process.kill(pid, "SIGTERM");
    } catch (e) {
      console.error(e.message);
    }
    for (let i = 0; i < 25; i++) {
      await sleep(200);
      try {
        process.kill(pid, 0);
      } catch (_) {
        break;
      }
    }
    try {
      fs.unlinkSync(daemonJson);
    } catch (_) {}
    console.log("1agents stopped.");
    process.exit(0);
  }

  if (command === "status") {
    let running = false;
    let pid = null;
    let addr = "";
    try {
      if (fs.existsSync(daemonJson)) {
        const info = JSON.parse(fs.readFileSync(daemonJson, "utf8"));
        pid = info.pid;
        addr = info.listen_addr;
        if (pid) {
          process.kill(pid, 0);
          running = true;
        }
      }
    } catch (_) {
      running = false;
    }
    if (running) {
      console.log(`● running pid=${pid} addr=${addr}`);
    } else {
      console.log("● stopped");
      try {
        if (fs.existsSync(daemonJson)) fs.unlinkSync(daemonJson);
      } catch (_) {}
    }
    process.exit(0);
  }

  if (command === "logs") {
    if (!fs.existsSync(logFile)) {
      console.log("No log file.");
      process.exit(0);
    }
    const follow = userArgs.includes("-f") || userArgs.includes("--follow");
    console.log(fs.readFileSync(logFile, "utf8").split("\n").slice(-50).join("\n"));
    if (follow) {
      fs.watchFile(logFile, { interval: 250 }, (curr, prev) => {
        if (curr.size <= prev.size) return;
        try {
          const fd = fs.openSync(logFile, "r");
          const buf = Buffer.alloc(curr.size - prev.size);
          fs.readSync(fd, buf, 0, buf.length, prev.size);
          fs.closeSync(fd);
          process.stdout.write(buf.toString());
        } catch (_) {}
      });
      await new Promise(() => {});
    }
    process.exit(0);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
