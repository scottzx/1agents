# @1agents/happy-adapter（已重定位 → adapter/）

> **⚠️ 实现已迁到结构化的 [`adapter/`](../../adapter/) 接缝。** 本目录的 `index.mjs` 现在只是
> 兼容 shim,re-export `adapter/rpc/index.mjs` 的 `register`,保留旧的
> `HAPPY_RPC_ADAPTER_ENTRY=…/modules/happy-adapter/index.mjs` 路径不破。
> **新部署请把 `HAPPY_RPC_ADAPTER_ENTRY` 指向 `adapter/rpc/index.mjs`。** 文档以
> [`adapter/README.md`](../../adapter/README.md) 为准。

1Agents 自有的 **C2 接缝胶水**。把所有 1Agents 专属的 machine-scoped RPC handler 从
happy-cli 里搬出来，放进本仓（1agents_app），让 happy-cli 保持**零 1Agents 代码的只读蓝图**，
可干净同步上游 slopus/happy。详见 [docs/happy-cli-fork-sync.md](../../docs/happy-cli-fork-sync.md)、
[docs/csc-architecture? / open-vs-closed-boundary](../../../1agents_server/docs/open-vs-closed-boundary.md)。

## 它注册了什么

| RPC method | 作用 |
|---|---|
| `1agents-proxy` | 控制面：把中转来的请求转发到本机 Go 后端 HTTP API（`ONEAGENTS_BACKEND_URL`，默认 `http://127.0.0.1:38080`）。 |
| `1agents-chat-open/send/close` | Agent 聊天流过中转（issue #17）：开聊天桥（建 Happy session + dial 本地 `/api/agent/chat/ws`，Go event 加密镜像成 session 消息经 `new-message` 扇出给 H5）；send 写回 Go WS；close 清理。终端 ttyd **不**走这里。 |

加密：内容用 `ctx.encrypt`（machine key）→ 中转看不到明文，H5 用同一 machine key 解密。

## happy-cli 怎么加载它（通用扩展点，零耦合）

happy-cli 是只读蓝图，**不 import 本模块**。它的 daemon 在 machine scope 启动时读环境变量
`HAPPY_RPC_ADAPTER_ENTRY`，动态 `import()` 该路径并调用其 `register(ctx)`：

```bash
# 启动 happy daemon 时（电脑端 C2）：
export HAPPY_RPC_ADAPTER_ENTRY="/abs/path/to/1agents_app/modules/happy-adapter/index.mjs"
export ONEAGENTS_BACKEND_URL="http://127.0.0.1:38080"   # 本机 Go 后端（按需覆盖）
happy daemon start
```

未设 `HAPPY_RPC_ADAPTER_ENTRY` 时 happy-cli 不注册任何 1Agents handler（纯净蓝图）。

## 运行时

- Node ≥ 22：用全局 `WebSocket` / `fetch`，**零依赖、零构建**（纯 ESM）。
- Node < 22：回退到 `ws` 包（需在运行环境可解析到）。

## ctx 契约（与 happy-cli `loadRpcAdapter.ts` 保持同步）

```ts
{
  registerHandler(method: string, handler: (params) => Promise<any>): void
  serverUrl: string   // happy-server 中转
  token: string       // machine token (Bearer)
  encrypt(body): string        // base64(encrypt(machineKey, variant, body))
  decrypt(b64): unknown | null // 用 machineKey 解密
  log?(msg, ...args): void
}
```
