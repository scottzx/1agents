#!/usr/bin/env bash
#
# server-h5-ota.sh — 自托管 happy-server 静态 H5 目录的 OTA 拉取更新器。
#
# 补上 OTA "三层更新矩阵"缺的第四块:Web 前端文件本身在自托管服务器上的更新。
# 读根 manifest 的 components.frontend → 下载 frontend tarball → 校验 sha256 →
# 原子热替换静态目录(保留 api/embed/)→ 重启 happy-server。幂等(版本标记)。
#
# 设计要点:
#   - frontend tarball 只含 html/dist 内容(顶层 index.html / app.*.js / *.css ...);
#     api/embed/{skills,cc-connect}-embed.js 极少变,更新时**原样保留**,不随包覆盖。
#   - manifest.components.frontend.entry 为空 → 直接 no-op 退出(CI 尚未发布 frontend 时安全)。
#   - 失败/健康检查不过 → 自动回滚到上一版。
#
# 用法:
#   server-h5-ota.sh [--force] [--dry-run]
# 可覆盖环境变量:
#   OTA_MANIFEST_URL  manifest 地址 (默认 COS 全球加速)
#   H5_STATIC_DIR     静态目录    (默认 /root/1agents_h5)
#   PM2_APP           pm2 进程名  (默认 1agents-server)
#   HEALTH_URL        健康检查    (默认 http://127.0.0.1:3005/)
set -euo pipefail

OTA_MANIFEST_URL="${OTA_MANIFEST_URL:-https://1agents-ota-1258742922.cos.accelerate.myqcloud.com/manifest.json}"
H5_STATIC_DIR="${H5_STATIC_DIR:-/root/1agents_h5}"
PM2_APP="${PM2_APP:-1agents-server}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:3005/}"
LOCKFILE="/run/1agents-h5-ota.lock"
LOG_TAG="[h5-ota]"

FORCE=0
DRY=0
for a in "$@"; do
    case "$a" in
        --force)   FORCE=1 ;;
        --dry-run) DRY=1 ;;
        *) echo "$LOG_TAG unknown arg: $a" >&2; exit 2 ;;
    esac
done

log() { echo "$LOG_TAG $(date '+%Y-%m-%d %H:%M:%S') $*"; }
die() { log "ERROR: $*"; exit 1; }

# 串行化:5 分钟定时器避免叠跑。拿不到锁就安静退出。
exec 9>"$LOCKFILE"
flock -n 9 || { log "another run holds the lock — skip"; exit 0; }

command -v pm2 >/dev/null 2>&1 || die "pm2 not found in PATH"

# 1) 拉 manifest,解析 frontend.{version,entry,integrity}
manifest="$(curl -fsS -m 20 "$OTA_MANIFEST_URL")" || die "fetch manifest failed: $OTA_MANIFEST_URL"
read -r FE_VER FE_URL FE_INTEGRITY < <(python3 - "$manifest" <<'PY'
import sys, json
m = json.loads(sys.argv[1])
fe = (m.get("components") or {}).get("frontend") or {}
print(fe.get("version","") or "-", fe.get("entry","") or "-", fe.get("integrity","") or "-")
PY
)

[ "$FE_URL" = "-" ] && { log "manifest.frontend.entry is empty — CI 尚未发布 frontend 包,no-op"; exit 0; }

# 2) 幂等:版本一致且非 --force 就退出
CUR_VER="$(cat "$H5_STATIC_DIR/.ota-version" 2>/dev/null || echo '')"
if [ "$CUR_VER" = "$FE_VER" ] && [ "$FORCE" -eq 0 ]; then
    log "up-to-date (version=$FE_VER) — skip"
    exit 0
fi
log "update available: current='${CUR_VER:-none}' → target='$FE_VER'"
[ "$DRY" -eq 1 ] && { log "dry-run: would download $FE_URL"; exit 0; }

# 3) 下载 + sha256 校验
TMP="$(mktemp -d /tmp/h5-ota.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT
tarball="$TMP/frontend.tar.gz"
curl -fsS -m 120 -o "$tarball" "$FE_URL" || die "download failed: $FE_URL"

want="${FE_INTEGRITY#sha256-}"
got="$(sha256sum "$tarball" | awk '{print $1}')"
if [ "$want" != "-" ] && [ -n "$want" ]; then
    [ "$want" = "$got" ] || die "sha256 mismatch: want=$want got=$got"
    log "sha256 verified ($got)"
else
    log "WARN: manifest 无 integrity,跳过校验"
fi

# 4) 解包到 staging,组装新目录(保留旧 api/embed)
stage="$TMP/stage"; mkdir -p "$stage"
tar -xzf "$tarball" -C "$stage" || die "extract failed"
[ -f "$stage/index.html" ] || die "tarball 内未见 index.html(结构异常),中止"

newdir="$TMP/new"; mkdir -p "$newdir"
cp -a "$stage/." "$newdir/"
if [ -d "$H5_STATIC_DIR/api/embed" ]; then
    mkdir -p "$newdir/api"
    cp -a "$H5_STATIC_DIR/api/embed" "$newdir/api/"
    log "preserved api/embed/ from current deploy"
fi
echo "$FE_VER" > "$newdir/.ota-version"

# 5) 原子热替换(备份上一版供回滚)
backup="${H5_STATIC_DIR}.bak"
rm -rf "${backup}.old" 2>/dev/null || true
[ -d "$backup" ] && mv "$backup" "${backup}.old"
mv "$H5_STATIC_DIR" "$backup"
mv "$newdir" "$H5_STATIC_DIR"
log "swapped static dir (backup at $backup)"

# 6) 重启 happy-server(@fastify/static 启动时 glob,必须重启才识别新文件)
pm2 restart "$PM2_APP" --update-env >/dev/null 2>&1 || { log "pm2 restart failed — rolling back"; ROLLBACK=1; }

# 健康检查:轮询最多 ~40s。happy-server 用 PGlite,冷启动可能数秒到十几秒,不能只等一下。
health_poll() {
    local code=000
    for _ in $(seq 1 20); do
        sleep 2
        code="$(curl -s -o /dev/null -m 10 -w '%{http_code}' "$1" 2>/dev/null)"; code="${code:-000}"
        [ "$code" = "200" ] && { echo 200; return 0; }
    done
    echo "$code"; return 1
}

# 7) 健康检查,失败回滚
code=000
[ "${ROLLBACK:-0}" != "1" ] && code="$(health_poll "$HEALTH_URL")"
if [ "${ROLLBACK:-0}" = "1" ] || [ "$code" != "200" ]; then
    log "health check FAILED (http=$code) — rolling back to previous"
    mv "$H5_STATIC_DIR" "${H5_STATIC_DIR}.failed-$FE_VER" 2>/dev/null || true
    mv "$backup" "$H5_STATIC_DIR"
    pm2 restart "$PM2_APP" --update-env >/dev/null 2>&1 || true
    code2="$(health_poll "$HEALTH_URL" || true)"
    die "rolled back (post-rollback http=$code2). failed build kept at ${H5_STATIC_DIR}.failed-$FE_VER"
fi

log "✅ updated to $FE_VER and healthy (http=200)"
