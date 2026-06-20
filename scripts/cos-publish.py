#!/usr/bin/env python3
"""Upload staged OTA artifacts to Tencent COS, then keep only the last N versions.

Layout on the bucket:
    <tag>/1agents-<os>-<arch>.tar.gz   per-version artifacts
    <tag>/manifest.json                per-version manifest
    manifest.json                      latest-version pointer (overwritten each run)

Usage:
    cos-publish.py <tag> <stage_dir> [retain=3]

Env: COS_SECRET_ID, COS_SECRET_KEY, COS_BUCKET
     COS_ENDPOINT (optional) / COS_REGION (optional) — when neither is set,
     defaults to COS global acceleration (cos.accelerate.myqcloud.com) so the
     overseas GitHub runner uploads via the nearest PoP.

Objects are written public-read so OTA clients can download them without
credentials (the bucket must not block public access).
"""
import os
import re
import sys

from qcloud_cos import CosConfig, CosS3Client

ACCELERATE_ENDPOINT = "cos.accelerate.myqcloud.com"


def main() -> None:
    tag = sys.argv[1]
    stage = sys.argv[2]
    retain = int(sys.argv[3]) if len(sys.argv) > 3 else 3

    bucket = os.environ["COS_BUCKET"]
    endpoint = os.environ.get("COS_ENDPOINT", "").strip()
    region = os.environ.get("COS_REGION", "").strip()

    kwargs = {
        "SecretId": os.environ["COS_SECRET_ID"],
        "SecretKey": os.environ["COS_SECRET_KEY"],
    }
    if endpoint:
        kwargs["Endpoint"] = endpoint
    elif region:
        kwargs["Region"] = region
    else:
        kwargs["Endpoint"] = ACCELERATE_ENDPOINT  # global acceleration
    client = CosS3Client(CosConfig(**kwargs))

    # 1) Upload this version's files under <tag>/, refresh the latest pointer.
    for name in sorted(os.listdir(stage)):
        path = os.path.join(stage, name)
        if not os.path.isfile(path):
            continue
        key = f"{tag}/{name}"
        ctype = "application/json" if name.endswith(".json") else "application/octet-stream"
        # Single PUT (not upload_file): files are < a few hundred MB, and a
        # simple PUT only needs cos:PutObject — avoiding the multipart APIs
        # (ListMultipartUploads etc.) that the minimal CI policy doesn't grant.
        with open(path, "rb") as fh:
            client.put_object(
                Bucket=bucket, Key=key, Body=fh, ContentType=ctype, ACL="public-read"
            )
        print(f"[cos] uploaded {key}")

    with open(os.path.join(stage, "manifest.json"), "rb") as fh:
        client.put_object(
            Bucket=bucket,
            Key="manifest.json",
            Body=fh,
            ContentType="application/json",
            ACL="public-read",
        )
    print("[cos] refreshed /manifest.json (latest pointer)")

    # 2) Prune: keep the newest `retain` version prefixes (vYYYYMMDD-N/).
    resp = client.list_objects(Bucket=bucket, Delimiter="/")
    prefixes = [p["Prefix"] for p in resp.get("CommonPrefixes", [])]
    verre = re.compile(r"^v\d{8}-\d+/$")
    versions = sorted(p for p in prefixes if verre.match(p))  # vYYYYMMDD-N sorts chronologically
    old = versions[:-retain] if len(versions) > retain else []
    for pfx in old:
        marker = ""
        while True:
            r = client.list_objects(Bucket=bucket, Prefix=pfx, Marker=marker)
            for c in r.get("Contents", []):
                client.delete_object(Bucket=bucket, Key=c["Key"])
            if r.get("IsTruncated") == "true":
                marker = r.get("NextMarker", "")
            else:
                break
        print(f"[cos] pruned old version {pfx}")
    print(f"[cos] kept newest {retain}: {versions[-retain:] if versions else []}")


if __name__ == "__main__":
    main()
