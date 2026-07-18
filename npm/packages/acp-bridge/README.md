# `@1agents/acp-bridge`

WebSocket bridge microservice used by 1agents Chat (default `ws://127.0.0.1:38082`).

Built from monorepo `modules/1acp/bridge-server.js`, but imports runtime from the published **`acpx`** package (`acpx/runtime`) so global npm installs work **without** `modules/1acp` or `tsx`.

```bash
# usually installed as a dependency of @1agents/cli
node node_modules/@1agents/acp-bridge/bridge-server.mjs
# or
ACPX_PORT=38082 1agents-acp-bridge
```

**Not** the same as the `acpx` CLI alone — upstream `acpx` does not ship this bridge.
