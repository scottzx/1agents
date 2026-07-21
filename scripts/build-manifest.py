#!/usr/bin/env python3
"""Build the root OTA manifest (manifest.json) from CI build artifacts.

Usage:
    python scripts/build-manifest.py \
        --version v20260615-1 \
        --artifacts ./_artifacts \
        --repo scottzx/1Agents \
        --output manifest.json

The script scans the artifacts directory for per-platform tarballs named either:

* ``1agents-{os}-{arch}.tar.gz`` (CDN stage / legacy)
* ``1agents-{version}-{os}-{arch}.tar.gz`` (auto-release GitHub asset names)

and an optional ``frontend-v{version}.tar.gz`` entry, computes SHA256 hashes,
and writes a manifest.json that matches the schema documented in
docs/ota-architecture.md §4.1.

Asset URL rules:
* With ``--base-url`` (COS/CDN): ``{base}/{version}/1agents-{os}-{arch}.tar.gz``
  (files are staged under the version prefix with unversioned basenames).
* Without ``--base-url`` (GitHub Releases): use the **actual** artifact basename
  so the URL matches what softprops/action-gh-release uploaded
  (``1agents-{version}-{os}-{arch}.tar.gz``).

CI integration (auto-release.yml):
    The release job calls this script after all build-* jobs complete and
    their artifacts have been downloaded into the working directory.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
from datetime import datetime, timezone

MANIFEST_VERSION = 1  # bump when the schema changes

# Unversioned (CDN stage): 1agents-linux-amd64.tar.gz
_UNVERSIONED = re.compile(
    r"^1agents-(?P<os>linux|darwin|windows)-(?P<arch>amd64|arm64)\.tar\.gz$"
)
# Versioned (GitHub release assets): 1agents-v20260720-2-linux-amd64.tar.gz
_VERSIONED = re.compile(
    r"^1agents-(?P<ver>.+)-(?P<os>linux|darwin|windows)-(?P<arch>amd64|arm64)\.tar\.gz$"
)


def sha256_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while chunk := f.read(1 << 20):  # 1 MiB
            h.update(chunk)
    return h.hexdigest()


def _norm_tag(version: str) -> str:
    """Normalize a release tag for loose matching (strip leading v)."""
    return version.lstrip("vV").strip()


def find_tarballs(artifacts_dir: str, version: str) -> dict[str, str]:
    """Return {platform_key: abs_path} for backend tarballs.

    Prefers a tarball whose embedded version matches ``version`` when both
    versioned and unversioned copies exist for the same platform.
    """
    if not os.path.isdir(artifacts_dir):
        return {}

    want = _norm_tag(version)
    # platform -> (path, score)  score: 2 = exact version match, 1 = unversioned, 0 = other version
    best: dict[str, tuple[str, int]] = {}

    for name in os.listdir(artifacts_dir):
        path = os.path.join(artifacts_dir, name)
        if not os.path.isfile(path):
            continue

        m = _UNVERSIONED.match(name)
        if m:
            platform = f"{m.group('os')}-{m.group('arch')}"
            score = 1
        else:
            m = _VERSIONED.match(name)
            if not m:
                continue
            platform = f"{m.group('os')}-{m.group('arch')}"
            score = 2 if _norm_tag(m.group("ver")) == want else 0

        prev = best.get(platform)
        if prev is None or score > prev[1]:
            best[platform] = (path, score)

    return {plat: path for plat, (path, _) in sorted(best.items())}


def frontend_asset_name(version: str) -> str:
    """Canonical frontend OTA asset name: frontend-vYYYYMMDD-N.tar.gz."""
    tag = version if version.lower().startswith("v") else f"v{version}"
    return f"frontend-{tag}.tar.gz"


def find_frontend(artifacts_dir: str, version: str) -> str | None:
    """Return the path to the frontend OTA tarball, if present.

    Accepts the canonical ``frontend-vYYYYMMDD-N.tar.gz`` name plus a few
    legacy spellings (double-v, bare date) so older staged artifacts still
    resolve.
    """
    tag = version if version.lower().startswith("v") else f"v{version}"
    bare = _norm_tag(version)
    candidates = [
        frontend_asset_name(version),  # frontend-v20260720-9.tar.gz
        f"frontend-v{tag}.tar.gz",  # legacy double-v: frontend-vv...
        f"frontend-v{bare}.tar.gz",
        f"frontend-{bare}.tar.gz",
        f"frontend-{version}.tar.gz",
    ]
    seen: set[str] = set()
    for name in candidates:
        if name in seen:
            continue
        seen.add(name)
        path = os.path.join(artifacts_dir, name)
        if os.path.isfile(path):
            return path
    return None


def _asset_url(
    repo: str,
    version: str,
    base_url: str | None,
    filename: str,
) -> str:
    """Build the download URL for a release asset.

    When ``base_url`` is None we emit the canonical GitHub Releases URL.
    When ``base_url`` is set (self-hosted mirror) we emit
    ``{base_url}/{version}/{filename}`` instead.
    """
    if base_url:
        return f"{base_url.rstrip('/')}/{version}/{filename}"
    return f"https://github.com/{repo}/releases/download/{version}/{filename}"


def build_manifest(
    version: str,
    repo: str,
    channel: str,
    artifacts_dir: str,
    base_url: str | None = None,
) -> dict:
    backend_platforms = {}
    for platform, path in find_tarballs(artifacts_dir, version).items():
        size = os.path.getsize(path)
        sha = sha256_file(path)
        # CDN stage renames to unversioned basenames under {version}/;
        # GitHub Releases keep the versioned asset name produced by CI.
        if base_url:
            filename = f"1agents-{platform}.tar.gz"
        else:
            filename = os.path.basename(path)
        url = _asset_url(repo, version, base_url, filename)
        backend_platforms[platform] = {
            "url": url,
            "size": size,
            "sha256": sha,
        }

    frontend = {"version": version, "entry": "", "integrity": ""}
    fe_path = find_frontend(artifacts_dir, version)
    if fe_path:
        # CDN objects use the canonical basename under {version}/;
        # GitHub Releases use whatever file was actually uploaded.
        fe_name = frontend_asset_name(version) if base_url else os.path.basename(fe_path)
        fe_url = _asset_url(repo, version, base_url, fe_name)
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
    p.add_argument(
        "--base-url",
        default=None,
        help=(
            "Self-hosted mirror base URL (e.g. https://agents-ota.dreammate.work). "
            "When set, asset URLs become {base_url}/{version}/1agents-{os}-{arch}.tar.gz. "
            "When omitted, GitHub Releases URLs use the actual artifact basename."
        ),
    )
    args = p.parse_args()

    manifest = build_manifest(
        args.version, args.repo, args.channel, args.artifacts, args.base_url
    )

    with open(args.output, "w") as f:
        json.dump(manifest, f, indent=2)
        f.write("\n")

    print(f"[manifest] wrote {args.output}")
    platforms = manifest["components"]["backend"]["platforms"]
    if not platforms:
        print(
            "[manifest] WARNING: no backend platforms found — "
            "expected 1agents-{os}-{arch}.tar.gz or "
            f"1agents-{args.version}-{{os}}-{{arch}}.tar.gz under {args.artifacts}",
            file=sys.stderr,
        )
    for platform, meta in platforms.items():
        print(f"[manifest]   {platform}  {meta['url']}")
    fe = manifest["components"]["frontend"]
    if fe.get("entry"):
        print(f"[manifest]   frontend  {fe['entry']}")
    else:
        print("[manifest]   frontend  (no frontend-v*.tar.gz — entry empty)")


if __name__ == "__main__":
    main()
