#!/usr/bin/env python3
"""训记 每次训练的总容量(volume) —— SQL 够不着的嵌套数组聚合。
读 stdin 的 NDJSON(每行一条 silver_xunji 行),遍历 movements[].sets[],
累加 已完成组 的 weight*reps,输出 NDJSON 到 stdout(每行一条 gold 行)。
框架负责建表 / cursor / upsert;脚本只做纯变换,不碰数据库、不碰网络。"""
import sys, json


def volume(payload_str):
    try:
        p = json.loads(payload_str or "{}")
    except (ValueError, TypeError):
        return 0.0, 0
    total_vol, total_sets = 0.0, 0
    for m in p.get("movements", []):
        for s in m.get("sets", []):
            if not s.get("done"):
                continue
            total_sets += 1
            try:
                total_vol += float(s.get("weight") or 0) * int(s.get("reps") or 0)
            except (ValueError, TypeError):
                pass
    return round(total_vol, 1), total_sets


for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    r = json.loads(line)
    vol, sets = volume(r.get("payload"))
    print(json.dumps({
        "source": "xunji",
        "external_id": r.get("external_id"),
        "datestr": r.get("datestr", ""),
        "total_volume_kg": vol,
        "total_sets": sets,
        "updated_at": r.get("updated_at", 0),
    }, ensure_ascii=False))
