# PRD：Agent Turn 与项目动态时间轴

| 字段 | 内容 |
|------|------|
| 状态 | **产品方案已确认 · 待人工实施** |
| 版本 | **v0.1** |
| 日期 | 2026-07-27 |
| 看板 | Epic **#281**；人工执行任务 **#282–#288**；里程碑 **Agent Turn 与项目动态时间轴** |
| 定位一句话 | 将每次用户请求及其项目操作持久化为 Agent Turn，并由不可变 `project_events` 自动生成项目上下文时间轴 |
| 详细设计 | [design.md](./design.md) |
| 关联 | [project-model](../project-model/design.md)、[issue-model](../issue-model/design.md)、[pm-standalone](../pm-standalone/prd.md)、[verification-gate](../verification-gate/design.md) |

---

## 0. 产品命题

当前 ChatUI 已经能够把一轮中的过程消息、工具调用和最终回答组合成 Turn，并将历史 Turn 的过程折叠，只保留最后回答。

下一步不是继续增加前端分组规则，而是让 Turn 成为系统的正式领域对象：

> **Turn 是一次用户意图到项目事实变更之间的可审计因果批次。**

系统应当能够准确回答：

- 这一轮创建了多少 ProjectItem；
- 更新、关闭、取消或重新打开了哪些 ProjectItem；
- 建立或移除了哪些依赖；
- 更新了哪些里程碑；
- 发起了哪些 TaskRun；
- 哪些操作成功、失败或被规则拒绝；
- 最终回答是什么；
- 这些变化来自哪个 Project、Session、Turn 和 Actor。

在此基础上，系统可以自然生成一个基于 Project 上下文的动态时间轴：

> **Project Activity 是 `project_events` 的只读聚合投影；有 Turn 的事件按 Turn 聚合，没有 Turn 的事件按系统操作批次或单事件展示。**

---

## 0.1 已锁定决策

| # | 决策 | 说明 |
|---|------|------|
| D1 | 底层名称使用 `AgentTurn` | PM 是使用场景，不是底层存储边界 |
| D2 | Session 不强制绑定 ProjectItem | project-wide Session 可以跨多个 Items 工作 |
| D3 | 一轮影响多个 Items 时，通过 Events 建立 N:M 关系 | 不扩展 `sessions.task_id` 来表达批量影响 |
| D4 | 事件源使用通用 `project_events` | 覆盖 ProjectItem、Milestone、Dependency、Session、TaskRun、Verification |
| D5 | Project Activity 不重复存储 | 从 Turn + Events 生成只读时间轴投影 |
| D6 | 时间轴阅读粒度为 Turn，审计粒度为 Event | 摘要易读，展开可核查 |
| D7 | CLI/MCP 写操作自动归因 | 不要求 Agent 创建后再执行 bind |
| D8 | 宿主内使用稳定 Session ID，后端解析动态 active Turn | 长驻 MCP 不能使用固定 Turn 环境变量 |
| D9 | 宿主外 CLI 仍可独立工作 | 无 Session 时 `turn_id = null`，保留 actor/event |
| D10 | 只默认收录产生项目影响或被显式关联的 Turn | 避免项目动态退化成聊天记录 |
| D11 | 不保存智能体私有思维链 | 只记录用户可见过程、工具事实、状态和最终回答 |
| D12 | Turn 不替代 TaskRun 或完成审计 | 最终回答不是 completed 的验收证据 |
| D13 | 显式 `attach-item` 非 MVP | 仅用于“讨论但未修改”或人工修复关系 |
| D14 | 旧历史继续使用前端边界推断 | 新旧 Session 可同时展示 |

---

## 1. 用户问题

### 1.1 当前缺口

目前系统可以看到：

- ProjectItem 当前是什么状态；
- Task Detail 中有哪些 Replies 和 Sessions；
- ChatUI 中某一轮的过程和最终回答。

但系统不能稳定回答：

