# Agent Turn：从 ChatUI 折叠单元到 PM 可审计因果批次

| 字段 | 内容 |
|------|------|
| 状态 | **已实现（#282–#288，2026-07-29）** |
| 版本 | **v1.0** |
| 日期 | 2026-07-29 |
| 范围 | ChatUI、Agent Bridge、ProjectItems CLI/MCP、PM 看板、Task Detail、meta.db |
| 产品 PRD | [prd.md](./prd.md)（产品决策与 MVP 范围以 PRD 为准） |
| 实现走查 | [walkthrough.md](./walkthrough.md) |
| 关联 | [issue-model](../issue-model/design.md)、[project-model](../project-model/design.md)、[pm-standalone](../pm-standalone/prd.md)、[verification-gate](../verification-gate/design.md) |

---

> **Phase 0 ADR（2026-07-29）：**本文是 #282 冻结后的实施基线。事件源唯一命名为 `project_events`；Project Activity 是按 `turn_id / correlation_id` 聚合 Events 的只读投影。本文所有“候选”均明确标为非 MVP，不是后续任务的待决策项。

## 0. 一句话定位

> **Turn 是一次用户意图到项目事实变更之间的可审计因果批次。**

用户发起一次请求，智能体在这一轮内进行对外可见的过程输出、调用工具、创建或修改若干 ProjectItem，并最终给出一个回答；这些动作共同属于同一个 Turn。

ChatUI 展示 Turn，PM 看板汇总 Turn，Task Detail 投影 Turn，TaskRun 和验收证据向下支撑 Turn。

Turn 不是：

- 仅由前端按消息顺序推断出的视觉分组；
- 一张 Task Card 的别名；
- 一次 TaskRun；
- 完整保存的智能体私有思维链；
- 只属于 PM 页面的一套旁路日志。

---

## 1. 背景与现状

### 1.1 ChatUI 已经具备 Turn 的展示雏形

当前 ChatUI 已经可以根据用户消息边界，将历史消息折叠为 Turn：

- 当前轮过程展开并支持 sticky；
- 历史轮过程折叠；
- 历史轮只保留最后回答；
- 旧消息不需要后端 Turn ID 也能按顺序推断。

这解决的是长对话的阅读问题，但当前 Turn 仍然是展示层概念：

- 没有稳定的后端 `turn_id`；
- 刷新、重连、排队、取消时只能依赖消息顺序；
- 工具对 ProjectItem 的修改无法反查到具体 Turn；
- 一个 Turn 创建多个 Tasks 时，Task Detail 无法解释它们为何同时出现；
- Agent 最终回答和实际项目变更之间没有系统级核对关系。

### 1.2 后端已有隐式 Turn 生命周期

`backend/internal/agent/acpx_client.go` 当前已经隐含以下过程：

1. 收到用户 prompt；
2. 清空本轮文本缓冲；
3. 接收文本和工具事件；
4. 收到 `done`；
5. 将最后一段 Agent 回答写回 Task timeline。

这说明 Turn 的生命周期已经存在，但没有显式身份、持久化状态和项目操作归因。

### 1.3 ProjectItem 与 Session 当前是软关联

现有 [issue-model](../issue-model/design.md) 已确认：

- Session 可以没有 `taskId`；
- 从 Task Detail 启动的 Session 可以绑定一个 Task；
- 普通侧边栏 Session 可以只绑定 Project，不绑定 Task；
- `sessions.task_id` 是可空的单值软关联。

这个决策应继续保留。项目级 PM 对话天然可能跨越多个 ProjectItem，不能为了追踪 Turn 而强制把 Session 绑定到某一张 Task Card。

---

## 2. 核心术语与边界

### 2.1 推荐内部名称

数据库和后端领域对象使用 **`AgentTurn`**，产品 UI 可以显示为“Turn / 本轮”。

不建议命名为 `PMTurn`：

- 普通 Chat Session 也可以调用 ProjectItems 工具；
- Task 执行 Session 也会产生 Turn；
- CLI/MCP 与完整工作台需要共享同一个模型；
- PM 是 Turn 的一种业务投影，不应成为底层存储边界。

### 2.2 领域关系

```text
Project
  ├── ChatSession
  │     ├── AgentTurn #1
  │     ├── AgentTurn #2
  │     └── AgentTurn #3
  │
  ├── ProjectItem A
  ├── ProjectItem B
  └── ProjectItem C

AgentTurn #2
  ├── 用户请求
  ├── 最终回答
  ├── ProjectEvent → 创建 A
  ├── ProjectEvent → 创建 B
  ├── ProjectEvent → 更新 C
  └── 可选：触发一个或多个 TaskRun
```

关系基数：

```text
Project       1 ── N ChatSession
ChatSession   1 ── N AgentTurn
AgentTurn     N ── M ProjectItem（通过 ProjectEvent）
ProjectItem   1 ── N TaskRun
AgentTurn     1 ── N TaskRun（可选 origin_turn_id）
```

### 2.3 与相邻概念的区别

