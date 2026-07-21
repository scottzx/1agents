#!/usr/bin/env bash
#
# package-1skills-python.sh — stage a slim host-Python 1skills tree for release.
#
# Layout written to OUT (default: build/1skills):
#   skill_manager/   Python package (no __pycache__)
#   frontend/dist/   UI assets for skill-manager serve (if present)
#   requirements.txt
#   pyproject.toml
#
# The release does NOT ship a PyInstaller binary or CPython runtime; the
# 1agents SkillsSupervisor creates a venv with the host python3 and installs
# requirements on first launch.
#
# Usage: scripts/package-1skills-python.sh [OUT_DIR]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/modules/1skills"
OUT="${1:-$ROOT/build/1skills}"

if [ ! -d "$SRC/skill_manager" ]; then
  echo "✗ modules/1skills/skill_manager not found" >&2
  exit 1
fi

echo "=== packaging 1skills Python source -> $OUT"
rm -rf "$OUT"
mkdir -p "$OUT"

# Copy package sources without caches / egg-info (portable: no rsync required)
mkdir -p "$OUT/skill_manager"
tar -C "$SRC/skill_manager" \
  --exclude='__pycache__' --exclude='*.pyc' --exclude='*.pyo' \
  --exclude='.pytest_cache' --exclude='*.egg-info' \
  -cf - . | tar -C "$OUT/skill_manager" -xf -

cp "$SRC/requirements.txt" "$OUT/requirements.txt"
cp "$SRC/pyproject.toml" "$OUT/pyproject.toml"

if [ -d "$SRC/frontend/dist" ]; then
  mkdir -p "$OUT/frontend/dist"
  tar -C "$SRC/frontend/dist" -cf - . | tar -C "$OUT/frontend/dist" -xf -
else
  echo "WARNING: modules/1skills/frontend/dist missing; skill-manager UI may be empty"
fi

# Optional marker for operators / debugging
cat > "$OUT/README.txt" <<'EOF'
1skills (skill-manager) Python source tree for host Python.

1agents will create a venv (under ~/.1agents/1skills/.venv or this tree's
.venv) and `pip install -r requirements.txt` on first launch. Requires
Python >= 3.11 on the host.
EOF

echo "=== 1skills python package ready:"
du -sh "$OUT" "$OUT/skill_manager" "$OUT/frontend/dist" 2>/dev/null || du -sh "$OUT"
