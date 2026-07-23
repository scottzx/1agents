# Workspace Inbox：项目邮箱 + Workforce 接力

**Status:** 已定稿（实现中）  
**Author:** scott + Grok  
**Date:** 2026-07-22  
**Scope:** `backend/internal/meta`（inbox_items）、HTTP `/api/inbox*`、`projectitems` MCP、前端 Inbox UI、角色提示词  
**术语:** [名称定义表](../../product/名称定义表.md)（Workspace `kind=workforce|project`；ProjectItem；executor）  
**关联:** 既有 [#60](https://github.com/scottzx/1agents/issues/60) 全局 Inbox、[#61](https://github.com/scottzx/1agents/issues/61) PMO 分发；引擎化见 [inbox-context-engine](../inbox-context-engine/design.md)（正交，不阻塞本设计）

---

## 1. Goal

把「收件箱」从**全局独立功能**泛化为：

> **每个 Workspace（助理 / 项目）自带 Inbox。**  
> 外界与组织内的一切可交接信息，都先被处理成 Inbox 可接收的统一信封，**投递到目标 Workspace 的 Inbox**；由该 Workspace 的智能体（或人开会话）**check → 处理 → 落 ProjectItem 或再派件到下一环**。

| 场景 | 投递方 | 收件 Workspace | 下一步 |
|------|--------|----------------|--------|
| 外部进组织 | function / 数据源适配器 | 第一道 **助理 (workforce)** | 整理 → 再派件 |
| 组织内接力 | 上游助理 Agent | 下游助理 / 项目 | 判定 / 拆需求 |
| 跨项目交接 | A 项目 Agent | B 项目 Inbox | accept → 需求池 |
| 人机门禁 | 人确认后 | 业务项目 Inbox | PM 排期执行 |

### 竞品监测示例

```
function 爬取竞品动态
    │  规范化信封 source=function|data_source
    ▼
助理「情报收集」Inbox  ── Agent 会话/心跳 check
    │  提取竞品功能摘要 → send_mail
    ▼
助理「跟进判定」Inbox  ── 判定跟/不跟（可 human）
    │  建议跟进 → send_mail
    ▼
项目「某产品线」Inbox  ── PM accept_mail → requirement → 拆 task
```

---

## 2. 概念修正（相对 #60 现状）

| 旧 | 新 |
|----|----|
| 全局 Inbox 是公司雷达；项目是分发目标 | **Inbox 是每个 Workspace 的基本能力**；「全局」若存在，只是某个特殊助理（总助 / PMO workforce）的邮箱 |
| `inbox_items` 无归属 | **每条邮件必属一个 Workspace**（`workspace_id`） |
| PMO 分发 = 唯一跨项目路径 | **派件 Deliver** 是一等能力；采纳进需求池是收件方的一种动作 |
| source 偏外部渠道 | source = **投递方类型**；function / agent / human / 渠道 统一走投递接口 |
| 人在 UI 点「分发」为主 | **Agent 工具优先**；早期人开会话触发 check；心跳后置 |

不保留「与项目无关的独立 Inbox 产品心智」。全局列表 UI 演进为：默认看当前 Workspace 邮箱；聚合视图可后置。

---

## 3. Core model

### 3.1 Workspace Inbox

```
Workspace (kind = workforce | project)
  ├── Inbox[]           ← 投递 / 查收 / 处理
  └── ProjectItems[]    ← 既有看板
```

- **助理 (workforce)**：可弱化完整 PM 外壳；适合流水线节点（收集、判定、总助分发）。
- **项目 (project)**：完整 PM 外壳；Inbox 是进需求池前的缓冲。

### 3.2 统一信封（Envelope）

落在 `inbox_items` 扩字段（不新建平行业务表）：

| 字段 | 含义 |
|------|------|
| `workspace_id` | **收件** Workspace（必填） |
| `source` | `manual` \| `agent` \| `function` \| `im` \| `email` \| `rss` \| `data_source` \| `misc` |
| `from_workspace_id` | 派件方 Workspace（接力时填） |
| `from_ref` | 可选：function 名 / agent 角色 / 数据源 id |
| `title` / `content` / `url` / `summary` / `tags` | 正文 |
| `payload` | JSON 扩展 |
| `status` | `unread` \| `read` \| `archived` |
| `kind`（可选标签） | `lead` \| `signal` \| `handoff` \| `fyi` … |

**唯一写入口：**

```
Deliver(envelope) → inbox_item in target workspace
```

数据源 / function 不直接写需求池；只负责 **规范化信封 + 投递**。

### 3.3 收件处理（Relay）

收件方三选一（可组合）：

1. **采纳** → 本 Workspace `requirement`（默认；复用 PMO `Dispatch` + `dispatched-from` backlink + 标已读）
2. **再派件** → `send_mail` 到另一 Workspace Inbox
3. **归档 / 驳回** → 留痕

- **早期：** 人打开会话 → Agent `check_inbox`  
- **进阶：** 周期 ProjectItem / scheduler 心跳（本期接口预留，不强制自动）

### 3.4 与 executor 的关系

| 通道 | 用途 |
|------|------|
| `function` | 刚性采集 / 规范化后 Deliver |
| `agent` | 研判、摘要、跟/不跟、拆需求 |
| `human` | 门禁确认 |

符合名称定义表「能 function 不 agent，能 agent 不 human」。

---

## 4. Per-source 策略

| source | 典型生产者 | 默认收件 | 建议处理 |
|--------|------------|----------|----------|
| `function` / `data_source` | 爬虫、同步 | 指定助理 | 摘要 → 再派或 discussion |
| `agent` | 上游 Agent | 下游助理/项目 | 采纳 / 再派 / 归档 |
| `manual` | 人粘贴 | 当前 Workspace | 人/Agent 共同 triage |
| `im` / `email` / `rss` | 外部适配器 | 总助或归属项目 | L0 收口后再接力 |
| `misc` | 开源吸收等 | 配置默认助理 | sink 改为投指定 workspace |

策略挂在**收件 Workspace 的角色提示词 / 项目配置**上，不写死全局引擎。

---

## 5. API

| Method | Path | 说明 |
|--------|------|------|
| `POST` | `/api/inbox/deliver` | 统一投递 |
| `GET` | `/api/inbox?workspaceId=` | 按 Workspace 列表 |
| `POST` | `/api/inbox` | 手动 capture（须带 workspaceId） |
| `POST` | `/api/inbox/{id}/accept` | 采纳为 requirement |
| `POST` | `/api/inbox/{id}/archive` 等 | 既有 status 动作 |
| `GET` | `/api/inbox/targets` | 可派件 Workspace 列表 |

`workspaceId` 与 `projects.id` 同实体（Workspace = Project 同一 id）。  
既有 `POST /api/pmo/dispatch` 保留；`accept` 内部复用同一写需求路径。

---

## 6. Agent 工具（MCP · project_items 扩展）

| 工具 | 作用 | 锁 |
|------|------|-----|
| `check_inbox` | 列本箱未读/全部 | 仅当前 Workspace |
| `get_mail` | 单封详情 | 仅当前 |
| `accept_mail` | → requirement | 仅当前；内部 Dispatch |
| `archive_mail` | 归档 + 可选 reason | 仅当前 |
| `list_mail_targets` | 可投递 Workspace | 只读 |
| `send_mail` | 投递到目标 Inbox | **from 强制当前**；只写目标 inbox 行 |

权限：`send_mail` 是**唯一**允许的跨 Workspace 写（仅 Inbox 行）；禁止改他项 ProjectItems。

---

## 7. 迁移

旧 `inbox_items` 无归属：

1. backfill 到 builtin **总助** workforce（无则 bootstrap 或默认 builtin 助理）
2. 禁止长期无归属行

---

## 8. 实施阶段

| Phase | 交付 | 验收 |
|-------|------|------|
| **P1 模型地基** | schema + Deliver/ListByWorkspace/Accept + 迁移 + 单测 | A deliver → B list 仅 B；accept 落 B 的 requirement + backlink |
| **P2 Agent 工具** | MCP 六工具 + pm/workforce 提示词 | 人工会话跑通两跳派件 + 采纳 |
| **P3 function 投递** | `/api/inbox/deliver` + 一条数据源/sink 改造 | function 产出出现在目标助理 Inbox |
| **P4 UI 最小** | Inbox 绑定当前 Workspace；source/来源展示；accept；badge | 项目壳内可见本箱并一键采纳 |
| **P5 心跳（后置）** | 周期 check；配额/去重 | 投递后自动处理或产生待办说明 |

**本期完成线：P1–P2。** P5 不阻塞。

---

## 9. Success criteria

1. 每条邮件有 `workspace_id`；API/UI 默认按 Workspace 查  
2. function 投递与 agent `send_mail` 走同一 `Deliver`  
3. 助理 A → 助理 B → 项目 C 两跳可测  
4. `accept_mail` → `ItemType=requirement` + `dispatched-from`  
5. 收件 Agent 不能改他项 ProjectItems  
6. 术语：助理(workforce) / 项目(project) / ProjectItem；不写 AIWorkforce  

---

## 10. Out of scope

- 真实 SMTP / 独立邮件服务器  
- 本期强制自动心跳  
- 完整「立项建新 Project」审批流  
- Inbox 引擎 domain/depth / kwiki L2 全量（见 inbox-context-engine RFC）  
- 全局 Inbox 与项目邮箱长期双轨  

---

## 11. 默认裁决（Open points）

| # | 问题 | 默认 |
|---|------|------|
| 1 | workforce 可否 accept 落 requirement？ | **允许**；流水线靠提示词再派件 |
| 2 | 旧全局数据挂谁？ | builtin 总助 / 默认助理 |
| 3 | API 字段名 | JSON `workspaceId` |
| 4 | 独立 `accepted` status？ | 否；用 read + backlink |

---

## 12. 关键代码路径

| 区域 | 路径 |
|------|------|
| Schema/store | `backend/internal/meta/db.go`, `inbox.go`, `types.go`, `pmo.go` |
| HTTP | `inbox_http.go`, `server.go` 路由 |
| MCP | `backend/internal/projectitems/tools.go` |
| Roles | `backend/internal/agent/roles/pm.md` 等 |
| UI | `frontend/.../drawer/Inbox/`, `inboxService.ts` |
| 术语 | `docs/product/名称定义表.md` |

---

## 13. 与 inbox-context-engine 的关系

| 本文 | inbox-context-engine RFC |
|------|--------------------------|
| **邮箱拓扑**：每 Workspace 一箱 + 派件/收件/采纳 | **引擎大脑**：domain/depth、kwiki、自动调研、角色吸收 |
| 先落地闭环 | 可并行，不阻塞派件语义 |

「雷达」语义保留，但实现为 **某个（或若干）助理的 Inbox**，不再是平行全局产品对象。

---

## 附录 A：数据源 → 信封字段约定（#206）

| 生产者 | `source` | `from_ref` 建议 | `workspaceId` 默认 |
|--------|----------|-----------------|--------------------|
| Agent `send_mail` | `agent` | 角色名（如 `inbox-collector`） | 调用方指定 `to` |
| function / HTTP `POST /api/inbox/deliver` | `function` | function 名 | 调用方必填 |
| 数据源（opensource sink） | `misc` | `opensource:<full_name>` | **default**（总助） |
| 腾讯 Agent Mail 导入 | `email` | `agentmail:<message_id>` | **default** |
| 手动 capture | `manual` | — | 当前 Workspace |

CLI（与 MCP 对等，供 bash 友好引擎）：

```bash
1agents project-items mail list --status unread
1agents project-items mail deliver --to default --source email --from-ref agentmail:msg_x --title "…"
1agents project-items mail import-agentmail --to default   # agently-cli → 默认箱 unread
1agents project-items mail unread <id>
1agents project-items mail accept <id>
```
