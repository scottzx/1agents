# Turn 全生命周期闭环补全设计（v1.1）

| 字段     | 内容                                                                                                  |
| -------- | ----------------------------------------------------------------------------------------------------- |
| 状态     | **Draft / Ready for review**                                                                          |
| 日期     | 2026-07-29                                                                                            |
| 目标     | 在现有 Session 大闭环上，把 Turn 从请求接收、排队、执行、流式事件、终态、恢复到审计投影真正串成一条链 |
| 基线     | `#281`、`#282–#288` 已完成的 AgentTurn / ProjectEvent / Activity / TaskRun 领域实现                   |
| 关联文档 | [PRD](./prd.md)、[v1 设计](./design.md)、[实现走查](./walkthrough.md)                                 |
| 方案性质 | v1.1 补全方案；不推翻 v1 领域模型，不重开已关闭任务                                                   |

---

## 0. 结论

现有 Turn 已经具备后端领域模型，但还没有形成与 Session 同等级别的端到端闭环。

当前系统里实际存在四套不完全相同的“Turn”：

1. Browser 中由用户气泡和 `typing` 状态推断的 UI Turn；
2. Go 后端 `agent_turns` 中持久化的 AgentTurn；
3. `bridge-server.js` 中的 `activeTurn`、`promptQueue` 和临时 `requestId`；
4. 1acp runtime 中以 `requestId`、用户消息边界和 SessionRecord 表达的 prompt turn。

它们目前依靠顺序、Prompt 文本和“当前 active Turn”间接对应，没有共享一个从开始到结束都不变化的身份。

本方案的核心改造是：

> **由 Go 后端创建唯一 `turn_id`，并让这个 ID 贯穿 Browser、Go、bridge、1acp runtime、SessionRecord、流式事件、终态事件和恢复对账。**

最终形成：

```text
1agents Session ID
  └── AgentTurn ID
        ├── client request id（提交幂等）
        ├── Go durable queue
        ├── bridge active turn
        ├── 1acp runtime request id
        ├── SessionRecord User.id / turn result
        ├── realtime events
        ├── ProjectEvents / TaskRun / Replies
        └── terminal + recovery reconciliation
```

本方案同时锁定以下方向：

- Go 后端是 Turn 身份和队列的唯一真源；
- 1acp 不再为宿主 Turn 自行生成另一套 `turn_<timestamp>`；
- 新链路中每个 prompt 必须有非空 `client_request_id`；
- 每个 Turn 事件必须携带 `turn_id`；
- generic `error` 不再等价于 Turn 终态；
- SessionRecord 保存详细对话，`agent_turns` 保存索引、状态和审计摘要；
- Browser 历史展示不再以 Prompt 文本匹配 Turn；
- Backend 重启仍不自动重放 Turn，但要先根据 1acp 持久化状态做对账；
- Grok 预热只创建 Session，不创建 Turn；预热 record rebind 到真实 Session ID 后才能接受第一个 Turn。

---

## 1. 背景：Session 已经接近闭环，Turn 仍是分段闭环

### 1.1 当前 Session 大闭环

当前 Session 已经形成了较清晰的两层持久化：

```text
Browser
  │ sessionId
  ▼
Go / meta.db
  │ sessions.id
  │ acp_session_id
  ▼
bridge-server
  │ activeSessions[sessionId]
  ▼
1acp runtime
  │ acpxRecordId = sessionId
  │ acpSessionId
  │ agentSessionId
  ▼
~/.1agents/acpx-state/sessions/<sessionId>.json
```

Grok 预热命中后的 record rebind 又补上了一个关键缺口：

```text
prewarm_grok_<timestamp>
  ── adoptSession(realSessionId) ──►
真实 1agents Session ID
```

因此，对新命中的预热 Session：

- SQLite `sessions.id`；
- 1acp `acpx_record_id`；
- JSON 文件名；
- runtime handle `sessionKey/acpxRecordId`；
- manager 的 pooled client key；

都可以统一为真实 1agents Session ID。

### 1.2 当前 Turn 的四段实现

当前 Turn 链路是：

```text
Browser send(prompt)
  │ 没有 requestId
  ▼
Go queuePrompt
  │ 创建 agent_turns.id
  │ 内存 activeTurn / pendingTurns
  │ 给下行 prompt 注入 turnId
  ▼
bridge-server prompt
  │ 忽略传入 turnId
  │ 自行生成 requestId = turn_<timestamp>
  │ 自己还有 activeTurn / promptQueue
  ▼
1acp runtime.startTurn
  │ requestId = bridge 临时 ID
  │ SessionRecord.lastRequestId
  │ messages 没有 host turnId
  ▼
bridge events
  │ text/tool/status/error 大多没有 turnId
  ▼
Go
  │ 依靠 activeTurn 把 done/error 对应到数据库 Turn
  ▼
Browser
  │ 实时过程按“当前轮”接收
  │ 刷新后按 Prompt 文本/时间顺序重新匹配 Turn
```

这是一条“顺序上大多能工作”的链，而不是“身份上可证明一致”的链。

---

## 2. 当前事实审计

以下结论以 2026-07-29 当前工作树为准。

### 2.1 已经存在的能力

| 能力                  | 当前实现                                                      |
| --------------------- | ------------------------------------------------------------- |
| Turn 表               | `agent_turns`                                                 |
| Turn 状态             | `queued/running/completed/failed/cancelled`                   |
| 单 Session 单 running | SQLite partial unique index                                   |
| 提交幂等索引          | `(session_id, client_request_id)`，但主 UI 未传 requestId     |
| 生命周期事件          | `turn.queue/start/complete/fail/cancel` 写入 `project_events` |
| Go 队列               | `ActiveBridge.activeTurn/pendingTurns`                        |
| 重启恢复              | running→failed，queued→cancelled                              |
| 项目写入归因          | 使用可信 Session token，解析当前 running Turn                 |
| Activity              | 按 Turn 聚合 ProjectEvents                                    |
| TaskRun               | 可记录 `origin_turn_id`                                       |
| Reply                 | 已有 `replies.turn_id`                                        |
| UI 投影               | 查询 AgentTurn + Activity，给历史消息补 `turnId`              |

### 2.2 已确认的端到端缺口

