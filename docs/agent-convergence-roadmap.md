# Agent 层收敛路线图:happy 传输融合 + acpx 引擎热替换(C 方案)

> 状态:**两条链路、两阶段**。Phase 1(传输链路)已部分落地(#181 seam glue);Phase 2(引擎热替换)待启动。
> 配套:[集成骨架](happy-integration-skeleton.md)。跟踪 issue:见本仓 epic(C 方案重建,替代旧 #180 的 B 方向描述)。

> ## ⭐ 一句话定调(C 方案)
>
> **Go 始终是 agent 前门(大脑 + 驱动者),适配层永远是哑加密管道。**
> 我们**不**把 agent 抬到 Node 前面(那是被否决的 B 方向);而是**在 Go 后面把 agent 引擎从 acpx 热替换成 happy backend**。
> 终态拓扑恒为:`适配(Node) → Go → 【Go 后面的 agent 引擎】 → agent CLI`。
> 引入 happy 的**首要目标是传输层(E2EE + relay + PTY)**,agent 引擎替换是**第二阶段的纯优化**,与传输完全解耦。

---

## 拓扑:三选项,选 C

现网 agent chat 实跑链路(Phase 1 之上):

```
H5 → relay → happy daemon → adapter(Node,哑管道) → Go /api/agent/chat/ws
                                                          │ Go 内部驱动(引擎)
                                                          ▼
                                              1acp/acpx 子进程 → claude-agent-acp / codex-acp → agent CLI
```

| | A:保留 1acp | **C:引擎换 happy,留 Go 后面(本路线图)** | B:agent 抬到 Node 前面(被否决) |
|---|---|---|---|
| 拓扑 | 适配→Go→acpx→agent | **适配→Go→happy sidecar→agent** | agent 在 Node 跑→Go 退后做扇出 |
| Go 角色 | 大脑+驱动者 | **大脑+驱动者(不变)** | 降级成扇出 |
| agent 前门 | Go | **Go** | Node |
| happy vs 1acp | 共存(各管一层) | **1acp 被干净替换** | 两套 ACP 打架 |
| 收益 | 简单、不动现状 | A 拓扑 + B 收益 | — |

**为什么选 C:** acpx 今天本就是「Go 拉起、Go 驱动的 Node 子进程」。把它的内核从 `bridge-server.js` 换成
「happy `AgentBackend` + `runAcp.ts` 的 DI 重写」,在结构上**与 acpx 同构**——Go 的驱动接口
(`acpx_client.go` 的 `WsMessage`,字段已齐)**几乎不动**。这是一次 **Go 接口不变的引擎热替换**:拿到 B 的
全部收益(原生传输不丢特性、甩掉 acpx fork),却**不反转 owner**,"适配 → Go → Agent" 始终成立。

**C 买到什么 / 没买到什么(别误判收益):**
- ✅ Claude/Codex 走**原生传输不丢特性**(happy 直接 spawn Claude Code CLI stream-json,不再套 ACP 适配器降级)。
- ✅ 甩掉 acpx fork 维护面(4 patch + 1218 行 `bridge-server.js` 不再背)。
- ✅ 1acp 被**替换而非共存**,架构收敛成一套。
- ❌ **那一跳 Node 子进程仍在**(`Go→sidecar(Node)→agent`,跳数同 acpx)。C 消的是 acpx 的**协议开销与特性损耗**,不是子进程本身。真要连子进程都消,是另立任务「Go 原生驱动 agent CLI」,与 happy 无关。
- ❌ **行为对拍的活一点没少**(4 patch 等价验证 + golden-file),这是 Phase 2 的真实成本。

---

## 两阶段

### Phase 1 — 传输链路打通(1acp 引擎不动)⭐ 当前重点

**目标:** 把"加密 + relay + PTY"这条传输链路端到端跑通,让远程(H5/小程序)经中转加密访问本机的
**聊天 + 终端**。**agent 引擎保持 1acp/acpx 原样**,这一阶段一行都不碰它。

- happy-server = 中转 relay,E2EE(machine key),中转看不到明文。
- happy-cli daemon(本机 machine scope)启动时 `import(HAPPY_RPC_ADAPTER_ENTRY)` → `register(ctx)` 加载
  `adapter/` 接缝(详见 [adapter/README.md](../adapter/README.md))。
- `adapter/` 注册的 handler(全部哑搬运 + 加密,**零业务逻辑**):
  - `1agents-proxy`(控制面):解密中转请求 → `fetch` 本机 Go HTTP API → 回包。
  - `1agents-chat-open/send/close`(聊天流):开本机 Go `/api/agent/chat/ws`,每个 Go `WsMessage`
    **逐字节加密镜像**成 relay session 消息扇出 H5。**注意:这里盲转的就是 1acp 经 Go 产出的 WsMessage**——
    Phase 1 不需要 `wire/envelope.mjs`,引擎仍是 acpx。
  - `terminal-*`(终端流,**待实现**):`node-pty` spawn `tmux attach` → 分帧/背压 → relay。决策(2026-06-21):
    终端定走 relay,不设旁路隧道;遇瓶颈靠把流做高效(分帧/批量/背压/tmux 控制模式),不切传输。
- 前端 relay 客户端:`html/src/core/services/relay/{relayClient,relayChatSocket,relayTerminalSocket}.ts`。
- 小程序聊天接入 `@1agents/wire` E2E 加密(纯 JS crypto shim + 密钥获取)。

**Phase 1 验收闸:** 远程经 relay 加密,**聊天 + 终端**端到端可用;agent 引擎仍是 acpx,行为与直连一致。

### Phase 2 — 引擎热替换(acpx → happy backend,留 Go 后面)

**前置:** Phase 1 链路已稳。**本阶段只换 Go 后面的引擎,适配层(Phase 1)完全不动**——它继续盲转 Go 的
`/api/agent/chat/ws`,根本不知道引擎换了。

- 实现 `adapter/agent/runAgent.mjs`:happy `src/agent/acp/runAcp.ts` 的 **DI 重写**(注入 session 客户端 /
  权限处理器 / envelope 发送器的等价物,**绝不** import `@/api`、`@/daemon`、`@/persistence`)。
  **它作为「Go 拉起的 Node sidecar」存在(替换 acpx 的 `bridge-server.js`),不是喂 relay 的前门。**
- `wire/envelope.mjs`(#182,已实现+测)在此阶段接入,位置 = **sidecar → Go 边界**:happy `MessageAdapter`
  的 ACP 输出 `ACPMessageData` → `WsMessage` → 交给 Go(Go 接口不变)。
- 顺序:**claude 先**(原生 stream-json 收益最大)→ codex(AppServer)→ gemini(happy `AcpBackend`)。
- `AgentRegistry` 接管传输选择,`catalog.go` 静态表拆分:registry 管传输路由,catalog 缩成 label/安装命令等 UI 元数据。
- **去 fork 化(绑进退役):** 全部 agent 有原生/ACP happy backend 后,长尾 ACP agent 改为 happy `AcpBackend`
  **spawn `npx acpx`(npm 包,非 fork)** → 删 `modules/1acp` submodule + Go `supervisor/acpx.go`、
  `handler` 的 `acpxClient.Bridge`、`catalog` 的 1acp 解析 → 清 `.gitmodules`/`Makefile submodules`。

**Phase 2 验收闸:** 各 agent 聊天时间线与现网 acpx 路径**逐字节一致**(golden-file 契约测试,见下)。

---

## 映射层(wire,Phase 2)

```
happy ACPMessageData(ACP 形) ── adapter/wire/envelope.mjs ──▶ Go WsMessage ──▶ happy-wire SessionEnvelope
                                  (位于 sidecar → Go 边界)
```

锚点:`backend/internal/agent/acpx_client.go` 的 `WsMessage` 已含对位字段
(`Action`/`Event`/`Text`/`Type`/`ToolName`/`ToolCallID`/`Arguments`/`Summary`/`AcpSessionID`),
所以是**字段级映射,非结构重写**。这让 Go 后端继续当大脑、sidecar 侧说 ACP/wire。

### Wire 源以 ACP 为准(thinking 一等公民)

映射的 **FROM 源是 happy 的 ACP 形 App 线协议 `ACPMessageData`,不是内部 `AgentMessage` union**:

- happy 内部 `AgentMessage` 的 `model-output` 只有 `textDelta`/`fullText` —— **没有 thinking 通道**,
  thinking 在那层降级成 generic `EventMessage{name:'thinking'}`。
- happy 真正的 App 线协议 `ACPMessageData`(`src/api/apiSession.ts`)是 **ACP 形、thinking/reasoning
  一等公民**(`{type:'thinking';text}` / `{type:'reasoning';message}`)。
- 故 `envelope.mjs` 把 `thinking`/`reasoning` 映射成 `text_delta type:'thought'`,前端 `reducer.ts`
  的 `applyTextDelta` 据 `type==='thought'` 渲染独立 ThinkingBubble —— 与现网 acpx 路径一致。

**Phase 2 `runAgent` 必须 tap happy `MessageAdapter` 之后的 ACP 输出**(`ACPMessageData`),
**不可消费内部 `AgentMessage` union** —— 否则 thinking 被降级。

---

## ⭐ Phase 2 迁移重点:bridge-server.js 职责 + 4 patch 等价验证

迁移**不是搬代码,是行为对拍**。`bridge-server.js`(+1218,我们自己的代码,却住在 fork 里)的职责
必须在 happy sidecar + adapter 逐条复现,并以**现网 acpx 路径产出为基线录 golden-file**,迁移后逐字段对拍:

| 验证项(原 fork patch) | 验什么 |
|---|---|
| session_ready 门禁 / ready 翻转 | ready 状态时序一致 |
| permission callback + 模式归一(原 `conversation-model.ts +42`)| approve-all/deny-all 语义一致 |
| tool_call 富信息:toolCallId / rawInput(原 `events.ts +74`)| **前端渲染依赖**,字段不丢 |
| history replay / adapters(`acpSessionId`/`resumeSessionId`)| 历史回放一致 |
| per-session mcpServers 注入(原 `session-options.ts +8`)| 每会话 MCP 生效 |
| codex model 不回退(原 `codex-acp patch +26`)| codex 离开 acpx 后仍保留 config.toml model(此 patch 大概率自然变无关,但行为仍需验)|
| prompt attachments 转发 + systemContext 追加 system prompt | 附件/系统上下文一致 |

---

## ACP 版本策略

- ⚠️ **版本号别混**:`acpx`(npm 包)自身 = **0.11.0**(= 我们 fork 基线);飙到 **0.28.x** 的是它依赖的
  `@agentclientprotocol/sdk`。`npm i acpx@latest` 拿到的是 0.11.0,**现在去 fork 化无版本收益**,且会丢上述
  4 patch —— 所以**不现在换**,绑进 Phase 2 退役。SDK 新鲜度由 acpx 传递依赖带入,与用 fork 还是 npm 包无关。
- happy 端 `@agentclientprotocol/sdk`(^0.14.1)**不本地 bump**(违反零耦合、每次同步上游冲突);靠 ACP wire
  协商对接即可,**两端 SDK 版本无需一致**。要升走"上游 happy 提 PR"。
- 长尾 fallback:happy `AcpBackend` 本就是通用 ACP client(`spawn(command, args)`,见 happy-cli
  `acpAgentConfig.ts`),acpx 作为**被 spawn 的命令**接入,**不 import、不改 happy 源码**。

## 约束与开放问题

- **硬约束:不改 `modules/happy-cli/` 源码**(保持零-1agents-代码、可干净同步上游 slopus/happy);所有专属逻辑
  落 `adapter/` 接缝,经 `HAPPY_RPC_ADAPTER_ENTRY` 加载。要 bump SDK / 扩 agent 走"上游 happy 提 PR"。
- **判废线:** 适配层任何 handler 一旦开始"理解 agent"(路由/鉴权/语义判断)而非"搬运+加密",说明滑向了 B —— 那段逻辑该回 Go。
- **`runAcp.ts` 必须 DI 重写,绝不直接 import** —— 它是唯一耦合 happy 基础设施的 Tier-1 文件。
- Tier-1 引入机制(`file:` vs `dist`)= Phase 2 决策(见骨架风险 #2)。
- `catalog.go` 双角色(传输路由 + 安装元数据)须在 Phase 2 拆开,否则退役传输路由会丢 UI 依赖的元数据。
- 可干净复用的 happy Tier-1(零 happy 依赖):`agent/core/*`、`agent/transport/*`、
  `agent/adapters/*`、`agent/acp/{AcpBackend,AcpSessionManager}`。
- L3 控制面/配置层(提示词·工具·skills·权限·acceptance criteria)衔接任务引擎 #130。
