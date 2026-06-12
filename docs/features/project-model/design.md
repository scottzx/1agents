# Project Management Layer: SQLite + CLI + Table View

**Status:** Implemented (S0–S3 落地于 2026-06-12；见 walkthrough.md)
**Author:** scottzx + Claude
**Date:** 2026-06-12
**Scope:** `backend/internal/`（新增 store 包 + CLI 子命令）, `html/src/`, 根目录元数据库
**Relation:** 本文档定义 [issue-model](../issue-model/design.md) 的**底座**。issue-model 的数据模型/时间线/回写设计不变，但其存储载体由本文档从 per-workspace JSON 升级为全局 SQLite。

---

## 1. 层级模型（已确认决策）

```
项目 Project（= 一个 workspace 目录，全局管理）
  └── 任务 Task（Jira 式字段 + 自带话题时间线 = issue-model 的 Task）
        ├── Description / Replies[]（话题层，见 issue-model PRD）
        └── 会话 Session（执行层，软关联 taskId）
```

**三层，不是四层**：Task 即话题（Task=Issue），每个 Task 自带一条时间线。issue-model PRD 全部沿用，只在其上加"项目"一层。

### 2026-06-12 确认决策

| # | Dimension | Choice | Notes |
|---|---|---|---|
| 1 | 层级 | **Project → Task(含话题) → Session 三层** | issue-model PRD 不动，加项目层 |
| 2 | 存储 | **全量迁 SQLite，根目录全局库** | 废弃各项目 `.1agents/tasks.json` / `agent-sessions.json`，首次启动自动迁移 |
| 3 | CLI 写入 | **直写 SQLite（WAL）** | `1agents` 二进制加子命令；服务没起也能用 |
| 4 | 落地页主视图 | **多维表格式列表** | 行=任务，列=状态/计划/实际/依赖；看板、时间轴作为后续视图切换加入 |

---

## 2. 存储设计

### 2.1 库位置

```
~/.1agents/meta.db          # 全局元数据库（SQLite, WAL 模式）
```

- 所有项目、任务、回复、会话索引集中一库 → 跨项目查询/统计天然支持
- `PRAGMA journal_mode=WAL` + `PRAGMA busy_timeout=5000` → 服务进程与 CLI 进程并发读写安全
- 驱动选 **`modernc.org/sqlite`（纯 Go，无 cgo）** —— 本项目跨 Mac/Linux 编译打包（见根 Makefile 哲学），免 cgo 交叉编译链

### 2.2 Schema

```sql
CREATE TABLE projects (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    workspace_path TEXT NOT NULL UNIQUE,   -- 项目 ↔ workspace 目录 1:1
    status         TEXT NOT NULL DEFAULT 'active',  -- active | archived
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE tasks (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id),
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',         -- issue-model: Markdown 正文
    issue_state    TEXT NOT NULL DEFAULT 'open',     -- issue-model: open | closed
    status         TEXT NOT NULL DEFAULT 'pending',  -- workflow: pending|queued|running|completed|failed|cancelled|blocked
    schedule_type  TEXT NOT NULL DEFAULT 'immediate',
    scheduled_at   TEXT,
    planned_start  TEXT,                             -- 🆕 计划开始
    planned_end    TEXT,                             -- 🆕 计划完成
    started_at     TEXT,                             -- 实际开始
    completed_at   TEXT,                             -- 实际完成
    summary        TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX idx_tasks_project ON tasks(project_id, status);

CREATE TABLE task_deps (                             -- 前置依赖（原 dependsOn[]）
    task_id        TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, depends_on)
);

CREATE TABLE replies (                               -- issue-model: 话题时间线
    id             TEXT PRIMARY KEY,
    task_id        TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_kind    TEXT NOT NULL,                    -- user | agent
    author_name    TEXT NOT NULL,
    agent_type     TEXT,
    text           TEXT NOT NULL,
    session_ref    TEXT,
    acp_session_id TEXT,
    in_reply_to    TEXT,
    mode           TEXT NOT NULL,                    -- new | follow_up | pure_comment
    created_at     TEXT NOT NULL
);
CREATE INDEX idx_replies_task ON replies(task_id, created_at);

CREATE TABLE sessions (                              -- 原 ChatSessionRecord（agent-sessions.json）
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id),
    task_id        TEXT,                             -- 软关联，可空（issue-model 决策 3）
    name           TEXT NOT NULL,
    agent_type     TEXT NOT NULL,
    cc_session_id  TEXT NOT NULL DEFAULT '',
    acp_session_id TEXT NOT NULL DEFAULT '',
    cc_project     TEXT NOT NULL DEFAULT '',
    session_key    TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    last_event_at  TEXT NOT NULL
);
CREATE INDEX idx_sessions_project ON sessions(project_id, last_event_at DESC);
CREATE INDEX idx_sessions_task ON sessions(task_id);

CREATE TABLE schema_meta (version INTEGER NOT NULL); -- 迁移版本号
```