| #   | 缺口                                   | 当前证据                                                                         | 后果                                                                       |
| --- | -------------------------------------- | -------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| G1  | Browser prompt 没有 `requestId`        | `promptAction(sessionId, text)` 只发送 action/sessionId/text                     | `client_request_id` 为空，提交幂等索引在主链路失效                         |
| G2  | UI 不消费 Go Turn 生命周期事件         | Go 发 `turn_queued/turn_started/turn_state`；前端 wire union/switch 没有对应闭环 | queued badge、Turn ID、取消目标无法可靠绑定                                |
| G3  | Go 和 bridge 各自维护一套 prompt queue | Go `pendingTurns`；bridge `promptQueue`                                          | 出现双队列、双 request ID 和取消语义分叉                                   |
| G4  | bridge 丢失 host `turnId`              | 下行 prompt 已被 Go 注入 `turnId`，bridge `runPromptTurn` 没有使用               | 1acp runtime 不知道真实 AgentTurn ID                                       |
| G5  | bridge 自建 runtime request ID         | `turn_${Date.now()}`                                                             | 同一个业务 Turn 出现第二套身份                                             |
| G6  | 流式事件没有 `turnId`                  | text/tool/status/permission/done 大多只带 sessionId                              | 重连、迟到事件、跨轮竞争只能靠“当前轮”猜测                                 |
| G7  | SessionRecord 没有 Turn 边界           | `SessionMessage.User.id` 存在，但没有使用 host turnId；仅有全局 `lastRequestId`  | 无法从持久化 transcript 精确恢复某个 Turn                                  |
| G8  | 前端按 Prompt 文本和时间匹配           | `projectChatTurns` 先匹配 promptText，再按时间兜底                               | 重复 Prompt、归一化 Prompt、分页和截断都会误配                             |
| G9  | generic `error` 被当成 Turn 终态       | Go 对任何 `event == error` 调 `finishActiveTurn`                                 | 控制操作错误、非终态错误可能错误结束 active Turn                           |
| G10 | final answer 由 Go 流式缓冲重建        | Go 在 tool_call 时清空文本，只保留最后文本块                                     | 与 1acp 已持久化 transcript 存在双真源和崩溃窗口                           |
| G11 | queued cancel 缺稳定目标               | UI 依赖 bridge 的临时 `prompt_queued.requestId`，Go 队列使用 Turn ID             | 取消可能只作用于某一层队列                                                 |
| G12 | Reply 启动关系可能被复用               | `bridge.ReplyID` 是连接级字段，后续 Turn 仍可能复用                              | 多 Turn 场景中 `initiating_reply_id` 和 `replies.turn_id` 可能被覆盖或误绑 |
| G13 | runtime 终态与 meta 终态无法对账       | 1acp record 没有 per-turn terminal marker                                        | runtime 已完成、Go 尚未落终态时，重启只能统一判 failed                     |
| G14 | 文档“已实现”高于运行态证据             | 本机只读抽样：666 Sessions、0 AgentTurns、0 Turn Events、0 带 turn_id Replies    | 迁移/代码存在不等于生产链路已经被实际使用                                  |

本机只读抽样只说明当前安装实例没有形成 Turn 数据，不单独证明某个代码分支错误；但它足以说明不能把“表已存在”作为端到端闭环验收。

---

## 3. 目标与非目标

### 3.1 v1.1 目标

1. 每次 Browser prompt 在发送前获得稳定、非空的 `client_request_id`。
2. Go 接受请求时创建唯一 `turn_id`，并立即把两者关联回传。
3. `turn_id` 贯穿 Go、bridge、1acp runtime、SessionRecord 和所有 realtime events。
4. Go 成为唯一 durable Turn queue；bridge 同一 Session 只运行一个已分配 Turn。
5. queued/running/terminal/cancel/reconnect 都以 `turn_id` 为目标。
6. 1acp SessionRecord 能精确识别每个 Turn 的消息边界和终态。
7. Turn terminal 必须来自明确的 `turn_terminal` 契约，不能由 generic error 推断。
8. terminal 在可见前完成 runtime 持久化；Go 再完成 meta.db 终态事务。
9. Backend 重启时，能够区分：
   - runtime 已落终态但 Go 未完成终态；
   - runtime 未完成或状态不可确认；
   - 仍在 Go 队列、从未下发的 Turn。
10. 历史 UI 使用持久化 `turn_id` 精确分组，不再依赖 Prompt 文本。
11. 旧 Session、旧 runtime record、旧 bridge 继续使用 legacy fallback。
12. ProjectEvent、TaskRun、Reply 继续引用同一个 canonical `turn_id`。

### 3.2 非目标

| 非目标                                        | 原因                                              |
| --------------------------------------------- | ------------------------------------------------- |
| 自动重放 backend 重启前的 queued/running Turn | 工具调用可能非幂等                                |
| 保存私有思维链                                | 隐私和产品边界不变                                |
| 将所有 token delta 写入 meta.db               | 写放大过高；详细 transcript 归 1acp SessionRecord |
| 让 Session 强制绑定单个 ProjectItem           | 与 project-wide Session 决策冲突                  |
| 整轮 Undo                                     | 需要独立的逆向操作与冲突设计                      |
| 清洗所有历史 Turn                             | 无法可靠补造过去不存在的因果身份                  |
| 第一阶段支持跨 Session 并行执行同一 Turn      | 一个 Turn 只属于一个 Session                      |

---

## 4. 核心设计决策

| #   | 决策                                                    | 说明                                                                      |
| --- | ------------------------------------------------------- | ------------------------------------------------------------------------- |
| D1  | `agent_turns.id` 是 canonical Turn ID                   | 由 Go 后端生成，任何下游不得替换                                          |
| D2  | `client_request_id` 由 Browser 生成                     | 只负责提交幂等，不等于 Turn ID                                            |
| D3  | runtime `requestId` 直接使用 `turn_id`                  | 消除 bridge 的第二套 `turn_<timestamp>`                                   |
| D4  | Go 是唯一 durable queue owner                           | 只有 Go 同时拥有 SQLite Turn 状态和 Session 身份                          |
| D5  | bridge 不为 host-managed prompt 排队                    | 收到第二个 active Turn 应拒绝为协议错误；legacy path 可保留兼容队列       |
| D6  | 每个 Turn event 必须带 `turnId`                         | 包括 text/tool/status/permission/usage/terminal                           |
| D7  | generic error 不代表 terminal                           | 只有 `turn_terminal` 可以驱动 AgentTurn 终态                              |
| D8  | `SessionMessage.User.id = turn_id`                      | 直接复用现有稳定用户消息 ID 表达 transcript Turn 边界                     |
| D9  | SessionRecord 保存最小 per-turn result 索引             | 用于 terminal 对账，不复制 AgentTurn 全表                                 |
| D10 | runtime 详细 transcript、meta.db 状态摘要双真源分工明确 | 两边通过 turn_id 对账，不再通过文本猜测                                   |
| D11 | terminal durable-before-visible                         | 先保存 1acp record，再发 terminal；Go 再事务化完成 meta 状态              |
| D12 | 取消必须显式指定 `turn_id`                              | 防止取消请求到达时 active Turn 已切换                                     |
| D13 | queued Turn 不进入 1acp                                 | 只有 queued→running 后才写 runtime record                                 |
| D14 | 断线不等于失败                                          | Browser 断线只影响订阅；runtime/Go owner 丢失才进入恢复                   |
| D15 | 不自动重放                                              | 无法确认的 running→failed；未下发 queued→cancelled                        |
| D16 | 旧数据保持 fallback                                     | 没有 turn_id 的 history 仍使用旧分组规则                                  |
| D17 | Grok 预热不创建 Turn                                    | 必须先完成 Session record rebind，再接受首个 prompt                       |
| D18 | 已完成 `#282–#288` 作为 v1 基线                         | 新建 v1.1 follow-up Epic，不篡改历史交付记录                              |
| D19 | 重新编辑是前端组合交互，不是后端领域关系                | 取消旧 Turn；旧 Prompt 回填 Composer；用户再次发送时创建全新 Request/Turn |

---

## 5. ID 模型

### 5.1 ID 清单

