# Agent Turn：从 ChatUI 折叠单元到 PM 可审计因果批次

| 字段 | 内容 |
|------|------|
| 状态 | **设计草案 · 待讨论** |
| 版本 | **v0.1** |
| 日期 | 2026-07-27 |
| 范围 | ChatUI、Agent Bridge、ProjectItems CLI/MCP、PM 看板、Task Detail、meta.db |
| 产品 PRD | [prd.md](./prd.md)（产品决策与 MVP 范围以 PRD 为准） |
| 关联 | [issue-model](../issue-model/design.md)、[project-model](../project-model/design.md)、[pm-standalone](../pm-standalone/prd.md)、[verification-gate](../verification-gate/design.md) |

---

> **2026-07-27 决策补充：**项目动态需要覆盖 ProjectItem、Milestone、Dependency、Session、TaskRun 与 Verification，因此目标事件源由本文初稿中的 `project_item_events` 收敛为通用 `project_events`。Project Activity 不单独存储，而是按 `turn_id / correlation_id` 聚合 Events 的只读投影。详细产品契约见 [PRD](./prd.md)。

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
  ├── ProjectItemEvent → 创建 A
  ├── ProjectItemEvent → 创建 B
  ├── ProjectItemEvent → 更新 C
  └── 可选：触发一个或多个 TaskRun
```

关系基数：

```text
Project       1 ── N ChatSession
ChatSession   1 ── N AgentTurn
AgentTurn     N ── M ProjectItem（通过 ProjectItemEvent）
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
| ProjectItemEvent | 项目事实具体发生了什么变化？ | 一次不可变变更事件 |
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
ProjectItemEvent E1
turn_id = T100
operation = create
target_id = Task A

ProjectItemEvent E2
turn_id = T100
operation = create
target_id = Task B

ProjectItemEvent E3
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
   - 可展开查看三个 ProjectItemEvent。

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
  → 同事务写 ProjectItemEvent
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

这是 `ProjectItemEvent.turn_id`，应自动完成。

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
- 自动 ProjectItemEvent 只是记录本轮实际写操作。

是否允许把一个已经运行中的 project-wide Session 转为 task-scoped Session，需要单独讨论，不能作为 Turn MVP 的隐式行为。

---

## 5. 数据模型

### 5.1 `agent_turns`

建议在 `meta.db` 增加：

```sql
CREATE TABLE agent_turns (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL REFERENCES projects(id),
    session_id          TEXT NOT NULL REFERENCES sessions(id),
    initiating_reply_id TEXT,
    agent_type          TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL,
    prompt_text         TEXT NOT NULL DEFAULT '',
    final_answer        TEXT NOT NULL DEFAULT '',
    error_text          TEXT NOT NULL DEFAULT '',
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX idx_agent_turns_session
    ON agent_turns(session_id, created_at);

CREATE INDEX idx_agent_turns_project
    ON agent_turns(project_id, created_at DESC);
```

MVP 状态：

```text
queued | running | completed | failed | cancelled
```

后续候选：

```text
waiting_approval | partial_failure
```

### 5.2 `project_item_events`

```sql
CREATE TABLE project_item_events (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    turn_id     TEXT REFERENCES agent_turns(id),
    task_run_id TEXT,
    target_type TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    operation   TEXT NOT NULL,
    before_json TEXT,
    after_json  TEXT,
    actor_kind  TEXT NOT NULL,
    actor_name  TEXT NOT NULL DEFAULT '',
    sequence    INTEGER NOT NULL,
    status      TEXT NOT NULL,
    error_text  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);

CREATE INDEX idx_project_item_events_turn
    ON project_item_events(turn_id, sequence);

CREATE INDEX idx_project_item_events_target
    ON project_item_events(project_id, target_type, target_id, created_at DESC);
```

MVP `target_type`：

```text
project_item | milestone | dependency
```

MVP `operation`：

```text
create | update | close | reopen | delete | link | unlink
```

### 5.3 `replies.turn_id`

给现有 `replies` 增加：

```sql
ALTER TABLE replies ADD COLUMN turn_id TEXT;
CREATE INDEX idx_replies_turn ON replies(turn_id, created_at);
```

同一轮用户 Reply 和最终 Agent Reply 使用同一个 `turn_id`。

旧数据允许 `turn_id IS NULL`，ChatUI 使用当前的消息边界算法兼容。

### 5.4 `task_runs.origin_turn_id`

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

不变量：

1. 一个 Session 同一时间最多只有一个 `running` Turn；
2. 一个 Session 可以有多个 `queued` Turn；
3. 工具调用只归因到当前 `running` Turn；
4. `done/error/cancel` 必须终结当前 Turn；
5. Session 断开不等于 Turn 自动完成；
6. reconnect 后必须能恢复当前 Turn；
7. Turn 完成后不再接受新的 ProjectItemEvent。

### 6.2 Bridge 上下文

当前 `turnText` 应升级为显式上下文：