说明：
- 时间一律 RFC3339 文本（SQLite 惯例，Go `time.Time` 直接序列化）
- `Task.Sessions[]`（原 SessionMetadata 内嵌数组）不再单独建表 —— 由 `sessions.task_id` 反向聚合得到，消除 issue-model PRD 里"两套会话存储"的根源问题。`SessionMetadata.ReplyIDs` 同理由 `replies.session_ref` 反查
- 原 `Task.WorkspacePath` 字段由 `project_id → projects.workspace_path` 取代

### 2.3 迁移（一次性，服务启动时）

1. 启动时若 `meta.db` 不存在 → 建库建表
2. 扫描已注册 workspace 的 `.1agents/tasks.json` / `.1agents/agent-sessions.json`：
   - 每个 workspace 目录 → upsert 一条 `projects` 记录（name = 目录名）
   - tasks / sessions 逐条导入；`dependsOn[]` 拆进 `task_deps`
3. 导入成功后把原 JSON 改名 `*.json.migrated`（保底可回滚，不删）
4. 全部读写路径切到 SQLite store；`TasksStore` / `Store`(chat) 的接口签名尽量保持，内部换实现 —— handler 层改动最小化

---

## 3. Go 后端结构

```
backend/
├── cmd/backend/          # 现有 server 入口
├── internal/
│   ├── agent/            # handler / acpx_client 不变，store 依赖注入替换
│   ├── meta/             # 🆕 SQLite store 包（唯一持库方）
│   │   ├── db.go         # open/migrate/PRAGMA
│   │   ├── projects.go
│   │   ├── tasks.go      # 含 AppendReply / SetIssueState 等 issue-model 方法
│   │   └── sessions.go
│   └── cli/              # 🆕 CLI 子命令实现
```

CLI 与 server 共用 `internal/meta`，各自独立开库连接（WAL 并发安全）。

---

## 4. CLI 设计

`1agents` 二进制加子命令（agent 在会话里也可以直接调它回填任务字段）：

```bash
1agents project list
1agents project add --name foo --path /path/to/workspace

1agents task list   [--project foo] [--status running] [--json]
1agents task add    --project foo --title "优化登录" [--desc ...] [--planned-start ...] [--planned-end ...] [--depends-on <id>,<id>]
1agents task update <id> [--status running] [--planned-end 2026-07-01] [--started-at now] [--completed-at now] ...
1agents task show   <id>            # 含时间线
1agents task close  <id> / reopen <id>
1agents task comment <id> --text "..."   # 写一条 pure_comment 到时间线
```

- 全部直写 `~/.1agents/meta.db`，服务无需在跑
- `--json` 输出供 agent / 脚本消费
- 写操作均更新 `updated_at`；表格 UI 轮询或 WS 通知刷新（首期轮询即可）

---

## 5. API 变化（叠加在 issue-model §7 之上）

```
GET    /api/projects                          🆕 项目列表
POST   /api/projects                          🆕 注册项目
GET    /api/projects/{id}/tasks               🆕 表格视图数据（含 PM 字段 + 依赖）
# issue-model 的 /api/agent/tasks/* 系列保留，按 project 维度查询
```

前端 workspace 切换 ↔ project 切换合一（`workspace_path` 即锚点）。

---

## 6. 前端：落地页 = 任务表格

主视图从看板换成**多维表格式列表**（行 = 任务，列可排序/筛选）：

```
┌──────────────────────────────────────────────────────────────────────┐
│  项目: [1agents ▾]                                    [+ 新建任务]     │
├──────────┬────────┬────┬──────────┬──────────┬──────────┬───────────┤
│ 任务      │ 状态    │🔓/🔒│ 计划开始  │ 计划完成  │ 实际完成   │ 前置依赖   │
├──────────┼────────┼────┼──────────┼──────────┼──────────┼───────────┤
│ 优化登录   │ running │ 🔓 │ 6/10     │ 6/20     │ —        │ #4 ✓      │
│ 修复CI    │ pending │ 🔓 │ 6/15     │ 6/18     │ —        │ 优化登录    │
│ 文档整理   │ done    │ 🔒 │ 6/01     │ 6/05     │ 6/06     │ —         │
└──────────┴────────┴────┴──────────┴──────────┴──────────┴───────────┘
        ↓ 点任务行
   Issue 详情卡（issue-model §10.2：描述 + 时间线 + 回复框）
```

