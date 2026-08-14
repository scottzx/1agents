# 自动任务可信执行内核：实施清单

> 状态：Ready for implementation（工程评审完成）  
> 更新日期：2026-08-14  
> 近期范围：ACP-only `Run now` + one-shot `at`  
> 实施策略：A — Phase 1 拆成三个可独立合并、可验证、可回滚的后端纵切片  
> 上游设计：`/Users/scott/.gstack/projects/scottzx-1agents/scott-main-design-20260813-235616.md`

本文是后续开发、跟踪和复盘的执行基线。若实现改变了本文的状态机、不变量或阶段边界，必须先更新“决策记录”，不能只改代码。

## 1. 目标与完成定义

本阶段不追求更多自动化能力，只解决一个基础问题：系统必须可靠回答“一次自动运行用了什么输入、包含哪些 Turn、何时以及为何结束、结果属于哪次 Run”。

Phase 1 完成时必须同时满足：

- `TaskRun` 是一次完整运行的唯一事实源，`ProjectItem` 只是当前运行结果的投影。
- 一个 Run 可以 append 多个 Turn；Turn terminal 只结束该 Turn，不直接结束 Run。
- `Run now` 与 one-shot `at` 走同一套 Start/Finish 契约。
- 启动、结束、重复信号和进程重启后，不留下无法解释的永久 `running`。
- Run 使用不可变输入快照；运行中修改 Item/Job 不改变历史 Run。
- 旧 Run 无权覆盖当前 Item 的新版本或新 active Run。
- 自动执行 Session 后续人工 Turn 不会反向污染已完成 Run。
- Turn 成功后按冻结的可选核验策略路由：无策略直接完成；有策略则在同一 Run 内完成机器/人工核验与修复循环。
- 机器核验失败和人工 `changes_requested` 都以系统生成的结构化反馈追加新 Turn，直至通过、预算耗尽或人工终止。
- 当前五条狗粮配方形成固定回归夹具，作为每个切片的合并门禁。

## 2. 已确认的工程决策

| ID | 决策 | 实施含义 |
| --- | --- | --- |
| D1 | `ProjectItem` 有业务契约版本；`ExecutionJob` 不作为版本对象 | 新 Run 冻结 `ProjectItem.version` 和解析后的 Job 配置；旧 Job revision 仅兼容读取 |
| D2 | `TaskRun 1:N AgentTurn` | Turn 通过 `task_run_id + run_seq` append，terminal 后不可覆盖 |
| D3 | `Turn terminal != Run terminal` | runtime/bridge 只封存 Turn；持久 TurnTerminalHook 显式路由为 Finish、核验或修复 Continue |
| D4 | `RunCoordinator` 位于 `backend/internal/execution` | execution 拥有自动 Run 生命周期；agent 只是 ACP/runtime adapter；meta 只提供持久化原语 |
| D5 | 第一阶段 Item 级 `overlap=forbid` | 一个 ProjectItem 同时最多一个非终态 execution Run；`overlap=allow` 不开放 |
| D6 | Phase 1 只保留 `running -> terminal` 原子终结 | 暂不引入 `finishing/cancelling`；SQLite 终态事务失败即整体回滚，重启时由持久 runner policy 重驱动 Finish |
| D7 | Phase 1 不上 lease fencing | “严格单 backend automation writer”是硬约束；用 status、`active_run_id`、expected-last-turn CAS；若部署不能保证单 writer，lease epoch 必须前移到 Slice 2 |
| D8 | one-shot `at` 的 occurrence 必须持久消费 | overlap 时记录 `skipped_overlap` 并 exhausted；禁止暗中改成三分钟后重试 |
| D9 | 快照 hash 对“实际存储的 typed JSON bytes”做 SHA-256 | 不引入 RFC 8785 依赖；typed struct 序列化一次，原字节同时用于存储和 hash |
| D10 | 新自动任务的核验属于同一个 execution TaskRun | 核验是 Run 完成前的阶段；机器/人工步骤是 Run 子记录，修复以新 Turn append；不再创建新的 verification TaskRun |
| D11 | `ProjectItem.verificationPolicy` 是可选业务契约 | 上游有则随 Item 带入；没有则无核验。独立自动任务也通过自己的 ProjectItem 配置；ExecutionJob 不复制该字段 |
| D12 | 修复后默认从核验步骤 1 重新开始 | 修复可能使旧脚本结果和人工批准失效；历史 attempt 保留但不跨 round 复用，先保证正确性再考虑增量重跑 |
| D13 | 机器核验是有序脚本链，人工核验是有序签核链 | `npm run ...` 等脚本逐条执行；人工当前节点 approved 后才解锁下一节点，禁止跳级、代签或并行阈值替代顺序 |
| D14 | Phase 1 人工核验只启用 Item 作者/本地 Owner | policy 使用可扩展 actor selector；V1 只开放 `item_author` 并解析为 server-derived local owner principal，未来接入身份目录后增加 stable user/role，不把 CreatedBy/显示名当授权身份 |

### 2.1 Phase 1 的执行与核验状态机

```text
StartRun
   |
   v
running / phase=executing
   |
   +-- Turn failed/cancelled ----------------------------> FinishRun(failed/cancelled)
   |
   +-- Turn completed --> TurnTerminalHook(runID, turnID)
                              |
                              +-- verificationPolicy empty
                              |       --> FinishRun(runtime_completed_no_verification)
                              |
                              +-- verificationPolicy present
                                      --> phase=verifying, round=N
                                            |
                                            +-- command script passed
                                            |       --> next step
                                            |
                                            +-- human approver step
                                            |       --> phase=awaiting_human
                                            |             | approved --> next step
                                            |             | changes_requested
                                            |             |     --> phase=repairing
                                            |             |     --> Append Turn #N+1
                                            |             + rejected_final --> FinishRun(failed)
                                            |
                                            +-- machine failed
                                            |       --> persist result + phase=repairing
                                            |       --> inject feedback + Append Turn #N+1
                                            |
                                            +-- all steps passed
                                                    --> FinishRun(verification_passed)

新修复 Turn terminal 后创建下一 verification round，并从步骤 1 重跑。

所有 terminal 写入：TaskRun + 核验子记录 + 子 Turn 收敛 +
ProjectItem ownership/projection + ProjectEvent，必须在同一个 SQLite 事务中提交。
```

Run lifecycle 仍只使用 `running -> terminal`；`executing | verifying | awaiting_human | repairing` 是持久化 `phase`，不是第二套 terminal status。`finishing`、`cancelling`、durable cancel intent 和 cancel deadline 留到取消/Function 多阶段能力进入时再增加。Phase 1 没有跨事务的 ApplyFinish 窗口，因此不需要先承担这组状态复杂度。

## 3. NOT in scope

