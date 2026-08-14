# Automation 实施基线（轻量两段管线）

> **状态：实施中（Job preamble / core.script / 两段调度 / 配方台入口已落地）**
>
> **日期：2026-08-13**
>
> **产品面：** 自动任务 / Automation = 配方台
> **术语权威：** [名称定义表](../product/名称定义表.md)
> **执行对象边界：** [ExecutionJob × AgentProfile 运行架构](./execution-job-agent-profile-architecture.md)
>
> 本文是下一轮编码的唯一规格。未写进本文的能力（n8n 画布、第三段脚本、Email 通知、无 ProjectItem 的后台 Job）一律不做。

---

## 0. 已锁定前提

1. Automation 是配方台，不是跨壳 inbox。
2. 编排层最多两段。第三段写进那一个 Python 脚本，不抬到 Job。
3. 一条配方 = 一条 `ProjectItem(task)` + 一条 `ExecutionJob`。Function 是 preamble，不是第二条 Job。
4. Function 失败则 ACP 不启动。
5. 三种合法形态：只 ACP、Function → ACP、只 Function。
6. Function → ACP 的注入格式只有一块 `function_context` JSON。

---

## 1. 一句话模型

```text
Trigger 到期或手动 Run
    → 若 Job.preambleFunctionType 非空：跑 Function，写 preamble TaskRun
        → 失败：停，ACP 不启动
        → 成功：把 result JSON 拼进 Instructions
    → 若终点是 agent：ensure_session(cwd, Profile) + prompt
    → 若终点是 function：现有 function-only 路径，不改语义
    → ACP 之后不再接 Function
```

```text
executorKind = agent | function | human     // 终点，含义不变
preambleFunctionType                        // 仅 agent Job 可填；function/human 必须为空
```

| 形态 | executorKind | preambleFunctionType | 行为 |
|---|---|---|---|
| 只 ACP | `agent` | 空 | 与现在完全一致 |
| Function → ACP | `agent` | 如 `core.script` | 先 Function，再 ACP |
| 只 Function | `function` | 必须空 | 与现在完全一致 |

禁止：`human` 带 preamble；`function` 带 preamble；ACP 后再跑 Function。

---

## 2. 对象怎么改

不新增 Automation 表。配方是产品外壳：

| 配方字段 | 落到 |
|---|---|
| 名称 | `ProjectItem.title` |
| Instructions | `ProjectItem.description` 正文 |
| 项目归属 | `ProjectItem` 所在 Workspace；创建时用户必须选一个项目 |
| PWD | `ExecutionJob.cwd`；空则回退该项目 workspace path |
| Profile | `ExecutionJob.profileId` |
| 终点执行器 | `ExecutionJob.executorKind` |
| 前置 Function | `ExecutionJob.preambleFunctionType` |
| 脚本路径 | `ExecutionJob.capabilities` 中的 `script:<relpath>`；缺省 `automation.py` |
| 何时跑 | `ExecutionTrigger` |
| 每次证据 | `TaskRun`；agent Run 还可通过 `origin_turn_id` 读同一份 Turn Change Report（本轮资产变化） |
| 配方标记 | `ExecutionJob.businessRef = "automation:<itemId>"` |

从 Automation UI 创建的 Job 必须写 `businessRef`。配方列表只展示带此前缀的 Job。看板里给普通 task 配的 Job 仍走现有执行控制面，不出现在配方列表，但可以出现在运行记录。

---

## 3. 后端切片

### 3.1 Job 字段

`kernel_execution_jobs` 增加一列：

```text
preamble_function_type TEXT NOT NULL DEFAULT ''
```

`CREATE TABLE IF NOT EXISTS` 不会给旧库加列。必须像 `task_runs` 一样：**先 reconcile 缺列，再读写**。测试覆盖「旧表打开后自动补列」。

Go / HTTP / CLI：

```text
Job.PreambleFunctionType string   // json: preambleFunctionType
CreateJobInput / UpdateJobInput 同样带该字段
1agents execution create ... --preamble core.script
```

校验：

- `executorKind=agent` 且 preamble 非空 → `Lookup(preamble)` 必须已注册，否则拒绝创建/更新。
- `executorKind!=agent` 且 preamble 非空 → 拒绝。
- 更新终点或 preamble 时 `revision` 递增。

不要复用 `functionType` 表示 preamble。`functionType` 仍只属于 function 终点 Job。

### 3.2 FunctionContext

`RunFunction` 已经拿到 `workspacePath`，但没传给 handler。补上：

```text
FunctionContext.WorkspacePath
FunctionContext.Cwd            // Job.cwd 或 workspacePath
FunctionContext.Script         // 解析后的脚本相对路径，仅 core.script 使用
```

`core.script` 之外的现有 handler 忽略新字段。

Preamble 调用必须走新入口，例如 `RunFunctionPreamble`：

