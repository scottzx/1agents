#!/usr/bin/env bash
# Prune old OTA release directories on the self-hosted mirror, keeping
# only the N most-recent versions (by directory mtime). The 40G server
# can't hold every historical release, so the publish workflow calls this
# after each rsync.
#
# Usage:
#   ota-prune.sh [RETAIN] [RELEASES_DIR]
#     RETAIN        how many version dirs to keep (default 3)
#     RELEASES_DIR  where versioned release dirs live
#                   (default /var/www/1agents/releases)
#
# Only directories are considered for pruning — the top-level
# manifest.json "latest" pointer is a file and is never touched.
set -euo pipefail

RETAIN="${1:-3}"
RELEASES_DIR="${2:-/var/www/1agents/releases}"

if ! [[ "$RETAIN" =~ ^[0-9]+$ ]] || [ "$RETAIN" -lt 1 ]; then
    echo "[prune] ERROR: RETAIN must be a positive integer, got '$RETAIN'" >&2
    exit 2
fi
if [ ! -d "$RELEASES_DIR" ]; then
    echo "[prune] ERROR: releases dir not found: $RELEASES_DIR" >&2
    exit 2
fi

# Version dirs, newest first by mtime.
mapfile -t dirs < <(
    find "$RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d -printf '%T@\t%f\n' \
        | sort -rn | cut -f2-
)

total="${#dirs[@]}"
kept=0
removed=0
for d in "${dirs[@]}"; do
    kept=$((kept + 1))
    if [ "$kept" -gt "$RETAIN" ]; then
        echo "[prune] removing old release: $d"
        rm -rf -- "${RELEASES_DIR:?}/$d"
        removed=$((removed + 1))
    fi
done

echo "[prune] releases dir: $RELEASES_DIR"
echo "[prune] total=$total retain=$RETAIN removed=$removed kept=$((total - removed))"
