#!/usr/bin/env node

"use strict";

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");
const http = require("http");

const PACKAGE = require("./package.json");

// Map NPM version (e.g. 20260523.1.0) back to Git Tag (e.g. v20260523-1)
const versionParts = PACKAGE.version.split(".");
if (versionParts.length < 2) {
  throw new Error(`[1agent] Invalid package version: ${PACKAGE.version}`);
}
const VERSION = `v${versionParts[0]}-${versionParts[1]}`;
const NAME = "1agents";

const GITHUB_REPO = "scottzx/1Agents";

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function getPlatformInfo() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];
  if (!platform || !arch) {
    throw new Error(
      `Unsupported platform: ${process.platform}/${process.arch}. ` +
        `Supported: linux/darwin/windows x64/arm64`
    );
  }
  const ext = platform === "windows" ? ".zip" : ".tar.gz";
  const filename = `${NAME}-${VERSION}-${platform}-${arch}${ext}`;
  return { platform, arch, ext, filename };
}

function getDownloadURLs(filename) {
  return [
    `https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${filename}`,
  ];
}

function fetch(url, redirects = 5) {
  return new Promise((resolve, reject) => {
    if (redirects <= 0) return reject(new Error("Too many redirects"));
    const mod = url.startsWith("https") ? https : http;
    mod
      .get(url, { headers: { "User-Agent": "1agent-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return resolve(fetch(res.headers.location, redirects - 1));
        }
        if (res.statusCode !== 200) {
          res.resume();
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        const chunks = [];
        res.on("data", (c) => chunks.push(c));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function download(urls) {
  for (const url of urls) {
    try {
      console.log(`[1agent] Downloading from ${url}`);
      const data = await fetch(url);
      console.log(`[1agent] Downloaded ${(data.length / 1024 / 1024).toFixed(2)} MB`);
      return data;
    } catch (err) {
      console.warn(`[1agent] Failed: ${err.message}, trying next source...`);
    }
  }
  throw new Error(
    `[1agent] Could not download binary from any source.\n` +
      `  Tried: ${urls.join(", ")}\n` +
      `  You can download manually from https://github.com/${GITHUB_REPO}/releases`
  );
}

function extractTarGz(buffer, destDir) {
  const tmpFile = path.join(destDir, "_tmp.tar.gz");
  fs.writeFileSync(tmpFile, buffer);
  try {
    execSync(`tar xzf "${tmpFile}" -C "${destDir}"`, { stdio: "pipe" });
  } finally {
    try {
      fs.unlinkSync(tmpFile);
    } catch {}
  }
}

function extractZip(buffer, destDir) {
  const tmpFile = path.join(destDir, "_tmp.zip");
  fs.writeFileSync(tmpFile, buffer);
  try {
    try {
      execSync(`unzip -o "${tmpFile}" -d "${destDir}"`, { stdio: "pipe" });
    } catch {
      execSync(`powershell -Command "Expand-Archive -Force '${tmpFile}' '${destDir}'"`, {
        stdio: "pipe",
      });
    }
  } finally {
    try {
      fs.unlinkSync(tmpFile);
    } catch {}
  }
}

async function main() {
  const { platform, arch, ext, filename } = getPlatformInfo();
  console.log(`[1agent] Platform: ${platform}/${arch}`);

  const packageDir = __dirname;
  const binDir = path.join(packageDir, "bin");
  const extSuffix = platform === "windows" ? ".exe" : "";
  const agentPath = path.join(binDir, NAME + extSuffix);
  const ttydPath = path.join(binDir, "ttyd" + extSuffix);

  // Check if binaries already exist and match version
  if (fs.existsSync(agentPath) && fs.existsSync(ttydPath)) {
    console.log(`[1agent] Binaries already exist in ${binDir}. Replacing with ${VERSION}...`);
    try {
      fs.unlinkSync(agentPath);
      fs.unlinkSync(ttydPath);
    } catch (err) {
      // Ignore if they can't be deleted, we will overwrite
    }
  }

  // Create bin and dist directory structure just in case
  fs.mkdirSync(binDir, { recursive: true });

  const urls = getDownloadURLs(filename);
  const data = await download(urls);

  console.log(`[1agent] Extracting files into ${packageDir}...`);
  if (ext === ".tar.gz") {
    extractTarGz(data, packageDir);
  } else {
    extractZip(data, packageDir);
  }

  // Make binaries executable on Darwin and Linux
  if (platform !== "windows") {
    if (fs.existsSync(agentPath)) {
      fs.chmodSync(agentPath, 0o755);
    }
    if (fs.existsSync(ttydPath)) {
      fs.chmodSync(ttydPath, 0o755);
    }
  }

  // Darwin Quarantine Attribute Removal
  if (platform === "darwin") {
    try {
      if (fs.existsSync(agentPath)) {
        execSync(`xattr -d com.apple.quarantine "${agentPath}"`, { stdio: "pipe" });
      }
      if (fs.existsSync(ttydPath)) {
        execSync(`xattr -d com.apple.quarantine "${ttydPath}"`, { stdio: "pipe" });
      }
      console.log(`[1agent] Removed macOS quarantine attribute`);
    } catch {
      // xattr fails if attribute doesn't exist, which is fine
    }
  }

  console.log(`[1agent] Successfully installed to ${packageDir}`);
}

main().catch((err) => {
  console.error(err.message);
  console.error(
    "[1agent] Installation failed. You can install manually:\n" +
      `  https://github.com/${GITHUB_REPO}/releases/tag/${VERSION}`
  );
  process.exit(1);
});