```text
TurnContext
- turnID
- requestID / clientTurnID
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

如果前端协议可以携带 `clientTurnId`，后端应校验并持久化；如果旧客户端没有，则后端生成 ID 并通过事件返回。

### 6.3 终止条件

| 事件 | Turn 结果 |
|------|-----------|
| `done` | 保存最终回答，`completed` |
| Agent error | 保存错误，`failed` |
| 用户取消当前轮 | `cancelled` |
| 取消排队 prompt | 对应 queued Turn `cancelled` |
| WS 临时断开、runtime 仍运行 | 保持 `running` |
| 后端重启后发现失联运行轮 | 恢复或经超时修复为 `failed`，策略待定 |

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

1. 验证 internal token；
2. 验证 Session 属于当前 Project；
3. 查询该 Session 当前唯一的 `running` Turn；
4. 生成 `MutationContext`；
5. 在同一事务中写 ProjectItem 和 ProjectItemEvent。

不允许普通外部请求通过任意 Header 声明 `turn_id`。

### 7.3 CLI 在宿主外运行

用户在普通终端直接执行：

```bash
1agents project-items create ...
```

如果没有 `ONEAGENTS_SESSION_ID`：

- 操作仍然正常；
- `turn_id = null`；
- `actor_kind = user` 或 `cli`；
- 仍写 ProjectItemEvent；
- 不伪造一个 Agent Turn。

这样保持 [pm-standalone](../pm-standalone/prd.md) 的“CLI 自运行、无完整工作台也能记任务”定位。

---

## 8. 写入一致性

ProjectItem 变更和 ProjectItemEvent 必须在同一 SQLite 事务中提交。

推荐底层写入接口接受：

```text
MutationContext
- projectID
- actorKind
- actorName
- sessionID
- turnID
- taskRunID
- origin
```

写入原则：

```text
BEGIN
  读取 before snapshot
  校验业务规则和权限
  修改 ProjectItem
  写 after snapshot
  INSERT ProjectItemEvent
COMMIT
```

失败时整个事务回滚。

ProjectItemEvent 应由底层 Store 生成，而不是由 Agent 最终回答、MCP 外层或前端推测。

---

## 9. API 草案

### 9.1 Turn 查询

```text
GET /api/agent/turns?session_id=<id>&cursor=<cursor>
GET /api/agent/turns/<turn-id>
GET /api/agent/project-items/<item-id>/turns?cursor=<cursor>
GET /api/agent/projects/<project-id>/turns?cursor=<cursor>
```

项目和任务历史应分页，不把全部 Turn 塞进现有 ProjectItem 列表响应。

### 9.2 Turn 响应

```json
{
  "id": "T100",
  "projectId": "P1",
  "sessionId": "S10",
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
      "status": "completed"
    }
  ]
}
```

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

回执来自 ProjectItemEvent，不从最终自然语言回答中解析。

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
- ProjectItemEvent 记录失败；
- Turn 回执显示“未完成：缺少验收证据”；
- Agent 的自然语言不能绕过系统规则。

---

## 13. 分阶段实施

### Phase 0：契约与 ADR

- 锁定术语和边界；
- 明确 queued/running/cancelled；
- 明确 Session ID 如何传给 CLI/MCP；
- 明确不保存私有思维链；
- 决定后端重启时 running Turn 的恢复策略。

### Phase 1：持久化

- 新增 `agent_turns`；
- 新增 `project_item_events`；
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

### Phase 4：读取 API 与 UI

- Session/ProjectItem/Project 的 Turn 查询；
- ChatUI 显式 ID + 旧数据回退；
- Task Detail Turn 投影；
- Project Activity；
- 本轮项目操作回执。

### Phase 5：TaskRun 和完成审计

- `task_runs.origin_turn_id`；
- 完成裁定引用证据；
- Turn 回执区分“Agent 声称完成”和“系统验收完成”。

### Phase 6：可选显式关联

确认真实需求后再决定：

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

## 15. 待讨论问题

### Q1：是否需要 `turns attach-item`

当前建议：**非 MVP**。先依赖写操作自动产生 Event；只有“讨论但未修改”的关系确实影响用户体验时再增加。

### Q2：是否允许把 project-wide Session 转成 task-scoped Session

这会改变上下文、UI 徽章和 MCP 权限边界，需要独立设计。不能因为某一轮创建了 Task 就自动转换。

### Q3：Turn 的 prompt 是否保存全文

候选：

- 保存全文；
- 只引用用户 Reply；
- 保存可检索摘要，全文仍在 Session history。

需要结合隐私、搜索和 Session history 的可靠性决定。

### Q4：本轮过程保存到哪里

当前建议：

- `agent_turns` 保存生命周期和最终回答；
- `project_item_events` 保存领域变更；
- 完整可见过程继续由 Session transcript 承载；
- 不把工具日志复制到每张 Task。

需要确认 Session transcript 的长期持久化和清理策略。

### Q5：后端重启后的 active Turn

需要决定：

- ACP runtime 可恢复时继续 running；
- 无法恢复时标记 failed；
- 超过阈值后由修复任务收口。

### Q6：是否支持整轮 Undo

非 MVP。Undo 必须包含：

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
| 非 Task Session 创建 3 个 Tasks 如何追踪 | 一个 Turn + 三个 ProjectItemEvents |
| 是否自动把 Session 绑定到其中一张 Task | 否 |
| 是否要求 Agent 额外调用 CLI bind | 否 |
| CLI/MCP 如何归因 | 静态 Session ID，后端解析当前 Turn |
| 是否需要显式 attach-item | 后续可选，仅用于 referenced/人工修复 |
| 是否保存私有思维链 | 否 |
| Turn 是否替代 TaskRun | 否 |
| Turn 最终回答是否是完成证据 | 否 |
| 旧 Chat 历史如何兼容 | 无 ID 时继续按用户消息边界推断 |