| ID                   | 生成方           | 生命周期            | 是否落库            | 用途                        |
| -------------------- | ---------------- | ------------------- | ------------------- | --------------------------- |
| `session_id`         | 1agents Go       | 多个 Turn           | SQLite + 1acp JSON  | 宿主会话身份                |
| `acp_session_id`     | Agent/ACP        | 多个 Turn           | SQLite + 1acp JSON  | ACP resume/load             |
| `agent_session_id`   | Agent，可选      | 多个 Turn           | SQLite/1acp JSON    | Agent 原生线程身份          |
| `client_request_id`  | Browser          | 一次提交及重试      | `agent_turns`       | 防重复创建/执行             |
| `turn_id`            | Go backend       | 一次用户请求到终态  | 所有层              | canonical Turn 身份         |
| `runtime_request_id` | 直接取 `turn_id` | 一次 runtime turn   | 1acp record         | runtime cancel/result/usage |
| `prompt_message_id`  | 直接取 `turn_id` | transcript 生命周期 | SessionRecord       | 消息边界                    |
| `tool_call_id`       | Agent/ACP        | Turn 内一次工具调用 | SessionRecord/Event | 工具事件关联                |
| `event_sequence`     | bridge/runtime   | Turn 内单调递增     | 可选 checkpoint     | 去重与迟到事件过滤          |

### 5.2 不允许的混用

- `client_request_id` 不能作为数据库主键；
- `turn_id` 不能由 Browser 指定；
- `tool_call_id` 不能作为 Turn ID；
- `acp_session_id` 不能作为 1agents Session ID；
- `bridge requestId` 不能再自行使用时间戳；
- queue position 不是稳定 ID；
- Prompt 文本不是关联键。

### 5.3 Request ID 的复用边界

`client_request_id` 标识“一次已经发出的提交”，不是一份可以反复修改的草稿。

| 场景                                     | Request ID | Turn ID             |
| ---------------------------------------- | ---------- | ------------------- |
| 网络超时后重试同一次未确认提交           | 复用       | 返回同一个 Turn     |
| WebSocket 断线后重放同一提交             | 复用       | 返回同一个 Turn     |
| 用户尚未发送，只在 Composer 中修改草稿   | 尚未生成   | 尚未创建            |
| 用户取消已提交 Prompt                    | 旧 ID 保留 | 旧 Turn → cancelled |
| 用户取消后重新发送，即使文本相同         | 新 ID      | 新 Turn             |
| 用户点击“编辑”，修改后再次发送           | 新 ID      | 新 Turn             |
| 用户主动重新生成或复制历史 Prompt 再发送 | 新 ID      | 新 Turn             |

一个映射一旦建立就永久不变：

```text
(session_id, client_request_id) → agent_turns.id
```

旧 Turn 即使 cancelled/failed，也不能把它的 Request ID 重新绑定到新 Turn。

同一个 Request ID 重复到达时：

- 请求内容一致：返回已有 Turn，不重复创建或执行；
- 请求内容不一致：返回 `409 IDEMPOTENCY_CONFLICT`；
- 禁止修改已有 Turn 的 `prompt_text`。

服务端应保存或计算请求指纹，至少覆盖：

```text
text + attachments + prompt mode + 会影响执行语义的提交配置
```

---

## 6. 权威数据分工

| 数据                           | 权威来源                                             | 副本/投影                      |
| ------------------------------ | ---------------------------------------------------- | ------------------------------ |
| Turn 身份、状态、时间、错误    | `meta.db.agent_turns`                                | UI、Activity                   |
| Turn durable queue             | `agent_turns.status=queued` + Go owner               | Browser queue projection       |
| 当前 runtime active Turn       | Go `ActiveBridge.activeTurn`                         | bridge `activeTurn.turnId`     |
| Turn 详细消息、工具调用、usage | 1acp SessionRecord                                   | history response / ChatUI      |
| Turn transcript 边界           | `SessionMessage.User.id = turn_id`                   | frontend ChatItem.turnId       |
| Turn runtime 终态标记          | SessionRecord `acpx.turn_results[turn_id]`           | startup reconciliation         |
| 最终回答摘要                   | runtime terminal result → `agent_turns.final_answer` | Reply / Activity               |
| 项目事实变化                   | `project_events`                                     | Project Activity / Task Detail |
| 执行与核验                     | `task_runs` / Evidence / ClosedBy                    | Task Detail                    |

关键原则：

> meta.db 不复制完整 transcript；1acp SessionRecord 不替代 AgentTurn 的项目审计状态。

---

## 7. Turn 状态机

### 7.1 状态

继续沿用 v1：

```text
queued
running
completed
failed
cancelled
```

合法转换：

```text
queued  ── start ─────► running
queued  ── cancel ────► cancelled

running ── complete ──► completed
running ── fail ──────► failed
running ── cancel ────► cancelled
```

### 7.2 状态语义

| 状态      | 已落库条件                                    | runtime 条件                                          |
| --------- | --------------------------------------------- | ----------------------------------------------------- |
| queued    | Go 已接受且持久化 prompt                      | 未进入 1acp                                           |
| running   | Go 已取得本 Session 执行权                    | 即将或已经调用 `runtime.startTurn`                    |
| completed | runtime 明确返回 completed 且结果已持久化     | 存在 completed turn result                            |
| failed    | runtime 明确失败，或 owner 丢失且无法确认成功 | 存在 failed result，或恢复裁定                        |
| cancelled | queued 被取消，或 active runtime 确认取消     | queued 无 runtime result；running 有 cancelled result |

### 7.3 不新增 `partial` 状态

Turn completed 与 ProjectEvent 部分失败继续分层：

```text
Turn.status = completed
Activity.status = partial/failed（由 Events 聚合）
```

Turn 只回答“Agent 请求是否自然到达终点”，ProjectEvents 回答“业务写操作是否全部成功”。

---

## 8. 统一 Wire Protocol

### 8.1 Browser → Go：提交 Prompt

Browser 在创建用户气泡前生成 UUID：

```json
{
  "action": "prompt",
  "sessionId": "session-123",
  "requestId": "client-req-uuid",
  "text": "继续调研 1acp 的 Turn 生命周期"
}
```

规则：

- `requestId` 必填；
- 同一 Session 内重复提交相同 `requestId` 返回同一个 Turn；
- 相同 `requestId` 但请求指纹不同，返回 `409 IDEMPOTENCY_CONFLICT`；
- Request ID 在用户点击发送时生成，而不是在开始编辑草稿时生成；
- 取消后再次发送必须生成新 `requestId`，即使 Prompt 文本未变化；
- Browser 不允许传 `turnId`；
- Go 收到 Browser 自带 `turnId` 时拒绝或丢弃，不能信任。

### 8.2 Go → Browser：接受/状态同步

统一为一个 `turn_state` 事件，旧事件可在兼容期并发发送：

```json
{
  "event": "turn_state",
  "sessionId": "session-123",
  "turnId": "turn-abc",
  "requestId": "client-req-uuid",
  "status": "queued",
  "queuePosition": 2,
  "createdAt": "2026-07-29T03:00:00Z"
}
```

开始执行：

