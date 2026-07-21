# `@1agents/acp-bridge`

WebSocket bridge microservice used by 1agents Chat (default `ws://127.0.0.1:38082`).

Built from monorepo `modules/1acp/bridge-server.js`, but imports runtime from the published
**`@1agents/acpx`** package (`@1agents/acpx/runtime`) so global npm installs get the 1agents
fork (including Grok `_x.ai/ask_user_question` and `_x.ai/exit_plan_mode`) **without**
`modules/1acp` or `tsx`.

```bash
# usually installed as a dependency of @1agents/1agents
node node_modules/@1agents/acp-bridge/bridge-server.mjs
# or
ACPX_PORT=38082 1agents-acp-bridge
```

**Not** the same as upstream npm `acpx` — that package lacks 1agents Grok host extensions.
Depends on **`@1agents/acpx`** (fork of acpx from `modules/1acp`).
