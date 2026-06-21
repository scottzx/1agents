# Agent 层收敛路线图:1acp/acpx → happy AgentBackend

> 状态:路线图(M1 骨架已落,M2/M3 待实现)。配套:[集成骨架](happy-integration-skeleton.md)
> · 跟踪 issue:[scottzx/1agents#180](https://github.com/scottzx/1agents/issues/180)

> **⚠️ 1acp 是承重主链路,不是装饰 —— 现在不可删。** 现网 web agent chat 是
> `前端 → Go → bridge-server(1acp) → claude-agent-acp/codex-acp` 的共生主链路:
> Go `main.go:266`(`supervisor.NewAcpx` 开机拉起)、`handler.go:1162`(`acpxClient.Bridge` 全走它)、
> `catalog.go:183`(适配器从 `modules/1acp` 解析);前端 `agentService.ts:5`「Web chat runs purely on 1acp」、
> `wireProtocol.ts`/`permission.ts`/`hooks.ts` 的 ready/permission 全靠 bridge 回报。
> 所以本路线图的本质是**用 happy+adapter 逐条复现 bridge-server.js 的职责后再退役它**,不是"加个传输"。

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
happy ACPMessageData(ACP 形) ── adapter/wire/envelope.mjs ──▶ Go WsMessage ──▶ happy-wire SessionEnvelope
```

锚点:`backend/internal/agent/acpx_client.go` 的 `WsMessage` 已含对位字段
(`Action`/`Event`/`Text`/`Type`/`ToolName`/`ToolCallID`/`Arguments`/`Summary`/`AcpSessionID`),
所以是**字段级映射,非结构重写**。这让 Go 后端继续当大脑、Node 侧说 ACP/wire。

### Wire 源以 ACP 为准(thinking 一等公民)

映射的 **FROM 源是 happy 的 ACP 形 App 线协议 `ACPMessageData`,不是内部 `AgentMessage` union**。
原因(thinking 是重要字段,以 ACP 为准):

- happy 内部 `AgentMessage` 的 `model-output` 只有 `textDelta`/`fullText` —— **没有 thinking 通道**,
  thinking 在那层被降级成 generic `EventMessage{name:'thinking'}`。
- happy 真正的 App 线协议 `ACPMessageData`(`src/api/apiSession.ts`,注释:"the unified format for
  all agent messages - CLI adapts each provider's format to ACP")是 **ACP 形、thinking/reasoning
  一等公民**(`{type:'thinking';text}` / `{type:'reasoning';message}`)。
- 故 `envelope.mjs` 把 `thinking`/`reasoning` 映射成 `text_delta type:'thought'`,前端 `reducer.ts`
  的 `applyTextDelta` 据 `type==='thought'` 渲染独立 ThinkingBubble —— 与现网 acpx 路径一致。

**M2 `runAgent` 必须 tap happy `MessageAdapter` 之后的 ACP 输出**(`sendAgentMessage` 携带的
`ACPMessageData`),**不可消费内部 `AgentMessage` union** —— 否则 thinking 被降级。
(顺带:happy 原生 claude 另有 `sendClaudeSessionMessage(RawJSONLines)` 通道,保留 Claude SDK
thinking 块最忠实;若 M2 走原生 claude backend,thinking 同样保真。)

## 分阶段

### M1 — 骨架 + spike(已落地)
- `adapter/` 目录树 + submodule + 文档。
- 不迁 agent,acpx 仍驱动一切。
- 两个 spike 验传输可行性(见骨架文档验证段)。

### M2 — Claude 原生优先(收益最大)
- 实现 `adapter/agent/runAgent.mjs`:happy `runAcp.ts` 的 **DI 重写**(注入 api/daemon/persistence
  等价物,**不** import `@/api`、`@/daemon`、`@/persistence`)。
- 只让 `claude` 走 happy 原生 Claude Code backend;happy `MessageAdapter` 的 ACP 输出
  `ACPMessageData` 经 `adapter/wire` 映射成 `WsMessage`,复用 chat 扇出路径(见上「Wire 源以 ACP
  为准」—— 不消费内部 `AgentMessage` union,保 thinking)。
- `catalog.go` 的 Claude 行从 ACP 路由翻成"由 Node agent backend 处理",其余 agent 仍走 acpx。
- **验收闸:** Claude 聊天时间线与 acpx 路径逐字节一致(golden-file 契约测试)。

### M3 — Registry 替代 catalog,退役 acpx + 去 fork 化
- 迁 Codex(AppServer)、Gemini(happy `AcpBackend`)。
- `AgentRegistry` 成传输选择的唯一真相;`catalog.go` 静态表缩成安装元数据(label / 安装命令)。
- 全部 agent 有原生/ACP happy backend 后,弃用 `modules/1acp` + `acpx_client.go`。
- **去 fork 化(绑进退役步骤):** 长尾 ACP agent 改为 happy `AcpBackend` **spawn `npx acpx`(npm 包,非 fork)**
  → 删 `modules/1acp` submodule + Go `supervisor/acpx.go`、`handler` 的 Bridge、`catalog` 的 1acp 解析
  → 清 `.gitmodules` / `Makefile submodules`。结果:零 fork 维护面。

## ⭐ 迁移重点:bridge-server.js 职责 + 4 patch 等价验证

迁移**不是搬代码,是行为对拍**。`bridge-server.js`(+1218,我们自己的代码,却住在 fork 里)的职责
必须在 happy+adapter 逐条复现,并以**现网 acpx 路径产出为基线录 golden-file**,迁移后逐字段对拍:

| 验证项(原 fork patch) | 验什么 |
|---|---|
| session_ready 门禁 / ready 翻转 | ready 状态时序一致 |
| permission callback + 模式归一(原 `conversation-model.ts +42`)| approve-all/deny-all 语义一致 |
| tool_call 富信息:toolCallId / rawInput(原 `events.ts +74`)| **前端渲染依赖**,字段不丢 |
| history replay / adapters(`acpSessionId`/`resumeSessionId`)| 历史回放一致 |
| per-session mcpServers 注入(原 `session-options.ts +8`)| 每会话 MCP 生效 |
| codex model 不回退(原 `codex-acp patch +26`)| codex 离开 acpx 后仍保留 config.toml model(此 patch 大概率自然变无关,但行为仍需验)|
| prompt attachments 转发 + systemContext 追加 system prompt | 附件/系统上下文一致 |

## ACP 版本策略

- ⚠️ **版本号别混**:`acpx`(npm 包)自身 = **0.11.0**(= 我们 fork 基线);飙到 **0.28.x** 的是它依赖的
  `@agentclientprotocol/sdk`。`npm i acpx@latest` 拿到的是 0.11.0,**现在去 fork 化无版本收益**,且会丢上述
  4 patch —— 所以**不现在换**,绑进 M3 退役。SDK 新鲜度由 acpx 传递依赖带入,与用 fork 还是 npm 包无关。
- happy 端 `@agentclientprotocol/sdk`(^0.14.1)**不本地 bump**(违反零耦合、每次同步上游冲突);靠 ACP wire
  协商对接即可,**两端 SDK 版本无需一致**。要升走"上游 happy 提 PR"。
- 长尾 fallback:happy `AcpBackend` 本就是通用 ACP client(`spawn(command, args)`,见 happy-cli
  `acpAgentConfig.ts`),acpx 作为**被 spawn 的命令**接入,**不 import、不改 happy 源码**。

## 约束与开放问题

- **`runAcp.ts` 必须 DI 重写,绝不直接 import** —— 它是唯一耦合 happy 基础设施的 Tier-1 文件。
- Tier-1 引入机制(`file:` vs `dist`)= M2 决策(见骨架风险 #2)。
- `catalog.go` 双角色(传输路由 + 安装元数据)须在 M3 拆开,否则退役传输路由会丢 UI 依赖的元数据。
- 可干净复用的 happy Tier-1(零 happy 依赖):`agent/core/*`、`agent/transport/*`、
  `agent/adapters/*`、`agent/acp/{AcpBackend,AcpSessionManager}`。