| 概念 | 回答的问题 | 生命周期 |
|------|------------|----------|
| Session | 长期对话发生在哪里？ | 多个 Turn |
| Turn | 这次用户意图产生了什么结果？ | 一次请求到最终回答 |
| ProjectItem | 项目中需要跟踪的事实是什么？ | 从创建到关闭/归档 |
| TaskRun | 某个 Task 这一次如何执行？ | 一次执行/验收/返工尝试 |
| ProjectEvent | 项目事实具体发生了什么变化？ | 一次不可变变更事件 |
| Completion Audit | 为什么允许判定 completed/closed？ | 一次有证据的裁定 |

---

## 3. 关键场景：非 Task Session 创建三个 Tasks

### 3.1 场景

用户从普通侧边栏进入一个项目级会话。该 Session 没有 `taskId`。

用户说：

> 把 Turn 能力拆成三个可执行任务并写入看板。

Agent 在同一轮中创建：

- Task A：Turn 存储模型；
- Task B：Bridge 生命周期与 MCP 归因；
- Task C：Task Detail 展示。

### 3.2 推荐结果

这个 Session 继续保持：

```text
session.project_id = 当前项目
session.task_id = null
```

系统创建一个 Turn：

```text
AgentTurn T100
session_id = S10
project_id = P1
status = completed
```

每次创建 Task 时，后端同时写入事件：

```text
ProjectEvent E1
turn_id = T100
operation = create
target_id = Task A

ProjectEvent E2
turn_id = T100
operation = create
target_id = Task B

ProjectEvent E3
turn_id = T100
operation = create
target_id = Task C
```

最终得到：

```text
普通项目会话 S10
  └── Turn T100
        ├── created Task A
        ├── created Task B
        └── created Task C
```

三张 Task Card 在各自详情页都可以反查到 `T100`，项目活动流也可以把三次创建合并展示为“一轮 PM 操作”。

### 3.3 不应做什么

不应把 Session 自动绑定到三张 Task Card：

- 当前 `sessions.task_id` 是单值字段，不能表达三张卡；
- 即使扩展成数组，也会混淆“会话上下文”和“本轮影响对象”；
- Session 可能在下一轮继续创建第四张 Task；
- 一个项目级 PM Session 不属于任何单一 Task；
- Task executor 的 `task_id` 还承担权限收窄语义，不能与普通 PM 关联混用。

也不应任选其中一张作为 Session 的 `task_id`。这会让另外两张 Task 丢失来源，并错误暗示整个会话从属于第一张 Task。

### 3.4 跟踪入口

同一事实从三个入口展示：

1. **ChatUI Turn**
   - 最终回答；
   - “本轮创建 3 个 Tasks”回执；
   - 三张卡的跳转链接。

2. **Project Activity**
   - 以 Turn 为单位展示一次批量操作；
   - 可展开查看三个 ProjectEvent。

3. **Task Detail**
   - 每张 Task 只投影与自身相关的 Event；
   - 显示“由 Turn T100 创建”；
   - 可以跳回原始 Session 的 T100。

---

## 4. 是否需要 CLI 的 Task Card 绑定功能

### 4.1 当前建议

> **需要考虑显式绑定能力，但它不是追踪新建 Tasks 的前置条件，也不应放在第一阶段。**

正常链路应该是自动归因：

```text
CLI/MCP create/update
  → 请求携带可信 Session 上下文
  → 后端解析当前 running Turn
  → 写 ProjectItem
  → 同事务写 ProjectEvent
```

Agent 不需要在每次创建后再执行一次：

```text
bind task to current turn
```

否则容易出现：

- Task 创建成功但 bind 失败；
- Agent 忘记 bind；
- 同一个 Task 被重复 bind；
- 最终回答与真实绑定不一致；
- CLI 和 MCP 行为分叉。

### 4.2 必须区分三种“绑定”

“绑定 Task Card”可能实际指三种不同关系，不能用一个模糊的 `bind` 命令承载。

#### A. 操作归因：这个 Turn 创建或修改了哪些 Items

这是 `ProjectEvent.turn_id`，应自动完成。

不需要用户或 Agent 手动执行 CLI bind。

#### B. Session 主任务：这个 Session 是否是某个 Task 的执行会话

这是现有 `sessions.task_id`。

它适合：

- 从 Task Detail 启动 Session；
- Scheduler 为 Task 创建 executor Session；
- verifier Session；
- 权限需要收窄到单一 Task 的场景。

它不适合项目级 PM Session，也不适合表达一次 Turn 影响了多张卡。

#### C. 人工补充关系：这个 Turn 讨论了某个既有 Item，但没有修改它

这类关系可以作为后续的显式关联：

```text
relation = referenced
```

例如用户在对话中讨论了 Task #12，但本轮没有调用 update。系统无法仅凭写事件得知 Task #12 与本轮有关，此时才可能需要人工或模型显式关联。

### 4.3 推荐的 CLI 方向

第一阶段不增加 `task card bind`。

先让现有写命令自动产生 Turn 归因：

```bash
1agents project-items create ...
1agents project-items update '#12' ...
1agents project-items close '#12'
```

