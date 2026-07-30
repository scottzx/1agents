#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BANNED_SOURCE_DIRS=(
  "$ROOT/modules/HarnessKit/public/icons"
  "$ROOT/modules/HarnessKit/src/components/shared/agent-mascot"
)

BANNED_HASHES=(
  2f434dd9b3345c12172439556b38ba3f092dea643e06ced10efc365ccec4fe0e
  89d7cc3e54fabada79baf2c3879591b89f6c5451f240c3003338e6682c77d834
  c77e2149e48a4a24e7e6d14af2d96e1443d564e600fbe068ecd956588dfc53bd
  b0c846526a809ee0300cbeb769cd80cb0aee2a2ebde7577cfb7beb9b07919eca
  fd4edf646cf997fd6eae90aa4c5c7df62452d03275f4b34259e7973e36884e5e
  1da221deb74972167731e11abecc6eabc36c7de8d8d78d9422bbe988bc476b6e
  2890716a77bee84b06cc1c717e2e124c2d5a4b5058cdc764cd1f5e54bf6b4a03
  6cf638545cd7dbfc3edde5490db83b96b648ac807df6fa114227d09b5fafd189
  53c9df969a159676ebd696e565875f4b5574a16908e614480ef24c37ddeedbe4
  e4c20dcd35b98b4c7eaca4aa8aebbe3283fba84604674e55a917e5fa30a4e0cb
  783b6b1f232da487a8706f3a5387fa69e63195f5951e37662fa9ee6cdc292db5
  90531a452f99d6cf413dce9a275894afa4f67b094c4700cc41ab0223d84ebcd1
  fed798cb35202a842375fc24f3a5b797336946c11fef8c4217e0c46fd21799e1
  ef36d4996a0c04778bf61168681da50e354605a25be7e11e1a71c4711d4d2221
  88490ce567f009276784e04e61fe4472e97464951b50f28a0718ed27da8452c6
  37713b264d6fff27679255cfb177735199dda37a459c92a7554d07519be7abf7
  5ff792d69349d4b64a0283e58db11ff761b4c66f73c99164bc14ae277ba44198
)

for banned_dir in "${BANNED_SOURCE_DIRS[@]}"; do
  if [ -d "$banned_dir" ] && find "$banned_dir" -type f -print -quit | grep -q .; then
    echo "ERROR: protected HarnessKit artwork source remains: $banned_dir" >&2
    exit 1
  fi
done

if command -v sha256sum >/dev/null 2>&1; then
  hash_file() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  hash_file() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo "ERROR: sha256sum or shasum is required" >&2
  exit 1
fi

TASK_TMP="$(mktemp -d "${TMPDIR:-/tmp}/1agents-harnesskit-audit.XXXXXX")"
trap 'rm -rf "$TASK_TMP"' EXIT

scan_file() {
  local tree="$1" file="$2"
  local rel digest banned

  rel="${file#"$tree"/}"
  case "$rel" in
    *public/icons/*|*agent-mascot/*|*app-icon-1.png|*app-icon-2.png|*harnesskit-icons.png)
      echo "ERROR: protected HarnessKit artwork path in artifact: $rel" >&2
      return 1
      ;;
  esac

  digest="$(hash_file "$file")"
  for banned in "${BANNED_HASHES[@]}"; do
    if [ "$digest" = "$banned" ]; then
      echo "ERROR: protected HarnessKit artwork hash in artifact: $rel ($digest)" >&2
      return 1
    fi
  done
}

scan_tree() {
  local tree="$1" file
  while IFS= read -r -d '' file; do
    scan_file "$tree" "$file"
  done < <(find "$tree" -type f -print0)
}

scan_source_tree() {
  local tree="$1" file
  while IFS= read -r -d '' file; do
    scan_file "$tree" "$file"
  done < <(
    find "$tree" \
      \( -name .git -o -name node_modules -o -name target -o -name dist -o -name dist-embed \) -prune \
      -o -type f -print0
  )
}

scan_source_tree "$ROOT/modules/HarnessKit"

if [ "$#" -eq 0 ]; then
  echo "HarnessKit protected source artwork is absent."
  exit 0
fi

index=0
for target in "$@"; do
  if [ ! -e "$target" ]; then
    echo "ERROR: artifact target does not exist: $target" >&2
    exit 1
  fi

  index=$((index + 1))
  unpacked="$TASK_TMP/$index"
  mkdir -p "$unpacked"

  if [ -d "$target" ]; then
    scan_tree "$target"
  else
    case "$target" in
      *.tar|*.tar.gz|*.tgz)
        tar -xf "$target" -C "$unpacked"
        scan_tree "$unpacked"
        ;;
      *.zip)
        unzip -q "$target" -d "$unpacked"
        scan_tree "$unpacked"
        ;;
      *)
        cp "$target" "$unpacked/"
        scan_tree "$unpacked"
        ;;
    esac
  fi
done

echo "HarnessKit artifact audit passed for $# target(s)."
