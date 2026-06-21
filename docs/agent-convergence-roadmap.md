# Agent 层收敛路线图:1acp/acpx → happy AgentBackend

> 状态:路线图(M1 骨架已落,M2/M3 待实现)。配套:[集成骨架](happy-integration-skeleton.md)

## 为什么收敛

当前**两套重叠的 ACP** 并存:

| | 1agents 现状(1acp/acpx) | happy-cli(agent/ 层) |
|---|---|---|
| 形态 | `modules/1acp` acpx `bridge-server.js` **子进程**,Go `acpx_client.go` 驱动 | TS `AgentBackend` 接口 + `AgentRegistry` + `AgentMessage` union |
| 传输 | **一切皆 ACP**(vendored `claude-agent-acp`、`codex-acp` 适配器) | **每 agent 选最佳传输** |
| 跳数 | 3 跳:browser → Go WS → acpx 子进程 → agent | 统一接口,直驱 agent |

**happy 的混合传输(关键优势):**

| Agent | happy 传输 | 是 ACP |
|---|---|---|
| Claude | 原生 Claude Code CLI(stream-json,`claude_local_launcher.cjs`)| ❌ 原生 |
| Codex | v2 JSON-RPC AppServer(`CodexAppServerClient`)| ❌ AppServer |
| Gemini | `gemini --experimental-acp` + `AcpBackend` + Zed `@agentclientprotocol/sdk` | ✅ ACP |
| OpenClaw | 私有 WS | ❌ |
| Devin | ACP | ✅ |

→ Claude/Codex 走原生协议**不丢特性**,还省掉 acpx 子进程一跳。ACP 保留在它本就是原生协议处。

## 映射层(载重设计)

```
happy AgentMessage(union) ── adapter/wire/envelope.mjs ──▶ Go WsMessage ──▶ happy-wire SessionEnvelope
```

锚点:`backend/internal/agent/acpx_client.go` 的 `WsMessage` 已含对位字段
(`Action`/`Event`/`Text`/`ToolName`/`ToolCallID`/`Arguments`/`Summary`/`AcpSessionID`),
所以是**字段级映射,非结构重写**。这让 Go 后端继续当大脑、Node 侧说 happy/wire。

## 分阶段

### M1 — 骨架 + spike(已落地)
- `adapter/` 目录树 + submodule + 文档。
- 不迁 agent,acpx 仍驱动一切。
- 两个 spike 验传输可行性(见骨架文档验证段)。

### M2 — Claude 原生优先(收益最大)
- 实现 `adapter/agent/runAgent.mjs`:happy `runAcp.ts` 的 **DI 重写**(注入 api/daemon/persistence
  等价物,**不** import `@/api`、`@/daemon`、`@/persistence`)。
- 只让 `claude` 走 happy 原生 Claude Code backend;`AgentMessage` 经 `adapter/wire` 映射成
  `WsMessage`,复用 chat 扇出路径。
- `catalog.go` 的 Claude 行从 ACP 路由翻成"由 Node agent backend 处理",其余 agent 仍走 acpx。
- **验收闸:** Claude 聊天时间线与 acpx 路径逐字节一致(golden-file 契约测试)。

### M3 — Registry 替代 catalog,退役 acpx
- 迁 Codex(AppServer)、Gemini(happy `AcpBackend`)。
- `AgentRegistry` 成传输选择的唯一真相;`catalog.go` 静态表缩成安装元数据(label / 安装命令)。
- 全部 agent 有原生/ACP happy backend 后,弃用 `modules/1acp` + `acpx_client.go`。

## 约束与开放问题

- **`runAcp.ts` 必须 DI 重写,绝不直接 import** —— 它是唯一耦合 happy 基础设施的 Tier-1 文件。
- Tier-1 引入机制(`file:` vs `dist`)= M2 决策(见骨架风险 #2)。
- `catalog.go` 双角色(传输路由 + 安装元数据)须在 M3 拆开,否则退役传输路由会丢 UI 依赖的元数据。
- 可干净复用的 happy Tier-1(零 happy 依赖):`agent/core/*`、`agent/transport/*`、
  `agent/adapters/*`、`agent/acp/{AcpBackend,AcpSessionManager}`。