这些命令的响应增加：

```json
{
  "item": {"id": "...", "number": 301},
  "origin": {
    "sessionId": "S10",
    "turnId": "T100",
    "eventId": "E1"
  }
}
```

后续如果确认有“讨论但未修改”的强需求，可以增加明确命名的命令，而不是宽泛的 `bind`：

```bash
# 候选，非 MVP
1agents turns attach-item <turn-id> <item-id> --relation referenced
1agents turns detach-item <turn-id> <item-id>
```

在 1agents 宿主会话内，还可以提供当前 Turn 的便捷形式：

```bash
# 候选，非 MVP
1agents turns attach-item current '#12' --relation referenced
```

这类命令必须：

- 只允许同一 Project；
- 留下独立审计事件；
- 不改变 `sessions.task_id`；
- 不改变 executor/verifier 的任务权限；
- 支持撤销人工关联；
- 不允许把任意外部 Turn ID 伪装成当前 Agent Turn。

### 4.4 如果用户确实想把整个 Session 转成某个 Task 的执行会话

这是另一项能力，命令应使用 Session 语义，例如：

```bash
# 仅为候选命名
1agents sessions attach-task <session-id> <item-id>
1agents sessions detach-task <session-id>
```

它与 `turns attach-item` 不同：

- `sessions attach-task` 改变后续会话上下文和可能的权限边界；
- `turns attach-item` 只补充历史/审计关系；
- 自动 ProjectEvent 只是记录本轮实际写操作。

是否允许把一个已经运行中的 project-wide Session 转为 task-scoped Session，需要单独讨论，不能作为 Turn MVP 的隐式行为。

---

## 5. 数据模型

### 5.1 `agent_turns`

建议在 `meta.db` 增加：

```sql
CREATE TABLE agent_turns (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL,
    session_id          TEXT NOT NULL,
    client_request_id   TEXT NOT NULL DEFAULT '',
    initiating_reply_id TEXT NOT NULL DEFAULT '',
    agent_type          TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL CHECK (
                            status IN ('queued','running','completed','failed','cancelled')
                        ),
    prompt_text         TEXT NOT NULL DEFAULT '',
    final_answer        TEXT NOT NULL DEFAULT '',
    error_code          TEXT NOT NULL DEFAULT '',
    error_text          TEXT NOT NULL DEFAULT '',
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX idx_agent_turns_session
    ON agent_turns(session_id, created_at, id);

CREATE INDEX idx_agent_turns_project
    ON agent_turns(project_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX idx_agent_turns_one_running
    ON agent_turns(session_id)
    WHERE status = 'running';

CREATE UNIQUE INDEX idx_agent_turns_client_request
    ON agent_turns(session_id, client_request_id)
    WHERE client_request_id != '';
```

MVP 状态：

```text
queued | running | completed | failed | cancelled
```

后续候选：

```text
waiting_approval | partial_failure
```

`client_request_id` 接收前端/Bridge 的 `requestId`。旧客户端不提供时由后端生成；同一 Session
重复提交同一个非空 ID 时返回已有 Turn，不能再创建第二轮。

`prompt_text` 只保存用户实际提交的可见文本，不保存合并后的 role/system context、附件二进制、
私有思维链或工具原始日志。`final_answer` 只保存最终用户可见回答；过程消息继续由 Session
transcript 承载。`error_text` 只能写可展示的脱敏错误，内部堆栈留在服务日志。

### 5.2 `project_events`

```sql
CREATE TABLE project_events (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    turn_id        TEXT,
    session_id     TEXT NOT NULL DEFAULT '',
    task_run_id    TEXT NOT NULL DEFAULT '',
    actor_kind     TEXT NOT NULL,
    actor_name     TEXT NOT NULL DEFAULT '',
    origin         TEXT NOT NULL,
    event_type     TEXT NOT NULL,
    target_type    TEXT NOT NULL,
    target_id      TEXT NOT NULL,
    operation      TEXT NOT NULL,
    before_json    TEXT NOT NULL DEFAULT '{}',
    after_json     TEXT NOT NULL DEFAULT '{}',
    status         TEXT NOT NULL CHECK (
                       status IN ('succeeded','rejected','failed')
                   ),
    error_code     TEXT NOT NULL DEFAULT '',
    error_text     TEXT NOT NULL DEFAULT '',
    sequence       INTEGER NOT NULL,
    created_at     TEXT NOT NULL
);

CREATE INDEX idx_project_events_project
    ON project_events(project_id, created_at DESC, id DESC);

CREATE INDEX idx_project_events_turn
    ON project_events(turn_id, sequence)
    WHERE turn_id IS NOT NULL;

CREATE INDEX idx_project_events_session
    ON project_events(session_id, created_at DESC, id DESC)
    WHERE session_id != '';

CREATE INDEX idx_project_events_correlation
    ON project_events(correlation_id, sequence)
    WHERE correlation_id != '';

CREATE INDEX idx_project_events_target
    ON project_events(project_id, target_type, target_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX idx_project_events_turn_sequence
    ON project_events(turn_id, sequence)
    WHERE turn_id IS NOT NULL;

CREATE UNIQUE INDEX idx_project_events_correlation_sequence
    ON project_events(correlation_id, sequence)
    WHERE turn_id IS NULL AND correlation_id != '';
```