```json
{
  "event": "turn_state",
  "sessionId": "session-123",
  "turnId": "turn-abc",
  "requestId": "client-req-uuid",
  "status": "running",
  "startedAt": "2026-07-29T03:00:03Z"
}
```

Browser 通过 `requestId` 把 optimistic user bubble 绑定到 `turnId`。

### 8.3 Go → bridge：执行 Prompt

Go 只会下发已经从 queued 迁移为 running 的 Turn：

```json
{
  "action": "prompt",
  "sessionId": "session-123",
  "turnId": "turn-abc",
  "requestId": "turn-abc",
  "text": "继续调研 1acp 的 Turn 生命周期"
}
```

这里：

- `turnId` 是显式业务身份；
- runtime `requestId` 与 `turnId` 相同；
- 原 Browser `client_request_id` 已在 meta.db，不需要透传给 Agent。

### 8.4 bridge → Go/Browser：流式事件

每个 Turn 内事件必须包含：

```json
{
  "event": "text_delta",
  "sessionId": "session-123",
  "turnId": "turn-abc",
  "sequence": 17,
  "text": "..."
}
```

适用事件：

- `text_delta`
- `tool_call`
- `tool_result`
- `permission_request`
- `permission_timeout`
- `ask_user_question`
- `exit_plan_mode`
- `plan`
- `usage`
- `mode_changed`（若发生在 Turn 内）
- `error`（非终态也必须说明 scope）
- `turn_terminal`

### 8.5 终态事件

新增唯一终态契约：

```json
{
  "event": "turn_terminal",
  "sessionId": "session-123",
  "turnId": "turn-abc",
  "sequence": 42,
  "status": "completed",
  "stopReason": "end_turn",
  "finalAnswer": "调研结论如下……",
  "runtimeRequestId": "turn-abc",
  "promptMessageId": "turn-abc",
  "completedAt": "2026-07-29T03:01:20Z"
}
```

失败：

```json
{
  "event": "turn_terminal",
  "sessionId": "session-123",
  "turnId": "turn-abc",
  "status": "failed",
  "stopReason": "runtime_error",
  "error": {
    "code": "ACP_PROMPT_FAILED",
    "message": "..."
  }
}
```

取消：

```json
{
  "event": "turn_terminal",
  "sessionId": "session-123",
  "turnId": "turn-abc",
  "status": "cancelled",
  "stopReason": "cancelled_by_user",
  "finalAnswer": "已经产生的可见部分回答"
}
```

兼容期可以继续发送旧 `done/error`，但 Go 新状态机只以 `turn_terminal` 驱动明确 Turn 的终态。

---

## 9. 从开始到结束的落库时序

### T0：Browser 创建提交身份

Browser：

1. 生成 `client_request_id`；
2. 创建 optimistic user bubble；
3. bubble 保存 `clientRequestId`；
4. 发送 prompt。

此时没有创建 Turn，不应在 Browser 生成 `turn_id`。

### T1：Go 接受并持久化 queued Turn

Go 在一个事务中：

1. 校验 Session 存在且属于 Project；
2. 以 `(session_id, client_request_id)` 查询幂等记录；
3. 不存在则创建 `agent_turns(status=queued)`；
4. 写 `turn.queue` ProjectEvent；
5. 对 task-scoped Session，创建或绑定本轮用户 Reply；
6. commit；
7. 返回 `turn_state(queued)`。

**Turn 的第一次落库发生在 T1。**

### T2：Go 取得执行权

如果 Session 没有 running Turn：

1. CAS `queued → running`；
2. 写 `started_at`；
3. 同事务写 `turn.start` Event；
4. 设置 Go `activeTurn=turnId`；
5. 发送 `turn_state(running)`；
6. 向 bridge 下发带 `turnId` 的 prompt。

如果已有 running Turn，只保留在 Go durable queue，不下发 bridge。

### T3：1acp 在调用 Agent 前持久化 Turn 起点

1acp runtime 在真正发起 ACP prompt 前：

1. 加载真实 `sessionId` 对应 SessionRecord；
2. 写入用户消息，`SessionMessage.User.id = turnId`；
3. 设置 `lastRequestId = turnId`；
4. 写入：

```text
acpx.turn_results[turnId] = {
  status: running,
  prompt_message_id: turnId,
  started_at: ...
}
```

5. 原子保存 SessionRecord；
6. 再调用 Agent prompt。

这样可区分：

- Go 已 running，但 runtime 从未接受；
- runtime 已接受并落下 Turn 起点。

### T4：执行与 checkpoint

执行期间：

- 1acp 按现有 conversation checkpoint 保存消息；
- bridge 为每个对外事件加 `turnId` 和单调 `sequence`；
- Go 不把每个 token delta 写入 meta.db；
- 可按节流频率更新 `agent_turns.last_event_seq/updated_at`，但不是正确性前提；
- ProjectItem 写入继续生成带同一 `turn_id` 的 ProjectEvent。

### T5：1acp 持久化 runtime 终态

runtime 得到 terminal result 后，先在同一次 SessionRecord 保存中完成：

```text
messages = 最终对话
lastRequestId = turnId
acpx.turn_results[turnId] = {
  status: completed | failed | cancelled,
  prompt_message_id: turnId,
  stop_reason: ...,
  completed_at: ...,
  last_event_seq: ...
}
```

然后 `AcpRuntimeTurnResult` 返回：

- status；
- stopReason；
- finalAnswer；
- promptMessageId；
- usage；
- error。

**必须先持久化，再 resolve `turn.result`。**

### T6：bridge 发唯一 terminal

bridge 根据 `AcpRuntimeTurnResult` 发 `turn_terminal`，不再通过 generic `error` 猜测终态。

bridge 只有在 terminal 已发出或明确连接失败后，才清空：

```text
session.activeTurn
```

bridge 不主动启动下一条宿主 Turn；下一条只由 Go 调度。

### T7：Go 完成 AgentTurn

Go 收到匹配当前 `activeTurn.turnId` 的 terminal 后，在一个事务中：

1. CAS `running → terminal`；
2. 保存 `final_answer/error/stop_reason/completed_at`；
3. 写 `turn.complete/fail/cancel` ProjectEvent；
4. 写或关联 Agent Reply；
5. 必要时写 TaskRun Evidence/terminal result；
6. commit；
7. 清空 Go activeTurn；
8. 向 Browser 发送 terminal `turn_state`；
9. 从 durable queue 取 FIFO 头并执行 T2。

### T8：前端稳定展示

Browser：

- realtime 过程按事件中的 `turnId` 归组；
- terminal 后关闭对应 Turn，而不是关闭“当前最后一组”；
- 刷新后从 history 中直接读取 `turnId`；
- Activity 和 TaskRun 使用相同 `turnId`。

---

## 10. 1acp SessionRecord 扩展

### 10.1 不升级完整消息 schema

现有用户消息已经有稳定 ID：

```ts
type SessionUserMessage = {
  id: string;
  content: SessionUserContent[];
};
```

宿主 Turn 直接使用：

```text
User.id = turn_id
```

因此不需要给每个 Agent message、ToolResult 复制一个 `turn_id`。

分组规则：

```text
从 User.id = turn_id 开始
到下一个 User 消息之前
全部属于该 Turn
```

