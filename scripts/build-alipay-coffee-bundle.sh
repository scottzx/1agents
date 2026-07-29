#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE="$ROOT/modules/alipay-coffee"
STAGE="${1:-$ROOT/build/alipay-coffee}"

if [ ! -f "$SOURCE/package-lock.json" ]; then
  echo "✗ alipay-coffee package-lock.json is missing" >&2
  exit 1
fi

if [ ! -f "$SOURCE/node_modules/alipay-sdk/package.json" ] || [ ! -f "$SOURCE/node_modules/express/package.json" ]; then
  echo "=== Installing locked alipay-coffee production dependencies..."
  (cd "$SOURCE" && npm ci --omit=dev --ignore-scripts --no-audit --no-fund)
fi

echo "=== Staging alipay-coffee bundle -> $STAGE"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp "$SOURCE/package.json" "$SOURCE/package-lock.json" "$STAGE/"
cp -R "$SOURCE/src" "$SOURCE/public" "$SOURCE/node_modules" "$STAGE/"

echo "=== alipay-coffee bundle ready"
