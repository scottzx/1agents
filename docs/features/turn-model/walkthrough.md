# Agent Turn 与项目动态时间轴 — 实现走查

**日期：** 2026-07-29  
**范围：** Epic #281、任务 #282–#288  
**规范：** [PRD](./prd.md) · [详细设计](./design.md)

## 1. 交付结果

本次实现把 Turn 从 ChatUI 的临时分组提升为可持久化、可恢复、可审计的领域对象，并以
`project_events` 作为项目事实的唯一事件源：

```text
用户 Prompt
  → AgentTurn（稳定 ID、队列、生命周期）
  → Session 身份签名
  → CLI / MCP 项目写入
  → project_events（原子写入、不可变）
  → Project Activity / Task Detail 投影
  → TaskRun（执行 / 核验）
  → Evidence + Verdict + ClosedBy
```

最终回答只代表 Agent 的陈述，不是 Task 的完成证据。执行型 Task 进入 `completed` 前，
必须先成功写入 TaskRun 审计；审计写入失败时，完成门会把任务置为 `failed`。

## 2. 数据与生命周期

### 2.1 AgentTurn

- schema v26 新增 `agent_turns` 与 `project_events`，并为 Reply 增加 `turn_id`；
- 一个 Session 同时最多一个 running Turn，其余 Prompt 按 FIFO 保存在 pending 队列；
- `queued → running → completed | failed | cancelled` 是唯一允许的状态迁移；
- prompt、最终回答、错误、开始/结束时间持久化，服务重启后可恢复 running/pending 边界；
- 用户 Reply、Agent Reply、失败/取消回执都携带显式 Turn ID；
- 旧历史没有 Turn ID 时，前端继续按用户消息边界做只读 fallback。

### 2.2 ProjectEvent

- ProjectItem、依赖、里程碑等变更与对应 Event 在同一事务提交；
- Event 记录 project/session/turn/correlation、actor、origin、target、operation 和结果；
- Event 是 append-only；外部调用不能更新或删除历史 Event；
- Session 身份由宿主签发，CLI/MCP 请求不能伪造其他 Session 或跨 Project 引用 Turn；
- 宿主外 CLI 保持可用，此时 Session/Turn 来源为空，不伪造因果关系。

### 2.3 TaskRun 完成审计

- schema v27 新增 `task_runs`，区分 `execution` 与 `verification`；
- `origin_turn_id` 指向创建该 Task 的发起 Turn，执行 Session 独立记录在 `session_id`；
- execution、function、interactive bridge、human override、verification 均接入完成门；
- Evidence 只保存紧凑摘要和引用，不复制原始日志、Prompt 全文或环境变量；
- verification 保存结构化 Verdict；
- completed Task 保存 ClosedBy，其中包含 TaskRun、Evidence、Turn、Session 和裁决引用；
- TaskRun 创建、开始、完成/失败/取消、verification 结果及项目项完成均进入 Activity。

## 3. 关键验收场景

### 场景 A：同一 Session 连续三轮

1. 连续发送三个 Prompt；
2. 第一轮运行时，第二、三轮分别形成不同的 queued Turn；
3. 每次 done 后只启动队首 Turn；
4. 刷新、WebSocket 重连或服务恢复后，Turn ID 与顺序保持不变；
5. ChatUI 历史轮默认折叠过程，只保留最终回答；当前轮展开。

验证点：三个 Prompt 对应三个稳定 Turn ID，不会因消息数组重新分组而改变。

### 场景 B：project-wide Turn 创建三个 Tasks

1. Session 不绑定 Task；
2. 同一个 Turn 内连续执行三个 ProjectItem create；
3. 三个 create Event 共用一个 `turn_id`；
4. Project Activity 聚合为一个批次；
5. 三张 Task Detail 分别投影与自身有关的 target；
6. 每张 Task 都能反向打开同一个 Origin Session / Turn；
7. Session 仍是 project-wide，不会被自动转成任一 task-scoped Session。

验证点：Turn 是因果批次，Task 只是该批次的目标；系统不需要额外 attach/bind 命令。

### 场景 C：队列、取消、断线与部分失败

- 取消 queued Prompt 只取消对应 Turn，不影响 active Turn；
- 取消 active Turn 会写 cancelled 终态和明确回执；
- 工具调用只归入当时 active Turn，不会错误写到尚未开始的 queued Turn；
- 工具批次部分成功时，成功 Event 保留，失败操作以 rejected/failed 结果可见；
- WebSocket 临时断开不直接把 Turn 判为 completed；
- runtime 丢失或恢复失败时写 failed，而不是伪造成功。

### 场景 D：执行完成

1. executor 的最终回答写入对应 Turn；
2. 系统创建/复用 execution TaskRun；
3. 记录 artifact/result 或 agent completion Evidence；
4. 无核验要求时由 completion gate 写 ClosedBy 并完成 Task；
5. 有核验要求时 execution TaskRun 完成，但 Task 进入 `pending_review`，尚无 ClosedBy；
6. Task Detail 可分别打开 Origin Session / Turn 和实际执行 Session。

验证点：Turn 的“已完成”文本本身不能改变 Task 为 completed。

### 场景 E：核验通过

