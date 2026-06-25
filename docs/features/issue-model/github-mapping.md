# Task ↔ GitHub Issue/PR 字段映射对照

> 本文档是 [issue #74] 的正式契约：把 1agents 的 task model 字段对齐到 GitHub
> Issue/PR，并定义同步引擎所需的映射规则。**本阶段只做字段/契约对齐，不实现同步
> 引擎**（创建/更新 Issue、webhook、冲突解决、Projects v2 写入、PR 流程、鉴权均为
> 后续 follow-up）。

## 1. 字段对齐矩阵

| 1agents task 字段 | GitHub Issue/PR 对应 | 归类 | 同步处理约定 |
|---|---|---|---|
| `title` | `issue.title` | Issue 原生 | 直接映射 |
| `description` | `issue.body` | Issue 原生 | 直接映射（可把 `acceptanceCriteria` 拼成任务清单追加到 body）|
| `issueState` (open/closed) | `state` (open/closed) | Issue 原生 | 双向同步 |
| `number` (#N, per-project) | `issue.number` (per-repo) | 命名空间不同 | 本地 #N 保留；远端编号单独存 `githubNumber` |
| `type` (task/requirement/bug) | Issue Types / label | Issue 原生 | 映射到 GitHub Issue Types 或约定 label |
| `labels` (自由文本[]) | `labels[].name` (repo 预定义) | Issue 原生 | 缺失 label 自动创建或忽略（同步引擎约定）|
| `milestone` (string) | `milestone` (title) | Issue 原生 | 按 title 匹配/创建 |
| `links` (closes/relates) | `Closes #N` / linked issues | Issue 原生 | closes→正文关键字；relates→交叉引用 |
| `replies` (时间线) | issue comments | Issue 原生 | 双向同步评论 |
| `assignee`（**执行 agent**：claudecode/codex） | — | **本地专属** | **不映射到 GitHub**；这是执行任务的 AI agent 类型，与 GitHub 用户无关 |
| `githubAssignees []string` | `assignees[].login`（**GitHub 用户**） | Issue 原生 | 人类协作者登录名；与 `assignee` 正交 |
| `priority` (urgent/high/medium/low) | 无原生 | **Projects v2** | 映射到 Projects v2 自定义字段，或约定 `priority:*` label |
| `status`（执行态 pending/queued/running…） | 无原生 | **Projects v2** | 本地执行态保持本地；可选映射 Projects v2 Status 字段 |
| `sprint` | Projects v2 Iteration | **Projects v2** | Projects v2 字段（非 Issue 字段）|
| `plannedStart` / `plannedEnd` | Projects v2 Date 字段 | **Projects v2** | Projects v2 字段 |
| `dependsOn` / `parentId` | sub-issues / "blocked by" | 待定 | 映射到 GitHub sub-issues / dependencies（新功能）|
| `githubRepo` / `githubKind` / `githubNumber` / `githubNodeId` / `githubUrl` / `githubState` / `lastSyncedAt` | repo / issue\|pr / number / node_id / html_url / state / — | **同步锚点** | 见 §3 |

**结论**：
- **Issue 原生字段**（title/body/state/labels/milestone/assignees/comments/type/sub-issues）可直接走 Issue REST/GraphQL API。
- **PM 专属字段**（priority/sprint/status/计划日期）是 GitHub **Projects v2 自定义字段**，必须走 Projects v2 GraphQL API，**不是** Issue 字段。

## 2. assignee 的语义拆分（关键）

GitHub 的 `assignees` 是**人类用户登录名**；1agents 的 `assignee` 历史上是**执行任务的
AI agent 类型**（claudecode/codex/…）。二者语义冲突，因此拆成两个正交字段：

| 字段 | 含义 | 是否同步到 GitHub |
|---|---|---|
| `assignee` | 执行 agent type（claudecode/codex），驱动调度执行 | 否（本地专属） |
| `githubAssignees` | GitHub 登录名数组（人类协作者） | 是（`issue.assignees[].login`） |

前端表单、MCP `create_task` / `update_task` schema、工具描述均明确区分这两者。

## 3. 同步锚点字段（schema v12，#74 新增）

每个 task 最多绑定一个 GitHub 对象。这些字段是绑定关系的锚点，**通常由后续的同步
引擎回填**，在 task 创建/编辑界面里作为只读展示（可选预留入参）：

| 字段 | 类型 | 含义 |
|---|---|---|
| `githubRepo` | string | `owner/repo` |
| `githubKind` | `issue` \| `pr` | 绑定对象类型 |
| `githubNumber` | int | 远端 #N（per-repo），区别于本地 `number` |
| `githubNodeId` | string | GraphQL global node id（Projects v2 API 需要） |
| `githubUrl` | string | `html_url` |
| `githubState` | string | 远端 open/closed（用于冲突检测） |
| `lastSyncedAt` | timestamp | 最近一次成功同步时间（nil = 从未同步） |

## 4. 落地位置

- **数据模型**：`backend/internal/meta/types.go`（`Task` struct）、`backend/internal/meta/db.go`
  （`ensureTasksColumns` 幂等加列，schema v12）、`backend/internal/meta/tasks.go`
  （`taskCols` / `scanTask` / `upsertTaskTx`）。
- **HTTP**：`backend/internal/agent/handler.go`（POST / PATCH `/api/agent/tasks` body）。
- **MCP**：`backend/internal/mcptasks/tools.go`（`create_task` / `update_task` schema + 描述）。
- **前端**：`frontend/src/components/drawer/TaskList/types.ts`（`Task` 接口）。

## 5. 迁移说明

schema v12 通过 `ensureTasksColumns`（**无版本门控、幂等**）新增 8 个 task 列，沿用
#225 修复 v9 分支碰撞时确立的方式：即使 DB 的 `user_version` 已经被抬到最新但缺列，
重开时也会被自愈补齐。所有新列对旧行默认空值（文本 `''`、整数 `0`、`github_assignees`
默认 `'[]'`、`last_synced_at` 可空），已有 meta.db 平滑升级，无数据丢失。

[issue #74]: https://github.com/scottzx/1agents/issues/74