`Resume` 不创建新 Turn，只属于 Session 恢复边界。

### 10.2 最小终态索引

在 `SessionAcpxState` 中增加可选字段：

```ts
type RuntimeTurnResultSnapshot = {
  status: "running" | "completed" | "failed" | "cancelled";
  prompt_message_id: string;
  started_at: string;
  completed_at?: string;
  stop_reason?: string;
  last_event_seq?: number;
  error_code?: string;
};

type SessionAcpxState = {
  // existing fields...
  turn_results?: Record<string, RuntimeTurnResultSnapshot>;
};
```

键就是 canonical `turn_id`。

它只保存对账所需的小字段，不复制：

- Prompt 全文；
- Final answer；
- Tool input/output；
- 私有思维；
- ProjectEvents。

### 10.3 裁剪策略

当 conversation trimming 删除某个用户消息时：

- 若对应 Turn 已 terminal，可删除 `turn_results[turnId]`；
- 最近一段可恢复窗口中的 Turn 结果继续保留；
- meta.db 的 AgentTurn 审计记录不受影响。

建议默认保留与当前 SessionRecord messages 同范围的 Turn result。

### 10.4 History 输出

`history_response.items` 增加可选：

```ts
type HistoryItem = {
  // existing shape
  turnId?: string;
};
```

bridge 从 `User.id` 开始给这一段所有 history items 填相同 `turnId`。

旧 record 的 User.id 不是合法 AgentTurn ID 时不填，前端继续走 legacy fallback。

---

## 11. Queue：只保留一个调度中心

### 11.1 Go durable queue

Go 负责：

- 创建 queued Turn；
- FIFO；
- queued cancel；
- running 互斥；
- 启动下一条；
- 重启后取消未执行 queued Turn。

队列顺序继续使用：

```text
ORDER BY created_at, id
```

但实际调度下一条时应从 store 查询 `NextQueued(sessionId)`，而不是只依赖内存 `pendingTurns`。

内存列表只能作为缓存，SQLite 是恢复和对账依据。

### 11.2 bridge 行为

host-managed 模式下：

- bridge 每个 Session 只允许一个 active Turn；
- 收到已有 active Turn 时的第二个 prompt，返回：

```json
{
  "event": "protocol_error",
  "code": "TURN_ALREADY_ACTIVE",
  "sessionId": "...",
  "turnId": "new-turn-id"
}
```

- 不加入 `promptQueue`；
- 不创建 `queued_<timestamp>`；
- 不决定下一条运行。

bridge 原有 queue 可以在兼容期只服务没有 `turnId` 的 legacy caller。

### 11.3 为什么不能让 bridge 做第二队列

bridge 不拥有：

- `agent_turns` 事务；
- client request 幂等索引；
- ProjectEvent；
- Browser optimistic bubble；
- backend restart policy。

因此 bridge 无法成为 durable queue owner。

---

## 12. 取消语义

统一请求：

```json
{
  "action": "cancel_turn",
  "sessionId": "session-123",
  "turnId": "turn-abc"
}
```

### queued

Go：

1. 校验 Turn 属于 Session；
2. CAS `queued → cancelled`；
3. 写 terminal Event；
4. 返回 `turn_state(cancelled)`；
5. 不向 bridge 发送取消。

### running

Go：

1. 校验 `turnId == activeTurn.id`；
2. 转发给 bridge；
3. bridge 再校验自己的 active Turn；
4. runtime cancel；
5. 等待 runtime 返回 cancelled；
6. 正常走 terminal 持久化。

### stale cancel

如果取消到达时 Turn 已 terminal：

- 返回当前 terminal 状态；
- 不取消后来启动的新 Turn；
- 操作幂等。

这是要求取消显式携带 `turnId` 的主要原因。

### 前端“取消”和“编辑”

后端不增加 edit/replace/supersede 领域操作。前端提供两个用户动作：

#### 取消

```text
点击“取消”
  → cancel_turn(turnId)
  → 旧 Turn 进入 cancelled
  → Composer 不回填旧 Prompt
```

#### 编辑

```text
点击“编辑”
  → cancel_turn(turnId)
  → 等待旧 Turn 确认 cancelled
  → 将旧 Prompt 文本回填 Composer
  → 聚焦输入框
  → 用户本地修改
  → 用户再次点击发送
  → 生成新的 client_request_id
  → Go 创建新的 turn_id
```

重要边界：

- 点击“编辑”本身不会创建新 Request 或新 Turn；
- 用户回填后放弃编辑，不留下第二个 Turn；
- 新 Turn 不保存 `supersedes_turn_id`；
- 旧 Turn 与新 Turn 不建立后端继承/替代关系；
- 不实现 replace API；
- 不继承旧 Turn 的队列位置；
- 新提交按普通规则进入当前队列尾部；
- UI 只保留“旧 Turn cancelled”和“Composer 中的新草稿”两个简单事实。

对于 running Turn，“编辑”需要等待 runtime 返回 cancelled 后再开放发送，避免旧 Turn 尚未停止时又提交新意图。对于 queued Turn，取消通常可以立即完成。

---

## 13. Error 与 Terminal 分离

### 13.1 Error 分类

```text
scope = turn | control | session | transport
terminal = true | false
```

示例：

| 错误                       | scope        | terminal |
| -------------------------- | ------------ | -------- |
| prompt RPC 失败            | turn         | true     |
| set mode 失败              | control      | false    |
| permission response 找不到 | control      | false    |
| session store 读取失败     | session      | 视情况   |
| Browser WS 断开            | transport    | false    |
| bridge ↔ runtime 连接丢失  | turn/session | true     |

### 13.2 唯一终态入口

Go 不再用：

```text
if event == "error" then finishActiveTurn
```

而是：

```text
if event == "turn_terminal" and event.turnId == activeTurn.id
  then finalize
```

非终态 error 只展示、记录或触发控制恢复，不迁移 AgentTurn 状态。

---

## 14. Final Answer 单一来源

当前 Go 根据流式 text/tool 顺序重建最终回答。这可以作为 legacy fallback，但不应继续作为新链路真源。

新链路：

1. 1acp runtime 完成 conversation 保存；
2. runtime 根据 `promptMessageId=turnId` 找到本 Turn 最后一个可见 Agent 文本块；
3. 返回 `AcpRuntimeTurnResult.finalAnswer`；
4. bridge 放入 `turn_terminal.finalAnswer`；
5. Go 保存到 `agent_turns.final_answer`；
6. 对 task-scoped Session 写 Agent Reply。

好处：

- final answer 与持久化 history 一致；
- Go 崩溃后可以从 runtime record 对账；
- tool_call update 不会意外清空最终答案；
- 不依赖 Browser 是否在线；
- 不保存私有 thought。

---

## 15. 重连、接管与恢复

### 15.1 Browser 临时断线

不改变 Turn 状态。

新 Browser 连接后，Go 发送：

```json
{
  "event": "turn_sync",
  "sessionId": "session-123",
  "active": {
    "turnId": "turn-running",
    "requestId": "client-request",
    "status": "running"
  },
  "queued": [
    {
      "turnId": "turn-next",
      "requestId": "client-request-2",
      "status": "queued",
      "queuePosition": 1
    }
  ]
}
```

前端随后：