1. verifier Session 启动 verification TaskRun；
2. `submit_review` 提交逐项 CriterionResult；
3. 服务端计算最终 pass/needs-human/fail，不信任客户端直接声明整体状态；
4. pass 在同一请求内写 Evidence、结构化 Verdict 和 ClosedBy；
5. 只有审计事务成功后，API 才返回 completed；
6. Session 结束时发现 TaskRun 已终态则幂等返回，不重复写审计。

验证点：审计表不可用时，任务回退为 failed；不存在“先完成、后补证据”的成功响应。

### 场景 F：人工决策与函数执行

- `complete_human_project_item` 走统一 PATCH，生成 `human_override` Evidence；
- IM 卡片对 `pending_review` 的一键批准生成 `im_human_decision` Evidence；
- 旧离线 `task update --status completed` 生成 `cli_override` Evidence；
- function executor 记录 function result/error Evidence；
- 纯容器 Task 在所有子任务完成时生成 `children_terminal` Evidence；
- 这些入口都使用同一 TaskRun/ClosedBy 完成门；
- 关闭 requirement/Epic 等非执行容器仍采用 Issue 语义，不伪造成一次执行 TaskRun。

## 4. UI 走查

### ChatUI

- 消息优先按显式 Turn ID 分组；
- running 展开，历史 completed 折叠；
- failed/cancelled 显示终态回执；
- Turn 内项目写操作以事实回执展示，不用最终回答推断项目状态。

### Project Activity

- 项目工作台展示 Turn、correlation 或单 Event 聚合后的只读时间轴；
- 支持 Session、Turn、target、status、origin 和 cursor 筛选；
- 同一 Turn 多目标变更只显示一个确定性批次摘要；
- TaskRun、核验和完成门事件使用紧凑摘要。

### Task Detail

- 只投影 target 为当前 Item 的相关 Activity；
- 显示 execution/verification TaskRun、attempt、status、Evidence、Verdict 和 ClosedBy；
- 可打开创建任务的 Origin Session / Turn；
- 可独立打开实际 execution/verification Session；
- 页面不展示原始日志、环境变量或模型私有推理。

## 5. 自动化验证

交付门已于 2026-07-29 执行：

```bash
cd backend
go test ./...
go test -race ./internal/meta ./internal/agent

cd ../frontend
yarn test
yarn check
yarn build

cd ..
node --check modules/1acp/bridge-server.js
git diff --check
```

实际结果：

- `go test ./...`：通过；
- `go test -race ./internal/meta ./internal/agent`：通过；
- 前端 `node:test`：101/101 通过；
- `yarn check`：通过；
- `yarn build`：通过，仅保留项目原有 Sass legacy API / `@import` 弃用警告；
- `node --check modules/1acp/bridge-server.js`：通过；
- `git diff --check`：通过。

专项覆盖包括：

- migration、AgentTurn 状态机、幂等与重启恢复；
- ProjectEvent 原子性、不可变性、cursor 与跨 Project 拒绝；
- 三 Prompt FIFO、取消、error、断线和 runtime 丢失；
- Session 签名、CLI/MCP 自动归因、伪造拒绝和一轮创建三 Tasks；
- Project/Task Activity 聚合与筛选；
- 显式 Turn/legacy fallback 前端分组；
- final answer 不绕过完成门；
- execution、verification、human、IM、CLI、function 和子任务聚合的 Evidence/ClosedBy；
- TaskRun 反向关联 Origin Turn 与执行 Session。

## 6. 隐私与权限边界

- Activity、TaskRun 和 Evidence 只保存产品可解释的事实摘要；
- 原始执行日志继续留在有权限控制的 Session transcript；
- 不持久化 chain-of-thought；
- Prompt 文本遵循 AgentTurn 的项目访问边界，不进入跨项目 Activity 摘要；
- Session/Turn 归因由宿主签名，调用方提供的裸 ID 不构成授权；
- target 与 Turn 必须属于同一 Project。

## 7. 已知限制与非 MVP

- legacy Session 没有 Turn ID 时只能做展示 fallback，不能补造精确因果关系；
- project-wide Session 不会自动绑定或转换为 task-scoped Session；
- “讨论但没有项目写入”不会自动建立 Turn ↔ ProjectItem 关系；
- 原始日志仍按 Session 查询，TaskRun 只提供摘要与引用；
- `turns attach-item`、Session attach/detach、整轮 Undo、waiting approval、partial-failure
  独立状态和 Turn 成本指标属于后续候选；
- Project Activity 是事件投影，不是第二套可编辑日志。

## 8. 看板映射

| 项目 | 交付 |
|------|------|
| #282 | PRD、ADR、状态机、事件注册表和测试矩阵冻结 |
| #283 | AgentTurn / ProjectEvent / Reply Turn 存储 |
| #284 | Bridge 生命周期、FIFO、取消、恢复与回写 |
| #285 | Session 签名与 CLI/MCP 自动归因 |
| #286 | Activity 聚合、API、筛选和 cursor |
| #287 | ChatUI、Project Activity、Task Detail 投影与 legacy fallback |
| #288 | TaskRun、Evidence、Verdict、ClosedBy、完成门与全量回归 |
