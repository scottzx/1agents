#!/usr/bin/env bash
# 本地 dev:启动 1agents Go 后端,并让其 spawn 的 happy daemon 信任 mkcert 本地 CA,
# 以便连到自签 HTTPS 的 happy-server(serverUrl 取自 ~/.happy/settings.json)。
# 用法:scripts/dev-backend.sh [额外参数透传给 build/1agents]
set -e
export NODE_EXTRA_CA_CERTS="${NODE_EXTRA_CA_CERTS:-$(mkcert -CAROOT)/rootCA.pem}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec "$ROOT/build/1agents" -no-ttyd "$@"
