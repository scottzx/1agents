#!/usr/bin/env node
/**
 * Set the same version on all @1agents packages under npm/packages.
 * Usage: node scripts/npm-set-version.mjs 20260718.1.0
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "..");
const packagesDir = path.join(root, "npm/packages");
const version = process.argv[2];

if (!version) {
  console.error("usage: node scripts/npm-set-version.mjs <version>");
  process.exit(1);
}

const OUR = (name) => name.startsWith("@1agents/");

for (const ent of fs.readdirSync(packagesDir, { withFileTypes: true })) {
  if (!ent.isDirectory()) continue;
  const pkgPath = path.join(packagesDir, ent.name, "package.json");
  if (!fs.existsSync(pkgPath)) continue;
  const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf8"));
  pkg.version = version;
  for (const field of ["dependencies", "optionalDependencies", "peerDependencies", "devDependencies"]) {
    if (!pkg[field]) continue;
    for (const [dep, ver] of Object.entries(pkg[field])) {
      if (OUR(dep) && (ver === "0.0.0-dev" || /^\d{8}\.\d+\.\d+$/.test(ver) || ver === version)) {
        pkg[field][dep] = version;
      }
    }
  }
  fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");
  console.log(`set ${pkg.name}@${version}`);
}
