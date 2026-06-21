#!/usr/bin/env bash
#
# Start the happy-cli daemon (C2 sidecar) wired to 1Agents.
#
# Injects HAPPY_RPC_ADAPTER_ENTRY → adapter/rpc/index.mjs, so the daemon loads
# 1Agents' RPC glue (1agents-proxy + 1agents-chat-*) from THIS repo instead of from
# happy-cli source. happy-cli itself stays a clean upstream blueprint.
#
# Defaults run the daemon from the happy-cli submodule (modules/happy-cli); override
# HAPPY_CLI_DIR to use another build (e.g. the 1agents_server workspace) during transition.
#
# Usage:
#   scripts/start-happy-daemon.sh [start|stop|status]      # default: start
# Overridable env:
#   HAPPY_SERVER_URL        relay base   (default https://agents.dreammate.work)
#   ONEAGENTS_BACKEND_URL   local Go API (default http://127.0.0.1:38080)
#   HAPPY_CLI_DIR           happy-cli dir (default <repo>/modules/happy-cli)
#   HAPPY_RPC_ADAPTER_ENTRY adapter entry (default <repo>/adapter/rpc/index.mjs)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export HAPPY_RPC_ADAPTER_ENTRY="${HAPPY_RPC_ADAPTER_ENTRY:-$ROOT/adapter/rpc/index.mjs}"
export ONEAGENTS_BACKEND_URL="${ONEAGENTS_BACKEND_URL:-http://127.0.0.1:38080}"
export HAPPY_SERVER_URL="${HAPPY_SERVER_URL:-https://agents.dreammate.work}"

HAPPY_CLI_DIR="${HAPPY_CLI_DIR:-$ROOT/modules/happy-cli}"
CMD="${1:-start}"

# --- preflight ---
if [ ! -f "$HAPPY_RPC_ADAPTER_ENTRY" ]; then
    echo "✗ adapter not found: $HAPPY_RPC_ADAPTER_ENTRY" >&2
    exit 1
fi
if [ ! -d "$HAPPY_CLI_DIR" ]; then
    echo "✗ happy-cli not found: $HAPPY_CLI_DIR" >&2
    echo "  init the submodule:  git submodule update --init modules/happy-cli" >&2
    exit 1
fi
if [ ! -f "$HAPPY_CLI_DIR/dist/index.mjs" ]; then
    echo "✗ happy-cli not built: $HAPPY_CLI_DIR/dist/index.mjs missing" >&2
    echo "  build it once:  ( cd \"$HAPPY_CLI_DIR\" && pnpm install && pnpm build )" >&2
    exit 1
fi

echo "happy-cli : $HAPPY_CLI_DIR"
echo "adapter   : $HAPPY_RPC_ADAPTER_ENTRY"
echo "relay     : $HAPPY_SERVER_URL"
echo "backend   : $ONEAGENTS_BACKEND_URL"
echo "daemon    : $CMD"

exec node "$HAPPY_CLI_DIR/bin/happy.mjs" daemon "$CMD"