Event 是只追加事实。Store 和 HTTP API 不提供 UPDATE/DELETE；任何更正都写一个新的反向或修正
Event。成功变更必须与目标写入同事务提交。业务规则拒绝写 `status=rejected` 且目标不变；内部失败
可以在身份和 Project 已验证后尽力写 `status=failed`，但数据库本身不可用时不承诺审计落盘。

`before_json/after_json` 是按目标类型构造的字段白名单，不是数据库整行 dump：

| 目标 | 允许保留值 | 必须排除或只记 `changed=true` |
|------|------------|------------------------------|
| `project_item` | id、number、title、type、issueState、status、priority、assignee、executor、milestone、labels、dependsOn、计划时间 | description、acceptanceCriteria、taskTarget、result、replies、凭据 |
| `milestone` | id、name、targetDate、predecessorId、position | description 正文 |
| `dependency` | taskId、dependsOn、relation | 无额外自由文本 |
| `session` | id、projectId、taskId、role、agentType、archivedAt | sessionKey、permission token、transcript |
| `turn` | id、sessionId、status、时间、errorCode | promptText、finalAnswer、errorText |
| `task_run` | id、taskId、originTurnId、status、attempt、时间 | 原始日志、环境变量、凭据 |
| `verification` | id、taskId、taskRunId、verdict、closedBy、证据引用 | 证据正文和可能含密钥的命令输出 |

### 5.3 Event 注册表

`event_type` 固定为 `<target_type>.<operation>`。MVP 只接受下表组合；增加组合必须先更新
注册表和兼容性测试，不能把任意字符串直接写入数据库。

| `target_type` | 允许的 `operation` | 对应 `event_type` |
|---------------|--------------------|-------------------|
| `project_item` | `create`、`update`、`close`、`reopen`、`complete`、`cancel`、`delete` | `project_item.<operation>` |
| `milestone` | `create`、`update`、`delete` | `milestone.<operation>` |
| `dependency` | `link`、`unlink` | `dependency.<operation>` |
| `session` | `create`、`update`、`archive`、`reopen` | `session.<operation>` |
| `turn` | `queue`、`start`、`complete`、`fail`、`cancel` | `turn.<operation>` |
| `task_run` | `create`、`start`、`complete`、`fail`、`cancel` | `task_run.<operation>` |
| `verification` | `create`、`complete`、`fail` | `verification.<operation>` |

`operation=complete/cancel` 用于 Task status 的终态；`close/reopen` 只用于 requirement/bug
的 `issueState`。这两个生命周期不能混用。

### 5.4 `replies.turn_id`

给现有 `replies` 增加：

```sql
ALTER TABLE replies ADD COLUMN turn_id TEXT;
CREATE INDEX idx_replies_turn ON replies(turn_id, created_at);
```

同一轮用户 Reply 和最终 Agent Reply 使用同一个 `turn_id`。

旧数据允许 `turn_id IS NULL`，ChatUI 使用当前的消息边界算法兼容。

### 5.5 `task_runs.origin_turn_id`

当 `task_runs` 事件源落地时，建议增加：

```text
origin_turn_id nullable
```

关系示例：

```text
一个 PM Turn
  ├── 创建 3 个 Tasks
  └── 发起 3 个 TaskRuns
```

Turn 不能替代 TaskRun；Turn 表达用户意图和编排，TaskRun 表达单个 Task 的一次执行尝试。

---

## 6. Turn 生命周期

### 6.1 状态机

```text
queued ──► running ──► completed
   │           ├─────► failed
   │           └─────► cancelled
   └─────────────────► cancelled
```

只允许以下迁移：

| 当前状态 | 允许下一状态 | 触发 |
|----------|--------------|------|
| 不存在 | `queued` | 后端接受 prompt 并持久化 Turn |
| `queued` | `running` | 该 Session 无 running Turn，且队首 prompt 开始发送给 ACP |
| `queued` | `cancelled` | 用户取消排队 prompt，或服务重启取消未确认派发的队列 |
| `running` | `completed` | 收到对应 Turn 的自然 `done` |
| `running` | `failed` | Agent/Bridge error、ACP runtime 丢失或后端启动恢复 |
| `running` | `cancelled` | 用户停止当前轮；随后到达的旧 `done/error` 只记诊断日志 |

终态不可再迁移。`completed` 只表示 AgentTurn 正常给出最终回答，不表示其中每次项目写入都成功，
也不表示任何 Task 已通过完成门。Activity 根据同一 Turn 的 Event 计算
`succeeded | partial | failed | cancelled` 展示状态。

不变量：