- 加载带 `turnId` 的 history；
- 将本地 optimistic bubble 与 persisted Turn 合并；
- 忽略 sequence 小于等于已消费值的重复事件。

### 15.2 多 Tab 接管

保留 `session_taken_over`。

新连接接管后必须收到 `turn_sync`，旧连接不再发送取消或控制操作。

### 15.3 bridge/runtime 连接丢失

如果 active runtime 无法继续：

- 1acp 尽力写 failed turn result；
- bridge 发送 `turn_terminal(failed, runtime_lost)`；
- Go fail active Turn；
- queued Turn 仍在 Go，可选择继续启动新 runtime 或按 Session policy 取消。

默认安全策略：

```text
active failed
queued cancelled
```

与 v1 一致。

### 15.4 Backend 重启对账

不能在进程构造时无条件先把所有 running Turn 标 failed。

推荐启动流程：

1. 扫描 `agent_turns.status IN (running, queued)`；
2. queued：
   - 标 cancelled；
   - `error_code=backend_restarted`；
   - 不重放；
3. running：
   - 根据 `session_id` 加载 1acp SessionRecord；
   - 查 `acpx.turn_results[turn_id]`；
4. 若 runtime snapshot 已 terminal：
   - 使用 snapshot 和 transcript 补齐 meta terminal；
   - `terminal_source=reconciled_runtime_record`；
5. 若 snapshot 为 running 或不存在：
   - 先关闭/取消旧 runtime owner；
   - 标 failed；
   - `error_code=backend_restarted` 或 `runtime_state_unknown`；
6. 写 reconciliation 日志和指标。

### 15.5 关键崩溃窗口

| 崩溃点                                  | 可观察状态                      | 恢复结果                     |
| --------------------------------------- | ------------------------------- | ---------------------------- |
| T1 前                                   | 无 AgentTurn                    | Browser 用同 requestId 重试  |
| T1 后、ack 前                           | queued Turn 存在                | 重试返回同 Turn              |
| running 已落库、runtime 未保存起点      | meta running，无 runtime result | failed: dispatch interrupted |
| runtime running 已保存                  | 两边 running                    | cancel old owner 后 failed   |
| runtime terminal 已保存、Go 未 finalize | runtime terminal、meta running  | reconcile 为真实 terminal    |
| Go terminal commit 后、Browser 未收到   | meta terminal                   | reconnect/sync 返回 terminal |
| queued 未下发、Backend 重启             | meta queued、runtime 无记录     | cancelled，不重放            |

---

## 16. Project Mutation 归因与 Turn Fence

### 16.1 MVP 保留 Session token → running Turn 解析

现有可信归因链继续保留：

```text
ONEAGENTS_SESSION_ID
ONEAGENTS_SESSION_TOKEN
  ──► backend validates Session
  ──► RunningBySession
  ──► MutationContext.turnId
```

在单 Session 单 running Turn、同步 MCP 调用必须在 prompt 返回前结束的前提下，这条链仍然成立。

### 16.2 必须增加的约束

- Turn terminal commit 前，不得启动下一 Turn；
- runtime 必须等待本轮同步工具调用结束后才返回 terminal；
- terminal 后到达的 Session-attributed写入返回 409；
- 不支持后台进程在 Turn 结束后继续以 Session token 写项目；
- ProjectEvent 的 `turn_id` 必须对应 running Turn。

### 16.3 后续硬化：per-turn fence token

如果未来允许后台任务、subagent 或异步 MCP 写入，需要引入：

```text
turn_fence_token = HMAC(session_id, turn_id, runtime_epoch)
```

并由每次工具调用携带。仅有静态 Session token 无法区分“旧 Turn 的迟到后台写入”和“当前 Turn 的合法写入”。

这项能力列为 P2，不阻塞同步 MCP 的 v1.1 闭环。

---

## 17. Reply 关系修正

### 17.1 区分两个 Reply

每个 Turn 最多涉及：

1. `initiating_reply_id`：触发这个 Turn 的用户 Reply；
2. `final_reply_id`：Turn 结束后写入的 Agent Reply。

连接级 `bridge.ReplyID` 不能作为所有后续 Turn 的 initiating reply。

### 17.2 推荐方案

task-scoped Session 的每个 prompt：

1. T1 事务中创建用户 Reply；
2. 用户 Reply 直接写 `turn_id`；
3. AgentTurn 保存 `initiating_reply_id`；
4. T7 创建 Agent Reply；
5. Agent Reply 保存相同 `turn_id` 和 `in_reply_to=initiating_reply_id`；
6. AgentTurn 可新增 `final_reply_id`，便于反查。

旧的 WS query `reply_id` 只用于：

- 首次从 Task timeline 打开 Session 时定位来源；
- legacy compatibility；
- 不再作为每轮动态关联键。

---

## 18. 数据库变更

### 18.1 `agent_turns` 建议新增字段

```sql
ALTER TABLE agent_turns ADD COLUMN runtime_record_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_turns ADD COLUMN runtime_request_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_turns ADD COLUMN prompt_message_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_turns ADD COLUMN final_reply_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_turns ADD COLUMN stop_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_turns ADD COLUMN terminal_source TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_turns ADD COLUMN last_event_seq INTEGER NOT NULL DEFAULT 0;
```

含义：

| 字段               | 用途                                                     |
| ------------------ | -------------------------------------------------------- |
| runtime_record_id  | 对账使用；正常等于真实 Session ID                        |
| runtime_request_id | 正常等于 Turn ID                                         |
| prompt_message_id  | 正常等于 Turn ID                                         |
| final_reply_id     | 精确反查 Agent Reply                                     |
| stop_reason        | `end_turn/cancelled/runtime_error/...`                   |
| terminal_source    | `live_runtime/reconciled_runtime_record/recovery_policy` |
| last_event_seq     | 重连去重和诊断                                           |

这些字段中前三个存在值相等的冗余，但它们表达不同层的事实，能在迁移期检测链路是否断裂。

### 18.2 不新增 token/event 明细表

v1.1 不创建 `turn_stream_events`：

- 高频流式事件仍由 1acp SessionRecord 保存；
- meta.db 只保存状态和项目审计；
- 避免 token 级写放大。

如果未来需要完整事件回放，再独立设计 append-only event log。

---

## 19. API 与前端投影

### 19.1 API

保留：

```text
GET /api/agent/turns?workspace_id=&session_id=
GET /api/agent/activity?workspace_id=&turn_id=
```

建议增加：

```text
GET /api/agent/turns/{turn_id}
GET /api/agent/sessions/{session_id}/turn-state
```

第一个用于 Activity/Task Detail 精确打开；第二个用于非 WS fallback。

### 19.2 Browser state

每个 user item 增加：

```ts
{
  clientRequestId: string;
  turnId?: string;
  turnStatus?: AgentTurnStatus;
  queuePosition?: number;
}
```

每个 realtime item 都从 wire event 继承 `turnId`。

### 19.3 History

新 history 有显式 `turnId`：

```text
history item.turnId
  ──► ChatItem.turnId
  ──► MessageList group
```

不再需要：

```text
promptText exact match
chronological fallback
```

但 fallback 保留给：