- 某张 Task 是在哪一轮被创建的；
- 一轮 PM 对话同时创建了哪几张 Tasks；
- 一次批量更新究竟成功了几项；
- 一个普通项目会话创建的 Tasks 如何与会话建立来源关系；
- Project 中最近发生的变化应该怎样按业务语义聚合；
- Agent 声称“已完成”的内容是否与真实写入一致。

### 1.2 为什么不能只依赖 Session

一个 Session 可能持续数小时或数天，并包含多个用户请求。

```text
Session S1
  ├── Turn T1：调研现状
  ├── Turn T2：创建三个 Tasks
  ├── Turn T3：调整其中两个 Tasks 的依赖
  └── Turn T4：发起执行
```

如果项目动态以 Session 为粒度，就无法表达每次变化的边界、顺序和结果。

### 1.3 为什么不能只依赖最终回答

最终回答属于自然语言叙述，可能出现：

- 数字不准确；
- 部分工具调用失败但回答遗漏；
- Agent 声称已更新，实际事务回滚；
- 状态变更被完成门拒绝；
- 多个并发操作发生覆盖。

因此项目动态的数量、对象和 Diff 必须来自结构化 Event，而不是解析最终回答。

---

## 2. 目标与非目标

### 2.1 MVP 目标

1. 每个新 Chat Turn 获得稳定的后端 `turn_id`。
2. Turn 能正确经历 queued、running、completed、failed、cancelled。
3. CLI/MCP/REST 的项目写操作生成不可变 `project_events`。
4. 宿主内写操作自动关联当前 Session 和 Turn。
5. 同一 Turn 的多个 Events 可聚合为一条 Project Activity。
6. 普通 project-wide Session 创建多个 Tasks 时不需要绑定其中任意一张。
7. Task Detail 可以反查影响当前 Item 的 Turns。
8. ChatUI 优先按显式 `turn_id` 分组，旧历史继续回退推断。
9. Project Activity 支持按来源、对象类型和时间分页筛选。
10. Turn 最终回答与系统操作回执分开显示。

### 2.2 非目标

| 非目标 | 原因 |
|--------|------|
| 保存完整私有思维链 | 隐私、安全和产品语义不合适 |
| 整轮 Undo | 需要版本冲突和逆向操作设计 |
| Turn 重放 | 涉及非幂等工具和外部副作用 |
| 自动把 project-wide Session 绑定到 Task | 会混淆上下文、影响对象和权限 |
| 要求 Agent 手动 bind 每个新 Task | 不可靠且产生双写窗口 |
| 第一版记录所有纯问答 Turn 到 Project Activity | 噪声过高 |
| 用 Turn 最终回答作为完成证据 | 与验证门冲突 |
| 替代 `task_runs` | 两者表达不同生命周期 |

---

## 3. 核心用户场景

### 3.1 普通项目会话创建三个 Tasks

初始状态：

```text
Project P1
Session S10
session.task_id = null
```

用户在 Turn T100 中要求拆解工作，Agent 依次创建 A、B、C：

```text
Turn T100
  ├── Event E1：create ProjectItem A
  ├── Event E2：create ProjectItem B
  ├── Event E3：create ProjectItem C
  └── final answer
```

预期：

- `Session S10` 继续保持 project-wide；
- A、B、C 都能反查 `Turn T100`；
- Project Activity 显示一条“创建 3 个 Tasks”的动态；
- 每张 Task Detail 只显示与自身有关的创建 Event；
- ChatUI 最终回答下显示系统操作回执；
- 不要求执行额外 bind 命令。

### 3.2 一轮部分成功

用户要求更新五张 Tasks，其中四张成功、一张因完成证据不足被拒绝。

预期 Project Activity：

```text
更新 4 / 5 个项目项

成功：
- #301 priority: medium → high
- #302 milestone: null → M2
- #303 dependsOn: [] → [#301]
- #304 status: pending → cancelled

失败：
- #305 completed：缺少验收证据
```

Turn 可以是 `completed`，但项目操作结果明确显示 `partial`；是否增加 `partial_failure` Turn 状态留到后续，不阻塞 MVP。

