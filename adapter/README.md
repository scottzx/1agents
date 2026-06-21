# @1agents/adapter — C2 接缝胶水

1Agents 自有的**电脑端(C2)接缝代码**,不属于 happy-cli。happy-cli 作为只读蓝图 + 库,
通过通用扩展点 `HAPPY_RPC_ADAPTER_ENTRY` 指向 [`rpc/index.mjs`](rpc/index.mjs),在 daemon
(machine scope)启动时动态加载并调用 `register(ctx)`,把所有 1Agents 专属能力注册进中转。

这样 happy-cli 源码保持**零 1Agents 代码**,可干净同步上游 slopus/happy
(见 [docs/happy-cli-fork-sync.md](../docs/happy-cli-fork-sync.md))。整体设计见
[docs/happy-integration-skeleton.md](../docs/happy-integration-skeleton.md)。

## 模块边界(硬约束)

`wire/` 是叶子,`rpc/` 是根组合层;**任何代码都不改 `modules/happy-cli/` 源码**。

| 模块 | 职责 | 可 import | 禁止 import |
|---|---|---|---|
| [`rpc/`](rpc/) | 单一 `register(ctx)` 入口,注册所有 machine-scoped handler | `chat/`、`terminal/`、`agent/`、`wire/`、stdlib、注入的 `ctx` | happy-cli 内部(只经 `ctx`);Go/TSX |
| [`chat/`](chat/) | 聊天桥:拨本地 Go `/api/agent/chat/ws`,Go event 加密镜像成 relay session 消息经 `new-message` 扇出 | `wire/`、`ctx`、Node WS/fetch | happy-cli 内部;ttyd 细节 |
| [`terminal/`](terminal/) | **(占位)** node-pty attach tmux,stdout 分帧过 relay,stdin/resize 回写 | `wire/`、`ctx`、`node-pty` | 直接耦合 ttyd 二进制 |
| [`agent/`](agent/) | **(占位)** 未来消费 happy `AgentBackend`/`AgentRegistry` + DI 重写 `runAcp` | happy-cli Tier-1(库)、`wire/` | happy `runAcp.ts`、`@/api`、`@/daemon`、`@/persistence` |
| [`wire/`](wire/) | ACP 形 `ACPMessageData` ↔ Go `WsMessage` 字段映射(`toWsMessage`/`fromWsMessage`,thinking 一等公民)| stdlib(零依赖)| 其它一切(叶子)|

## 当前状态(M1 骨架)

- ✅ `rpc/`、`chat/` —— 从 `modules/happy-adapter/index.mjs` 原样重定位,**行为不变**。
  旧入口 `modules/happy-adapter/index.mjs` 改为薄 shim re-export,`HAPPY_RPC_ADAPTER_ENTRY`
  旧路径仍可用。
- ✅ `wire/` —— `envelope.mjs` 实现 happy ACP 形 `ACPMessageData` ↔ Go `WsMessage` 双向字段映射
  (M2 的叶子依赖,零依赖、零构建)。**FROM 源取 ACPMessageData 而非内部 `AgentMessage` union**
  —— 后者 model-output 纯文本会把 thinking 降级;ACPMessageData 是 ACP 形、thinking/reasoning
  一等公民,映射成 `text_delta type:'thought'`。golden 对拍 + 往返测试见 `wire/envelope.test.mjs`
  (`npm test` 或 `node --test`)。⚠️ golden 编码的是从两端类型定义推导的契约基线,
  **非现网 acpx 抓包** —— M2 闸(逐字节对拍现网产出)仍未做,见 envelope.mjs 顶部「验收边界」。
- 🚧 `terminal/`、`agent/` —— 占位 + 文档注释 + TODO,**不含可运行逻辑**。
  见 [agent 收敛路线图](../docs/agent-convergence-roadmap.md)的 M1/M2/M3。

## 怎么被加载(零耦合)

```bash
# 启动 happy daemon 时(电脑端 C2):
export HAPPY_RPC_ADAPTER_ENTRY="/abs/path/to/1agents_app/adapter/rpc/index.mjs"
export ONEAGENTS_BACKEND_URL="http://127.0.0.1:38080"   # 本机 Go 后端(按需覆盖)
happy daemon start
```

未设 `HAPPY_RPC_ADAPTER_ENTRY` 时 happy-cli 不注册任何 1Agents handler(纯净蓝图)。

## 运行时

- Node ≥ 22:用全局 `WebSocket` / `fetch`,纯 ESM、零构建(`terminal/` 的 node-pty 例外,需原生构建)。
- Node < 22:`chat/` 回退到 `ws` 包(若可解析)。

## ctx 契约

由 happy-cli 注入,精确形状见 [`rpc/ctxContract.d.ts`](rpc/ctxContract.d.ts)
(与 happy-cli `loadRpcAdapter.ts` 保持同步 —— upstream 同步后的复核点之一)。