| 延后项 | 最早进入阶段 | 原因 |
| --- | --- | --- |
| recurrence、misfire、连续周期历史 | Phase 3 | 先证明一次 Run 可被可信收敛，再扩大频率 |
| Function→ACP 与 `task_run_stages` | Phase 4 | 当前仅 ACP-only；Stage 表没有 Phase 1 消费者 |
| 通用 `task_run_evidence` 独立表与大 artifact | Phase 2 | Phase 1 只为核验 attempt 保存受限输出与 artifact ref；通用 Run evidence 聚合仍后移 |
| 通用 `execution_invocations` ledger | Phase 3 | Phase 1 用 manual request id 唯一索引和 one-shot Trigger disposition 满足窄范围幂等 |
| lease owner / epoch / heartbeat fencing | Phase 3 前硬门 | 当前只有单 daemon；recurrence、多 worker 或远程 worker 上线前必须完成 |
| Runs 完整 UI、Evidence/Turns 分页交互 | Phase 2 | Phase 1 只提供最小 Run detail API 用于取证和验收 |
| Instructions 写保护 | Phase 2 P0 | 需要 server-derived actor context；不得用客户端自报 `task_run_id` 伪授权 |
| retry UI 与“严格复用旧快照” | Phase 2+ | 重试必须创建新 Run；第一阶段不自动重跑 |
| 历史 verification TaskRun 物理合并/删除 | 未排期 | 历史记录继续只读；新 automation Run 改用同 Run verification attempts，避免破坏旧审计链 |
| 修复后只从失败步骤增量重跑 | 未排期 | 默认从步骤 1 重跑更可信；未来有足够依赖/影响分析再优化成本 |
| 非幂等机器核验自动恢复 | 未排期 | Phase 1 机器核验必须声明 safe-to-retry；不满足时重启后转人工/outcome_unknown |
| `overlap=allow` | 未排期 | 未定义多个 Run 对一个 Item 的确定投影语义 |
| 自动规划、任务调度规划、持续工作的 Agent | Phase 5+ | 它们是执行内核的上层消费者，不进入底层状态机 |
| 趋势雷达、项目守夜人、信任等级、自动学习 | 远期 | 产品方向保留，但不与本次基础设施交付耦合 |

## 4. What already exists

| 现有能力 | 位置 | 复用方式 / 缺口 |
| --- | --- | --- |
| TaskRun 创建、CAS 终结、ProjectEvent | `backend/internal/meta/task_runs.go` | 复用数据和事件结构；新 execution Run 的执行、核验和终结全部收归 Coordinator；历史 verification Run 只读兼容 |
| AgentTurn append、状态迁移、request id 幂等 | `backend/internal/meta/turns.go` | 增加 Run 归属与序号；保留 Turn 自身状态机 |
| TaskRun occurrence、snapshot、client request 字段 | `backend/internal/meta/task_runs.go` | 迁移为统一 `RunInputSnapshotV1`，避免平行历史表 |
| SQLite `BEGIN IMMEDIATE`、WAL、busy timeout | `backend/internal/meta/db.go` | 适合短事务 CAS；严禁在事务内调用 ACP/Function/网络 |
| ExecutionJob / Trigger repository 与 scheduler | `backend/internal/execution` | 复用 Job/Trigger 定义；改为 Coordinator 先创建 Run 再 dispatch |
| ACP headless runner 与 workspace lock | `backend/internal/agent/runner.go`, `scheduler.go` | 降级为 runtime adapter；不再直接 Finish TaskRun 或 Mutate Item terminal |
| Verifier Agent、拒绝反馈、人工升级和 review budget | `backend/internal/agent/review.go`, `runner.go` | 复用策略与提示词经验；把 `ReviewCount/ReviewPool/Review` 从 Item 运行态迁到 Run verification attempts |
| ProjectItem `Verifier` 等 legacy 字段 | `backend/internal/meta/types.go` | 兼容读取时可合成单个 `agent_review` step；新写使用可选 `verificationPolicy` |
| verification-gate command check 设计 | `docs/features/verification-gate/design.md` | 复用“后端独立执行、只信退出码、输出截断”的原则；补齐失败回灌与多轮人工核验 |
| Session / AgentTurn journal 与 runtime record | `backend/internal/agent/acpx_client.go` | 作为恢复时 liveness 权威来源；Session 状态本身不够 |
| Run 列表 API 和 Audit Trail | execution HTTP / frontend activity | Phase 1 保留兼容；Phase 2 再构建完整 Runs UI |

当前实现中有四个不能与新内核并存的行为：

1. `runner.go` 把 `turn_terminal` 当成整个 TaskRun 完成。
2. `runner.finish` 先 Finish TaskRun，再用第二次 `TasksStore.Mutate` 写 Item，形成分裂事务。
3. `AgentTurnStore.RecoverInterrupted` 会无差别失败/取消全部 running/queued Turn。
4. Execution Service 遇到 workspace busy 会创建或改写一个“三分钟后重试”的 Trigger。
5. 旧核验会创建独立 verification TaskRun，并直接修改 ProjectItem 的 `pending_review/ReviewCount/ReviewPool`；它不能与“同一 Run 内核验”同时作为新自动任务写路径。

## 5. 目标调用链与模块边界

```text
HTTP / CLI / Scheduler
        |
        v
execution.Service
        |
        v
RunCoordinator  -------- short SQLite tx --------> ProjectItem
   |       |                                      TaskRun
   |       +-------------------------------------> AgentTurn binding
   |       +-------------------------------------> VerificationRound / Attempt
   |
   +-- after commit --> ExecutionDispatch{runID, snapshot, sessionID}
                           |
                           v
                    agent ACP adapter
                           |
                    Turn facts / runtime facts
                           |
                           v
                  TurnTerminalHook
                   /            \
          no verification     VerificationCoordinator
                |              command/agent/human
                +-------------> ContinueRun / FinishRun
```

边界规则：

- `backend/internal/execution`：自动 Run 的 Start、AppendTurn、TurnTerminalHook、VerificationCoordinator、Finish、one-shot occurrence 和 recovery orchestration。
- `backend/internal/meta`：SQL schema、行级读取、事务内 mutation helper；不解释 runtime 事件。
- `backend/internal/agent`：建立/恢复 ACP runtime、封存 Turn、执行受信 command/agent verifier adapter、把事实回报 Coordinator；不投影 ProjectItem terminal。
- `backend/internal/taskapi` 和 legacy verification：保留历史读取/兼容入口，但禁止它们关闭带 `job_id` 的新 execution Run或再创建其 verification TaskRun。
- frontend：Phase 1 仅做类型兼容和必要状态展示，不扩 UI 产品范围。

建议在 `RunCoordinator`、`VerificationCoordinator` 和 `RunInputSnapshotV1` 实现处加入上述 ASCII 图和状态机注释，避免未来又把 Turn terminal、verification result 与 Run terminal 合并。

## 6. 数据契约

### 6.1 ProjectItem

新增：

- `version INTEGER NOT NULL DEFAULT 1`
- `active_run_id TEXT NULL`
- `automation_migration_state TEXT NULL`，仅用于隔离历史冲突，不作为 lifecycle status
- `verification_policy_json TEXT NULL`，可选业务契约；NULL/空 steps 均规范化为无核验

业务契约字段变化才递增 version：

- title
- description（Automation UI 标签为 Instructions）
- acceptanceCriteria
- dependsOn
- executor
- businessRef
- target.agent / profile_id / cwd / capabilities
- verificationPolicy（步骤、顺序、命令/核验者、超时、最大修复轮次）

status、started/completedAt、summary/result、retryCount、closedBy 等运行投影不递增 version。

新 API/规划器必须发送 `expectedVersion`。旧 whole-config Save 暂时兼容，但 version 必须由数据库基于新旧契约值计算，不能信任调用者回传值。

#### 可选 VerificationPolicy

`ProjectItem.verificationPolicy` 是单一上游入口，缺失或 `steps=[]` 表示无需核验。ExecutionJob 不新增同名字段，也没有覆盖优先级。

```json
{
  "maxRepairRounds": 3,
  "steps": [
    {
      "id": "install-check",
      "kind": "command",
      "name": "依赖与类型检查",
      "command": "npm run check",
      "cwd": "frontend",
      "timeoutMinutes": 10,
      "safeToRetry": true
    },
    {
      "id": "build",
      "kind": "command",
      "name": "前端构建",
      "command": "npm run build",
      "cwd": "frontend",
      "timeoutMinutes": 10,
      "safeToRetry": true
    },
    {
      "id": "owner-approval",
      "kind": "human",
      "name": "作者确认",
      "assignee": { "kind": "item_author" },
      "prompt": "确认交付是否符合预期"
    }
  ]
}
```

