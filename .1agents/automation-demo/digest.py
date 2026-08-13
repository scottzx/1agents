#!/usr/bin/env python3
"""Demo preamble for Function → ACP. stdout must be a single JSON object."""

import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path


def run(cmd: list[str]) -> str:
    try:
        out = subprocess.check_output(cmd, stderr=subprocess.DEVNULL, text=True)
        return out.strip()
    except Exception:
        return ""


cwd = Path.cwd()
branch = run(["git", "rev-parse", "--abbrev-ref", "HEAD"])
head = run(["git", "rev-parse", "--short", "HEAD"])
status = run(["git", "status", "--short"])
changed = [line for line in status.splitlines() if line.strip()]
payload = {
    "generated_at": datetime.now(timezone.utc).isoformat(),
    "cwd": str(cwd),
    "git": {
        "branch": branch,
        "head": head,
        "changed_count": len(changed),
        "changed_sample": changed[:20],
    },
}
print(json.dumps(payload, ensure_ascii=False))
