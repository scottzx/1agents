#!/usr/bin/env python3
"""Build the root OTA manifest (manifest.json) from CI build artifacts.

Usage:
    python scripts/build-manifest.py \
        --version v20260615-1 \
        --artifacts ./_artifacts \
        --repo scottzx/1Agents \
        --output manifest.json

The script scans the artifacts directory for per-platform tarballs named
``1agents-{os}-{arch}.tar.gz`` and a ``frontend-v{version}.tar.gz`` entry,
computes SHA256 hashes, and writes a manifest.json that matches the schema
documented in docs/ota-architecture.md §4.1.

CI integration (auto-release.yml):
    The release job calls this script after all build-* jobs complete and
    their artifacts have been downloaded into the _artifacts directory.
"""

import argparse
import hashlib
import json
import os
import re
import sys
from datetime import datetime, timezone

MANIFEST_VERSION = 1  # bump when the schema changes


def sha256_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while chunk := f.read(1 << 20):  # 1 MiB
            h.update(chunk)
    return h.hexdigest()


def find_tarballs(artifacts_dir: str) -> dict[str, str]:
    """Return {platform_key: abs_path} for backend tarballs."""
    result = {}
    pattern = re.compile(r"^1agents-(linux|darwin|windows)-(amd64|arm64)\.tar\.gz$")
    if not os.path.isdir(artifacts_dir):
        return result
    for name in os.listdir(artifacts_dir):
        m = pattern.match(name)
        if m:
            platform = f"{m.group(1)}-{m.group(2)}"
            result[platform] = os.path.join(artifacts_dir, name)
    return result


def find_frontend(artifacts_dir: str, version: str) -> str | None:
    """Return the path to frontend-v{version}.tar.gz, or None."""
    name = f"frontend-v{version}.tar.gz"
    path = os.path.join(artifacts_dir, name)
    return path if os.path.isfile(path) else None


def build_manifest(
    version: str,
    repo: str,
    channel: str,
    artifacts_dir: str,
) -> dict:
    backend_platforms = {}
    for platform, path in sorted(find_tarballs(artifacts_dir).items()):
        size = os.path.getsize(path)
        sha = sha256_file(path)
        url = (
            f"https://github.com/{repo}/releases/download/{version}"
            f"/1agents-{platform}.tar.gz"
        )
        backend_platforms[platform] = {
            "url": url,
            "size": size,
            "sha256": sha,
        }

    frontend = {"version": version, "entry": "", "integrity": ""}
    fe_path = find_frontend(artifacts_dir, version)
    if fe_path:
        fe_url = (
            f"https://github.com/{repo}/releases/download/{version}"
            f"/frontend-v{version}.tar.gz"
        )
        frontend["entry"] = fe_url
        frontend["integrity"] = f"sha256-{sha256_file(fe_path)}"

    return {
        "channel": channel,
        "released_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "min_supported": "0.0.0",
        "manifest_version": MANIFEST_VERSION,
        "components": {
            "frontend": frontend,
            "backend": {
                "version": version,
                "platforms": backend_platforms,
            },
        },
        "previous": [],
    }


def main() -> None:
    p = argparse.ArgumentParser(description="Build 1agents OTA root manifest")
    p.add_argument("--version", required=True, help="Release tag, e.g. v20260615-1")
    p.add_argument("--artifacts", required=True, help="Directory with build artifacts")
    p.add_argument("--repo", default="scottzx/1Agents", help="GitHub slug")
    p.add_argument("--channel", default="stable", help="Release channel")
    p.add_argument("--output", default="manifest.json", help="Output file path")
    args = p.parse_args()

    manifest = build_manifest(args.version, args.repo, args.channel, args.artifacts)

    with open(args.output, "w") as f:
        json.dump(manifest, f, indent=2)
        f.write("\n")

    print(f"[manifest] wrote {args.output}")
    for platform in manifest["components"]["backend"]["platforms"]:
        print(f"[manifest]   {platform}")


if __name__ == "__main__":
    main()
