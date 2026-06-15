#!/usr/bin/env python3
"""Upload files to Aliyun Drive via the official web API.

Used by the release pipeline to push desktop installers (.dmg/.msi/.exe) to a
private Aliyun Drive folder instead of publishing them to the public GitHub
Release. Desktop builds are sold as a one-time purchase, so installers are
distributed privately; the server build / npm package release normally.

Usage:
    ALIYUNPAN_REFRESH_TOKEN=<token> \
        python3 scripts/upload-aliyundrive.py <local_path> <remote_dir>

  <local_path>  a file, or a directory (uploaded recursively)
  <remote_dir>  absolute Aliyun Drive path, e.g. /1Agents发布/v20260615-1

Auth: ALIYUNPAN_REFRESH_TOKEN is the Aliyun Drive web refresh_token
(obtained from alipan.com localStorage). It rotates/expires; on failure
re-grab the token and update the GitHub secret.
"""

import json
import math
import os
import sys
import urllib.request

API = "https://api.aliyundrive.com"
CHUNK = 100 * 1024 * 1024  # 100 MiB per upload part


def api(url, payload, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(
        url, data=json.dumps(payload).encode(), headers=headers, method="POST"
    )
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)


def get_access(rt):
    tok = api(f"{API}/token/refresh", {"refresh_token": rt})
    return tok["access_token"], tok["default_drive_id"], tok.get("nick_name")


def ensure_folder(drive, at, remote_dir):
    parent = "root"
    for name in [p for p in remote_dir.strip("/").split("/") if p]:
        r = api(
            f"{API}/adrive/v2/file/createWithFolders",
            {
                "drive_id": drive,
                "parent_file_id": parent,
                "name": name,
                "type": "folder",
                "check_name_mode": "refuse",
            },
            at,
        )
        parent = r["file_id"]
    return parent


def upload_file(drive, at, parent, path):
    size = os.path.getsize(path)
    nparts = max(1, math.ceil(size / CHUNK))
    create = api(
        f"{API}/adrive/v2/file/createWithFolders",
        {
            "drive_id": drive,
            "parent_file_id": parent,
            "name": os.path.basename(path),
            "type": "file",
            "check_name_mode": "auto_rename",
            "size": size,
            "part_info_list": [{"part_number": i + 1} for i in range(nparts)],
        },
        at,
    )
    if create.get("rapid_upload"):
        print(f"  rapid-upload hit: {os.path.basename(path)}")
        return
    file_id, upload_id = create["file_id"], create["upload_id"]
    urls = [p["upload_url"] for p in create["part_info_list"]]
    with open(path, "rb") as f:
        for i, u in enumerate(urls):
            data = f.read(CHUNK)
            put = urllib.request.Request(u, data=data, method="PUT")
            # OSS presigned URL is signed with an empty Content-Type; urllib would
            # otherwise auto-add application/x-www-form-urlencoded and trigger 403.
            put.add_header("Content-Type", "")
            try:
                with urllib.request.urlopen(put, timeout=1800) as r:
                    print(f"  part {i + 1}/{nparts} HTTP {r.status}")
            except urllib.error.HTTPError as e:
                print(f"  part {i + 1} PUT failed HTTP {e.code}: {e.read().decode()[:500]}")
                raise
    api(
        f"{API}/v2/file/complete",
        {"drive_id": drive, "file_id": file_id, "upload_id": upload_id},
        at,
    )
    print(f"  uploaded: {os.path.basename(path)} ({size} bytes)")


def collect(local_path):
    if os.path.isfile(local_path):
        return [local_path]
    files = []
    for root, _, names in os.walk(local_path):
        for n in names:
            files.append(os.path.join(root, n))
    return sorted(files)


def main():
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(2)
    local_path, remote_dir = sys.argv[1], sys.argv[2]

    rt = os.environ.get("ALIYUNPAN_REFRESH_TOKEN", "").strip()
    if not rt:
        print("::error::ALIYUNPAN_REFRESH_TOKEN is empty / not set")
        sys.exit(1)

    files = collect(local_path)
    if not files:
        print(f"::error::no files found under {local_path}")
        sys.exit(1)

    at, drive, nick = get_access(rt)
    print(f"login OK as {nick}, drive {drive}")
    parent = ensure_folder(drive, at, remote_dir)
    print(f"target folder ready: {remote_dir} ({parent})")
    for path in files:
        print(f"-> {path}")
        upload_file(drive, at, parent, path)
    print(f"DONE: {len(files)} file(s) uploaded to {remote_dir}")


if __name__ == "__main__":
    main()
