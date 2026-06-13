#!/usr/bin/env python3
"""Build per-platform Tauri updater manifests.

Usage:
    python scripts/build-tauri-manifest.py \
        --version v20260615-1 \
        --artifacts ./_artifacts \
        --repo scottzx/1Agents \
        --output-dir ./_artifacts/manifests/desktop

Each per-platform manifest is written as ``desktop-{target}-{arch}.json``
— the naming convention expected by tauri-plugin-updater's {{target}}
and {{arch}} template variables.

See docs/ota-architecture.md §4.2 for the JSON schema.
"""

import argparse
import json
import os
import re
from datetime import datetime, timezone

# tauri-plugin-updater platform keys are TARGET-ARCH (e.g. darwin-aarch64,
# linux-x86_64) — different from the root manifest which uses GOOS-GOARCH.
TAURI_TARGET_MAP = {
    "windows": "windows",
    "darwin": "darwin",
    "linux": "linux",
}

ARCH_MAP = {
    "amd64": "x86_64",
    "arm64": "aarch64",
}

# Extension to filename glob pattern
PLATFORM_GLOBS: list[tuple[list[str], list[str], str]] = [
    # (os_keys, arch_keys, file_glob)
    (["darwin"], ["amd64", "arm64"], "1Agents_*_*_{arch}.dmg"),
    (["darwin"], ["amd64", "arm64"], "1Agents_*_*_{arch}.app.tar.gz"),
    (["windows"], ["amd64"], "1Agents_*_*_{arch}-setup.exe"),
    (["windows"], ["amd64"], "1Agents_*_*_{arch}.msi"),
    (["linux"], ["amd64", "arm64"], "1Agents_*_*_{arch}.AppImage"),
    (["linux"], ["amd64", "arm64"], "1Agents_*_*_{arch}.deb"),
]


def find_bundles(artifacts_dir: str) -> dict[str, str]:
    """Return {tauri_platform_key: url_path}."""
    result: dict[str, str] = {}
    if not os.path.isdir(artifacts_dir):
        return result
    for name in os.listdir(artifacts_dir):
        # Parse name like "1Agents_0.4.0_x64-setup.exe"
        m = re.match(
            r"1Agents_[^_]+_(x64|aarch64)[\-.](dmg|exe|msi|AppImage|deb|app\.tar\.gz)",
            name,
        )
        if not m:
            continue
        arch = m.group(1)
        ext = m.group(2)
        if ext == "app.tar.gz":
            ext = "app.tar.gz"

        # Determine OS from extension
        for os_key, arch_keys, _ in PLATFORM_GLOBS:
            if arch in arch_keys:
                tauri_arch = ARCH_MAP.get(arch, arch)
                tauri_target = f"{TAURI_TARGET_MAP[os_key]}-{tauri_arch}"
                result[tauri_target] = name
    return result


def build_tauri_manifests(
    version: str, repo: str, notes: str, artifacts_dir: str
) -> dict[str, dict]:
    bundles = find_bundles(artifacts_dir)
    manifests: dict[str, dict] = {}
    for tauri_key, filename in sorted(bundles.items()):
        url = (
            f"https://github.com/{repo}/releases/download/{version}/{filename}"
        )
        manifests[tauri_key] = {
            "version": version,
            "notes": notes,
            "pub_date": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "platforms": {
                tauri_key: {
                    "url": url,
                    # V1: no signature — pubkey is "" in tauri.conf.json
                }
            },
        }
    return manifests


def main() -> None:
    p = argparse.ArgumentParser(description="Build Tauri updater manifests")
    p.add_argument("--version", required=True)
    p.add_argument("--artifacts", required=True)
    p.add_argument("--repo", default="scottzx/1Agents")
    p.add_argument("--notes", default="", help="Release notes (plain text)")
    p.add_argument("--output-dir", default="./_artifacts/manifests/desktop")
    args = p.parse_args()

    manifests = build_tauri_manifests(
        args.version, args.repo, args.notes, args.artifacts
    )
    os.makedirs(args.output_dir, exist_ok=True)

    for tauri_key, manifest in manifests.items():
        out_path = os.path.join(args.output_dir, f"desktop-{tauri_key}.json")
        with open(out_path, "w") as f:
            json.dump(manifest, f, indent=2)
            f.write("\n")
        print(f"[tauri-manifest] wrote {out_path}")

    if not manifests:
        print("[tauri-manifest] WARNING: no desktop bundles found — "
              "did you run build-desktop before this script?")


if __name__ == "__main__":
    main()
