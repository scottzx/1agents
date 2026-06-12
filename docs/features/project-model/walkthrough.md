# Project Model + Issue Model — 实现走查（2026-06-12）

## 已落地内容

### S0 存储底座（`backend/internal/meta/`）
- 全局库 `~/.1agents/meta.db`（SQLite，WAL + busy_timeout + `_txlock=immediate`，纯 Go 驱动 modernc.org/sqlite）
- 表：projects / tasks / task_deps / replies / sessions（schema 见 db.go）
- `SessionMetadata` 不再单独存储 —— 由 `sessions.task_id` 反向聚合，统一了原来"两套会话存储"
- **迁移**：服务启动时自动导入 `~/.1agents/agent-sessions.json` 和各 workspace 的 `.1agents/tasks.json`，原文件改名 `*.migrated`（保底不删）；TaskStore.Load 也会惰性导入未注册的 workspace
- `internal/agent` 的 `Store`/`TasksStore` 变为 meta 的别名，handler/scheduler/acpx 零改动迁移

### S1 CLI（`backend/internal/cli/`）
```bash
1agents project list|add
1agents task list|add|show|update|close|reopen|comment [--json]
```
直写 meta.db，服务不在跑也能用；与运行中的服务并发写已验证（WAL，10 并发无丢失）。

### S2 API
- `GET/POST /api/projects`
- `GET /api/agent/tasks/{id}`（含 description + replies 时间线）
- `PATCH /api/agent/tasks/{id}`（description / issueState）
- `POST /api/agent/tasks/{id}/replies`（closed 时 new/follow_up 返回 422，pure_comment 放行）
- `POST /api/agent/tasks` 接受 description / plannedStart / plannedEnd
- chat WS 新增 `reply_id` 参数 → 回填 `Reply.SessionRef` + `sessions.task_id`

### issue-model P2/P3（后端）
- **注入**：`buildIssueBackground()` 按 §9 模板渲染（描述 + 完整时间线），**仅 mode=new**（record 无 acpSessionId）时注入；resume 不注入
- **回写**：acpx_client 累积 turn 内 `text_delta`（output 流），`tool_call` 重置（保留最后一条 assistant 消息），`done` 时 `AppendReply` 写回时间线（每 turn 一条，含 SessionRef/AcpSessionID/InReplyTo）

### S3/P4/P5 前端
- 落地页 = 多维表格（状态 | 🔓/🔒 | 任务 | 计划开始 | 计划完成 | 实际完成 | 前置依赖）
- 点行 → Issue 详情卡：可编辑描述 + 时间线 + 回复框（纯评论 / 新会话 / 追问下拉）+ 关闭/重开
- 侧边栏会话带 📋 任务徽章（taskId 软关联）

## 已验证（自动化）
- `go test ./internal/...` 全过（meta 11 测试、agent 注入/回写/端点测试）
- API 级 E2E：迁移 → 建任务 → 回复 → 关闭 → 422/200 语义 → CLI 并发读写 ✅
- `yarn check` + `yarn build`（webpack 0 错误）✅

## 待手工验证（P6，需要真实 Claude 会话）
1. 重启 1agents 服务 → 确认日志出现 legacy 迁移，且老任务/会话在 UI 可见
2. 表格点任务 → 写回复选"启动新会话" → Claude 跑完 → 回详情卡确认时间线多了一条 agent 回复
3. 同任务再回复选"追问会话" → 确认 resume 同一会话（历史可见）且**系统提示不再注入背景块**
4. 关闭 Issue → 确认回复框只剩"纯评论"
5. 侧边栏确认该会话带 📋 徽章

## 注意事项
- **首次重启服务会迁移真实数据**（JSON → meta.db，原文件保留为 .migrated）
- 前端顺手修了 dev 分支三个预存的编译错误（MessageBubble Fragment、MessageList toolName、hooks.ts calls 收窄），`yarn build` 此前在 dev 上是红的
