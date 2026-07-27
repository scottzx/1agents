# BriefVersion 单一真源与迁移策略

本文记录 #275 引入的 Brief 版本契约。目标是让 Agent 提案、用户编辑、用户确认和 R2 输入都指向明确且可恢复的版本，同时兼容已有 `room.brief` 数据与旧自动化。

## 1. 单一真源

`agents_roundtable_brief_versions` 是 Brief 内容与生命周期的正式数据源。每个版本包含：

```text
room_id
version
status: draft | proposed | confirmed | superseded
content_json
proposed_by: user | referee
source_turn_id?
created_at
updated_at
confirmed_at?
```

版本内容在创建后不可修改。编辑会创建 `version + 1`，不会覆盖原版本。状态可因确认或被新版本替代而变化。

Room 持久化三个版本指针：

- `current_brief_version`：Inspector 当前显示和编辑的版本。
- `confirmed_brief_version`：最近由用户确认的版本。
- `r2_brief_version`：R2 启动前捕获的不可变输入快照。

Room 响应同时提供 `current_brief`、`confirmed_brief` 和 `r2_brief` 版本对象。旧 `brief` 字段暂时保留，内容等于 `current_brief.content`，仅供旧客户端读取；R2 不再读取它。

## 2. 正式写入契约

### Agent / referee proposal

```http
POST /api/roundtable/rooms/{room_id}/brief/propose
Content-Type: application/json

{
  "title": "...",
  "question": "...",
  "constraints": "...",
  "success_criteria": "...",
  "product_kind": "software",
  "expected_version": 2,
  "source_turn_id": "..."
}
```

该接口固定创建 `status=proposed`、`proposed_by=referee` 的新版本。请求中的额外 `status` 或确认字段不会改变该语义；proposal 不会推进到 R2。

Agent CLI 的正式路径是：

```bash
1agents roundtable propose-brief \
  --expected-version 2 \
  --title "..." \
  --question "..." \
  --constraints "..." \
  --success-criteria "..."
```

省略 `--expected-version` 时，CLI 会先读取 current version，再将该值用于原子写入；读取后若发生并发更新，写入仍会被拒绝。

### User draft

```http
POST /api/roundtable/rooms/{room_id}/brief/draft
```

请求为 Brief 四字段、可选 `product_kind` 和必需的 `expected_version`。成功后创建 `status=draft`、`proposed_by=user` 的新版本，并保持或返回 R1 重新讨论。

### User confirmation

```http
POST /api/roundtable/rooms/{room_id}/brief/confirm
Content-Type: application/json

{
  "version": 3,
  "expected_version": 3
}
```

确认请求不携带 Brief 正文，只能确认仍为 current 的指定版本。成功后该版本变为 `confirmed`，记录 `confirmed_at`，Room 进入 `waiting_r2`。Agent proposal API 和 Agent seed 均不包含确认能力。

## 3. 乐观版本冲突

创建 draft/proposal 和确认都在 SQLite 事务内比较 `expected_version` 与 Room 当前指针。若不一致，服务返回：

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "code": "brief_version_conflict",
  "message": "roundtable: stale brief version: expected=2 current=3",
  "expected_version": 2,
  "current_version": 3
}
```

冲突事务不会写入版本、状态或兼容 `brief_json`。前端应捕获 `BriefVersionConflictError`，重新加载 Room，并让用户比较/重做本地编辑。

## 4. R2 快照与重新讨论

R2 在生成任何席位 prompt 前，将当时的 `confirmed_brief_version` 原子写入 `r2_brief_version`，并只使用该版本的 `content` 构建五席 prompt 和 Summary₂。R3 继续读取同一 `r2_brief_version`，而不是读取可能变化的 current Brief。

R2 运行中拒绝修改 Brief。R2 完成后若创建 draft/proposal：

1. 必须创建新 version，不能修改 R2 已用版本。
2. Room 回到 `drafting_brief`，要求重新讨论和用户确认。
3. 旧 `r2_brief_version` 继续标明既有 R2 输出基于哪个快照。
4. 新版本确认后再次启动 R2，才会捕获新的 R2 快照。

本阶段保留旧 turns/summary 作为历史证据；后续 RoundRun 任务负责将每次运行与版本、进度和重试进一步绑定。

## 5. 旧数据与旧调用迁移

### 数据库

启动 Store 时自动增加 BriefVersion 表和 Room 版本指针。对满足以下条件的旧 Room：

- `brief_json` 非空；
- `current_brief_version = 0`；

系统创建 `v1 / confirmed / proposed_by=user`，将 `confirmed_at` 设为旧 Room 的 `updated_at`，并把 current/confirmed 指针都设为 1。迁移保留原 `brief_json`，因此新旧读取路径都可恢复内容。迁移使用 `INSERT OR IGNORE` 和指针条件更新，可重复执行。

### HTTP

旧 `POST /api/roundtable/rooms/{id}/brief` 暂时保留为管理兼容路径，执行“创建 user proposal + 确认该版本”。新前端和 Agent 不应再使用该路径。

### CLI

旧 `roundtable set-brief` 暂时保留为兼容/管理命令，维持一次写入并确认的旧行为。迁移规则：

- Agent seed 和正式示例全部改用 `propose-brief`。
- `set-brief` 的 help 明确标记为 deprecated、compatibility/administration only。
- 现有人工脚本可在迁移窗口继续运行，但应拆成 proposal/draft 与用户 confirm。
- 因 `set-brief` 具备管理级一站式确认语义，不应授予普通 Agent 工作流使用。

兼容路径的移除时间应在所有外部脚本完成迁移后另行决定；本任务不删除旧 API 或 CLI，以避免已有自动化中断。