### 3.3 用户手动批量修改

人工在看板批量更新三个 Tasks 时没有 Agent Turn。

系统应使用 `correlation_id` 聚合：

```text
Scott 批量更新 3 个 Tasks
```

底层仍是三个 Events，`turn_id = null`。

### 3.4 Scheduler 自动解阻

依赖满足后 Scheduler 将两个 Tasks 从 blocked 迁移到 pending。

Project Activity 显示：

```text
Scheduler 检测到依赖满足
- #302 blocked → pending
- #303 blocked → pending
```

### 3.5 讨论但未修改某个 Task

Turn 读取并讨论 #12，但没有写操作。

MVP 默认不把它显示在 #12 Task Detail 或 Project Activity。

未来可以通过：

```text
turns attach-item current #12 --relation referenced
```

显式加入；该能力不改变 Session 的 `task_id`。

---

## 4. 领域模型

### 4.1 AgentTurn

```text
AgentTurn
- id
- project_id
- session_id
- initiating_reply_id
- agent_type
- status
- prompt_text 或 prompt_reply_ref
- final_answer
- error_text
- started_at
- completed_at
- created_at
- updated_at
```

MVP 状态：

```text
queued | running | completed | failed | cancelled
```

不变量：

1. 一个 Session 同时最多有一个 running Turn；
2. 一个 Session 可以有多个 queued Turns；
3. 工具写操作只能归因到当前 running Turn；
4. `done/error/cancel` 必须终结对应 Turn；
5. reconnect 后 Turn 边界不变；
6. completed/failed/cancelled Turn 不再接受新 Event。

### 4.2 ProjectEvent

统一事件源：

```text
ProjectEvent
- id
- project_id
- correlation_id
- turn_id
- session_id
- task_run_id
- actor_kind
- actor_name
- origin
- event_type
- target_type
- target_id
- operation
- before_json
- after_json
- status
- error_text
- sequence
- created_at
```

MVP `target_type`：

```text
project_item | milestone | dependency | session | turn | task_run | verification
```

MVP `operation`：

```text
create | update | close | reopen | delete | link | unlink | start | complete | fail | cancel
```

### 4.3 ProjectActivityEntry

`ProjectActivityEntry` 是读模型，不单独作为事实写入：

```text
ProjectActivityEntry
- id: turn:<id> 或 correlation:<id> 或 event:<id>
- project_id
- actor
- source_type
- source_id
- status
- summary
- stats
- targets
- occurred_at
- events[]
```

其中：

```text
stats.created
stats.updated
stats.closed
stats.cancelled
stats.linked
stats.failed
```

统计由 Events 确定性聚合，第一版不冗余存储。

### 4.4 与 Reply、Session 和 TaskRun 的关系

- `replies.turn_id`：用户 Reply 和最终 Agent Reply 指向同一 Turn；
- `sessions.task_id`：仅表达 Session 的单一 Task 上下文和权限软关联；
- `task_runs.origin_turn_id`：表达某次执行由哪一轮发起；
- `project_events.turn_id`：表达项目事实由哪一轮造成；
- `project_events.task_run_id`：表达项目事实由哪次执行产生。

---

## 5. CLI/MCP 归因契约

### 5.1 宿主内

Agent shell 和 ProjectItems MCP 获得稳定的：

```text
ONEAGENTS_SESSION_ID
ONEAGENTS_WORKSPACE_ID
ONEAGENTS_BASE_URL
ONEAGENTS_INTERNAL_TOKEN
```

内部请求携带：

```text
X-1Agents-Session-ID
X-1Agents-Origin: cli | mcp
```

后端：

1. 验证内部身份；
2. 验证 Session 属于当前 Project；
3. 查找当前 running Turn；
4. 构造 MutationContext；
5. 同事务写 Project 数据和 Event。

### 5.2 宿主外

普通终端直接运行 CLI 时：

- cwd 继续解析 Project；
- 无 Session/Turn 也可读写；
- Event 保留 actor/origin；
- `turn_id = null`；
- 不伪造 Agent Turn。

