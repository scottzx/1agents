"use strict";

const fs = require("fs");
const path = require("path");

function mapPlatform() {
  const { platform, arch } = process;
  if (platform === "darwin" && arch === "arm64") return "darwin-arm64";
  if (platform === "linux" && arch === "x64") return "linux-x64";
  if (platform === "linux" && arch === "arm64") return "linux-arm64";
  throw new Error(`Unsupported platform: ${platform}/${arch}`);
}

function resolvePackageRoot(name) {
  return path.dirname(require.resolve(`${name}/package.json`));
}

function resolveBinary() {
  const plat = mapPlatform();
  const pkg = `@1agents/cc-switch-${plat}`;
  const root = resolvePackageRoot(pkg);
  const binName = process.platform === "win32" ? "cc-switch.exe" : "cc-switch";
  const p = path.join(root, "bin", binName);
  if (!fs.existsSync(p)) {
    throw new Error(
      `Missing binary ${p} (package ${pkg}). Reinstall @1agents/cc-switch at the matching version.`
    );
  }
  return p;
}

module.exports = { mapPlatform, resolveBinary };
