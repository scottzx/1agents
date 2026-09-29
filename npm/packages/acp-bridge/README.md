# @1agents/acp-bridge

Launcher for `@1agents/acp-service >=0.2.0`, used by the 1agents Go backend. It listens on `127.0.0.1:38082` by default (`ACPX_PORT` overrides the port). Each `/agents/<agent>` WebSocket endpoint carries ACP v1 JSON-RPC; `/` selects Codex.

```sh
ACPX_PORT=38082 1agents-acp-bridge
```

The launcher closes the service on stdin EOF or SIGINT/SIGTERM. It contains no Agent runtime or protocol implementation. Development checkouts use the sibling `services/acp-service` source through the supervisor; npm installs use this launcher and its declared service dependency.

The ACP wire migration requires the updated Go backend and service together. Versions before service 0.2.0 used `action/event` frames, which the new endpoint rejects. Chat view messages remain internal to the application's Go/frontend connection; `backend/internal/acpwire` maps them to ACP methods and responses. The package-fill script must not replace this launcher with `modules/1acp/bridge-server.js`.

Publish `@scottzx/1acp@0.15.1`, then `@1agents/acp-service@0.2.0`, before publishing a release of this launcher. The runtime version supplies Provider Profile launch argv and transient credentials; 0.14.0 cannot substitute for it. The service owns protocol documentation, persistent session bindings, replay logs and negotiated 1agents extensions.
