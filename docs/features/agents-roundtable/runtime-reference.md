# 圆桌当前运行机制参考

**Status:** Implemented
**Updated:** 2026-07-28
**Scope:** 当前代码中的多人讨论、进度管理、阶段门禁与失败恢复
**Source of truth:** `backend/internal/roundtable/`、前端 Roundtable typed client 与组件测试

本文只描述当前已经存在的行为，不把 PRD 中的未来目标写成已实现能力。设计原因与取舍见
[《为什么圆桌使用分层门禁》](./runtime-gates-explained.md)。

## 1. 一句话模型

当前圆桌是一套固定三阶段、六个真实 Agent 席位的编排：

```text
R1 提案与确认
  → R2 五席隔离独立分析
  → 裁判通过工具提交 Summary₂
  → R3 五席恢复原会话做交叉验证
  → 裁判通过工具提交 Summary₃
  → Done
```

系统同时维护四类状态：

1. `Room.state`：用户处于哪个业务阶段。
2. `RoundRun.status`：R2 或 R3 这一轮执行到了哪里。
3. `run_seats.status`：五个职能席各自是否排队、运行、完成、失败或跳过。
4. Brief / Summary 工件：阶段所需的正式产物是否已经通过受控入口落库。

只有同时满足状态和工件条件，系统才会开放下一阶段。

## 2. 固定席位与会话模型

每个房间固定创建 6 个席位：

| 席位 | 角色标识 | R1 | R2 | R3 |
|------|----------|----|----|----|
| 裁判 / 主持人 | `referee` | 与用户澄清并提出 Brief | 汇总五席并提交 Summary₂ | 审计交叉验证并提交 Summary₃ |
| 市场 | `market` | 不发言 | 隔离分析 | 恢复原会话做交叉验证 |
| 产品 | `product` | 不发言 | 隔离分析 | 恢复原会话做交叉验证 |
| 研发 | `eng` | 不发言 | 隔离分析 | 恢复原会话做交叉验证 |
| 运营 | `ops` | 不发言 | 隔离分析 | 恢复原会话做交叉验证 |
| 财务 | `finance` | 不发言 | 隔离分析 | 恢复原会话做交叉验证 |

一席对应：

- 一个 `kind=app` workspace；
- 一条 1agents Chat session，保存在 `seat.session_id`；
- 一条 Agent harness / ACP 会话，保存在 `seat.acp_session_id`；
- 一份按职能写入的 `AGENTS.md` 与 `Claude.md`；
- 一份 `.1agents-roundtable.json` sidecar，记录 `room_id`、`role`、`seat_id` 和
  `cli_bin`。

R2 首次启动职能席 ACP 会话；R3 必须用同一个 `acp_session_id` resume。不同席位之间不共享
ACP 会话。

代码：

- [types.go](../../../backend/internal/roundtable/types.go)
- [roles.go](../../../backend/internal/roundtable/roles.go)
- [service.go](../../../backend/internal/roundtable/service.go)

## 3. 房间状态机

### 3.1 正常路径

```text
drafting_brief
    │ 用户确认当前 Brief version
    ▼
waiting_r2
    │ 原子创建 / 复用 R2 RoundRun
    ▼
summarizing_r2
    │ 五席完成 + 裁判工具提交 Summary₂ + runner 核验
    ▼
waiting_r3
    │ 原子创建 / 复用 R3 RoundRun
    ▼
summarizing_r3
    │ 五席完成 + 裁判工具提交 Summary₃ + runner 核验
    ▼
done
```

任一活动阶段发生无法自动恢复的错误时可以进入 `failed`。

`summarizing_r2` 和 `summarizing_r3` 是历史命名。当前实现中，它们覆盖整轮的席位执行和裁判
总结，不只表示最后的总结几秒钟。更细的执行状态由 `RoundRun.status` 表示。

### 3.2 Room 对前端的派生字段

后端在返回 Room 时统一投影以下字段，前端不需要自己拼状态机：

| Room state | `phase` | `phase_status` | `next_action` |
|------------|---------|----------------|---------------|
| `drafting_brief` | `r1` | `running` | `confirm_brief` |
| `waiting_r2` | `r2` | `ready` | `start_r2` |
| `summarizing_r2` | `r2` | 当前 R2 Run 状态 | `wait` 或恢复动作 |
| `waiting_r3` | `r3` | `ready` | `start_r3` |
| `summarizing_r3` | `r3` | 当前 R3 Run 状态 | `wait` 或恢复动作 |
| `done` | `done` | `completed` | `none` |
| `failed` | 对应失败轮次 | 当前 Run 状态 | 由 `error_scope` 决定 |

