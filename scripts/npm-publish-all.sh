#!/usr/bin/env bash
# Publish all @1agents packages in dependency order.
# Requires: NPM_TOKEN or existing npm login; version already set via npm-set-version.mjs
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PKG="$ROOT/npm/packages"
TAG_DIST="${NPM_TAG:-latest}"

if [ -n "${NPM_TOKEN:-}" ]; then
  echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > "$ROOT/npm/.npmrc-publish"
  export npm_config_userconfig="$ROOT/npm/.npmrc-publish"
fi

publish_one() {
  local dir="$1"
  echo "=== publishing $(node -p "require('$dir/package.json').name")@$(node -p "require('$dir/package.json').version")"
  # --ignore-scripts: avoid happy postinstall on publisher
  ( cd "$dir" && npm publish --access public --ignore-scripts --tag "$TAG_DIST" )
}

# 1) platform binary packages (leaf)
for p in core-linux-x64 core-linux-arm64 core-darwin-arm64 \
         cc-connect-linux-x64 cc-connect-linux-arm64 cc-connect-darwin-arm64 \
         cc-switch-linux-x64 cc-switch-linux-arm64 cc-switch-darwin-arm64; do
  # skip empty platform packages (no real binary except .gitkeep)
  if ! find "$PKG/$p/bin" -type f ! -name '.gitkeep' 2>/dev/null | grep -q .; then
    echo "=== skip empty $p (no binaries)"
    continue
  fi
  publish_one "$PKG/$p"
done

# 2) content + meta
for p in web skills happy cc-connect cc-switch; do
  publish_one "$PKG/$p"
done

# 3) cli last
publish_one "$PKG/cli"

echo "=== npm-publish-all done"
rm -f "$ROOT/npm/.npmrc-publish"