### 5.3 写命令响应

```json
{
  "item": {
    "id": "item-id",
    "number": 301
  },
  "origin": {
    "sessionId": "session-id",
    "turnId": "turn-id",
    "eventId": "event-id"
  }
}
```

### 5.4 显式关联

MVP 不增加通用 `task card bind`。

后续候选命令必须区分：

```text
turns attach-item      # Turn 讨论/引用某 Item
sessions attach-task   # Session 转为某 Task 的上下文
```

二者不能共用一个模糊的 `bind` 动词。

---

## 6. 项目动态时间轴

### 6.1 聚合规则

优先级：

1. `turn_id != null`：同一 Turn 的 Events 聚合成一条；
2. `correlation_id != null`：同一人工/系统批次聚合成一条；
3. 其余 Event 单独展示。

排序：

- 按聚合项最后发生时间倒序；
- 同一聚合项内部按 `sequence` 正序；
- 使用 cursor 分页，避免 offset 在实时插入时重复或漏项。

### 6.2 默认收录

收录：

- ProjectItem、Milestone、Dependency 写操作；
- TaskRun 和 Verification 的关键状态；
- 系统状态裁定；
- 被显式关联的 Turn。

默认隐藏：

- 纯闲聊；
- 无项目影响的普通问答；
- 只读工具调用；
- text delta；
- 重复重试日志；
- 私有思维过程。

### 6.3 信息层级

L1 默认摘要：

```text
谁 · 什么时间 · 做了什么 · 影响多少对象 · 成功/失败
```

L2 影响对象：

```text
ProjectItem 链接、操作类型、状态
```

L3 审计详情：

```text
字段 Diff、TaskRun、证据、错误、原始 Session/Turn
```

### 6.4 页面入口

PM 主页面增加：

```text
概览 | 项目项 | 看板 | 里程碑 | 动态
```

Task Detail 使用同一读模型，但增加 `target_id` 过滤。

---

## 7. ChatUI 与 Task Detail

### 7.1 ChatUI

- 新消息优先按显式 `turnId` 分组；
- 无 ID 的旧历史继续按用户消息边界推断；
- 当前 Turn 展开并 sticky；
- 历史 Turn 折叠只显示最终回答；
- 最终回答下显示 Event 生成的系统操作回执；
- failed/cancelled 有独立状态，不伪装为完成。

### 7.2 Task Detail

Task Detail 展示当前 Task 相关的 Turn 投影：

```text
Agent Turn · completed

来源：项目会话 / Session 名称
对当前 Task 的影响：create / update / close / dependency
最终回答：……

[查看字段变化] [查看本轮过程] [打开原始会话]
```

不复制完整工具日志，不要求原始 Session 绑定当前 Task。

---

## 8. API

```text
GET /api/agent/turns?session_id=<id>&cursor=<cursor>
GET /api/agent/turns/<turn-id>
GET /api/agent/projects/<project-id>/activity?cursor=<cursor>&source=<source>
GET /api/agent/project-items/<item-id>/activity?cursor=<cursor>
```

筛选候选：

```text
source=agent|user|scheduler|system|github
target=project_item|milestone|dependency|task_run|verification
status=completed|failed|cancelled
```

ProjectItem 列表响应不内嵌完整历史，避免拖慢看板。

---

## 9. 权限与一致性

1. 外部请求不能任意声明其他 Session 或 Turn；
2. Session、Turn、Event、Target 必须属于同一 Project；
3. Project 写入与 Event 写入必须在同一 SQLite 事务；
4. Event 是不可变事实，修正通过新 Event 表达；
5. Turn 最终回答不能替代 Event；
6. completed 仍受验证门和完成审计约束；
7. Event 的 before/after 可能含敏感字段，实现时需定义脱敏策略；
8. Project Activity 只能展示当前用户有权访问的 Project 数据。

---

## 10. 成功指标

MVP 上线后应能无文本解析地得到：