同时返回：

```text
available_actions
progress.completed
progress.total
progress.active_roles
progress.failed_roles
progress.skipped_roles
active_run
```

投影逻辑在
[round_run.go](../../../backend/internal/roundtable/round_run.go) 的 `projectRoomRuntime`。

## 4. RoundRun 与真实进度

R2、R3 各自有一个持久化 `RoundRun`：

```text
queued → running → summarizing → completed
                    │
                    ├─ partial_failed
                    └─ failed
```

完整状态集合：

| 状态 | 含义 |
|------|------|
| `queued` | 已抢占房间和创建运行记录，后台执行尚未开始 |
| `running` | 五个职能席正在执行 |
| `summarizing` | 职能席已处理完毕，等待或执行裁判总结 |
| `completed` | 五席与总结均完成 |
| `partial_failed` | 存在失败或被跳过的席位；具体语义要结合 `error_scope` |
| `failed` | 房间级或总结级执行失败 |
| `canceled` | 数据类型已保留；当前没有公开取消 API 或 UI 操作 |

### 4.1 每席进度

每个 Run 固定创建 5 条 `run_seats` 记录，不包含裁判：

```text
queued → running → completed
                  ├─ failed → running     # 只重试该席
                  └─ skipped              # 用户接受缺席
```

`StartRunSeat` 使用条件更新 `queued → running`，因此同一席位同一 Run 至多被正常启动一次。
重试使用单独的 `failed → running` 原子门。

`progress.total` 当前固定为 `5`：

- `completed` 只统计状态为 `completed` 的职能席；
- `active_roles` 来自 `running`；
- `failed_roles` 来自 `failed`；
- `skipped_roles` 来自 `skipped`；
- 裁判总结不计入 `5`，通过 `phase_status=summarizing` 单独表达。

### 4.2 幂等启动

启动 R2 / R3 时，`ClaimRoundRun` 在同一个 SQLite 事务中完成：

1. 检查房间是否位于对应的 `waiting_r2` / `waiting_r3`；
2. 创建 Run；
3. 把房间切换为 `summarizing_r2` / `summarizing_r3`；
4. 为五席创建 `queued` 进度记录；
5. 写入第一条 Run event。

数据库使用 `UNIQUE(room_id, round)` 保证同一房间同一轮只存在一个 Run。并发窗口即使携带不同
`idempotency_key`，也只会有一个调用者创建 Run；其他调用者获得同一个 Run，并看到
`reused=true`。

