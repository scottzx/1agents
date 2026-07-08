#!/usr/bin/env python3
"""跨源联系人并集 —— 苹果 ∪ 飞书,按规范化手机号合并同一人。
读 stdin 的 NDJSON(每行一条:src/external_id/name/phones),在内存里按
entity_key 分组(有手机号 → phone:<规范化>,否则 <src>:<external_id>),
每个实体 emit 一行到 stdout。框架按 entity_key upsert 进 unified_contacts。

注:飞书 silver 目前无手机号字段,所以飞书联系人各自独立;一旦飞书侧
接入手机号,同一 SQL/脚本会自动把飞书人和苹果人按手机号并到一起。"""
import sys, json, re


def norm_phone(raw):
    d = re.sub(r"\D", "", raw or "")
    if not d:
        return ""
    if d.startswith("86") and len(d) > 11:   # 去国家码
        d = d[2:]
    if d.startswith("0") and len(d) > 11:     # 去长途 0
        d = d[1:]
    return d


def phones_of(row):
    raw = row.get("phones") or "[]"
    try:
        arr = json.loads(raw) if isinstance(raw, str) else raw
    except (ValueError, TypeError):
        arr = []
    out = []
    for p in arr or []:
        n = norm_phone(p if isinstance(p, str) else str(p))
        if n and n not in out:
            out.append(n)
    return out


entities = {}
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    r = json.loads(line)
    src = r.get("src", "")
    ext = str(r.get("external_id", ""))
    name = r.get("name", "") or ""
    phones = phones_of(r)
    key = "phone:" + phones[0] if phones else f"{src}:{ext}"

    e = entities.get(key)
    if e is None:
        e = {"entity_key": key, "name": name, "sources": [], "phones": [],
             "apple_ids": [], "feishu_open_ids": []}
        entities[key] = e
    if name and (not e["name"] or len(name) > len(e["name"])):
        e["name"] = name
    if src and src not in e["sources"]:
        e["sources"].append(src)
    for p in phones:
        if p not in e["phones"]:
            e["phones"].append(p)
    if src == "apple" and ext not in e["apple_ids"]:
        e["apple_ids"].append(ext)
    if src == "feishu" and ext not in e["feishu_open_ids"]:
        e["feishu_open_ids"].append(ext)

for e in entities.values():
    print(json.dumps({
        "entity_key": e["entity_key"],
        "external_id": e["entity_key"],  # viewer keys rows on external_id
        "name": e["name"],
        "sources": json.dumps(e["sources"], ensure_ascii=False),
        "phones": json.dumps(e["phones"], ensure_ascii=False),
        "apple_ids": json.dumps(e["apple_ids"], ensure_ascii=False),
        "feishu_open_ids": json.dumps(e["feishu_open_ids"], ensure_ascii=False),
        "source_count": len(e["sources"]),
    }, ensure_ascii=False))
