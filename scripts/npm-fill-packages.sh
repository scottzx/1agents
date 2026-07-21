#!/usr/bin/env bash
# Fill npm/packages/* with built artifacts for a single host platform.
# Usage:
#   NPM_PLAT=darwin-arm64 \
#   CORE_BIN_DIR=build \
#   WEB_DIST=frontend/dist \
#   ./scripts/npm-fill-packages.sh
#
# Env:
#   NPM_PLAT          linux-x64 | linux-arm64 | darwin-arm64  (required)
#   CORE_BIN_DIR      dir containing 1agents, ttyd, cc-connect, cc-switch
#   WEB_DIST          frontend dist dir
#   SKILLS_SRC        output of package-1skills-python.sh (optional)
#   HAPPY_OUT         dir with happy-cli/ + adapter/ from build-happy-bundle (optional)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PKG="$ROOT/npm/packages"
PLAT="${NPM_PLAT:?NPM_PLAT required (linux-x64|linux-arm64|darwin-arm64)}"
CORE_BIN_DIR="${CORE_BIN_DIR:-$ROOT/build}"
WEB_DIST="${WEB_DIST:-$ROOT/frontend/dist}"

echo "=== npm-fill-packages plat=$PLAT core=$CORE_BIN_DIR"

copy_bin() {
  local src="$1" dest_dir="$2" name="$3"
  mkdir -p "$dest_dir"
  if [ ! -f "$src" ]; then
    echo "ERROR: missing binary $src" >&2
    exit 1
  fi
  cp "$src" "$dest_dir/$name"
  chmod +x "$dest_dir/$name" || true
  echo "  + $dest_dir/$name"
}

# core: 1agents + ttyd
copy_bin "$CORE_BIN_DIR/1agents" "$PKG/core-$PLAT/bin" "1agents"
copy_bin "$CORE_BIN_DIR/ttyd" "$PKG/core-$PLAT/bin" "ttyd"

# cc-connect / cc-switch platform packages
copy_bin "$CORE_BIN_DIR/cc-connect" "$PKG/cc-connect-$PLAT/bin" "cc-connect"
copy_bin "$CORE_BIN_DIR/cc-switch" "$PKG/cc-switch-$PLAT/bin" "cc-switch"

# web (once, shared)
if [ -d "$WEB_DIST" ]; then
  rm -rf "$PKG/web/dist"
  mkdir -p "$PKG/web/dist"
  # copy without source maps when possible
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --exclude='*.map' "$WEB_DIST/" "$PKG/web/dist/"
  else
    tar -C "$WEB_DIST" --exclude='*.map' -cf - . | tar -C "$PKG/web/dist" -xf -
  fi
  echo "  + web/dist (maps excluded)"
else
  echo "WARNING: WEB_DIST missing: $WEB_DIST"
fi

# skills (once)
if [ -n "${SKILLS_SRC:-}" ] && [ -d "$SKILLS_SRC/skill_manager" ]; then
  rm -rf "$PKG/skills/skill_manager" "$PKG/skills/frontend"
  cp -R "$SKILLS_SRC/skill_manager" "$PKG/skills/skill_manager"
  [ -f "$SKILLS_SRC/requirements.txt" ] && cp "$SKILLS_SRC/requirements.txt" "$PKG/skills/"
  [ -f "$SKILLS_SRC/pyproject.toml" ] && cp "$SKILLS_SRC/pyproject.toml" "$PKG/skills/"
  [ -f "$SKILLS_SRC/README.txt" ] && cp "$SKILLS_SRC/README.txt" "$PKG/skills/"
  if [ -d "$SKILLS_SRC/frontend" ]; then
    cp -R "$SKILLS_SRC/frontend" "$PKG/skills/frontend"
  fi
  echo "  + skills from $SKILLS_SRC"
fi

# happy (once)
if [ -n "${HAPPY_OUT:-}" ] && [ -d "$HAPPY_OUT/happy-cli" ]; then
  rm -rf "$PKG/happy/happy-cli" "$PKG/happy/adapter"
  cp -R "$HAPPY_OUT/happy-cli" "$PKG/happy/happy-cli"
  [ -d "$HAPPY_OUT/adapter" ] && cp -R "$HAPPY_OUT/adapter" "$PKG/happy/adapter"
  # ensure no node_modules in published happy-cli (lock only)
  rm -rf "$PKG/happy/happy-cli/node_modules"
  echo "  + happy from $HAPPY_OUT"
fi

# @1agents/acpx — forked acpx runtime (Grok _x.ai/* host extensions) from modules/1acp
# Shared package (once per fill). Skipped when modules/1acp is absent (e.g. tarball-only fill).
ACPX_SRC="${ACPX_SRC:-$ROOT/modules/1acp}"
if [ -d "$ACPX_SRC" ] && [ -f "$ACPX_SRC/package.json" ]; then
  if [ ! -f "$ACPX_SRC/dist/runtime.js" ]; then
    echo "=== building modules/1acp dist for @1agents/acpx ==="
    if command -v pnpm >/dev/null 2>&1; then
      (cd "$ACPX_SRC" && pnpm install --frozen-lockfile && pnpm run build:quiet)
    else
      (cd "$ACPX_SRC" && npm install && npm run build)
    fi
  fi
  if [ ! -f "$ACPX_SRC/dist/runtime.js" ]; then
    echo "ERROR: modules/1acp dist/runtime.js missing after build" >&2
    exit 1
  fi
  rm -rf "$PKG/acpx/dist" "$PKG/acpx/skills"
  mkdir -p "$PKG/acpx/dist"
  # dist only (no source maps — smaller publish; runtime does not need them)
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --exclude='*.map' "$ACPX_SRC/dist/" "$PKG/acpx/dist/"
  else
    tar -C "$ACPX_SRC/dist" --exclude='*.map' -cf - . | tar -C "$PKG/acpx/dist" -xf -
  fi
  if [ -d "$ACPX_SRC/skills" ]; then
    cp -R "$ACPX_SRC/skills" "$PKG/acpx/skills"
  fi
  [ -f "$ACPX_SRC/LICENSE" ] && cp "$ACPX_SRC/LICENSE" "$PKG/acpx/LICENSE"
  # CLI self-ref uses package name "acpx/flows"; rewrite for scoped publish name.
  # Use sed (not perl double-quoted @ interpolation of @1agents).
  find "$PKG/acpx/dist" -type f \( -name '*.js' -o -name '*.d.ts' \) -print0 \
    | xargs -0 sed -i.bak 's|"acpx/flows"|"@1agents/acpx/flows"|g'
  find "$PKG/acpx/dist" -type f -name '*.bak' -delete
  echo "  + acpx dist from $ACPX_SRC"

  # Keep bridge-server.mjs in sync with modules/1acp/bridge-server.js
  # (import runtime from published @1agents/acpx, not monorepo ./src).
  if [ -f "$ACPX_SRC/bridge-server.js" ]; then
    sed 's|from "./src/runtime.js"|from "@1agents/acpx/runtime"|g' \
      "$ACPX_SRC/bridge-server.js" > "$PKG/acp-bridge/bridge-server.mjs"
    echo "  + acp-bridge/bridge-server.mjs synced from modules/1acp"
  fi
elif [ -f "$PKG/acpx/dist/runtime.js" ]; then
  echo "  + acpx: keeping pre-filled dist (modules/1acp not available)"
else
  echo "WARNING: modules/1acp missing and no pre-filled @1agents/acpx dist — Chat ACP bridge will lack Grok host extensions unless filled later"
fi

echo "=== npm-fill-packages done"