步骤按数组顺序严格串行：每个 `command` 就是一段独立机器核验脚本，前一脚本 exit 0 后才执行下一脚本；每个 `human` 就是一位人工核验人，当前核验人 approved 后才创建/解锁下一位的 request。默认机器脚本在前、人工签核在后；任何修复 Turn 都开启新 round，并从第一条机器脚本重新执行。

未来多人签核直接追加多个 human step，不使用并行票数或 threshold：

```json
[
  { "id": "author", "kind": "human", "assignee": { "kind": "item_author" } },
  { "id": "reviewer-li", "kind": "human", "assignee": { "kind": "user", "id": "user-li" } },
  { "id": "reviewer-wang", "kind": "human", "assignee": { "kind": "user", "id": "user-wang" } }
]
```

只有 author approved 才轮到 user-li；只有 user-li approved 才轮到 user-wang。某一级 changes_requested 后产物发生变化，旧 round 中此前所有脚本通过与人工批准都只保留为审计历史，新 round 从第一条脚本和第一位核验人重新走。

来源规则：

- 上游项目 Item 有 policy：自动任务启动时直接冻结进 snapshot。
- 上游 Item 没有 policy：该 Run 无核验，Turn 成功后直接 Finish。
- 独立自动任务：仍会创建/绑定自己的轻量 ProjectItem，可选配置同一个 policy。
- policy 更新触发 ProjectItem.version++，只影响未来 Run。
- legacy `Verifier + AcceptanceCriteria` 且没有新 policy 时，兼容层可合成一个内部 `agent_review` step；它在 UI/审计中标为“AI核验”，不与用户定义的机器脚本或人工签核混称。二者同时存在时只使用新 policy并给出弃用提示。

### 6.2 RunInputSnapshotV1

使用强类型 Go struct，包含：

- `schemaVersion=1`
- ProjectItem id/version 和全部业务契约字段
- 解析后的 Job/Profile/CWD/capabilities/timeout 配置
- manual 或 at 的固定 trigger schema
- resolved prompt
- 完整 verificationPolicy（没有则明确保存 `null`）

规则：

- credential-free；只存稳定 credential reference。
- typed struct `json.Marshal` 一次；原 bytes 写 `input_snapshot_json`，同一 bytes 计算 SHA-256。
- manual trigger 固定为 `{kind:"manual", requestId, occurrenceKey, overlapPolicy:"forbid"}`。
- at trigger保存真实 trigger id、scheduledFor、occurrenceKey、misfire/overlap policy。
- 运行启动后任何 Item/Job 修改都不能改变 snapshot 或 hash。
- 核验命令、顺序、预算和人工提示在 Run 内不可变；Agent 不能修改本次 policy。

### 6.3 TaskRun

Phase 1 最小新增字段：

- `input_snapshot_json`, `input_snapshot_hash`, `item_version`
- `completion_reason`
- `first_turn_id`, `final_turn_id`, `turn_count`, `next_turn_seq`
- `runner_policy_json`（Phase 1 固定支持 `verify_then_finish_v1`）
- `retry_of_run_id`
- `reconciled_at`
- `phase`：`executing | verifying | awaiting_human | repairing`
- `verification_round`、`active_verification_attempt_id`

继续兼容读取 `origin_turn_id`、legacy Job revision 和旧 snapshot 字段；新 execution Run 不再依赖它们表达事实。

数据库约束：

- 一个 Item 至多一个 `status='running' AND kind='execution'` Run 的 partial unique index。
- `(job_id, client_request_id)` 在 request id 非空时唯一，用于 manual HTTP 重试幂等。
- terminal Run 必须有 `completed_at` 和 `completion_reason`。
- `phase=awaiting_human` 是可解释等待，不得被 worker-lost 恢复逻辑误杀。

### 6.4 AgentTurn

新增：

- `task_run_id TEXT NULL`
- `run_seq INTEGER NULL`

约束：

- `UNIQUE(task_run_id, run_seq) WHERE task_run_id IS NOT NULL`
- 非绑定 Turn 两字段必须都为 NULL；禁止空字符串。
- 分配 `next_turn_seq`、insert Turn、更新 turn_count/first_turn_id 必须在一个事务。
- AppendTurn 只允许 Run `status='running'` 且 expectedLastTurnID 匹配。
- terminal Turn 的 finalAnswer/error/usage/change report 只读；下一轮必须创建新 Turn。

### 6.5 one-shot Trigger disposition

在不提前引入通用 invocation ledger 的前提下，one-shot Trigger 增加最小消费事实：

- `last_occurrence_key`
- `last_disposition`：`started | skipped_overlap | failed_to_start`
- `last_task_run_id`
- `last_decided_at`

Coordinator 在同一事务中将 at Trigger 置 `exhausted` 并记录 disposition。Scheduler 重启重放同一 occurrence 时只返回既有决定，不补跑。

### 6.6 Verification Round 与 Attempt

新增 `task_run_verification_rounds`：

- `id`, `task_run_id`, `round_no`, `trigger_turn_id`
- `status`：`running | passed | repair_requested | failed | cancelled`
- `started_at`, `completed_at`
- `UNIQUE(task_run_id, round_no)`
- `UNIQUE(task_run_id, trigger_turn_id)`，让重复 Turn terminal hook 幂等

新增 `task_run_verification_attempts`：

- `id`, `round_id`, `task_run_id`, `step_id`, `step_ordinal`, `kind`
- `status`：`pending | running | awaiting_human | passed | failed | infrastructure_error | cancelled`
- `source_turn_id`, `idempotency_key`
- command：`exit_code`, `output_tail`, `artifact_ref`
- agent review：结构化 criterion verdict、verifier/profile 引用
- human：冻结的 `expected_actor_ref/resolved_actor_id`、`decision`, `feedback`, `actor_id`, `decided_at`
- timestamps 与 `error_text`
- `UNIQUE(task_run_id, idempotency_key)`

机器输出只 inline 保存受限尾部（默认 8 KiB）；完整日志进入 artifact/file store并保存引用。核验 attempt 是 append-only 历史，不能把 ProjectItem 上的 `Review/ReviewPool` 当本次 Run 的事实源。

### 6.7 TurnTerminalHook 与失败回灌

`OnTurnTerminal(runID, turnID)` 只接受属于该 Run 且已 terminal 的 Turn：

1. Turn failed/cancelled：按明确原因 Finish Run，不启动核验。
2. Turn completed且 policy 为空：显式 `FinishRun(runtime_completed_no_verification)`。
3. Turn completed且有 policy：幂等创建 verification round，从 step 1 串行执行。
4. 全部步骤通过：`FinishRun(verification_passed)`。
5. 机器步骤失败或人工 `changes_requested`：保存 attempt，构造 server-authored feedback，幂等 Append 新 repair Turn。
6. repair Turn terminal：创建下一 round，从 step 1 重跑。
7. 达到 `maxRepairRounds` 或人工 `rejected_final`：`FinishRun(verification_exhausted | human_rejected)`。

回灌消息使用稳定 request id：`verification-repair:{runId}:{round}:{stepId}`。内容包含 step 名称、命令/判定、exit code、受限 output tail、artifact ref 和人工反馈；日志作为“不可信数据块”包裹，不得拼进 system instructions，也不得包含环境变量或凭据。

机器核验需要区分：

- `exit != 0`：业务核验失败，可以进入修复 Turn。
- runner/进程启动失败或平台超时：`infrastructure_error`，先按 verifier infra retry policy 重试；耗尽后以 `verification_infrastructure_error` 结束或转人工，不能伪装成代码失败。
- 重启时只有 `safeToRetry=true` 的机器步骤可自动重放；否则进入 `awaiting_human` 并显示 outcome unknown。

