#!/usr/bin/env bash
# Publish all @1agents packages in dependency order.
# Requires: NPM_TOKEN (or npm login) with permission to create packages under @1agents.
# Version must already be set via: node scripts/npm-set-version.mjs <ver>
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PKG="$ROOT/npm/packages"
TAG_DIST="${NPM_TAG:-latest}"

if [ -n "${NPM_TOKEN:-}" ]; then
  # Prefer explicit auth file so publish works even if setup-node used NODE_AUTH_TOKEN.
  echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > "$ROOT/npm/.npmrc-publish"
  echo "registry=https://registry.npmjs.org/" >> "$ROOT/npm/.npmrc-publish"
  export npm_config_userconfig="$ROOT/npm/.npmrc-publish"
  # setup-node also looks at NODE_AUTH_TOKEN
  export NODE_AUTH_TOKEN="${NODE_AUTH_TOKEN:-$NPM_TOKEN}"
fi

echo "=== npm whoami (auth check) ==="
if ! npm whoami 2>/dev/null; then
  echo "ERROR: npm is not authenticated. Set secrets.NPM_TOKEN (Automation or Granular token" >&2
  echo "  with Read+Write to the @1agents org, including permission to create NEW packages)." >&2
  echo "  E404 on PUT for a new name almost always means the token cannot create packages under @1agents." >&2
  exit 1
fi

# Soft check: wire must be readable if public; new packages will 404 until first publish — that is OK.
if npm view @1agents/wire version >/dev/null 2>&1; then
  echo "=== @1agents/wire is visible (org exists): $(npm view @1agents/wire version)"
else
  echo "WARNING: cannot view @1agents/wire — token may lack org read access"
fi

publish_one() {
  local dir="$1"
  local name version
  name="$(node -p "require('$dir/package.json').name")"
  version="$(node -p "require('$dir/package.json').version")"
  echo "=== publishing ${name}@${version}"
  if ! ( cd "$dir" && npm publish --access public --ignore-scripts --tag "$TAG_DIST" ); then
    echo "" >&2
    echo "ERROR: failed to publish ${name}@${version}" >&2
    echo "If you see E404 on PUT https://registry.npmjs.org/@1agents%2f..." >&2
    echo "  npm returns 404 (not 403) when the token cannot create/update that package." >&2
    echo "  Fix on npmjs.com:" >&2
    echo "  1) Open https://www.npmjs.com/settings/~/tokens" >&2
    echo "  2) Create Granular Access Token with:" >&2
    echo "       - Packages: Read and write" >&2
    echo "       - Organizations: 1agents → permission to publish" >&2
    echo "       - Packages: All packages (or allow creating new packages under @1agents)" >&2
    echo "  3) Put the token in GitHub secret NPM_TOKEN" >&2
    echo "  4) Confirm your user is a member of the @1agents org with publish rights" >&2
    echo "  Note: a token that only lists @1agents/wire cannot publish @1agents/core-linux-x64." >&2
    exit 1
  fi
}

# 1) platform binary packages (leaf)
for p in core-linux-x64 core-linux-arm64 core-darwin-arm64 \
         cc-connect-linux-x64 cc-connect-linux-arm64 cc-connect-darwin-arm64; do
  if ! find "$PKG/$p/bin" -type f ! -name '.gitkeep' 2>/dev/null | grep -q .; then
    echo "=== skip empty $p (no binaries)"
    continue
  fi
  publish_one "$PKG/$p"
done

# 2) content + meta
# acpx (forked ACP runtime) must publish before acp-bridge (depends on @1agents/acpx)
for p in web happy acpx acp-bridge cc-connect; do
  if [ "$p" = "acpx" ] && [ ! -f "$PKG/acpx/dist/runtime.js" ]; then
    echo "ERROR: npm/packages/acpx/dist/runtime.js missing — run npm-fill-packages.sh (builds modules/1acp)" >&2
    exit 1
  fi
  publish_one "$PKG/$p"
done

# 3) cli last
publish_one "$PKG/cli"

echo "=== npm-publish-all done"
rm -f "$ROOT/npm/.npmrc-publish"