1. 一个 Session 同一时间最多只有一个 `running` Turn；
2. 一个 Session 可以有多个 `queued` Turn；
3. 队列按 `(created_at, id)` FIFO；只有队首能迁移为 `running`；
4. 工具调用只归因到当前 `running` Turn；
5. `done/error/cancel` 必须按 `turn_id` 或 `client_request_id` 终结对应 Turn，不能只取“当前最后一轮”；
6. Session 断开不等于 Turn 自动完成；
7. reconnect 到仍存活的 Bridge 时恢复同一 Turn，不创建新 ID；
8. Turn 完成后不再接受新的 ProjectEvent；
9. 同一 `client_request_id` 的重复 prompt 返回原 Turn，不能重复执行；
10. Turn 状态变更和对应 `turn.*` Event 在同一事务中写入。

### 6.2 Bridge 上下文

当前 `turnText` 应升级为显式上下文：

```text
TurnContext
- turnID
- clientRequestID
- promptReplyID
- status
- finalText
- startedAt
```

Bridge 至少维护：

```text
activeTurn
pendingTurns[]
```

协议统一沿用现有 `requestId` 字段作为 `clientRequestID`。后端校验为非空、长度不超过 128
字节的 UTF-8 字符串并持久化；旧客户端没有该字段时，后端生成 ID 并在首个 `turn_queued`
或 `turn_started` 事件中返回。

### 6.3 终止条件

| 事件 | Turn 结果 |
|------|-----------|
| `done` | 保存最终回答，`completed` |
| Agent error | 保存错误，`failed` |
| 用户取消当前轮 | `cancelled` |
| 取消排队 prompt | 对应 queued Turn `cancelled` |
| WS 临时断开、backend 与 runtime 仍运行 | 保持 `running`，重连恢复同一 Turn |
| Bridge server/ACP runtime 连接丢失 | running→`failed(error_code=runtime_lost)`；queued→`cancelled` |
| 后端启动发现遗留 running | →`failed(error_code=backend_restarted)` |
| 后端启动发现遗留 queued | →`cancelled(error_code=backend_restarted)` |

后端重启后不自动重放 prompt。ACP 工具可能产生非幂等外部副作用，系统无法证明重启前是否已发送或
执行，因此自动重放会制造重复写入。用户重连后可以在原 ACP Session 历史上发起一个新的 Turn。

---

## 7. CLI/MCP 的可信 Turn 归因

### 7.1 为什么不能依赖 `ONEAGENTS_TURN_ID`

ProjectItems MCP 是 Session 级长驻进程，一个进程会服务多个 Turn。

在进程启动时注入固定 `ONEAGENTS_TURN_ID`，下一轮就会过期。动态修改父进程环境变量也不会更新已经运行的 MCP 子进程。

### 7.2 推荐方案：静态 Session 身份，后端解析动态 Turn

给宿主内的 MCP 和 Agent shell 注入稳定信息：

```text
ONEAGENTS_SESSION_ID
ONEAGENTS_WORKSPACE_ID
ONEAGENTS_BASE_URL
ONEAGENTS_INTERNAL_TOKEN
```

CLI/MCP 内部请求携带可信 Session ID：

```text
X-1Agents-Session-ID: S10
X-1Agents-Origin: cli | mcp
```

后端处理写操作时：

1. 只在 loopback 请求上验证 `Authorization: Bearer <ONEAGENTS_INTERNAL_TOKEN>`；
2. 读取 `X-1Agents-Session-ID`，并要求它与 token 所属进程注入的 Session 一致；
3. 验证 Session 存在、未跨 Project，且请求 workspace 与 Session.project_id 一致；
4. 查询该 Session 当前唯一的 `running` Turn；
5. 若没有 running Turn，返回 HTTP 409、`error.code=no_active_turn`，不得降级为 `turn_id=null`；
6. 若出现多个 running Turn，返回 HTTP 500、`error.code=turn_invariant_violated`，不得猜测；
7. 从 Session 记录推导 actor/agentType，从请求面推导 `origin=cli|mcp`；
8. 生成 `MutationContext`，在同一事务中写 Project 数据和 ProjectEvent。

不允许任何客户端直接声明 `turn_id`。非 loopback 请求即使携带上述 Header 也按普通外部请求
处理；无有效内部 bearer 时不得信任 Session、origin 或 actor Header。

### 7.3 CLI 在宿主外运行

用户在普通终端直接执行：

```bash
1agents project-items create ...
```

如果没有 `ONEAGENTS_SESSION_ID`：

- 操作仍然正常；
- `turn_id = null`；
- `actor_kind = user` 或 `cli`；
- 仍写 ProjectEvent；
- 不伪造一个 Agent Turn。

这样保持 [pm-standalone](../pm-standalone/prd.md) 的“CLI 自运行、无完整工作台也能记任务”定位。

宿主外 CLI 的 `actor_kind=user`，`origin=cli`。人工 UI 写入使用后端认证身份和
`origin=ui|api`；Scheduler 使用 `actor_kind=scheduler`。这些值都由可信服务端路径设置，
不接受调用方自由填写。

---

## 8. 写入一致性

ProjectItem 变更和 ProjectEvent 必须在同一 SQLite 事务中提交。

推荐底层写入接口接受：