- migration 前 SessionRecord；
- native agent 历史无法提供 Turn 边界；
- 外部创建、未经过 1agents AgentTurn 的 Session。

### 19.4 Queue UI

queued user bubble 直接展示：

- stable `turnId`；
- queue position；
- cancel by turnId；
- reconnect 后仍可恢复。

第一条 realtime event 不再负责“猜测 queued bubble 已经开始”。

---

## 20. 迁移与兼容

### 20.1 Feature gate

建议增加协议能力：

```json
{
  "turnProtocolVersion": 2
}
```

在 `session_ready/session_meta` 中广告。

### 20.2 v2 链路

满足以下条件才启用：

- Browser 支持 client request id；
- Go 支持 durable Turn queue；
- bridge 支持 explicit turnId；
- 1acp runtime 支持 requestId=turnId 和 turn result snapshot。

### 20.3 legacy 链路

旧 Browser 或旧 bridge：

- 允许无 requestId prompt；
- Go 可生成 server-side request ID，但应记录 `legacy_generated`；
- bridge legacy queue 继续存在；
- history 继续按 Prompt/时间推断；
- 不声称具备 exact Turn recovery。

### 20.4 历史数据

不批量伪造 Turn。

只做：

- 已有 `agent_turns` 保留；
- 已有 `replies.turn_id` 保留；
- 旧 SessionRecord 不修改；
- 新 Turn 从启用 v2 后开始精确闭环。

### 20.5 预热 Session

顺序必须是：

```text
prewarm ensureSession
  → real session arrives
  → adoptSession/rebind record
  → session_ready(real session id)
  → accept first prompt
  → create first AgentTurn
```

禁止在 `prewarm_grok_*` 身份下创建 AgentTurn。

---

## 21. 实施拆分

建议新建一个 follow-up Requirement：

> **Turn v1.1：端到端身份、持久化与恢复闭环**

不要重开 `#282–#288`；它们是 v1 领域基线。

### Phase A：协议与验收真源

交付：

- 冻结 ID 表；
- 冻结 `turn_state/turn_terminal/turn_sync`；
- 冻结 error scope；
- 标记 v1 文档中哪些“已实现”只是领域层完成；
- 建立端到端测试矩阵。

验收：

- 协议示例能覆盖 queued、running、terminal、cancel、reconnect；
- 不再存在未定义的 `requestId` 语义；
- 明确 Go 是唯一 queue owner。

### Phase B：Browser request ID 与状态 reducer

交付：

- prompt 前生成 UUID；
- user bubble 保存 clientRequestId；
- 消费 `turn_state/turn_sync`；
- cancel 明确携带 turnId；
- Turn 操作区提供“取消”和“编辑”；
- “编辑”复用 cancel 后将旧 Prompt 回填 Composer；
- 用户再次发送时才生成新 requestId；
- 支持 terminal 对指定 Turn 收口。

依赖：Phase A。

验收：

- 连续发送相同文本形成不同 requestId/turnId；
- 网络重试同 requestId 不重复；
- queued bubble 刷新后仍能恢复和取消。
- 编辑 queued/running Turn 时旧 Turn cancelled，Composer 回填文本；
- 放弃回填草稿不会创建新 Turn；
- 修改后发送创建全新的 requestId/turnId，并进入队尾。

### Phase C：Go durable queue 与 wire 注入

交付：

- `client_request_id` 必填或 legacy 生成；
- 从 SQLite `NextQueued` 调度；
- 下发 prompt 时注入 `turnId` 且 `requestId=turnId`；
- 统一 cancel；
- `turn_sync`；
- terminal CAS。

依赖：Phase A。

验收：

- 同 Session 永远最多一个 running；
- queue 不依赖内存才能查询；
- 重复 request 不会重复发送到 runtime；
- stale cancel 不会取消新 Turn。

### Phase D：bridge/runtime Turn 传播

交付：

- bridge 保存 explicit active turnId；
- 删除 host-managed 双队列；
- 每个事件加 turnId/sequence；
- runtime.startTurn 使用 turnId；
- `AcpRuntimeTurnResult` 返回 finalAnswer/stopReason/promptMessageId；
- 只发一个 `turn_terminal`。

依赖：Phase C。

验收：

- bridge 不再生成 `turn_<timestamp>`；
- control error 不会结束 Turn；
- 迟到事件不会归到下一 Turn；
- terminal 与 active turnId 不一致时拒绝。

### Phase E：SessionRecord Turn 边界与对账

交付：

- `User.id=turnId`；
- `acpx.turn_results`；
- terminal durable-before-visible；
- history item 输出 turnId；
- pruning。

依赖：Phase D。

验收：

- 仅查看 JSON 就能定位一个 Turn 的消息范围和终态；
- runtime terminal 后杀 Go，重启能还原 terminal；
- 重复 Prompt 不影响映射。

### Phase F：事务终态、Reply 与 TaskRun

交付：

- AgentTurn terminal + lifecycle event + Reply 关联同事务；
- initiating/final reply 分离；
- final answer 使用 runtime result；
- TaskRun/Evidence 引用同一 turnId；
- terminal_source/stop_reason。

依赖：Phase D、E。

验收：

- 一个 Turn 的 user/agent Reply 都能反查；
- agent reply 不再依赖 connection-level ReplyID；
- runtime 已完成但 Go 崩溃的窗口可对账。

### Phase G：Frontend exact projection 与兼容收尾

交付：

- history 直接使用 turnId；
- Prompt 文本匹配降级为 legacy-only；
- Project Activity/Task Detail 精确跳转；
- v1/v2 feature gate；
- 删除或隔离 bridge legacy queue。

依赖：Phase B、E、F。

验收：

- 相同 Prompt 连续发送三次仍精确分成三个 Turn；
- history 分页不把最新 Turn 配给旧消息；
- Activity 打开 Session 后精确聚焦 Turn。

### Phase H：恢复、指标与全链路验收

交付：

- startup reconciliation；
- orphan sweeper；
- mismatch 指标；
- crash-window 测试；
- live canary。

依赖：Phase C–G。

验收：

- 所有崩溃窗口得到确定结果；
- 无自动重放；
- 无 running Turn 永久悬挂；
- 运行实例中创建 Session 后发送 Prompt，`agent_turns`、SessionRecord 和 UI 出现相同 turnId。

---

## 22. 测试矩阵

### 22.1 ID 与幂等

- 同 Session 相同 requestId 提交两次，只创建一个 Turn；
- 同 Session 相同 requestId、不同请求内容返回幂等冲突；
- 相同文本、不同 requestId 创建两个 Turn；
- cancelled Turn 的 requestId 不会绑定到后续 Turn；
- Browser 伪造 turnId 被拒绝；
- Go→bridge→runtime requestId 全部等于 AgentTurn ID；
- SessionRecord User.id 等于 AgentTurn ID。

### 22.2 FIFO

- 三 Prompt：1 running、2 queued；
- 第一条 terminal 后只启动第二条；
- queued cancel 不影响 active；
- active cancel 不取消未指定的 queued；
- stale cancel 不影响后来 running 的 Turn。
- 点击“编辑”只取消旧 Turn 并回填 Composer，不自动创建新 Turn；
- 编辑后发送创建新 Request/Turn，并按普通规则进入队尾；
- 不存在 supersedes/replace/队列位置继承数据。

