"use strict";

/**
 * `1agents install [all|module] [--check]`
 *
 * Bootstraps module runtimes on the host after npm has placed packages.
 * Semantics:
 *   npm install -g @1agents/1agents  → package files in node_modules
 *   1agents install …                → venv / deps / resolve checks (idempotent)
 */

const { spawnSync } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

const {
  mapPlatform,
  corePackageName,
  resolveCoreBin,
  resolveWebDist,
  resolveSkillsRoot,
  tryResolveHappy,
  resolvePackageRoot,
} = require("./platform");

// ── paths ──────────────────────────────────────────────────────────────

function oneAgentsHome() {
  return process.env.ONEAGENTS_HOME || os.homedir();
}

/** Managed venv for npm-installed skills (source under node_modules is often read-only). */
function managedSkillsVenvDir() {
  return path.join(oneAgentsHome(), ".1agents", "1skills", ".venv");
}

function venvPython(venvDir) {
  return process.platform === "win32"
    ? path.join(venvDir, "Scripts", "python.exe")
    : path.join(venvDir, "bin", "python");
}

function isExecutable(p) {
  try {
    const st = fs.statSync(p);
    if (!st.isFile()) return false;
    if (process.platform === "win32") return true;
    return (st.mode & 0o111) !== 0;
  } catch {
    return false;
  }
}

function commandExists(cmd) {
  try {
    const which = process.platform === "win32" ? "where" : "which";
    const r = spawnSync(which, [cmd], { encoding: "utf8" });
    return r.status === 0;
  } catch {
    return false;
  }
}

function isSkillsSource(dir) {
  if (!dir) return false;
  try {
    return (
      fs.statSync(path.join(dir, "skill_manager")).isDirectory() &&
      fs.existsSync(path.join(dir, "requirements.txt"))
    );
  } catch {
    return false;
  }
}

function dirIsWritable(dir) {
  try {
    fs.accessSync(dir, fs.constants.W_OK);
    return true;
  } catch {
    return false;
  }
}

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

function skillsVenvPath(skillsRoot) {
  if (skillsRoot && dirIsWritable(skillsRoot)) {
    return path.join(skillsRoot, ".venv");
  }
  return managedSkillsVenvDir();
}

function checkSkills() {
  let root;
  try {
    root = resolveSkillsRoot();
  } catch (err) {
    return result(
      "skills",
      "skills (1skills Python venv)",
      true,
      "missing",
      err.message || String(err)
    );
  }
  if (!isSkillsSource(root)) {
    return result(
      "skills",
      "skills (1skills Python venv)",
      true,
      "missing",
      `@1agents/skills at ${root} is not a valid source (need skill_manager/ + requirements.txt)`
    );
  }
  const venvDir = skillsVenvPath(root);
  const py = venvPython(venvDir);
  if (isExecutable(py)) {
    return result("skills", "skills (1skills Python venv)", true, "ok", `venv ${venvDir}`);
  }
  return result(
    "skills",
    "skills (1skills Python venv)",
    true,
    "missing",
    `source ok at ${root}; venv missing at ${venvDir}`
  );
}

function bootstrapSkillsVenv(skillsRoot) {
  const venvDir = skillsVenvPath(skillsRoot);
  const req = path.join(skillsRoot, "requirements.txt");
  fs.mkdirSync(path.dirname(venvDir), { recursive: true });

  if (commandExists("uv")) {
    console.log(`[@1agents/1agents] skills: uv venv → ${venvDir}`);
    let r = spawnSync("uv", ["venv", venvDir], { stdio: "inherit" });
    if (r.status !== 0) {
      throw new Error(`uv venv failed (exit ${r.status})`);
    }
    const py = venvPython(venvDir);
    console.log(`[@1agents/1agents] skills: uv pip install -r requirements.txt`);
    r = spawnSync("uv", ["pip", "install", "--python", py, "-r", req], {
      cwd: skillsRoot,
      stdio: "inherit",
    });
    if (r.status !== 0) {
      throw new Error(`uv pip install failed (exit ${r.status})`);
    }
    return venvDir;
  }

  const pythonBin = process.env.ONEAGENTS_PYTHON || "python3";
  if (!commandExists(pythonBin) && process.platform !== "win32") {
    // Windows often has `py` launcher
    if (process.platform === "win32" && commandExists("py")) {
      // fall through with py -3
    } else {
      throw new Error(
        `host Python not found (${pythonBin}) and uv not installed — install Python >= 3.11 or uv`
      );
    }
  }

  console.log(`[@1agents/1agents] skills: ${pythonBin} -m venv → ${venvDir}`);
  let r;
  if (process.platform === "win32" && !commandExists(pythonBin) && commandExists("py")) {
    r = spawnSync("py", ["-3", "-m", "venv", venvDir], { stdio: "inherit" });
  } else {
    r = spawnSync(pythonBin, ["-m", "venv", venvDir], { stdio: "inherit" });
  }
  if (r.status !== 0) {
    throw new Error(`python -m venv failed (exit ${r.status})`);
  }
  const py = venvPython(venvDir);
  console.log(`[@1agents/1agents] skills: pip install -r requirements.txt`);
  r = spawnSync(py, ["-m", "pip", "install", "--upgrade", "pip"], { stdio: "inherit" });
  // pip upgrade failure is non-fatal
  r = spawnSync(py, ["-m", "pip", "install", "-r", req], {
    cwd: skillsRoot,
    stdio: "inherit",
  });
  if (r.status !== 0) {
    throw new Error(`pip install failed (exit ${r.status})`);
  }
  return venvDir;
}

function installSkills(checkOnly) {
  const checked = checkSkills();
  if (checked.status === "ok") return checked;
  if (checkOnly) return checked;

  let root;
  try {
    root = resolveSkillsRoot();
  } catch (err) {
    return result(
      "skills",
      "skills (1skills Python venv)",
      true,
      "error",
      err.message || String(err)
    );
  }
  if (!isSkillsSource(root)) {
    return result(
      "skills",
      "skills (1skills Python venv)",
      true,
      "error",
      `invalid skills source at ${root}`
    );
  }

  try {
    const venvDir = bootstrapSkillsVenv(root);
    const py = venvPython(venvDir);
    if (!isExecutable(py)) {
      return result(
        "skills",
        "skills (1skills Python venv)",
        true,
        "error",
        `bootstrap finished but python missing at ${py}`
      );
    }
    return result("skills", "skills (1skills Python venv)", true, "ok", `venv ${venvDir}`);
  } catch (err) {
    return result(
      "skills",
      "skills (1skills Python venv)",
      true,
      "error",
      err.message || String(err)
    );
  }
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
  skills: { id: "skills", label: "skills", required: true, run: installSkills },
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

const INSTALL_ORDER = ["core", "web", "skills", "cc-connect", "acp-bridge", "happy"];

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
1agents install → host runtimes (venv, happy deps, binary resolve)`);
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
  // skills/happy may fail without Python / optional deps — core still usable.
  const HARD_EXIT = new Set(["core", "web", "cc-connect", "acp-bridge"]);
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
  managedSkillsVenvDir,
  checkSkills,
  installSkills,
};