```text
MutationContext
- projectID
- actorKind
- actorName
- sessionID
- turnID
- taskRunID
- correlationID
- origin
```

写入原则：

```text
BEGIN
  读取 before snapshot
  校验业务规则和权限
  修改 ProjectItem
  写 after snapshot
  INSERT ProjectEvent
COMMIT
```

失败时整个事务回滚。

ProjectEvent 应由底层 Store 生成，而不是由 Agent 最终回答、MCP 外层或前端推测。

单个命令只有两种原子结果：

- 业务写入与 `status=succeeded` Event 一起提交；
- 业务写入不发生，身份和 Project 已验证后单独追加 `status=rejected|failed` Event。

一轮五个命令中四个成功、一个被完成门拒绝时，Turn 仍可在最终回答后成为 `completed`；
Activity 状态为 `partial`，并展示 4 个 succeeded 与 1 个 rejected Event。失败 Event
不能包含无权访问对象的 before/after，也不能泄露目标是否存在。

---

## 9. API 草案

### 9.1 Turn 查询

```text
GET /api/agent/turns?session_id=<id>&cursor=<cursor>
GET /api/agent/turns/<turn-id>
GET /api/agent/projects/<project-id>/activity?cursor=<cursor>&source=<source>&target=<target>&status=<status>
GET /api/agent/project-items/<item-id>/activity?cursor=<cursor>&source=<source>&status=<status>
```

项目和任务历史应分页，不把全部 Turn 塞进现有 ProjectItem 列表响应。

公共分页契约：

- `limit` 默认 50，最小 1，最大 200；
- 排序为 `(occurredAt DESC, lastEventId DESC)`；
- `cursor` 是上述二元组的 base64url 不透明编码，调用方不得解析或构造；
- 下一页严格查询 `< (occurredAt, lastEventId)`；
- 返回 `{items, nextCursor, hasMore}`；
- 非法 cursor 返回 HTTP 400、`error.code=invalid_cursor`；
- Activity 的 `occurredAt/lastEventId` 取聚合项最后一个 Event，实时插入不会让已翻过的项重复出现。

### 9.2 Turn 响应

```json
{
  "id": "T100",
  "projectId": "P1",
  "sessionId": "S10",
  "clientRequestId": "request-100",
  "status": "completed",
  "promptText": "把 Turn 能力拆成三个 Tasks",
  "finalAnswer": "已创建三个任务并建立依赖。",
  "startedAt": "2026-07-27T10:00:00Z",
  "completedAt": "2026-07-27T10:02:00Z",
  "operations": [
    {
      "id": "E1",
      "targetType": "project_item",
      "targetId": "A",
      "operation": "create",
      "status": "succeeded"
    }
  ]
}
```

Turn Detail 仅在调用方拥有 Project 访问权时返回 `promptText/finalAnswer/errorText`。Activity 和
Task Detail 默认只返回摘要及 Event 白名单快照，不把 Turn 正文复制进列表响应。

### 9.3 写操作响应

ProjectItems CLI/MCP/REST 的成功响应增加来源回执：

```json
{
  "item": {},
  "origin": {
    "sessionId": "S10",
    "turnId": "T100",
    "eventId": "E1"
  }
}
```

失败响应保持同样的来源形状，并增加机器可读错误：

```json
{
  "error": {
    "code": "completion_evidence_required",
    "message": "Task completion requires verification evidence."
  },
  "origin": {
    "sessionId": "S10",
    "turnId": "T100",
    "eventId": "E5"
  }
}
```

没有宿主 Turn 时：

```json
{
  "item": {},
  "origin": {
    "sessionId": null,
    "turnId": null,
    "eventId": "E1"
  }
}
```

---

## 10. ChatUI 展示

### 10.1 分组规则

1. 消息有 `turnId`：严格按 ID 分组；
2. 没有 `turnId`：继续使用当前用户消息边界推断；
3. 当前 `running` Turn 展开并 sticky；
4. 历史 `completed` Turn 折叠，只保留最终回答；
5. `failed/cancelled` Turn 显示状态和可用错误摘要；
6. 展开后显示用户可见过程，不展示或持久化私有思维链。

### 10.2 本轮操作回执

最终回答后增加系统事实回执：

```text
本轮操作

创建 3 个 Tasks
更新 1 个 Task
新增 2 条依赖

[查看全部变更]
```

回执来自 ProjectEvent，不从最终自然语言回答中解析。

---

## 11. PM 看板与 Task Detail

### 11.1 看板

Kanban/Grid 卡片保持紧凑，不直接铺完整 Turn。

MVP 可以显示：

```text
最近由 PM Turn 更新 · 3 分钟前
```

项目级增加 `Project Activity`：

```text
15:42 · Agent Turn · Codex
“把登录改造拆成前后端任务并排期”

创建 #301、#302
更新 #288

[最终回答] [本轮过程]
```

### 11.2 Task Detail

当前 Task Detail 以 Session branch 组织 Replies。目标结构：

```text
Session
  ├── Turn 1
  ├── Turn 2
  └── Turn 3
```