- 创建一条 TaskRun（`kind=execution`，evidence=`function_result` / `function_error`）
- **不得**把 `ProjectItem.status` 写成 completed/failed
- 成功返回 result JSON 字符串；失败返回 error，由调度器把本次 Job 尝试标失败

只 Function 的 Job 继续走现在的 `writeTerminal`（会写 item 终态）。

### 3.3 `core.script`

新注册 Function，不是工作流引擎。

1. 工作目录 = `FunctionContext.Cwd`，必须存在。
2. 脚本 = capabilities 里 `script:<relpath>`，否则 `automation.py`。
3. 路径必须相对 cwd，禁止 `..` 逃出 cwd，禁止绝对路径。
4. 执行：`python3 <script>`，stdin 关闭；超时用 Job.timeoutMinutes，未设则 10 分钟。
5. 退出码非 0 → 失败，stderr 进 error。
6. stdout 必须是一份 JSON（允许前后空白）。解析失败 → 失败。
7. 成功返回该 JSON（Go `any`）。

Python 内部可以有任意多个「节点」。编排层看不见它们。

### 3.4 调度与 prompt

改 `agent.Scheduler.RunExecutionJob`，不要绕过 `execution.Service`：

```text
if job.ExecutorKind == "function":  // 现状
if job.ExecutorKind == "agent" && job.PreambleFunctionType != "":
    result, err := RunFunctionPreamble(...)
    if err: 释放锁、item 保持失败投影、return
    把 result 放到本次执行的 preamble 上下文（不要当 item 终态）
然后 ExecuteExecutionJob
```

`buildTaskInstruction` 若本次有 preamble JSON，追加且只追加：

```text
=== function_context ===
<pretty JSON>
=== end function_context ===
以上 function_context 是触发时由确定性代码生成的事实，不要改写其中的原始字段。
```

其余 project_executor / 验收拼装不变。无 preamble 的 agent Job 字节级行为应保持不变。

ACP 会话参数沿用现状：`ensure_session` 的 `WorkspacePath` 用 `job.Cwd`（空则项目 path），`Launch` 用 Job 绑定的 Profile，`PermissionMode=approve-all`。

一次两段执行写两条 TaskRun：先 function preamble，再 agent。同一 `job_id` / `job_revision`。不要为此新建 Run 表。

---

## 4. 产品面

左侧「定时任务」和「工作聚合」收成一个入口：**自动任务**。

深链：新 tab id `automation`。旧 `reminders` / `aggregate` 打开同一表面，分别落到 `calendar` / `runs`。不要留两个并列一级入口。

| 视图 | 内容 | 来源 |
|---|---|---|
| `recipes`（默认） | 我的配方列表 + 建议模板 + 新建 | 对齐截图 2 |
| `editor` | 名称、Trigger、Instructions、可选前置 Function、Profile、PWD | 对齐截图 1 |
| `runs` | 成功/失败/立即执行/最近 Run | 现 `ExecutionOverview` 搬家 |
| `calendar` | 原 Reminders 月历，降为次级 | 现 `RemindersPane` |

编辑页第一期字段：

- 名称、Instructions（textarea）
- Trigger：`at` 或 `recurrence.everyMinutes`（复用现契约，不做 cron UI）
- 前置 Function：关 / `core.script` + 脚本相对路径
- Profile、PWD（目录选择器）
- 保存 = 创建或更新 ProjectItem + Job + 可选 Trigger
- 取消回列表

建议模板只是前端预填 Instructions（Email Auto-Responder / Daily Stock Tracker / Task Extractor），不落后端。

Connectors / Skills / Email 通知：第一期不做。App 侧可观测性靠 `runs` + 自动执行产生的 ChatSession。

样式走 Bento / 现有 semantic token，不要另起一套卡片皮肤。

---

## 5. 明确不做

- n8n 节点画布、多段 DAG、ACP 后置 Function
- 新 Automation 表、无 ProjectItem 的 Job
- 模板语言 / `{{slot}}`
- webhook / event Trigger（仍在执行架构路线图）
- Email 通知、Connectors 托盘
- 改 `1agents task` CLI

---

## 6. 实施顺序

1. Job 列 + Service 校验 + CLI/HTTP + 旧库补列测试
2. `FunctionContext` 扩展 + `RunFunctionPreamble` + `core.script` 测试
3. `RunExecutionJob` 两段调度 + `buildTaskInstruction` 注入测试（含 preamble 失败不启动 ACP）
4. 前端 `automation` 表面：列表 / 编辑 / runs；侧栏合并；旧 tab 别名
5. 日历降级为次级视图

验证门槛与执行架构相同：

```bash
cd backend && go test ./internal/execution ./internal/taskapi ./internal/agent
cd frontend && yarn check
```

涉及 SQLite 缺列、无效 preamble、function-only 不被 preamble 改语义的路径必须有测试。
