#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${1:?usage: stage-harnesskit-compliance.sh DEST_DIR}"

mkdir -p "$DEST"
cp "$ROOT/modules/HarnessKit/LICENSE" "$DEST/HarnessKit-LICENSE"
cp "$ROOT/modules/HarnessKit/UPSTREAM.md" "$DEST/HarnessKit-UPSTREAM.md"
cp "$ROOT/modules/HarnessKit/ASSET-LICENSES.md" "$DEST/HarnessKit-ASSET-LICENSES.md"
cp "$ROOT/THIRD_PARTY.md" "$DEST/THIRD_PARTY.md"
node "$ROOT/scripts/generate-harnesskit-sbom.mjs" "$DEST/harnesskit.spdx.json"