对 project-wide Session 创建的 Task，不需要把 Session 变成该 Task 的子会话。Task Detail 通过事件反查 Turn，显示一个外部来源节点：

```text
Agent Turn · 已完成

来源
项目会话：PM 规划讨论

对当前 Task 的影响
- 创建当前 Task
- priority = high
- milestone = Turn Model

最终回答
已创建三个任务并建立依赖……

[查看本轮过程] [打开原始会话]
```

只展示与当前 Task 有关的 Event，不复制整轮的全部工具日志。

---

## 12. 与完成审计和 TaskRun 的关系

Turn 最终回答不能作为 completed 的验收证据。

```text
AgentTurn
  = 谁在什么上下文提出并实施了哪些项目变更

TaskRun
  = 某个 Task 的一次执行/验收/返工尝试

Completion Audit
  = 为什么系统允许把 Task 判定为 completed
```

如果一个 Turn 请求把 Task 改成 `completed`：

- 底层仍需执行完成门校验；
- 缺少证据时更新失败；
- ProjectEvent 记录失败；
- Turn 回执显示“未完成：缺少验收证据”；
- Agent 的自然语言不能绕过系统规则。

---

## 13. 分阶段实施

### Phase 0：契约与 ADR

- 锁定术语和边界；
- 明确 queued/running/cancelled；
- 明确 Session ID 如何传给 CLI/MCP；
- 明确不保存私有思维链；
- 冻结后端重启、隐私、Event 注册表和测试矩阵。

### Phase 1：持久化

- 新增 `agent_turns`；
- 新增 `project_events`；
- 新增 `replies.turn_id`；
- Store、migration 和查询测试。

### Phase 2：Bridge 生命周期

- prompt 创建 Turn；
- 管理 active/pending Turns；
- done/error/cancel 收尾；
- 用户 Reply 和 Agent Reply 关联 Turn；
- reconnect 恢复。

### Phase 3：CLI/MCP 自动归因

- 注入稳定 Session ID；
- 内部请求携带 Session 上下文；
- 后端解析 active Turn；
- 项目变更与 Event 原子提交；
- CLI 输出 origin 回执。

### Phase 4：Activity 读模型与 API

- Turn/correlation/event 聚合；
- Project/ProjectItem Activity 查询；
- cursor、筛选、摘要和统计。

### Phase 5：前端

- ChatUI 显式 ID + 旧数据回退；
- Task Detail Turn 投影；
- Project Activity；
- 本轮项目操作回执。

### Phase 6：TaskRun 和完成审计

- `task_runs.origin_turn_id`；
- 完成裁定引用证据；
- Turn 回执区分“Agent 声称完成”和“系统验收完成”。

### Phase 7：后续候选

不属于 #281 的 MVP：

- `turns attach-item`；
- `turns detach-item`；
- Session attach/detach Task；
- 整轮 Undo；
- waiting approval。

---

## 14. 验收场景

### 14.1 基础 Turn

- 同一 Session 连续三次提问产生三个不同 Turn；
- 刷新和重连后 Turn 边界不变；
- 历史 Turn 折叠只显示最终回答；
- 旧 Session 没有 `turn_id` 时仍能正常展示。

### 14.2 非 Task Session 创建多个 Tasks

- project-wide Session 的 `task_id` 保持为空；
- 一轮创建三个 Tasks，产生一个 Turn 和三个 create Events；
- 三张 Task 都能反查同一个 Turn；
- Project Activity 只显示一个批次；
- 每张 Task Detail 只展示与自身有关的 Event；
- 不需要额外执行 bind 命令。

### 14.3 队列与失败

- 当前 Turn 运行时可以排队下一轮；
- 工具调用不会归到排队 Turn；
- 取消排队消息只取消对应 Turn；
- 工具部分成功时，成功 Event 与失败结果都可回看；
- WS 断开后不会错误标记 completed。

### 14.4 CLI/MCP

- 宿主内 CLI/MCP 写操作自动关联当前 Turn；
- 宿主外 CLI 仍可正常工作，`turn_id` 为空；
- 外部请求不能伪造其他 Session 的 Turn；
- 跨 Project 的 Turn/Item 关联被拒绝。

### 14.5 完成审计

- Turn 回答“已完成”不会直接满足完成门；
- completed Task 能追溯到 TaskRun、验收证据和 ClosedBy；
- Turn 可以继续作为发起来源被反查。

---

## 15. 已冻结 ADR 决策

### Q1：是否需要 `turns attach-item`

**决策：非 MVP。**先依赖写操作自动产生 Event；“讨论但未修改”的关系不进入第一版
Project Activity 或 Task Detail。

### Q2：是否允许把 project-wide Session 转成 task-scoped Session

**决策：MVP 禁止自动转换。**这会改变上下文、UI 徽章和 MCP 权限边界，需要独立需求。
不能因为某一轮创建了 Task 就自动转换。

### Q3：Turn 的 prompt 是否保存全文