注意：当前唯一键按 `room_id + round`，不是按讨论周期。它带来的当前边界见
[§12.1](#121-同一房间尚不支持第二个完整讨论周期)。

## 5. R1：Brief 提案与确认门禁

### 5.1 Brief 是版本化工件

Brief 的四个必填字段是：

```text
title
question
constraints
success_criteria
```

可选 `product_kind`：

```text
software | hardware | hybrid
```

空字符串、`—`、`TBD` 等占位值不能通过校验。

每次编辑或裁判提案都会创建新的不可变 `BriefVersion`：

```text
draft | proposed | confirmed | superseded
```

Room 保存三个指针：

| 字段 | 用途 |
|------|------|
| `current_brief_version` | Inspector 当前展示和编辑的版本 |
| `confirmed_brief_version` | 用户最后确认的版本 |
| `r2_brief_version` | R2 启动时捕获的不可变输入快照 |

### 5.2 权限门

- 裁判只能通过 `brief/propose` 或 `roundtable propose-brief` 创建
  `proposed_by=referee` 的提案。
- 用户可以保存 `draft`。
- 只有用户确认接口可以把当前版本标记为 `confirmed`。
- Agent 的提案请求没有确认字段，不能把提案直接变成已确认 Brief。
- 保存和确认都使用 `expected_version` 做乐观并发控制；旧版本写入返回
  `409 brief_version_conflict`。

### 5.3 R2 出站门

只有确认当前 Brief version，Room 才会进入 `waiting_r2`。

R2 启动事务会把：

```text
r2_brief_version = confirmed_brief_version
```

后续五席 R2、裁判 Summary₂、五席 R3 和裁判 Summary₃ 都读取这个快照，而不是读取可能后来变化的
`current_brief`。

详细版本契约见
[brief-version-migration.md](./brief-version-migration.md)。

## 6. R2：隔离独立分析

### 6.1 上下文门

五个职能席并行执行，每席恰好一个正常模型 turn。每席只获得：

```text
本席完整角色契约 + r2_brief_version 对应 Brief
```

R2 prompt 不包含：

- 其他席位正文；
- Summary₂；
- tool trace；
- 其他席位 process。

这保证市场、产品、研发、运营和财务先形成独立判断，再由裁判比较差异。

### 6.2 正常流程

```text
Run running
  → 五席并行 queued → running
  → 每席正文写入 Turn(round=2, kind=speech)
  → 五席全部完成
  → Run summarizing
  → 裁判收到五席正文
  → 裁判调用 submit-r2-summary
  → runner 核验 Summary₂ 工件
  → FinalizeRoundRun
  → Room waiting_r3
```

## 7. R2 裁判总结门禁

裁判必须调用：

```bash
1agents roundtable submit-r2-summary \
  --summary "各席要点、共识、分歧、缺失证据与 R3 待解问题"
```

普通对话回复中的总结不会被当作 Summary₂，也不能开启 R3。

工具提交需要同时满足：

1. 当前 cwd 能读取有效 `.1agents-roundtable.json`；
2. sidecar 角色是 `referee`；
3. sidecar 的 `room_id` 与目标房间一致；
4. sidecar 的 `seat_id` 确实是该房间裁判席；
5. Room 位于 `summarizing_r2`；
6. 若该轮存在 RoundRun，Run 必须位于 `summarizing`；
7. Summary₂ 尚未被其他内容占用。

工具在一个事务中：

- 写入 `rooms.summary_r2`；
- 写入 `Turn(round=2, kind=summary, seat_id=<referee>)`。

工具本身不把 Room 改成 `waiting_r3`。Round runner 会再次读取并核对 Room 中的 Summary₂ 和裁判
summary turn；只有两者存在且正文一致，才调用 `FinalizeRoundRun` 原子完成 Run 并开放 R3。

实现：

- [cli.go](../../../backend/internal/roundtable/cli.go)
- [service.go](../../../backend/internal/roundtable/service.go)
- [store.go](../../../backend/internal/roundtable/store.go)

## 8. R3：恢复会话与交叉验证

### 8.1 启动前置

R3 需要：

- Room 位于 `waiting_r3`；
- `r2_brief_version > 0`；
- 已存在有效 Summary₂；
- 每个成功进入 R3 的职能席有可 resume 的 `acp_session_id`。

缺少 ACP 会话的席位会按该席 R3 执行失败处理，不会静默创建一份与 R2 无关的新观点历史。

### 8.2 公开上下文

R3 每席恢复自己的 R2 ACP 会话，并在当前 turn 注入：

```text
r2_brief_version 对应 Brief
+ 五席 R2 Speech 全文
+ Summary₂
```

不注入其他席位的 tool trace 或 process。

R3 提示词要求每席：

- 点名正在验证的席位或具体观点；
- 标记 `保留 / 修正 / 反驳 / 新增证据或待验证`；
- 区分事实、推断和假设；
- 给出相对 R2 的最终立场及可推翻条件。

UI 当前仍把第 3 步标为“交叉回应”，但 Agent 提示词和门禁语义已经是“交叉验证”。

## 9. R3 裁判终稿门禁

裁判必须调用：

```bash
1agents roundtable submit-r3-summary \
  --summary "最终判断、假设变化、未收敛分歧、行动项与未决风险"
```

普通回复中的终稿不能结束圆桌。

`submit-r3-summary` 使用与 R2 相同的身份、房间、状态和 Run 校验，但要求：

```text
Room.state = summarizing_r3
RoundRun.status = summarizing
```

工具在同一事务中写入：

- `rooms.summary_r3`；
- `Turn(round=3, kind=summary, seat_id=<referee>)`。

Round runner 再次核验两份记录正文一致后，才原子执行：

```text
RoundRun → completed 或 partial_failed
Room     → done
summary event → completed
run event     → terminal
```

因此“裁判说完了”和“圆桌正式完成”是两个不同事件。

## 10. 失败恢复

`RoundRun.error_scope` 决定前端可以提供哪一种恢复动作。

| 场景 | Run 状态 / scope | Room 状态 | 保留内容 | 用户动作 |
|------|------------------|------------|----------|----------|
| 一个或多个职能席失败 | `partial_failed / seat` | 保持当前 `summarizing_r2/r3` | 所有成功席正文 | 只重试失败席，或跳过并总结 |
| 用户重试失败席 | `running / none` | 保持当前轮次 | 其他席不重跑 | 原子重开指定席 |
| 用户跳过失败席 | `summarizing / none` | 保持当前轮次 | 成功席正文 | 把失败席记为 `skipped`，继续裁判总结 |
| 跳过后总结成功 | `partial_failed / none` | `waiting_r3` 或 `done` | Summary 明确记录缺席 | 无需再恢复 |
| 裁判总结失败或未提交工具 | `failed / summary` | `failed` | 五席正文与已有工件 | 只重试裁判总结 |
| 房间级错误 | `failed / room` | `failed` | 已落库内容 | 重新同步房间 |

### 10.1 只重试失败席

重试继续使用原 `run_id`：

- 只允许 `failed` 的目标席原子切回 `running`；
- 已完成席不会再次执行；
- 旧失败 turn 保留，新成功 turn 追加；
- UI 读取该席当前轮次的最新 speech；
- 所有失败席清零后自动进入总结。

### 10.2 跳过并总结

跳过操作会：

- 把所有当前 `failed` run seat 改为 `skipped`；
- 把对应 Room seat 标为 `skipped`；
- 保留其他席结果；
- 进入 `summarizing`；
- 在 Summary 中追加系统生成的“缺席角色”记录。

### 10.3 只重试总结

总结重试是基础状态机之外的受控恢复入口：

- `Run failed/summary → summarizing`；
- `Room failed → summarizing_r2` 或 `summarizing_r3`；
- 不重新打开任何职能席执行门；
- 已经通过工具提交的 Summary 会优先被复用；
- 否则只重新提示裁判并等待对应提交工具。

## 11. 持久化、事件与前端进度

### 11.1 SQLite 数据职责

| 表 | 职责 |
|----|------|
| `agents_roundtable_rooms` | 房间阶段、Brief 指针、Summary₂、Summary₃ |
| `agents_roundtable_brief_versions` | Brief 不可变版本和确认状态 |
| `agents_roundtable_seats` | 六席 workspace、Chat / ACP 会话和当前席位状态 |
| `agents_roundtable_turns` | 用户可见正文；speech / summary / system / chat |
| `agents_roundtable_runs` | R2/R3 持久化运行实例、状态和错误范围 |
| `agents_roundtable_run_seats` | 五职能席逐席运行进度 |
| `agents_roundtable_events` | 按 `seq` 递增的 run / seat / summary 事件 |

Room、Run、逐席进度和终稿都在 `meta.db` 中，可在页面刷新或服务重启后重新读取。

### 11.2 事件游标

事件 API：

```http
GET /api/roundtable/rooms/{room_id}/events?after=<seq>&limit=200
```

也可以用：

```http
Last-Event-ID: <seq>
```

响应返回 `events` 和 `last_seq`。默认最多 200 条，单次上限 1000 条。只返回
`seq > after` 的事件，适合断线续传。

当前 Roundtable 页面尚未消费这个事件游标。它使用 `GET room` 轮询：

| Room 状态 | 基础轮询间隔 |
|-----------|--------------|
| `summarizing_r2/r3` | 1.5 秒 |
| `drafting_brief/waiting_r2/waiting_r3` | 4 秒 |
| `done/failed` | 停止轮询 |

页面检测到席位正在发言或本地操作 busy 时，也会把间隔收紧到最多 1.5 秒。

### 11.3 当前前端启动接法

后端正式 API 已支持：

```http
POST /r2 → 202 StartRoundResponse
POST /r3 → 202 StartRoundResponse
```

typed client 也提供 `runR2()` / `runR3()` 异步方法。

但当前 `RoundtableRoom.tsx` 的主按钮仍调用：

```text
POST /r2?wait=1
POST /r3?wait=1
```

这条兼容路径仍会先创建同一个持久化 RoundRun，只是 HTTP 请求会等到 Run 进入终态再返回。请求等待期间，
页面继续通过轮询读取 Room，因此用户仍能看到逐席进度。下一步若切换为纯 `202`，不需要改后端状态模型，
只需改前端启动与事件消费方式。

## 12. 当前实现边界

### 12.1 同一房间尚不支持第二个完整讨论周期

BriefVersion 已允许 R2 结束后创建新 Brief，并让 Room 回到 `drafting_brief`。但 RoundRun 当前使用：

```sql
UNIQUE(room_id, round)
```

因此同一房间只能拥有一个 R2 Run 和一个 R3 Run。现有 `ClaimRoundRun` 会优先返回旧 Run，不能在同一房间内
为新 Brief 创建第二个 R2 执行周期。

当前安全操作是为重新讨论创建新房间。若要支持原房间多周期，需要给 Run 增加 `cycle` / `brief_version`
维度，并把唯一键、turn 归属、事件和 UI 历史一起迁移。

### 12.2 事件 API 已落地，页面仍以轮询为主

持久化事件和 `after/last_seq` 已可用，但当前页面未保存事件游标。刷新恢复依赖完整 `GET room`，而不是事件
增量重放。

### 12.3 固定剧本

当前不可配置：

- 席位数量和角色；
- R2/R3 每席 turn 数；
- 动态增加轮次；
- 投票或动态主持；
- 运行中取消。

这使成本和门禁可预测，但不适用于开放式 GroupChat。

## 13. HTTP 与 CLI 入口

### 13.1 HTTP

| 方法 | 路径 | 用途 |
|------|------|------|
| `POST` | `/api/roundtable/rooms` | 创建房间和六席 |
| `GET` | `/api/roundtable/rooms/{id}` | 获取 Room、席位、turn 和运行投影 |
| `POST` | `/api/roundtable/rooms/{id}/chat` | R1 用户与裁判对话 |
| `POST` | `/api/roundtable/rooms/{id}/brief/draft` | 用户保存新 Brief draft |
| `POST` | `/api/roundtable/rooms/{id}/brief/propose` | 裁判提出 Brief version |
| `POST` | `/api/roundtable/rooms/{id}/brief/confirm` | 用户确认当前 Brief version |
| `POST` | `/api/roundtable/rooms/{id}/r2` | 异步启动或复用 R2 Run |
| `POST` | `/api/roundtable/rooms/{id}/r3` | 异步启动或复用 R3 Run |
| `GET` | `/api/roundtable/rooms/{id}/events` | 按事件序号恢复进度 |
| `POST` | `/api/roundtable/rooms/{id}/runs/{run}/seats/{role}/retry` | 只重试一个失败席 |
| `POST` | `/api/roundtable/rooms/{id}/runs/{run}/skip` | 跳过失败席并总结 |
| `POST` | `/api/roundtable/rooms/{id}/runs/{run}/summary/retry` | 只重试裁判总结 |

`POST /r2|r3?wait=1` 是同步等待兼容入口。

### 13.2 Agent CLI

正式 Agent 工作流：

```bash
1agents roundtable get --json

1agents roundtable propose-brief \
  --title "..." \
  --question "..." \
  --constraints "..." \
  --success-criteria "..."

1agents roundtable submit-r2-summary \
  --summary "..."

1agents roundtable submit-r3-summary \
  --summary "..."
```

`set-brief` 仍保留为 deprecated compatibility / administration 命令，不应授予普通 Agent 作为确认 Brief 的
正式路径。

CLI 在席位 cwd 中按以下优先级解析房间：

```text
--room
→ ONEAGENTS_ROUNDTABLE_ROOM_ID
→ .1agents-roundtable.json
→ workspace path 反查
```

总结 CLI 还必须从裁判 cwd 调用，并会在 service/store 层再次校验房间和裁判席身份。

## 14. 验证入口

关键自动化覆盖：

- Brief proposal / confirm / stale version / 并发 CAS；
- R2 隔离上下文；
- R3 resume 同一 ACP 和公开上下文；
- R2 / R3 并发启动 at-most-once；
- 普通裁判回复不能越过 Summary₂ / Summary₃ 工具门；
- 单席重试不重跑成功席；
- 跳过后 Summary 记录缺席；
- 总结重试不重跑五席；
- 事件序号断线续传；
- 前端阶段、真实进度、恢复动作和最终结论展示。

运行：

```bash
cd backend
go test -count=1 ./internal/roundtable

cd ../frontend
node --test --import tsx src/components/roundtable/*.test.ts
yarn check
```

## Related

- [为什么圆桌使用分层门禁](./runtime-gates-explained.md)
- [BriefVersion 单一真源与迁移策略](./brief-version-migration.md)
- [产品需求](./prd.md)
- [设计稿](./design.md)
- [验收清单](./ACCEPTANCE.md)