人工决定只有三种：

- `approved`：当前 step passed，继续下一步。
- `changes_requested`：feedback 必填，创建 repair Turn。
- `rejected_final`：feedback 必填，Run 以 human_rejected 失败。

产品交互保持用户描述的两个主按钮：“通过”映射 approved，“不通过”映射 changes_requested并要求输入反馈；“终止核验并判定失败”作为独立的次级危险操作映射 rejected_final，避免一次普通驳回意外结束整个 Run。

人工签核身份规则：

- `assignee.kind=item_author` 在 StartRun 时通过 `VerificationActorResolver` 解析并冻结。当前 `ProjectItem.CreatedBy` 只是业务字符串（常见值为 `user`），不能直接当授权身份；V1 单用户部署解析为 server-derived `local_owner` principal。
- schema 预留 `kind=user | role`，但 Phase 1 API 只接受 `item_author`；后续接入用户目录/组织角色后再开放其校验器。
- 决定请求中的 actor id 不被信任；只有当前 `awaiting_human` step 的 resolved actor与服务端认证 principal 匹配时才可提交，其他用户、上一位和后续核验人均返回 403/409。
- 当前 step approved 的同一事务才把下一 human step从 pending推进为 awaiting_human；不提前发送下一人的待办。
- 若任一未来 actor 在 Run start 时无法解析，StartRun 返回 `verification_actor_unresolved` / Item not_ready，绝不跳过该节点。
- 管理员代签、转交、会签/quorum和动态加签都属于后续能力；一旦实现必须留下原核验人、代理人、原因和时间的审计链。

## 7. 三个纵切片

### Slice 0：实施前置与回归基线

目标：避免在脏工作区和互相冲突的 lifecycle 改动上继续叠加。

- [ ] **S0.1** 先 review、合并或隔离当前 Turn terminal reconciliation / Turn Change Report 改动。
- [ ] **S0.2** 撤回或重写“`turn_terminal` 直接完成 Run”的断言；保留 Turn 封存能力。
- [ ] **S0.3** 撤回或隔离 workspace busy 自动延后 3 分钟逻辑，禁止它进入新 at 语义。
- [ ] **S0.4** 固化五条狗粮配方回归夹具：Item、Job、Trigger、Run、Session、全部 Turns、completion gate。
- [ ] **S0.5** 将“晚间行情要点”写成第一条红测：Turn terminal 后 Run 仍 running；显式 Finish 后 Run/Item 同事务 terminal。
- [ ] **S0.5a** 增加三类核验夹具：无核验、`yarn build` 机器核验失败后修复、人工 changes-requested 后修复并通过。
- [ ] **S0.6** 记录当前 schema version，实施时分配“合并后下一个未占用版本”，不在计划里硬编码 v33。
- [ ] **S0.7** 开启 automation start feature gate；迁移/校验期间禁止创建新 Run。
- [ ] **S0.8** 校验部署拓扑只有一个 backend automation writer；无法保证时，把最小 `lease_epoch` fencing 加入 Slice 2 合并门。

合并门：只增加夹具、门禁和冲突清理，不改变正常运行语义；当前 backend tests 全绿。

回滚：移除测试门禁/feature flag wiring；不涉及不可逆 schema。

### Slice 1：数据契约与可独立验证的存储原语

目标：先落不可变数据关系，不接管 runtime。

- [ ] **S1.1** 增加 ProjectItem `version`、`active_run_id`、migration state 迁移及扫描/序列化。
- [ ] **S1.2** 将契约字段变更与 runtime 投影变更分开；实现 `expectedVersion` CAS 更新入口。
- [ ] **S1.3** 为旧 whole-config Save 增加数据库侧契约 diff/version 计算，避免调用者覆盖 version。
- [ ] **S1.3a** 在 ProjectItem 增加可选 `verificationPolicy`；支持有序 command/human steps和可扩展 actor selector，变化参与 version CAS，ExecutionJob 不增加副本。
- [ ] **S1.3b** 扩展 ProjectItem create/update/read API 与前端/CLI wire type；字段缺失保持 NULL，不为旧 Item 自动生成默认核验。
- [ ] **S1.4** 定义强类型 `RunInputSnapshotV1`、manual/at trigger union 和 credential-free 校验。
- [ ] **S1.5** 实现“marshal once → store bytes → SHA-256 same bytes”的 snapshot builder。
- [ ] **S1.6** 增加 TaskRun snapshot、itemVersion、completionReason、turn pointers/count/seq、runner policy、retry/reconcile 字段。
- [ ] **S1.7** 增加 AgentTurn `task_run_id`、`run_seq` 与 partial unique index。
- [ ] **S1.8** 实现事务型 `AppendRunTurn`：CAS next seq + insert + turn_count + firstTurn；支持故障注入回滚测试。
- [ ] **S1.9** 增加 execution Run active partial unique index和 manual request id 唯一索引。
- [ ] **S1.10** 历史数据只按 `origin_turn_id` 回填 first/final/seq=1；禁止按 Session 猜测更多 Turn。
- [ ] **S1.11** 多个历史非终态 Run 的 Item 标记 `legacy_unresolved` 并阻止 Start；不自动猜测权威 Run。
- [ ] **S1.12** 旧 Job revision 保留读取兼容；新 snapshot 和并发控制不依赖它。
- [ ] **S1.13** 提供未接 runtime 的事务型 `CreateRunRecordWithSnapshot` 原语：同一事务重读 Item/version/Job、构造并插入 snapshot Run、设置 active ownership；测试后保持 production gate 关闭。
- [ ] **S1.14** 增加 TaskRun phase/verification cursor 字段，以及 verification rounds/attempts 表、状态约束与幂等索引。
- [ ] **S1.15** 实现 legacy `Verifier` 单步 policy 兼容读取；新数据不再把 ReviewCount/ReviewPool 当 execution Run 事实。

合并门：新表/列可被旧 runtime 忽略；所有迁移、version、snapshot、Turn append 单测通过；feature gate 仍关闭。

回滚：旧二进制继续忽略 additive columns/indexes；新写尚未启用。不要物理删除 legacy 列。

### Slice 2：RunCoordinator、Turn terminal hook、核验闭环与 ACP 接入

目标：让 manual Run now 完整走新生命周期；终态只有一个写入口。