**决策：保存用户可见 prompt 原文。**保存的是客户端提交的 `text`，不是合并后的系统提示。
不复制附件二进制、role/system context、私有思维链或工具原始日志。Task-scoped Session
可以同时写 `replies.turn_id`；project-wide Session 不依赖 Task Reply 才能恢复 prompt。

### Q4：本轮过程保存到哪里

**决策：**

- `agent_turns` 保存生命周期和最终回答；
- `project_events` 保存领域变更；
- 完整可见过程继续由 Session transcript 承载；
- 不把工具日志复制到每张 Task。

Session transcript 的长期清理策略不属于本 Epic；Turn/Activity 在 transcript 不可用时仍须依靠
prompt、final answer 和 Events 完成审计。

### Q5：后端重启后的 active Turn

**决策：不自动重放。**仅当前 backend 和 Bridge/runtime 都存活的临时 WS 断线可保持
running。runtime 丢失或 backend 重启时 running→failed、queued→cancelled，并记录稳定
`error_code`。用户重连后显式创建新 Turn。

### Q6：是否支持整轮 Undo

**决策：非 MVP。**Undo 必须包含：

- Item version/updated_at 冲突检测；
- 逆向操作；
- 创建项被后续引用时的保护；
- 部分撤销；
- 撤销本身的审计 Turn/Event。

不能简单用 `before_json` 覆盖当前状态。

---

## 16. 当前推荐决策摘要

| 决策 | 推荐 |
|------|------|
| 底层名称 | `AgentTurn` |
| 普通 Session 是否必须绑定 Task | 否 |
| 非 Task Session 创建 3 个 Tasks 如何追踪 | 一个 Turn + 三个 ProjectEvents |
| 是否自动把 Session 绑定到其中一张 Task | 否 |
| 是否要求 Agent 额外调用 CLI bind | 否 |
| CLI/MCP 如何归因 | 静态 Session ID，后端解析当前 Turn |
| 是否需要显式 attach-item | 后续可选，仅用于 referenced/人工修复 |
| 是否保存私有思维链 | 否 |
| prompt 如何保存 | 保存用户可见原文；排除系统提示、附件二进制和私有思维链 |
| runtime 丢失/后端重启 | 不重放；running→failed，queued→cancelled |
| 宿主内没有 running Turn | 409 拒绝写入，不降级为无归因事件 |
| 部分成功如何表达 | Turn 可 completed；Event 混合结果使 Activity=`partial` |
| Turn 是否替代 TaskRun | 否 |
| Turn 最终回答是否是完成证据 | 否 |
| 旧 Chat 历史如何兼容 | 无 ID 时继续按用户消息边界推断 |

---

## 17. Phase 0 冻结测试计划

后续任务必须按下表落测试；只有覆盖对应行，不能用单一 happy-path 测试宣称整阶段完成。

| 层 | 目标文件 | 必测场景 | 最少用例 |
|----|----------|----------|----------|
| Migration/Store | `backend/internal/meta/turns_test.go` | 新库建表、v25 升级、旧 Reply 兼容、单 Session 唯一 running、client request 幂等、终态拒绝迁移 | 8 |
| Migration/Store | `backend/internal/meta/project_events_test.go` | 成功原子写、回滚无半条数据、Event 不可修改、turn/correlation sequence、target/project cursor | 8 |
| Bridge | `backend/internal/agent/acpx_turn_test.go` | 三次 prompt 三个 ID、FIFO、done/error/当前取消/排队取消、临时断线、runtime 丢失、启动恢复 | 10 |
| CLI/MCP | `backend/internal/projectitems/attribution_test.go` | session header、无 active Turn 409、宿主外 null Turn、伪造 header、跨 Project、origin 回执、部分失败 | 8 |
| Activity/API | `backend/internal/agent/activity_test.go` | Turn 聚合、correlation 聚合、单 Event、target 过滤、source/status 过滤、cursor 实时插入、权限 | 8 |
| ChatUI | `frontend/src/components/chat/turns.test.ts` | 显式 turnId、legacy fallback、running 展开、历史折叠、failed/cancelled、事实回执 | 6 |
| PM UI | `frontend/src/components/drawer/TaskList/activity.test.ts` | Project Activity 聚合展示、Task Detail 只投影当前 Item、project-wide Session 跳转、分页去重 | 5 |
| 完成审计 | `backend/internal/agent/turn_audit_test.go` | 最终回答不能完成 Task、originTurnId、Evidence/Verdict/ClosedBy、关键执行事件进入 Activity | 5 |
| E2E | 后端集成测试 + 前端构建 | 普通 project-wide Session 一轮创建 3 Tasks，三处追溯一致；重启和权限回归 | 3 |

阶段门：

1. #283 必须先通过两组 Migration/Store 测试；
2. #284 必须通过 Bridge 状态机与恢复测试；
3. #285 必须通过 CLI/MCP 安全、原子性和来源回执测试；
4. #286 必须通过聚合、筛选和 cursor 测试；
5. #287 必须通过前端单测及 `frontend/yarn build`；
6. #288 必须通过完成审计、E2E、全部相关 Go 测试和 `git diff --check`。
