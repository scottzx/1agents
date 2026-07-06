#!/usr/bin/env bash
# 本地 dev:一条命令起 1agents Go 后端(:38080)。自签 HTTPS relay(mkcert)场景下固化:
#  - HAPPY_EXTRA_CA_CERTS: Go 侧 pairHTTP(pair/start、/v1/auth/*)信任 mkcert 本地 CA;
#    且 startHappyDaemon 会把它透传成 spawn 出的 node daemon 的 NODE_EXTRA_CA_CERTS
#    (否则 daemon 连自签 relay 时 "unable to verify the first certificate" 秒崩)。
#  - NODE_EXTRA_CA_CERTS: 兜底(手动/脚本直接跑 happy daemon 时也能信任)。
#  - HAPPY_SERVER_URL: 指向本地自签 relay(env 优先级最高,覆盖 settings.json/默认)。
# 默认带 ttyd + 托管 frontend/dist + 监听 127.0.0.1:38080;传任意参数即覆盖默认参数。
# 用法:scripts/dev-backend.sh [覆盖参数透传给 build/1agents]
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CA="$(mkcert -CAROOT 2>/dev/null)/rootCA.pem"

export HAPPY_EXTRA_CA_CERTS="${HAPPY_EXTRA_CA_CERTS:-$CA}"
export NODE_EXTRA_CA_CERTS="${NODE_EXTRA_CA_CERTS:-$CA}"
export HAPPY_SERVER_URL="${HAPPY_SERVER_URL:-https://127.0.0.1:3005}"

# 未显式传参 → 用固化的默认(带 ttyd、托管前端、监听回环)。传参则完全用调用方的,
# 便于覆盖(如自己另跑 ttyd 时:scripts/dev-backend.sh -no-ttyd -listen 127.0.0.1:38080)。
if [ "$#" -eq 0 ]; then
	set -- -ttyd-bin "$ROOT/build/ttyd" -static "$ROOT/frontend/dist" -listen 127.0.0.1:38080
fi

echo "[dev-backend] HAPPY_EXTRA_CA_CERTS=$HAPPY_EXTRA_CA_CERTS"
echo "[dev-backend] HAPPY_SERVER_URL=$HAPPY_SERVER_URL"
echo "[dev-backend] exec build/1agents $*"
exec "$ROOT/build/1agents" "$@"