- [ ] **S2.1** 在 `backend/internal/execution` 新增 `RunCoordinator` 和事务 repository。
- [ ] **S2.2** 实现 `StartManual(jobID, requestID)`：事务内重读 Item/Job、校验依赖/ownership、冻结 snapshot、创建 Run、设置 active_run_id/running。
- [ ] **S2.3** Start 事务提交后才 dispatch ACP；dispatch payload 只携带 `runID/sessionID/snapshot` 等冻结事实。
- [ ] **S2.4** 修改 dispatcher contract，禁止 agent runner 自行创建 TaskRun。
- [ ] **S2.5** ACP adapter 创建/追加 Turn 时必须走 `AppendRunTurn`，不能仅凭 Session 归属。
- [ ] **S2.6** runtime `turn_terminal` 只 Transition 当前 Turn 和记录 finalAnswer；删除 execution Run 的隐式 finish 分支。
- [ ] **S2.7** 默认 runner policy 改为 `verify_then_finish_v1`：Turn 封存后调用幂等 TurnTerminalHook，由它决定直接 Finish、进入核验或追加修复 Turn。
- [ ] **S2.8** 实现原子 `FinishRun(runID, expectedLastTurnID, finalTurnID, outcome, completionReason)`。
- [ ] **S2.9** Finish 校验 final Turn 属于本 Run且已 terminal；无 Turn 只允许明确的 startup/worker-lost/early-cancel 原因。
- [ ] **S2.10** Finish 同事务更新 TaskRun、收敛非 terminal 子 Turn、写 ProjectEvent/completion gate、清 active_run_id、投影 Item。
- [ ] **S2.11** Item version 未变化时投影同名 terminal；已升版时不写旧 result/closedBy，清 ownership并恢复 queued/not_ready。
- [ ] **S2.12** 重复/延迟/乱序 Finish 返回既有 terminal Run，不重复 evidence、事件或 completion gate。
- [ ] **S2.13** 禁止 `TaskRunStore.Finish` 关闭带 JobID 的新 execution Run；verification 与 legacy 例外要显式。
- [ ] **S2.14** 删除 `runner.finish` 之后第二次 `TasksStore.Mutate` 的 execution terminal 写入。
- [ ] **S2.15** 增加最小 Run detail API：Run、snapshot、Turn count、first/final Turn summary/finalAnswer、completion reason。
- [ ] **S2.16** Run now HTTP 要求/生成 client request id；同 id 返回同 Run，不同 id 遇 active Run 返回 `409 overlap_forbidden`。
- [ ] **S2.17** 在启动恢复入口先识别 automation-bound Turn：它们不得进入旧 `RecoverInterrupted`；Slice 2 的最小策略是把无法恢复的 Run/Turn 原子收敛为 `worker_lost`，不能留 running。
- [ ] **S2.18** 只有 S2.17 生效后，feature gate 才可对 manual Run now 小流量开启；旧 verification 不切换。
- [ ] **S2.19** 实现 VerificationCoordinator：按冻结 policy 串行创建 round/attempt并推进 step cursor。
- [ ] **S2.20** 无 policy 时显式 Finish `runtime_completed_no_verification`；不能把“空 policy”解释为隐式 agent 自报。
- [ ] **S2.21** 实现 command verifier adapter：严格按 step ordinal逐条执行冻结的 script/cwd/timeout，保存 exit code、8 KiB output tail、artifact ref；命令执行必须在数据库事务外。
- [ ] **S2.22** 实现 agent_review adapter，迁移现有结构化 criteria verdict、panel threshold 与 needs-human 能力，但结果写入当前 Run attempt。
- [ ] **S2.23** 实现有序人工核验 request/decision API：V1 解析并冻结 item_author，只有当前节点可 approved/changes_requested/rejected_final；后两者 feedback 必填，actor 由服务端身份派生。
- [ ] **S2.24** machine failed / human changes_requested 使用稳定 request id 生成 server-authored feedback并 Append 新 Turn到同一 Session/Run。
- [ ] **S2.25** 新 repair Turn append 后设置 `phase=repairing`；其 terminal hook 创建新 round并从 step 1 重跑。
- [ ] **S2.26** 达到 maxRepairRounds 以 `verification_exhausted` 失败；人工终止以 `human_rejected` 失败；infra failure 使用独立原因。
- [ ] **S2.27** 全部脚本和全部人工节点依序 passed 才允许 `FinishRun(verification_passed)`，并把 finalTurn 指向最后一次产生交付的执行/修复 Turn，不指向核验 attempt。
- [ ] **S2.28** 旧 verification TaskRun 仅做历史/非自动任务兼容；带 JobID 的新 execution Run不得创建 child verification TaskRun或直接改 Item pending_review。
- [ ] **S2.29** Run detail API 增加 phase、round、当前 step、attempt history和 pending human request；finalAnswer 仍来自 finalTurn。

Slice 2 内部按三个 gate 顺序开启：

1. S2a lifecycle：Start/Append/Finish + 无核验路径。
2. S2b machine verification：command/agent review + 自动修复回灌。
3. S2c human verification：人工等待、决定与反馈回灌。

合并门：manual happy path、失败、重复 Finish、多 Turn、Item 升版、后续人工 Turn 隔离、机器失败修复、人工驳回修复全部通过；五条夹具中的手动配方复测成功。

回滚：关闭新写 gate，确认无 running 新 Run，再恢复旧 dispatcher；additive schema 保留。若已有新 Run，必须先由新二进制收敛，禁止直接降级。

### Slice 3：重启对账与 one-shot `at`

目标：自动触发和进程重启后仍有唯一可解释终态。

- [ ] **S3.1** 启动顺序固定为：RunCoordinator 先查询/标记 automation-bound Turns，再由通用 `RecoverInterrupted` 处理剩余普通 Turns。
- [ ] **S3.2** 启动时查询所有新协议 `status='running'` execution Runs，按 phase 逐个 reconcile；替换 Slice 2 的保守 `worker_lost` 路径。
- [ ] **S3.3** Turn 已 terminal、Run 未 terminal：重放持久 `verify_then_finish_v1` hook，幂等恢复直接 Finish、verification round或repair Turn。
- [ ] **S3.4** runtime 明确 active：用同一 runtime/session id 恢复监控，不创建新 runtime/Turn。
- [ ] **S3.5** runtime unknown/bridge 超时：在确定窗口后以 `worker_lost` 失败，禁止无限 running。
- [ ] **S3.6** DB 已建 Run、ACP 启动结果未落库：先证明 `createOrGetRuntime(invocationKey)`/等价 ensure-session 幂等；不能证明则 `startup_outcome_unknown`，不得自动重复启动。
- [ ] **S3.6a** `phase=verifying`：已 passed attempt 从下一步继续；running command 仅在 safeToRetry 时重放，否则转 awaiting_human/outcome_unknown。
- [ ] **S3.6b** `phase=awaiting_human`：保留同一个 pending request，不得判 worker_lost或重复创建请求。
- [ ] **S3.6c** `phase=repairing`：按稳定 request id 查询/创建唯一 repair Turn，不重复注入反馈。
- [ ] **S3.7** 为 at Trigger 增加最小 occurrence disposition 字段和迁移。
- [ ] **S3.8** 实现 `StartAt(triggerID, scheduledFor)`：消费 occurrence、判断 ownership、创建 Run或写 `skipped_overlap` 均在同一事务。
- [ ] **S3.9** Scheduler 不再 `RunNow` 成功后另一次 Advance Trigger；只调用 Coordinator 的 at occurrence API。
- [ ] **S3.10** 移除 workspace busy 的隐式延时 Trigger；manual=409，at=持久 skipped/exhausted。
- [ ] **S3.11** scheduler 重启/重复 tick 复用稳定 occurrence key，返回既有 disposition。
- [ ] **S3.12** 增加 migration reconciliation 命令：有唯一 runtime evidence 才自动解除 legacy_unresolved，否则要求人工决定并写审计事件。
- [ ] **S3.13** 完成 downgrade preflight：存在新协议 running Run 或未识别状态时阻止旧二进制启动写路径。
- [ ] **S3.14** 使用两个独立 gate/commit：先开启 recovery 并完成 kill-point 验证，再开启 one-shot at；recurrence 继续 paused/禁用。

合并门：在 runtime 启动前、Turn running、Turn terminal 后、机器 attempt 后、人工待决和反馈注入前后杀进程，均不永久 running、不重复 runtime/核验/repair Turn；at 正常触发一次，overlap 只 skipped 一次。

回滚：只承诺“关闭 gate + drain 新 Run + 受控降级/roll-forward”，不承诺任意在线回旧二进制。先关闭 scheduler start gate并收敛所有新 Run；Trigger disposition 数据保留只读，skipped occurrence 永不重新 armed。

## 8. API 与错误契约

建议的 Phase 1 最小接口：

