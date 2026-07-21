"use strict";

/**
 * Map process.platform/arch → npm package suffix (npm/cpu convention).
 * @returns {"linux-x64"|"linux-arm64"|"darwin-arm64"}
 */
function mapPlatform() {
  const { platform, arch } = process;
  if (platform === "darwin" && arch === "arm64") return "darwin-arm64";
  if (platform === "linux" && arch === "x64") return "linux-x64";
  if (platform === "linux" && arch === "arm64") return "linux-arm64";
  throw new Error(
    `Unsupported platform: ${platform}/${arch}. Supported: darwin-arm64, linux-x64, linux-arm64.`
  );
}

/**
 * Resolve installed package root directory by name.
 * Binaries are expected to already live under node_modules via npm install —
 * there is no GitHub Release download path.
 */
function resolvePackageRoot(name) {
  try {
    return require("path").dirname(require.resolve(`${name}/package.json`));
  } catch (err) {
    const e = new Error(
      `Package ${name} is not installed. Install with: npm i -g ${name}@<same-version-as-cli>`
    );
    e.cause = err;
    throw e;
  }
}

function corePackageName(plat = mapPlatform()) {
  return `@1agents/core-${plat}`;
}

function resolveCoreBin(name, plat = mapPlatform()) {
  const path = require("path");
  const fs = require("fs");
  const root = resolvePackageRoot(corePackageName(plat));
  const binName = process.platform === "win32" ? `${name}.exe` : name;
  const p = path.join(root, "bin", binName);
  if (!fs.existsSync(p)) {
    throw new Error(
      `Missing ${binName} in ${corePackageName(plat)} (${p}). Reinstall the core platform package from the npm registry.`
    );
  }
  return p;
}

function resolveWebDist() {
  const path = require("path");
  const fs = require("fs");
  const root = resolvePackageRoot("@1agents/web");
  const dist = path.join(root, "dist");
  if (!fs.existsSync(dist)) {
    throw new Error(`@1agents/web has no dist/ at ${dist}`);
  }
  return dist;
}

function resolveSkillsRoot() {
  return resolvePackageRoot("@1agents/skills");
}

function tryResolveHappy() {
  try {
    const path = require("path");
    const root = resolvePackageRoot("@1agents/happy");
    return {
      root,
      happyCli: path.join(root, "happy-cli"),
      adapter: path.join(root, "adapter"),
    };
  } catch {
    return null;
  }
}

module.exports = {
  mapPlatform,
  resolvePackageRoot,
  corePackageName,
  resolveCoreBin,
  resolveWebDist,
  resolveSkillsRoot,
  tryResolveHappy,
};