- 行点击 → 打开 issue-model 的任务详情卡（描述 / 时间线 / 回复输入）
- 看板视图、时间轴（甘特）视图：后续迭代以视图切换形式加入，**首期不做**
- 左侧栏会话列表及 📋 徽章：维持 issue-model 设计不变

---

## 6.5 字段模型 v2 + 自动执行（2026-06-12 增补，已实现）

> 用户定位修正：**项目管理是驱动 agent 自动干活的抓手，时间即触发器** —— 前端看板只是人回看/补充的视图，不在执行路径上。对照 Jira/Linear 调研补全字段，并补上自动执行闭环（原 Scheduler 只改状态不执行）。

### 自动执行架构

```
Scheduler（5s tick，时间到 + 依赖满足 + 子任务全完成）
   └─ 按优先级取最优先就绪任务 → 拿 workspace 锁
        └─ TaskRunner（internal/agent/runner.go，无头执行器）
             直连 1acp bridge（ws://127.0.0.1:38082，不经前端）
             → ensure_session（注入 §9 背景 + 验收标准，permissionMode=approve-all）
             → prompt = 任务描述（即工作指令）
             → 累积 text_delta / done 时 AppendReply 回写时间线
             → completed / failed（失败按 max_retries 自动重跑，失败原因随时间线注入下次执行）
```

### tasks 表 v2 新增字段（schema user_version=2）

| 字段 | 默认 | 说明 |
|---|---|---|
| priority | medium | urgent/high/medium/low，调度排序依据 |
| assignee | ''（=claudecode） | 执行 agent 类型 |
| labels | [] | JSON 标签数组 |
| created_by | user | user / agent / scheduler |
| parent_id | '' | 父任务；**父任务天生以子任务为依赖**：子任务全 completed 父任务才执行；纯容器父任务（无描述）子任务全完成时直接自动 completed |
| milestone | '' | 里程碑分组 |
| acceptance_criteria | '' | 验收标准：注入 system prompt（`=== ACCEPTANCE CRITERIA ===` 段）+ 指令尾部要求自查 |
| recurrence | ''（不重复） | 简单枚举 JSON：`{"freq":"daily/weekly/monthly","weekday":0-6,"monthday":1-31,"at":"HH:MM"}`；完成后由调度器生成下一个任务实例（原任务保留为历史并摘除规则） |
| max_retries / retry_count | 1 / 0 | 失败自动重跑预算 |
| timeout_minutes | 0（=10min 空闲超时） | runner 静默超时上限 |

### 触发时间优先级

`scheduledAt`（scheduled 类型）→ `plannedStart` → 立即。closed 的 issue 不会被自动执行。

### E2E 验证（2026-06-12，零 UI 全自动）

CLI 建任务（描述+验收标准）→ 调度器 5s 自动拿锁 → runner 直连真实 1acp → 真 Claude 创建 hello.txt → agent 按验收标准输出自查对照表 → 回写时间线 → completed，实际起止时间入库。

## 7. Implementation Roadmap

> issue-model 的 P0–P6 整体顺延，建在 S 系列之上（其 P0 数据层并入 S0）。

| Phase | Scope | Verification |
|---|---|---|
| **S0 存储底座** | `internal/meta` 包：建库/建表/PRAGMA/迁移 JSON；Task 结构加 `PlannedStart/PlannedEnd`；issue-model 的 Description/IssueState/Replies 同步建表 | `go test ./internal/meta/...`；迁移后旧数据可读 |
| **S1 CLI** | `project`/`task` 子命令全集（§4） | CLI 增删改查 + 服务端同时读写不冲突（WAL 并发测试） |
| **S2 API** | `/api/projects*`、tasks 表格数据接口；agent handler 的 store 替换 | `go test ./internal/agent/...` |
| **S3 表格 UI** | 落地页换表格主视图 + 项目切换器 | `make frontend`；手工核对排序/筛选 |
| **S4+** | issue-model P1–P6（API/注入/回写/详情卡/徽章/E2E） | 见 issue-model §12 |

---

## 8. Out of Scope（首期明确不做）

- 看板 / 甘特 / 时间轴视图（数据字段已备好，视图后加）
- 多人协作、权限、远程同步（全本地单用户）
- 任务工时统计 / 燃尽图
- CLI 的交互式 TUI（先纯命令行参数）
- 跨库外键到 Claude Code 原生 JSONL（沿用 issue-model 的 acp_session_id 指针即可）