```text
POST /api/execution-jobs/{jobId}/run
  headers/body: clientRequestId (required for retry-safe clients)
  202: { disposition: "started", runId }
  200: { disposition: "existing", runId }
  409: { code: "overlap_forbidden", activeRunId }

GET /api/execution-jobs/{jobId}/runs/{runId}
  -> status, completionReason, itemVersion, inputSnapshot,
     phase, verificationRound/currentStep/attempts/pendingHumanRequest,
     turnCount, firstTurn, finalTurn, finalAnswer, timestamps

POST /api/execution-jobs/{jobId}/runs/{runId}/verification/{attemptId}/decision
  body: { decision: "approved" | "changes_requested" | "rejected_final", feedback? }
  200: updated verification state / repairTurnId
  409: attempt no longer awaiting human, stale decision, or Run terminal
```

内部错误必须是可 `errors.Is/As` 的 typed errors，HTTP 映射只在 handler：

- `ErrOverlapForbidden`
- `ErrVersionConflict`
- `ErrRunNotRunning`
- `ErrFinalTurnMismatch`
- `ErrExpectedLastTurnMismatch`
- `ErrLegacyUnresolved`
- `ErrRuntimeUnknown`
- `ErrVerificationStepMismatch`
- `ErrVerificationDecisionConflict`
- `ErrVerificationBudgetExhausted`
- `ErrVerifierInfrastructure`
- `ErrVerificationActorUnresolved`
- `ErrVerificationActorMismatch`

日志至少携带：`job_id`、`task_run_id`、`task_id`、`session_id`、`turn_id`、`occurrence_key`、`item_version`、`phase`、`verification_round`、`verification_step_id`、`completion_reason`。不要把 prompt、凭据、人工反馈、command output 或完整 finalAnswer 打进结构化日志。

## 9. 测试计划

### 9.1 覆盖图

```text
                         +-- existing request id --> existing Run
Manual Run now ----------+
                         +-- active Item ----------> 409 overlap
                         |
                         +-- Start tx --> snapshot + Run + active ownership
                                              |
At due --> occurrence tx +--------------------+
   |          |
   |          +-- active Item --> skipped_overlap + exhausted
   |          +-- replay ------> existing disposition
   v
ACP dispatch --> Turn #1 terminal --> TurnTerminalHook
                       |                 |
                       |                 +-- no policy --> Finish
                       |                 |
                       |                 +-- verification round
                       |                       |
                       |                       +-- machine pass --> next step
                       |                       +-- machine fail --+
                       |                       |                   |
                       |                       +-- human changes --+--> feedback
                       |                       |                         --> Turn #2..N
                       |                       +-- all pass --> Finish
                       |
                       +-- process crash --> phase-aware reconciler --> same decision

Session after Run terminal --> ordinary/unbound Turn (never appended to old Run)
```

### 9.2 必测矩阵

| ID | 场景 | 期望 | 层级 |
| --- | --- | --- | --- |
| T01 | ProjectItem 初始/契约/runtime 字段更新 | version 仅按契约递增；expectedVersion 冲突 | meta unit |
| T02 | Item/Job 在 Run 后修改 | snapshot bytes/hash 不变 | execution unit |
| T03 | manual 与 at 同输入 | snapshot 的业务/执行部分一致，仅 trigger provenance 不同 | execution unit |
| T04 | Run append 3 Turns | seq=1..3，旧 terminal 内容不变 | meta transaction |
| T05 | 注入 seq/update/insert 任一步失败 | 全事务回滚，无空洞/半记录 | meta fault injection |
| T06 | `turn_terminal` 到达 | Turn terminal，Run/Item 仍 running | agent regression P0 |
| T07 | 显式 Finish | Run/Item 同事务 terminal，finalTurn 正确 | integration P0 |
| T08 | duplicate/late/out-of-order Finish | 返回既有结果，不重复 event/gate | integration |
| T09 | finalTurn 属于别的 Run或非 terminal | Finish 被拒绝 | execution unit |
| T10 | Run 内 Item 升版 | 旧 Run terminal；新 Item 不被旧结果关闭 | integration P0 |
| T11 | Run 后人工继续 Session | 新 Turn 不绑定历史 Run | agent integration |
| T12 | 同 manual request id HTTP 重试 | 返回相同 Run | HTTP integration |
| T13 | active Run + 不同 manual id | 409，不排队、不改 Trigger | HTTP integration |
| T14 | at 正常到期 | 一次 Run + Trigger exhausted/started | scheduler integration |
| T15 | at 到期但 Item active | 无 Run；skipped_overlap 持久化并 exhausted | scheduler regression P0 |
| T16 | 重启后重复 scheduler tick | 返回已有 disposition，不补跑 | scheduler integration |
| T17 | Turn terminal 后、Finish 前杀进程 | recovery 重驱动 policy，唯一 terminal | process integration P0 |
| T18 | runtime active 时重启 | reconnect/resume，不创建第二 runtime/Turn | ACP integration |
| T19 | runtime unknown/bridge timeout | Run/Item failed(worker_lost)，无永久 running | recovery integration |
| T20 | ACP 已启动、DB runtime 回写前崩溃 | create-or-get 同 runtime；不支持则 startup_outcome_unknown | ACP contract P0 |
| T21 | 历史单一/多个 running Run 迁移 | 单一 ownership；冲突隔离且禁止 Start | migration integration |
| T22 | legacy verification TaskRun | 历史/非自动任务兼容；新 automation Run 不创建 child verification Run | regression |
| T23 | 竞态启动两个 Run | partial unique/CAS 只允许一个 active | concurrency |
| T24 | stale expectedLastTurn | Continue/Finish/Append 被拒绝 | concurrency |
| T25 | Run detail | 无 Session 推断即可读取 snapshot、finalAnswer、reason | HTTP integration |
| T26 | ProjectItem policy 为空/缺失 | snapshot 保存 null；Turn 成功后直接完成 | meta/execution |
| T27 | ProjectItem policy 更新 | version++；active Run policy/hash 不变 | meta/execution |
| T28 | legacy Verifier 且无新 policy | 合成一个 agent_review step；有新 policy 时不重复核验 | compatibility |
| T29 | command step exit 0 | attempt passed并进入下一步 | verifier unit |
| T30 | `yarn build` exit nonzero | attempt failed；错误被结构化注入唯一 repair Turn | integration P0 |
| T31 | command 启动失败/infra timeout | infrastructure_error，不伪装为代码失败 | verifier unit |
| T32 | repair Turn terminal | 新 round 从 step 1 开始；上一 round历史不覆盖 | integration P0 |
| T33 | 多个 command/agent/human steps | 严格按 ordinal 串行；全 passed 才 Finish | execution integration |
| T34 | 人工 approved | step passed并继续；重复同决定幂等 | HTTP integration |
| T35 | 人工 changes_requested | feedback 必填；创建唯一 repair Turn | HTTP/agent P0 |
| T36 | 人工 rejected_final | Run/Item failed(human_rejected)，不再创建 Turn | HTTP integration |
| T37 | repair budget exhausted | Run failed(verification_exhausted)，无自动重排 | execution unit |
| T38 | 重复/乱序 terminal hook | 同 trigger_turn 只有一个 round，不重复命令/反馈 Turn | concurrency P0 |
| T39 | verifying/awaiting_human/repairing 时重启 | 从持久 phase恢复，不误判 worker_lost | recovery P0 |
| T40 | command output 含秘密/超长/控制文本 | 只存截断安全块/引用，不进入 system instruction或日志 | security |
| T41 | 三条 command scripts | 严格依序；第2条失败时第3条不执行，修复后从第1条重跑 | integration P0 |
| T42 | author → user-li → user-wang | 只有当前人 approved 后才创建下一人的待办 | human workflow |
| T43 | 后续人提前审批/他人代签/重复决定 | 403/409，不推进 cursor，不覆盖 actor audit | authorization P0 |
| T44 | user-li changes_requested | 创建 repair Turn；新 round要求 scripts和author重新通过 | integration P0 |
| T45 | item_author/user selector 无法解析 | StartRun 拒绝为 not_ready，不跳过人工节点 | contract |