### 22.3 Event routing

- 每个 realtime event 有 turnId；
- 前一个 Turn 的迟到 delta 不进入后一个 Turn；
- duplicate sequence 被去重；
- control error 不结束 Turn；
- 每个 Turn 只有一个 terminal。

### 22.4 Persistence

- running 前 SessionRecord 已保存 User.id/turn result；
- terminal 可见前 SessionRecord 已保存 terminal；
- meta AgentTurn terminal 与 runtime snapshot 一致；
- final answer 与 history 最后可见回答一致；
- ProjectEvents、Replies、TaskRun 使用相同 Turn ID。

### 22.5 Reconnect

- Browser 断线，Turn 继续；
- 新连接收到 active/queued sync；
- takeover 后旧连接不能取消；
- history reload 保留精确 Turn 分组；
- queued bubble 不因刷新消失。

### 22.6 Crash windows

- queued commit 后 Go crash；
- running commit 后、runtime dispatch 前 crash；
- runtime start snapshot 后 crash；
- runtime terminal save 后、Go finalize 前 crash；
- Go terminal commit 后、Browser ack 前 crash；
- bridge/runtime process loss；
- Grok prewarm adopt 后首 Turn。

### 22.7 Legacy

- 旧 history 没有 turnId 时仍能展示；
- 旧 Browser 无 requestId 时服务端生成兼容 ID；
- 旧 bridge 不支持 v2 时不启用 exact recovery；
- 历史 record 不被错误补造 Turn。

---

## 23. 可观测性与对账

### 23.1 指标

```text
turn_accept_total
turn_idempotent_replay_total
turn_queue_wait_seconds
turn_runtime_duration_seconds
turn_terminal_total{status,source}
turn_reconcile_total{result}
turn_orphan_running_total
turn_protocol_mismatch_total
turn_event_without_id_total
turn_event_sequence_gap_total
turn_runtime_meta_status_mismatch_total
```

### 23.2 日志字段

每条相关日志统一包含：

```text
session_id
turn_id
client_request_id
runtime_request_id
acp_session_id
event_sequence
status
```

不得只打印：

```text
Turn finished for session ...
```

而不打印 Turn ID。

### 23.3 健康检查

建议增加只读诊断：

```text
1agents doctor turns
```

检查：

- running Turn 是否有对应 Session；
- running Turn 是否有 runtime start snapshot；
- terminal meta 是否与 runtime result 冲突；
- 是否存在空 client_request_id 的新协议 Turn；
- 是否存在新协议 event 无 turnId；
- 是否存在同 Session 多 running；
- 是否存在长期 queued/orphaned running。

---

## 24. 风险与缓解

| 风险                                | 缓解                                                       |
| ----------------------------------- | ---------------------------------------------------------- |
| Browser、Go、bridge 必须协同升级    | `turnProtocolVersion=2` feature gate                       |
| SessionRecord `turn_results` 增长   | 与 message trimming 同步裁剪                               |
| terminal 双发                       | bridge 单 terminal gate + Go CAS                           |
| bridge legacy queue 与新 queue 混用 | 按是否存在 turnId 分流，最终删除 legacy host queue         |
| runtime record 和 meta 状态冲突     | terminal_source + startup reconciliation + mismatch metric |
| 取消竞态                            | cancel 必带 turnId，三层校验                               |
| running Turn 点击编辑后取消耗时     | 显示“正在停止并准备编辑”，收到 cancelled 后再开放发送      |
| 旧 Prompt 匹配逻辑掩盖错误          | v2 history 禁止 fallback；只有 legacy 才 fallback          |
| 后台 CLI 写入跨 Turn                | v1.1 明确不支持；后续 per-turn fence token                 |
| 事务范围扩大                        | 先保证 AgentTurn/Event/Reply；TaskRun 通过幂等终态 API衔接 |
| 本机 live 数据没有 Turn 样本        | Phase H 必须做真实部署 canary，而不只跑单元测试            |

---

## 25. Definition of Done

只有同时满足以下条件，才能把 Turn 标记为“完整闭环”：

1. Browser 发送的每个新 Prompt 都有非空 clientRequestId；
2. Go 创建的 Turn ID 能在 bridge prompt 中看到；
3. 1acp runtime requestId 等于该 Turn ID；
4. SessionRecord 对应用户消息 ID 等于该 Turn ID；
5. 每个 realtime event 都带该 Turn ID；
6. Browser queued/running/terminal 都按该 Turn ID 更新；
7. 1acp terminal 先落盘，再发 terminal；
8. meta.db terminal 与 runtime terminal 一致；
9. Reply、ProjectEvent、TaskRun 引用同一 Turn ID；
10. 重连不改变 Turn ID；
11. 相同 Prompt 不会导致历史误配；
12. queued cancel 和 active cancel 都精确作用于指定 Turn；
13. control error 不会误终止 Turn；
14. backend crash 的各个窗口都有确定恢复策略；
15. 取消旧 Turn 后重新发送会创建新 requestId/turnId；
16. “编辑”只负责取消和回填 Composer，不产生 supersedes/replace 关系；
17. live canary 能证明：

```text
sessions.id
  == 1acp acpx_record_id

agent_turns.id
  == bridge turnId
  == runtime requestId
  == SessionRecord User.id
  == realtime event.turnId
  == replies.turn_id
  == project_events.turn_id
  == task_runs.origin_turn_id（适用时）
```

---

## 26. 推荐执行顺序

```text
Phase A 协议冻结
  │
  ├── Phase B Browser request/状态
  │
  └── Phase C Go durable queue
          │
          ▼
       Phase D bridge/runtime 传播
          │
          ▼
       Phase E SessionRecord 对账
          │
          ▼
       Phase F terminal/Reply/TaskRun
          │
          ▼
       Phase G frontend exact projection
          │
          ▼
       Phase H recovery + canary
```

最小可合并切片建议：

1. 先把 `client_request_id` 和 `turnId` 贯通到 bridge；
2. 再删除 host-managed 双队列；
3. 再给所有 events 加 turnId；
4. 再落 SessionRecord Turn 边界/结果；
5. 最后切换前端历史投影和 startup reconciliation。

这样每个阶段都可验证，且不会一次性同时改动所有持久化与 UI 逻辑。

---

## 27. 与 v1 文档的关系

[design.md](./design.md) 和 [prd.md](./prd.md) 中以下部分继续有效：

- Turn 的领域定义；
- AgentTurn / ProjectEvent / Activity 的关系；
- Session 不强制绑定 ProjectItem；
- Project write 自动归因；
- TaskRun / Evidence / ClosedBy 边界；
- 不保存私有思维链；
- 不自动重放。

本文件补充并在实现层面取代以下描述：

- Bridge Turn 生命周期已经端到端完成；
- FIFO 已经同时被 UI、Go、bridge 正确消费；
- ChatUI 已优先使用显式 Turn ID；
- runtime 丢失/重启已经具备精确 Turn 对账；
- `client_request_id` 幂等已经在真实 Browser 主链路生效。

完成 v1.1 前，建议产品状态使用：

> **Turn 领域层已实现；端到端运行时闭环待补全。**