- 每个 Turn 创建、更新、关闭、取消的 ProjectItem 数；
- 每个 Turn 成功和失败的操作数；
- 每张 Task 的创建来源 Turn；
- Project 中 Agent、用户、Scheduler 的变更占比；
- 从 Turn 创建 Task 到首次 TaskRun 的时间；
- 一轮创建的 Tasks 当前完成比例；
- 项目最近活动的结构化时间轴。

第一版不要求制作完整指标 Dashboard，但底层数据应支持后续聚合。

---

## 11. 验收标准

### 11.1 Turn

- 同一 Session 连续三个 prompts 产生三个稳定 Turn；
- queued、running、done、error、cancel 不串轮；
- reconnect 后 Turn ID 和状态正确；
- 旧历史无 `turn_id` 仍正常展示。

### 11.2 事件归因

- 一轮创建三个 Tasks，产生一个 Turn 和三个 create Events；
- 三个 Events 的 `turn_id` 相同；
- Session 的 `task_id` 保持为空；
- 不执行 bind 也能从三张 Task 反查该 Turn；
- Project 写入与 Event 写入原子成功或原子回滚。

### 11.3 CLI/MCP

- 宿主内 CLI/MCP 自动关联当前 Turn；
- 宿主外 CLI 正常工作且 `turn_id = null`；
- 跨 Project 和伪造 Session 请求被拒绝；
- 写命令返回 `eventId/turnId/sessionId` 来源回执。

### 11.4 Project Activity

- 同一 Turn 的三个 Events 只显示一条时间轴动态；
- 展开后可以看到三个操作和字段变化；
- 人工批量操作可按 correlation 聚合；
- 无项目影响的纯聊天默认不进入动态；
- 支持 cursor 分页和来源筛选。

### 11.5 Task Detail

- 每张 Task 只显示与自身相关的 Events；
- 可以跳回准确的 Session 和 Turn；
- 项目级 Session 无 Task 绑定时仍可正常跳转；
- 最终回答和系统事实回执有明确区分。

### 11.6 完成审计

- Agent 回答“已完成”不会绕过验证门；
- completed Task 可继续追溯到 TaskRun、Evidence、Verdict 和 ClosedBy；
- `origin_turn_id` 可以反查发起来源。

### 11.7 工程验证

- meta migration 测试覆盖旧数据库升级；
- 后端 Turn/Event/API 测试通过；
- CLI/MCP 归因和安全测试通过；
- ChatUI 与 Task Detail 前端测试通过；
- 在 `frontend/` 执行 `yarn build` 成功；
- `git diff --check` 无格式错误。

---

## 12. 分阶段范围

### Phase 0：契约冻结（#282）

- PRD/ADR 评审；
- 状态机、队列、重启恢复、隐私范围；
- Event 类型注册表；
- API 和 CLI/MCP 归因契约。

### Phase 1：Turn 与 Event 存储（#283）

- `agent_turns`；
- `project_events`；
- `replies.turn_id`；
- migration、Store、索引和测试。

### Phase 2：Bridge Turn 生命周期（#284）

- active/pending Turn；
- prompt/done/error/cancel；
- Reply 关联；
- reconnect/recovery。

### Phase 3：写入自动归因（#285）

- Session 身份注入；
- CLI/MCP Header；
- MutationContext；
- 原子 Event；
- 来源回执。

### Phase 4：Activity 读模型与 API（#286）

- Turn/correlation/event 聚合；
- Project/Task 查询；
- cursor 和筛选；
- 确定性摘要与统计。

### Phase 5：前端（#287）

- ChatUI 显式 Turn；
- 本轮操作回执；
- Project Activity 页面；
- Task Detail Turn 投影；
- legacy fallback。

### Phase 6：TaskRun、完成审计与回归（#288）

- `task_runs.origin_turn_id`；
- Verification/ClosedBy 关联；
- 迁移、权限、重启、队列和 E2E；
- `yarn build`。

### Phase 7：后续候选

- `turns attach-item`；
- `sessions attach-task`；
- waiting approval；
- partial failure 状态；
- Undo；
- Turn 级指标与成本。