推荐验证命令：

```bash
cd backend && go test ./internal/meta ./internal/execution ./internal/agent
cd backend && go test -race ./internal/meta ./internal/execution ./internal/agent
cd backend && go test ./...
cd frontend && yarn check
```

涉及真实 ACP 的进程恢复测试可单独 gated，但必须在 Slice 3 合并前由 CI 或本地受控环境执行；不能只靠 mock 宣称 crash-safe。

## 10. 失败模式与可见性

| 失败模式 | 防护 | 测试 | 用户看到什么 |
| --- | --- | --- | --- |
| 两个 Start 并发取得同一 Item | active_run_id CAS + partial unique index | T23 | 第二个收到 overlap，不静默 |
| Turn terminal 后进程崩溃 | 持久 Turn + runner policy recovery | T17 | Run 最终 completed/failed，带 reconciled 时间 |
| ACP 已启动但 runtime id 未回写 | stable invocation/session create-or-get | T20 | 无法证明时明确 startup_outcome_unknown |
| 旧 Run 晚到 Finish | active_run_id + itemVersion CAS | T10/T24 | 历史 Run终结，新 Item 不被覆盖 |
| at overlap | occurrence disposition 与 Trigger exhausted 同事务 | T15/T16 | Runs/Trigger 显示 skipped_overlap |
| Session 后续人工对话 | Turn 显式 task_run_id | T11 | 历史 Run Turn 数不变 |
| 迁移发现多个历史 running | legacy_unresolved gate | T21 | Item 显示需对账，不能继续 Start |
| SQLite busy | 5s busy timeout + 短事务 + typed error | concurrency tests | 明确可重试错误，不伪装 accepted |
| final Turn 错绑 | membership + terminal guard | T09 | Finish 拒绝并记录诊断 |
| legacy verification 与新 policy 双跑 | new-policy-wins compatibility gate | T22/T28 | 明确弃用提示，不创建第二个 Run |
| terminal hook 重放 | unique trigger_turn round + stable repair request id | T38 | 返回既有 round/Turn，不重复命令或反馈 |
| build 命令失败 | 退出码事实 + bounded output feedback | T30 | Run 显示 repairing，Session 收到修复 Turn |
| verifier 基础设施故障 | 与业务失败分型、有限 infra retry | T31 | 显示 verification_infrastructure_error |
| 人工核验长期待决 | 持久 awaiting_human request | T34/T39 | 明确“等待人工”，不显示“仍在执行” |
| 人工重复/迟到决定 | attempt status CAS + actor audit | T34–T36 | 返回 conflict 和当前决定 |
| 人工跳级或代签 | current-step + resolved-actor guard | T42/T43 | 403/409，下一核验人不解锁 |
| 多级签核中途要求修改 | 新 round重置全部旧 pass | T44 | 历史批准保留但标为旧 round，不继续算通过 |
| 核验人无法解析 | StartRun contract validation | T45 | Item not_ready并指出具体 step |
| 修复后旧通过结果被误复用 | 新 round 从 step 1 重跑 | T32/T33 | UI 保留旧 round但标为历史 |
| command output 注入/泄密 | 截断、脱敏、不可信数据块、artifact ref | T40 | UI按日志展示，不作为 system instruction |

当前无“无测试 + 无错误处理 + 静默失败”的已接受路径；若实现引入此类路径，视为 P0 阻塞合并。

## 11. 性能与并发预算

- Start/Finish/AppendTurn 事务内只允许 SQLite 读写；禁止 ACP、Profile 网络解析、Function、文件扫描。
- command/agent/human 核验都在数据库事务外执行或等待；事务只做 attempt claim/result CAS 和 step advance。
- 同一 Run 同时最多一个 running/awaiting verification attempt，避免重复 build和并行人工请求。
- Profile 与 prompt 解析中需要外部访问的部分在事务前准备；事务内重新校验稳定引用和 Item/Job 当前值。
- Reconciler 使用 `task_runs(kind,status,updated_at)` 索引，只扫描非终态 execution Runs。
- Turns 使用 `(task_run_id, run_seq)` 索引；Run detail 首屏最多读取 first/final Turn，不加载完整 journal。
- Verification 使用 `(task_run_id, round_no)` 与 `(round_id, step_ordinal)` 索引；首屏只加载当前 round和各历史 round摘要。
- at 调度使用现有 `(status,next_run_at)` 索引，消费每个 occurrence 为一个短事务。
- SQLite 保持 WAL、`BEGIN IMMEDIATE`、busy timeout；记录 start/finish tx duration 和 SQLITE_BUSY 计数。
- Phase 1 性能门槛：在 1,000 个历史 Runs / 10,000 Turns / 20,000 verification attempts 的夹具下，Run detail P95 < 200 ms；空闲 scheduler tick P95 < 50 ms；单个 Finish/step-advance 事务 P95 < 100 ms（开发机基线，CI 记录趋势，不作为跨机器绝对 SLA）。命令耗时单独展示，不计入数据库事务指标。

## 12. 发布、迁移与回滚

### 发布顺序

1. 合并 Slice 0，冻结回归证据，处理当前脏 lifecycle 改动。
2. 合并 Slice 1，feature gate 关闭，跑旧库迁移与回填审计。
3. 暂停 Automation Start，处理 `legacy_unresolved`，确认每个 Item 至多一个 running execution Run。
4. 合并 Slice 2a，先开启 manual 无核验 Run now，小流量复测。
5. 依次开启 Slice 2b 机器核验和 Slice 2c 人工核验；各自完成失败回灌、重复 hook和重启前置测试。
6. 合并 Slice 3，先开启 phase-aware restart reconciler，再开启 one-shot at。
7. 五条狗粮配方全量复测；recurrence 和 Function→ACP 仍保持 paused/禁用。

### 每次启用前的数据库断言

- terminal TaskRun 必有 completed_at 和 completion_reason。
- running execution Run 与 ProjectItem.active_run_id 双向一致。
- 每个 Item 至多一个 running execution Run。
- 绑定 Run 的 Turn 具有唯一正整数 run_seq。
- terminal Run 不存在 queued/running 的绑定 Turn。
- `phase=verifying/repairing` 至多一个 running verification attempt；`awaiting_human` 恰有一个 pending human attempt。
- completed Run 的 verification policy 若非空，则最后一个 round 必须 passed。
- 每个 repair Turn 都能追溯到唯一 failed/changes-requested attempt。
- exhausted at Trigger 对最近 occurrence 有 disposition。

### 回滚纪律

- 关闭新写 gate后再回滚二进制。
- 先让新协议 running Runs 由新二进制全部收敛。
- additive schema 保留；不做在线 drop column/table。
- skipped at occurrence 永不重新 armed。
- downgrade preflight 不通过时禁止旧版本启动 scheduler/runner。

## 13. 跟踪台账

### 13.1 里程碑

| 里程碑 | 状态 | Owner | Branch / PR | 开始 | 完成 | 验收证据 |
| --- | --- | --- | --- | --- | --- | --- |
| M0 回归基线与冲突清理 | not_started |  |  |  |  |  |
| M1 数据契约与迁移 | not_started |  |  |  |  |  |
| M2a Manual RunCoordinator / no verification | not_started |  |  |  |  |  |
| M2b Machine verification + repair loop | not_started |  |  |  |  |  |
| M2c Human verification + repair loop | not_started |  |  |  |  |  |
| M3a Restart recovery | not_started |  |  |  |  |  |
| M3b one-shot at | not_started |  |  |  |  |  |  |
| M4 五配方复测与 Phase 1 关闭 | not_started |  |  |  |  |  |

