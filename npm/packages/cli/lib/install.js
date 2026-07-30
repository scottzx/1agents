"use strict";

/**
 * `1agents install [all|module] [--check]`
 *
 * Bootstraps module runtimes on the host after npm has placed packages.
 * Semantics:
 *   npm install -g @1agents/1agents  → package files in node_modules
 *   1agents install …                → runtime/dependency checks (idempotent)
 */

const { spawnSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const {
  mapPlatform,
  corePackageName,
  resolveCoreBin,
  resolveWebDist,
  tryResolveHappy,
  resolvePackageRoot,
} = require("./platform");

// ── result helpers ─────────────────────────────────────────────────────

/** @typedef {"ok"|"missing"|"degraded"|"error"} Status */
/** @typedef {{ id: string, label: string, required: boolean, status: Status, detail: string }} ModuleResult */

function result(id, label, required, status, detail) {
  return { id, label, required, status, detail };
}

// ── modules ────────────────────────────────────────────────────────────

function checkCore() {
  try {
    mapPlatform();
    const agent = resolveCoreBin("1agents");
    const ttyd = resolveCoreBin("ttyd");
    return result(
      "core",
      "core (1agents + ttyd)",
      true,
      "ok",
      `${corePackageName()}: ${agent}, ${ttyd}`
    );
  } catch (err) {
    return result(
      "core",
      "core (1agents + ttyd)",
      true,
      "missing",
      err.message || String(err)
    );
  }
}

function installCore(checkOnly) {
  const r = checkCore();
  if (r.status === "ok" || checkOnly) return r;
  // Core ships as optional platform package — cannot download; only guide reinstall.
  return result(
    "core",
    "core (1agents + ttyd)",
    true,
    "error",
    `${r.detail}\n  Fix: npm i -g ${corePackageName()}@<same-version-as-@1agents/1agents>`
  );
}

function checkWeb() {
  try {
    const dist = resolveWebDist();
    return result("web", "web (frontend dist)", true, "ok", dist);
  } catch (err) {
    return result("web", "web (frontend dist)", true, "missing", err.message || String(err));
  }
}

function installWeb(checkOnly) {
  const r = checkWeb();
  if (r.status === "ok" || checkOnly) return r;
  return result(
    "web",
    "web (frontend dist)",
    true,
    "error",
    `${r.detail}\n  Fix: npm i -g @1agents/web@<same-version>`
  );
}

function checkHarnessKit() {
  try {
    const binary = resolveCoreBin("hk");
    const version = spawnSync(binary, ["--version"], { encoding: "utf8" });
    if (version.status !== 0) {
      return result(
        "harnesskit",
        "HarnessKit extension daemon",
        true,
        "error",
        `${binary} --version failed (exit ${version.status})`
      );
    }
    return result(
      "harnesskit",
      "HarnessKit extension daemon",
      true,
      "ok",
      `${binary}: ${(version.stdout || version.stderr || "").trim()}`
    );
  } catch (err) {
    return result(
      "harnesskit",
      "HarnessKit extension daemon",
      true,
      "missing",
      err.message || String(err)
    );
  }
}

function installHarnessKit(checkOnly) {
  const checked = checkHarnessKit();
  if (checked.status === "ok" || checkOnly) return checked;
  return result(
    "harnesskit",
    "HarnessKit extension daemon",
    true,
    "error",
    `${checked.detail}\n  Fix: reinstall ${corePackageName()} at the same version as @1agents/1agents`
  );
}

function checkHappy() {
  const h = tryResolveHappy();
  if (!h) {
    return result(
      "happy",
      "happy (happy-cli deps)",
      false,
      "missing",
      "@1agents/happy not installed (optional)"
    );
  }
  const nm = path.join(h.happyCli, "node_modules");
  if (fs.existsSync(nm)) {
    return result("happy", "happy (happy-cli deps)", false, "ok", h.happyCli);
  }
  if (!fs.existsSync(path.join(h.happyCli, "package.json"))) {
    return result(
      "happy",
      "happy (happy-cli deps)",
      false,
      "degraded",
      `happy-cli missing under ${h.root}`
    );
  }
  return result(
    "happy",
    "happy (happy-cli deps)",
    false,
    "missing",
    `package present; node_modules missing at ${nm}`
  );
}

function installHappyDeps(happyCli) {
  const hasLock = fs.existsSync(path.join(happyCli, "package-lock.json"));
  const args = hasLock
    ? ["ci", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund"]
    : ["install", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund"];
  console.log(`[@1agents/1agents] happy: npm ${args[0]} in ${happyCli}`);
  const r = spawnSync("npm", args, {
    cwd: happyCli,
    stdio: "inherit",
    shell: process.platform === "win32",
  });
  if (r.status !== 0) {
    throw new Error(`npm ${args[0]} failed (exit ${r.status})`);
  }
  // Strip bundled claude-agent-sdk forks (same as happy postinstall)
  const nm = path.join(happyCli, "node_modules", "@anthropic-ai");
  if (fs.existsSync(nm)) {
    for (const ent of fs.readdirSync(nm)) {
      if (ent.startsWith("claude-agent-sdk-")) {
        fs.rmSync(path.join(nm, ent), { recursive: true, force: true });
      }
    }
  }
}

function installHappy(checkOnly) {
  const checked = checkHappy();
  if (checked.status === "ok") return checked;
  if (checkOnly) return checked;

  const h = tryResolveHappy();
  if (!h) {
    return result(
      "happy",
      "happy (happy-cli deps)",
      false,
      "degraded",
      "@1agents/happy not installed — skip (optional). npm i -g @1agents/happy@<ver>"
    );
  }
  if (!fs.existsSync(path.join(h.happyCli, "package.json"))) {
    return result(
      "happy",
      "happy (happy-cli deps)",
      false,
      "degraded",
      `happy-cli missing under ${h.root}`
    );
  }
  try {
    installHappyDeps(h.happyCli);
    return result("happy", "happy (happy-cli deps)", false, "ok", h.happyCli);
  } catch (err) {
    return result(
      "happy",
      "happy (happy-cli deps)",
      false,
      "degraded",
      err.message || String(err)
    );
  }
}

function checkCcConnect() {
  try {
    const { resolveBinary } = require("@1agents/cc-connect");
    const p = resolveBinary();
    return result("cc-connect", "cc-connect binary", true, "ok", p);
  } catch (err) {
    return result(
      "cc-connect",
      "cc-connect binary",
      true,
      "missing",
      err.message || String(err)
    );
  }
}

function installCcConnect(checkOnly) {
  const r = checkCcConnect();
  if (r.status === "ok" || checkOnly) return r;
  return result(
    "cc-connect",
    "cc-connect binary",
    true,
    "error",
    `${r.detail}\n  Fix: npm i -g @1agents/cc-connect@<same-version>`
  );
}

function checkAcpBridge() {
  try {
    const root = resolvePackageRoot("@1agents/acp-bridge");
    const bridge = path.join(root, "bridge-server.mjs");
    if (!fs.existsSync(bridge)) {
      return result(
        "acp-bridge",
        "acp-bridge",
        true,
        "missing",
        `bridge-server.mjs missing under ${root}`
      );
    }
    return result("acp-bridge", "acp-bridge", true, "ok", bridge);
  } catch (err) {
    return result(
      "acp-bridge",
      "acp-bridge",
      true,
      "missing",
      err.message || String(err)
    );
  }
}

function installAcpBridge(checkOnly) {
  const r = checkAcpBridge();
  if (r.status === "ok" || checkOnly) return r;
  return result(
    "acp-bridge",
    "acp-bridge",
    true,
    "error",
    `${r.detail}\n  Fix: npm i -g @1agents/acp-bridge@<same-version>`
  );
}

// ── manifest ───────────────────────────────────────────────────────────

/**
 * Module install/check registry. Add new modules here.
 * `run(checkOnly)` returns ModuleResult.
 */
const MODULES = {
  core: { id: "core", label: "core", required: true, run: installCore },
  web: { id: "web", label: "web", required: true, run: installWeb },
  harnesskit: {
    id: "harnesskit",
    label: "harnesskit",
    required: true,
    run: installHarnessKit,
  },
  "cc-connect": {
    id: "cc-connect",
    label: "cc-connect",
    required: true,
    run: installCcConnect,
  },
  "acp-bridge": {
    id: "acp-bridge",
    label: "acp-bridge",
    required: true,
    run: installAcpBridge,
  },
  happy: { id: "happy", label: "happy", required: false, run: installHappy },
};

const INSTALL_ORDER = ["core", "web", "harnesskit", "cc-connect", "acp-bridge", "happy"];

// ── CLI surface ────────────────────────────────────────────────────────

function printHelp() {
  const names = INSTALL_ORDER.join(" | ");
  console.log(`Usage:
  1agents install all              Install/check all module runtimes
  1agents install <module>         Install one module (${names})
  1agents install --check          Diagnose all modules (no changes)
  1agents install status           Same as --check
  1agents install <module> --check Diagnose one module
  1agents install --help

After:  npm install -g @1agents/1agents
Run:    1agents install all

npm install  → package files
1agents install → runtime validation and optional dependency setup`);
}

function printTable(results) {
  const pad = (s, n) => String(s).padEnd(n);
  console.log("");
  console.log(`${pad("MODULE", 14)} ${pad("REQ", 6)} ${pad("STATUS", 10)} DETAIL`);
  console.log("-".repeat(72));
  for (const r of results) {
    const req = r.required ? "yes" : "opt";
    console.log(`${pad(r.id, 14)} ${pad(req, 6)} ${pad(r.status, 10)} ${r.detail}`);
  }
  console.log("");
}

/**
 * Entry for run.js when argv[0] === "install".
 * @param {string[]} args argv after "install"
 * @returns {number} process exit code
 */
function runInstall(args) {
  const raw = args.slice();
  if (raw.includes("-h") || raw.includes("--help") || raw[0] === "help") {
    printHelp();
    return 0;
  }

  const checkOnly =
    raw.includes("--check") ||
    raw.includes("-c") ||
    raw[0] === "status" ||
    raw[0] === "check";

  // strip flags / status aliases
  const targets = raw.filter(
    (a) =>
      a !== "--check" &&
      a !== "-c" &&
      a !== "status" &&
      a !== "check" &&
      a !== "--help" &&
      a !== "-h"
  );

  let ids;
  if (targets.length === 0 || targets[0] === "all") {
    ids = INSTALL_ORDER.slice();
  } else {
    ids = targets;
    for (const id of ids) {
      if (!MODULES[id]) {
        console.error(`Unknown module: ${id}`);
        console.error(`Known: all | ${INSTALL_ORDER.join(" | ")}`);
        return 2;
      }
    }
  }

  console.log(
    checkOnly
      ? "[@1agents/1agents] install --check"
      : `[@1agents/1agents] install ${ids.join(" ")}`
  );

  /** @type {ModuleResult[]} */
  const results = [];
  for (const id of ids) {
    const mod = MODULES[id];
    console.log(`\n→ ${mod.label}${checkOnly ? " (check)" : ""}…`);
    try {
      results.push(mod.run(checkOnly));
    } catch (err) {
      results.push(
        result(
          id,
          mod.label,
          mod.required,
          mod.required ? "error" : "degraded",
          err.message || String(err)
        )
      );
    }
  }

  printTable(results);

  // Hard exit only for packages that block core workbench start.
  const HARD_EXIT = new Set(["core", "web", "harnesskit", "cc-connect", "acp-bridge"]);
  const hardFail = results.some(
    (r) => HARD_EXIT.has(r.id) && (r.status === "error" || r.status === "missing")
  );
  const softFail = results.some(
    (r) => !HARD_EXIT.has(r.id) && r.status !== "ok"
  );

  if (hardFail) {
    console.error("[@1agents/1agents] install finished with required-module failures.");
    return 1;
  }
  if (softFail) {
    console.warn(
      "[@1agents/1agents] install finished; some modules not fully ready (core still usable)."
    );
    return 0;
  }
  console.log("[@1agents/1agents] install OK.");
  return 0;
}

module.exports = {
  runInstall,
  MODULES,
  INSTALL_ORDER,
  checkHarnessKit,
  installHarnessKit,
};
