#!/usr/bin/env bash
# Build HarnessKit + cc-connect embed ESM bundles and stage them into the
# main frontend static tree so production (-static / @1agents/web) can serve them.
#
# Outputs (default):
#   frontend/dist/embed/harnesskit-embed.js
#   frontend/dist/embed/cc-connect-embed.js
#
# Backend serves these at /api/embed/*.js via StaticDir/embed/ (see server.go).
#
# Usage:
#   ./scripts/build-module-embeds.sh              # stage into frontend/dist/embed
#   ./scripts/build-module-embeds.sh /path/to/out # custom out dir
#   FORCE_EMBED_REBUILD=1 ./scripts/build-module-embeds.sh  # always reinstall+rebuild
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/frontend/dist/embed}"
HARNESSKIT_SRC="$ROOT/modules/HarnessKit"
CC_WEB_SRC="$ROOT/modules/cc-connect/web"
FORCE="${FORCE_EMBED_REBUILD:-0}"

mkdir -p "$OUT"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: required command not found: $1" >&2
    exit 1
  }
}

need_cmd node
need_cmd npm

build_harnesskit_embed() {
  local dest_dir="$OUT/harnesskit"

  if [ ! -d "$HARNESSKIT_SRC" ] || [ ! -f "$HARNESSKIT_SRC/package.json" ]; then
    echo "ERROR: modules/HarnessKit missing (init submodule: git submodule update --init modules/HarnessKit)" >&2
    exit 1
  fi

  echo "=== building HarnessKit static SPA frontend"
  (
    cd "$HARNESSKIT_SRC"
    if [ ! -d node_modules ] || [ "$FORCE" = "1" ]; then
      npm ci --ignore-scripts 2>/dev/null || npm install --ignore-scripts
    fi
    npm run build
  )

  mkdir -p "$dest_dir"
  cp -R "$HARNESSKIT_SRC/dist/"* "$dest_dir/"
  echo "  + HarnessKit static assets staged into $dest_dir"
}

build_cc_connect_embed() {
  local dest="$OUT/cc-connect-embed.js"
  local built="$CC_WEB_SRC/dist-embed/cc-connect-embed.js"

  if [ ! -d "$CC_WEB_SRC" ] || [ ! -f "$CC_WEB_SRC/package.json" ]; then
    echo "ERROR: modules/cc-connect/web missing (init submodule: git submodule update --init modules/cc-connect)" >&2
    exit 1
  fi

  if [ "$FORCE" != "1" ] && [ -f "$built" ]; then
    echo "=== reuse existing $built"
  else
    echo "=== building cc-connect embed (npm run build:embed)"
    (
      cd "$CC_WEB_SRC"
      if [ ! -d node_modules ] || [ "$FORCE" = "1" ]; then
        npm ci --ignore-scripts 2>/dev/null || npm install --ignore-scripts
      fi
      npm run build:embed
    )
  fi

  if [ ! -f "$built" ]; then
    echo "ERROR: expected $built after build:embed" >&2
    exit 1
  fi
  cp "$built" "$dest"
  echo "  + $dest ($(wc -c <"$dest" | tr -d ' ') bytes)"
}

echo "=== staging module embeds -> $OUT"
build_harnesskit_embed
build_cc_connect_embed
echo "=== module embeds ready"
ls -la "$OUT"