状态只使用：`not_started | in_progress | blocked | verifying | completed | rolled_back`。

### 13.2 每次合并必须附的证据

- [ ] 对应 task ID 和本文件 checkbox 已更新。
- [ ] 迁移前后 schema/user_version 与 invariant query 输出。
- [ ] 新增/修改测试名与运行命令。
- [ ] 一条成功路径和至少一条失败/竞态路径的 Run detail JSON。
- [ ] 若涉及 recovery，记录 kill point、恢复日志和最终数据库状态。
- [ ] 若涉及 verification，记录 policy snapshot、round/attempt序列、失败回灌 Turn和最终 completion reason。
- [ ] 人工决定保留 actor/time/feedback 审计，但测试和日志中不泄露反馈正文。
- [ ] 若涉及 at，记录 occurrence key、disposition 与重放结果。
- [ ] 未提交用户改动未被覆盖的 `git diff` 审核结论。

### 13.3 决策记录模板

| 日期 | Decision ID | 变更 | 原因/证据 | 影响切片 | 决策人 |
| --- | --- | --- | --- | --- | --- |
| 2026-08-14 | D1–D10 | 建立本实施基线 | 狗粮结果 + 工程评审 | S0–S3 | user + Codex |

新增决策时必须说明它是否改变：状态机、数据库不变量、回滚方式、测试门槛。

### 13.4 切片复盘模板

每个 Slice 合并并运行至少 24 小时后补一段：

```text
Slice:
预期：
实际：
新增/修改行数：
测试耗时与失败次数：
发现的状态分叉：
线上/狗粮 Run 数与终态分布：
发生过的 recovery / overlap：
哪些假设被证伪：
下一切片必须调整什么：
是否满足退出条件：是 / 否（证据链接）
```

## 14. 后续阶段闸门

只有 M4 完成后才进入：

- Phase 2：Runs UI、Evidence、Instructions 保护、显式 retry。
- Phase 3：recurrence、invocation ledger、misfire、lease fencing。
- Phase 4：Function→ACP stages、跨阶段 recovery、outcome_unknown。
- Phase 5：自动规划与任务调度规划接入。

任何上层能力接入前都必须只依赖以下稳定契约：

```text
Planner updates ProjectItem.version
Scheduler chooses current Job/Trigger
Automation freezes RunInputSnapshotV1
RunCoordinator executes, verifies and closes one TaskRun
Consumers read TaskRun/Turns, never infer completion from Session
```

## 15. Implementation Tasks

下面是适合逐项关闭的扁平任务索引；详细验收条件见对应 Slice。

- [ ] **T1 (P1)** — preflight — 隔离当前 lifecycle 冲突改动、确认单 writer并固化五配方夹具（S0.1–S0.8）
- [ ] **T2 (P1)** — meta — 落 ProjectItem version/active ownership/migration state（S1.1–S1.3）
- [ ] **T3 (P1)** — execution — 落 typed snapshot 与同字节 SHA-256（S1.4–S1.5）
- [ ] **T4 (P1)** — meta/execution — 扩展 TaskRun/AgentTurn 关系、事务型 Turn append与原子 snapshot Run 原语（S1.6–S1.13）
- [ ] **T5 (P1)** — migration — 回填 legacy Turn 并隔离冲突 active Runs（S1.10–S1.12）
- [ ] **T6 (P1)** — execution — 实现 RunCoordinator StartManual 与 dispatch contract（S2.1–S2.5）
- [ ] **T7 (P1)** — agent — 将 Turn terminal 与 Run terminal 解耦（S2.6–S2.7）
- [ ] **T8 (P1)** — execution/meta — 实现原子 FinishRun 与 Item version/ownership 投影（S2.8–S2.14）
- [ ] **T9 (P2)** — HTTP/read model — 提供 request-id 幂等与最小 Run detail API（S2.15–S2.16）
- [ ] **T10 (P1)** — recovery — Slice 2 先分流 automation Turn并保守收敛，Slice 3 再实现 Run reconciliation（S2.17、S3.1–S3.6）
- [ ] **T11 (P1)** — scheduler — 原子消费 at occurrence并持久化 disposition（S3.7–S3.11）
- [ ] **T12 (P1)** — migration/ops — 实现 legacy reconciliation 和 downgrade preflight（S3.12–S3.13）
- [ ] **T13 (P1)** — QA — 完成 T01–T45、race、真实 ACP/verification kill-point 测试与五配方复测
- [ ] **T14 (P2)** — observability — 增加 lifecycle 结构化日志、事务耗时和 invariant audit
- [ ] **T15 (P2)** — docs — 修正上游设计文档错误的 `Supersedes` lineage，并在每个 Slice 后更新本台账/复盘
- [ ] **T16 (P1)** — project model — 增加可选 VerificationPolicy、version语义与 legacy Verifier兼容投影（S1.3a、S1.15）
- [ ] **T17 (P1)** — execution/meta — 增加 verification rounds/attempts、phase和幂等 TurnTerminalHook（S1.14、S2.7、S2.19–S2.20）
- [ ] **T18 (P1)** — verification — 实现 command/agent-review 核验、失败分型和自动 repair Turn（S2.21–S2.22、S2.24–S2.27）
- [ ] **T19 (P1)** — human verification — 实现 item_author 单节点和未来多人的有序签核、三类决定、actor审计与反馈 repair Turn（S2.23–S2.27）
- [ ] **T20 (P1)** — recovery/QA — 完成 verification phase恢复、T26–T45、命令输出安全与逐级签核授权测试（S3.6a–S3.6c）

## 16. Completion Summary

- Step 0 Scope Challenge：按建议缩小；Phase 1 拆成三纵切片，Stage/Evidence/ledger/lease 后移。
- Architecture Review：6 个关键问题已折入边界、状态机、恢复、overlap和同 Run verification loop；独立复核补强 atomic snapshot、恢复分流与受控回滚。
- Code Quality Review：5 个关键问题已折入唯一写入口、typed errors、legacy verification gate、append-only attempts 与 transaction boundary。
- Test Review：已产出覆盖图和 45 项矩阵；`turn_terminal`、脚本顺序、逐级人工签核、verification repair、at overlap、crash window 为 P0 regression。
- Performance Review：2 个风险已折入短事务约束、索引和基线指标。
- NOT in scope：已写明。
- What already exists：已写明复用和替换路径。
- TODOS.md：项目无该文件；不新建平行 backlog，全部跟踪项保存在本实施清单。
- Failure modes：0 个被接受的静默关键缺口。
- Outside Voice：独立代理已复核；无方向性 tension，atomic snapshot、最小恢复分流、occurrence 事实和受控回滚已折入。
- Parallelization：核心切片共享 meta/execution/agent lifecycle，采用顺序合并；测试夹具和文档可伴随开发但不单独抢占 lifecycle 文件。

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
| --- | --- | --- | --- | --- | --- |
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | 当前范围已由用户主动收缩 |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | 未运行 diff review |
| Outside Voice | `/plan-eng-review` fallback | Independent plan challenge | 1 | CLEAR | 无方向性冲突；补强切片启用顺序与回滚约束 |
| Eng Review | `/plan-eng-review` | Architecture & tests | 2 | CLEAR | Phase 1 保持三切片；新增同 Run脚本核验、逐级人工签核与45项测试门禁 |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | — | Phase 1 为后端内核，不阻塞 |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | 未运行 |

**VERDICT:** ENG CLEARED — 可以按 Slice 0 → 1 → 2 → 3 顺序实施；Phase 2+ 仍受闸门约束。

NO UNRESOLVED DECISIONS
