# Agents 圆桌脑暴 MVP

**Status:** MVP 与 vNext 主流程已实现；当前行为以 runtime-reference.md 为准
**Author:** scott + Grok  
**Date:** 2026-07-23  
**Scope:** 真多 session 编排（路线 B）、tmp 席位、1acp resume、发现中心/应用入口、固定 3 轮剧本  
**成功标准:** 三轮可可靠运行；Brief 单一真源；用户无需离开圆桌即可完成 R1；完成态优先展示最终结论
**执行人:** grok-build  
**当前实现参考:** [runtime-reference.md](./runtime-reference.md)
**门禁设计解释:** [runtime-gates-explained.md](./runtime-gates-explained.md)

---

## 1. Goal

在 1agents 远程工作台中提供 **Agents 圆桌脑暴**：

> 用户与 **真实 agent 二进制** 的裁判充分澄清议题后，由固定职能席位（同一 harness **Grok Build**、不同角色）在 **隔离 session** 下各发言两轮，裁判两轮总结；主 UI **默认只展示发言正文**。

| 非目标（MVP 不做） | 说明 |
|--------------------|------|
| 开放式 GroupChat / LLM 动态选人 | 易死循环、难控成本 |
| 混多二进制（Claude/Codex…） | 二期可配 `agent_type`；MVP 统一 Grok Build |
| 脑暴后自动写码 / Git 执行闭环 | 路线 C 后置 |
| 自定义 DAG 画布 / 投票 UI | 不做 |

---

## 2. Locked decisions

| 维度 | 决策 |
|------|------|
| 路线 | **B：真多 Agent 会话编排**（每席独立进程/session，非单 session 假扮多角色） |
| 席位运行时 | **一席 = 一个 `kind=app` workspace + 一个后端 agent session** |
| 会话复用 | 同席位跨轮用 **1acp `session/resume`** |
| 过程展示 | **默认折叠过程**；主时间线只展示该 turn **发言正文**（`content_text`） |
| 入口 | **更多 / 发现中心 / 应用中心** |
| 裁判 | **必须是真实 agent 二进制**（独立 app workspace + session） |
| 默认 harness | **全部 Grok Build**；差异化靠 **职能角色 seed** |
| 默认编制 | **裁判 + 市场 / 产品 / 研发 / 运营 / 财务**（共 **6** 个 session） |
| 领域语境 | 多半是 **IT 软件、硬件产品**；Brief 可注明软/硬/一体 |
| 讨论模式 | 固定 **3 轮**（见 §4） |

---

## 3. Default roster

```
Roundtable room
├── seat:referee  → Grok Build · 裁判/主持人     · app-rt-<id>-ref
├── seat:market   → Grok Build · 市场             · app-rt-<id>-mkt
├── seat:product  → Grok Build · 产品             · app-rt-<id>-prd
├── seat:eng      → Grok Build · 研发             · app-rt-<id>-eng
├── seat:ops      → Grok Build · 运营             · app-rt-<id>-ops
└── seat:finance  → Grok Build · 财务             · app-rt-<id>-fin
```

- 复用 `CreateAppWorkspace` / `kind=app`：轻量 cwd、seed `AGENTS.md`、不进入任务区或项目列表。
- UI 席位名展示 **职能**（市场/产品/研发/运营/财务），不只显示「Grok」。

### 3.1 Complete role prompts

`RoleSeedAGENTS(role)` 是六个席位的角色提示词单一真源。建房时，每个 `kind=app` workspace 都要写入完整的 `AGENTS.md` 和 `Claude.md`；首次 ACP 调用还要将同一份完整契约注入 `SystemContext`（裁判为 R1，五个职能席为 R2）。R3 resume 原会话，不另起一套角色定义。

每份完整提示词必须包含：

- **身份与使命**：该席位为什么存在，对什么决策负责。
- **职能分析框架**：必须覆盖的分析维度，不能只有一句角色标签。
- **明确行为设置**：如何处理证据、假设、取舍、异议与未知项。
- **输出焦点与边界**：应该产出什么，不应越俎代替哪个席位。
- **圆桌行为协议**：角色锁定、事实边界、结论优先、可执行性、R2/R3 轮次纪律与正文契约。
- **默认输出结构**：结论、关键判断、风险与反例、建议动作、待验证假设。

| 席位 | 专属分析框架 | 关键行为设置 |
|------|------------------|------------------|
| 裁判 | R1 Brief 澄清；R2 共识/分歧/证据缺口；R3 收敛与终稿 | 保持中立、标注来源、不伪造共识；R1 每轮优先问 1–3 个高影响问题 |
| 市场 | 人群与场景、证据、竞争定位、渠道增长、市场验证 | 不编造 TAM/SAM/SOM；优先收窄切入市场并给出可验证方案 |
| 产品 | 问题定义、价值指标、范围优先级、核心旅程、风险验收 | 必须明确做/不做，用用户问题而非功能数量证明范围 |
| 研发 | 可行性、方案边界、交付计划、质量属性、软硬件专项 | 工期必须绑定范围/人力/依赖；高不确定性用 spike、PoC 或打样验证 |
| 运营 | 运营模型、流程责任、容量履约、上线节奏、反馈闭环 | 不把“加人”当默认解法；动作必须有负责人、频率、指标与升级条件 |
| 财务 | 商业假设、成本结构、单位经济、现金回本、情景敏感性 | 不伪造精确数据；给公式/输入/阈值，并明确预算、投资条件和止损线 |

### 3.2 产品 vs 研发（防同质）

| | 产品 | 研发 |
|--|------|------|
| 主问 | 做什么、为谁、优先级与体验 | 能不能做、怎么做、多久、多险 |
| 输出 | 范围、路径、取舍 | 方案形态、约束、风险、验证 |

---

## 4. Three-round script (only mode)

```
R1 命题 ──► R2 首轮各自发言 ──► 裁判总结 ──► R3 次轮各自发言 ──► 裁判终稿 ──► done
```

| 轮 | 谁说话 | 上下文 | 产出 |
|----|--------|--------|------|
| **R1 命题** | 用户 ↔ 裁判 | 仅双方多轮 | 用户确认的 `Brief` |
| **R2 首轮** | 五职能 **各 1 turn** | **仅 Brief**（席位互不可见） | 5×`Speech₂` + 裁判 `Summary₂` |
| **R3 次轮** | 五职能 **各 1 turn** | **resume 本席** + Brief + **五席 Speech₂ 全文** + Summary₂ | 5×`Speech₃` + 裁判 `Summary₃` |

### R1

- Panelist 可不发言；session 可预创建或 R2 再起。
- 出站门：Brief 写入 app 后进入 `waiting_r2`。两条等价通道：
  1. **UI** `POST /api/roundtable/rooms/{id}/brief`（ConfirmBrief）
  2. **CLI** `1agents roundtable set-brief`（裁判在 seat cwd 跨 pwd 直写 meta.db；daemon 可关）
- Brief 最小字段：`title` / `question`、`constraints`、`success_criteria`；可选 `product_kind`: software \| hardware \| hybrid。
- **禁止占位**：空字段或纯 `—` / TBD 不得落库，也不得进入 R2 有效分析路径。
- **跨 cwd room 解析**（CLI，优先级）：`--room` → env `ONEAGENTS_ROUNDTABLE_ROOM_ID` → 席位 cwd 侧车 `.1agents-roundtable.json` → workspace path 反查 seats。
- CreateRoom 时每个 seat cwd 写入侧车（room_id / role / seat_id / **cli_bin**）；裁判 AGENTS.md 教用 `set-brief`，且把 bare `1agents roundtable` **改写为 daemon 绝对路径**（对齐 project-items / PM skill 的 `rewriteCLIBinaryPath` / `ONEAGENTS_CLI`）。开发环境常无 PATH 上的 `1agents`。

### R2

- 并行调五席合法（tmp 隔离、无写冲突）。
- 注入：只 Brief + 本席角色指令；**禁止**塞入其他席位输出。

### R3

- 每席 **1acp resume** 本席 `acp_session_id`（保留本席 R2 私有历史）。
- 本 turn prompt 再注入 **公开上下文包**（仅 **发言正文 + Summary₂**，不注入 tool trace）。
- 裁判 resume 后注入 R3 全文 → 终稿。

### 硬限制

- 每席 R2/R3 **恰好 1 次** model turn（R1 裁判可多轮至确认）。
- 单席失败：标记 `failed`，其余继续；总结注明缺席。
- 超时熔断 per seat。

---

## 5. Runtime mapping

### 5.1 State machine

```
drafting_brief → waiting_r2 → summarizing_r2 → waiting_r3 → summarizing_r3 → done
                                                                              ↘ failed
```

轻量编排器即可；**不上** AutoGen 式动态 GroupChat。

### 5.2 1acp

| 时机 | 动作 |
|------|------|
| R1 裁判多轮 | 同一 session 连续 prompt |
| R2 各席首发 | `session/new`（ensure）+ Brief |
| R3 各席再发 | **`session/resume`** + 公开上下文包 |
| R2/R3 裁判总结 | 裁判 session resume + 本轮全部 Speech |

编排层持久化 per-seat：`workspace_id`、`agent_type`（MVP 固定 grok-build 注册名）、`acp_session_id`、`role`。

### 5.3 Data model (minimal)

```text
Roundtable {
  id, title, state, created_at
  brief?: Brief
  summary_r2?, summary_r3?
  seats: Seat[]
  turns: Turn[]
}

Seat {
  id, role: referee | market | product | eng | ops | finance
  agent_type          // MVP: grok-build (注册名以仓库为准)
  workspace_id        // tmp-…
  acp_session_id?
  status
}

Turn {
  id, round: 1|2|3
  seat_id | "user"
  kind: chat | speech | summary | system
  content_text        // 主时间线唯一绑定
  process_ref?        // 折叠过程 → 底层 session 消息
  created_at
}

Brief {
  title, question, constraints, success_criteria
  product_kind?: software | hardware | hybrid
}
```

持久化：以 **刷新可恢复 room + 可 resume session** 为准（meta.db 或等价存储，实现时选型）。

### 5.4 vNext interaction data

MVP 的 `room.brief` 只表达最终值，无法区分 Chat 草案、Agent 提案和用户确认。vNext 增加两个最小领域对象：

```text
BriefVersion {
  room_id, version
  status: draft | proposed | confirmed | superseded
  content_json
  proposed_by: user | referee
  source_turn_id?
  created_at, updated_at, confirmed_at?
}

RoundRun {
  id, room_id, round
  status: queued | running | summarizing | completed |
          partial_failed | failed | canceled
  idempotency_key
  started_at, finished_at?, error?
}
```

- Room 保存当前/已确认 Brief version，不再把 Chat Markdown 当作 Brief 数据源。
- Agent 只能 propose；用户确认指定 version。
- R2/R3 启动必须先原子抢占状态并创建唯一 RoundRun。
- 房间响应向前端提供 `phase`、`phase_status`、`next_action` 和席位进度。
- 增加可恢复的房间事件序列：Brief 更新、阶段变化、席位开始/完成/失败、总结开始/完成。

RoundRun API 兼容约定：

- `POST /api/roundtable/rooms/{id}/r2|r3` 默认返回 `202` 和
  `{ run_id, run, room, reused }`，不等待五席或总结完成。
- 请求体 `idempotency_key` 或 `Idempotency-Key` 头可标识调用；即使多窗口使用不同
  key，同一房间同一轮也只会创建一个 Run，后续请求返回 `reused=true`。
- 旧同步调用方可显式传 `?wait=1`，继续获得原
  `RunR2Response`/`RunR3Response` 和 `200`；这是迁移兼容路径，新 UI 不使用。
- `GET /api/roundtable/rooms/{id}/events?after=<seq>` 返回持久化增量事件和
  `last_seq`；也接受 `Last-Event-ID`，重连时从最后已处理序号继续。

---

## 6. UI

> **vNext 交互决策（2026-07-27）**：圆桌是一个**按阶段变化的讨论工作台**。
> R1、R2、R3 和完成态分别突出该阶段的主任务；Brief、Summary₂、Summary₃ 各自只有一个完整正文实例。
> 本节替代 2026-07-23 的“固定底栏 Chat + 平铺时间线”布局决策。

### 6.1 Overall workbench

```
┌───────────────────────────────────────────────────────────┐
│ 圆桌标题            当前阶段 · 进度              主操作   │
├───────────────────────────────────────┬───────────────────┤
│                                       │ Inspector         │
│ 当前阶段的主工作区                     │ [议题] [参与者]    │
│                                       │                   │
│ R1：裁判对话                           │ 唯一 Brief 正文    │
│ R2：独立观点                           │ 版本 / 状态        │
│ R3：立场变化与交叉回应                  │ 席位状态 / 来源     │
│ Done：最终结论                         │                   │
├───────────────────────────────────────┴───────────────────┤
│ 仅在需要用户输入或确认时出现的阶段操作区                   │
└───────────────────────────────────────────────────────────┘
```

- 标题区只回答：当前圆桌、当前阶段、真实进度、下一动作。
- 席位完整状态移入 Inspector；标题区只显示总进度和当前运行席位。
- 常驻刷新按钮移除；断线或错误时提供恢复动作。
- 界面不出现 `waiting_r2`、`seat cwd`、`ONEAGENTS_CLI` 等内部术语。

### 6.2 R1 · Referee conversation + Brief Inspector

- R1 主区直接嵌入裁判 `EmbeddedChat`，绑定裁判 `seat.session_id`。
- 用户不需要离开圆桌打开普通 ChatUI。
- R1 Chat 消息不再同时复制为普通时间线卡。
- 裁判使用结构化 `propose-brief` 更新 Brief 草案；Chat 只显示“Brief 草案已更新至 vN”的紧凑事件。
- Inspector 是桌面端唯一完整 Brief 正文，支持编辑、保存、版本状态和用户确认。
- Agent 只能提案，只有用户可以确认。
- 确认事件显示“你已确认 Brief vN”，不得复制四字段全文。

### 6.3 R2 · Independent analysis

- 开始后立即显示真实 `completed / total`、当前运行席位和失败席位。
- 每席默认展示职能、状态、一句话结论和可展开正文。
- `process_ref` 默认折叠；完整底层会话按需打开。
- 单席失败时提供“重试”和“跳过并继续”，不丢失其他已完成席位。
- Summary₂ 全文只出现在主区；Inspector 只提供状态和锚点。

### 6.4 R3 · Cross review

- R3 继续 resume 同一席位 session。
- UI 优先呈现每席的“保留 / 修正 / 反驳 / 新增证据”，而不是再次平铺五张同质卡片。
- Summary₃ 是最终结论的数据来源，正文不在侧栏重复。

### 6.5 Done · Final-first

完成态首屏按以下顺序展示：

1. 最终建议；
2. 关键取舍；
3. 行动项与负责职能；
4. 未决风险；
5. Brief；
6. R2/R3 证据；
7. Agent 底层会话和 process。

历史时间线默认折叠，不再要求用户滚到页面底部查找最终结论。

### 6.6 Canonical artifact rule

- 同一页面中，Brief、Summary₂、Summary₃ 各自只能有一个完整正文实例。
- 其他位置只允许状态、版本、摘要、锚点或事件引用。
- 房间头部不渲染完整 Brief 卡。
- system turn 使用结构化 `event_type` / `artifact_ref`，渲染为紧凑事件行。
- 列表页允许展示 Brief question 的一句预览，因为它是跨房间导航摘要，不是房间内第二正文。

### 6.7 Inspector

- 桌面端右侧提供“议题 / 参与者”两个页签。
- “议题”展示当前 Brief 版本、状态、内容和确认动作。
- “参与者”展示六席状态、当前运行、失败恢复和底层会话入口。
- Summary 全文不再复制到 Inspector。

### 6.8 Global navigation

- 使用全局 `WorkspaceHeader` 导航，左侧为独立返回图标。
- 面包屑：`圆桌列表 › <具体圆桌> › <具体会话>`。
- 打开席位底层会话后保留圆桌上下文，可返回原圆桌。

### 6.9 Entry

| 入口 | 行为 |
|------|------|
| 发现中心 → 应用 | 卡片「Agents 圆桌」→ 打开圆桌列表 |
| 更多应用 | 同一 App |
| App manifest | `id: agents-roundtable`，mount 进应用中心 |

启动向导只要求用户输入要讨论的问题；固定六席折叠展示。主按钮为“创建并开始澄清”，不展示底层 harness/session 术语。

### 6.10 Mobile and accessibility

- 移动端使用“讨论 / 议题 / 参与者”分段视图，不把桌面侧栏简单堆到底部。
- R1 Composer 和主操作保持在安全区内可达。
- 触摸目标不小于 44×44px。
- 状态必须同时有文字/图标，不只靠颜色。
- 运行进度使用短 `aria-live` 消息；不得因轮询重绘而重复朗读整条时间线。
- 保存错误后聚焦首个错误字段；弹层关闭后恢复触发按钮焦点。

---

## 7. Acceptance (ship criteria)

1. 从发现中心能打开圆桌并创建 room。
2. 一局创建 6 个独立 Agent session（1 裁判 + 5 职能）。
3. R1 无需离开圆桌即可与裁判多轮澄清。
4. 同一页面只有一份完整 Brief 正文；proposal、编辑、确认指向明确版本。
5. Agent 不能替用户确认 Brief；确认事件不复制四字段全文。
6. R2 五席在隔离上下文下各一条发言；裁判生成 Summary₂。
7. R3 各席 resume；上下文含 R2 正文与 Summary₂；裁判生成 Summary₃。
8. 两个客户端并发启动同一轮时只创建一个 RoundRun。
9. 运行中显示真实 `n/5` 进度、当前席位和失败席位。
10. 单席失败可重试或跳过继续；其他结果不丢失。
11. 刷新或重连后恢复 room、Brief 版本和运行进度。
12. 完成态默认展示最终结论、行动项和未决风险。
13. 主 UI 默认只见正文；过程可折叠。
14. Summary 能区分职能来源；R3 能体现保留、修正和反驳。
15. 全局标题栏提供返回图标与圆桌路径面包屑。
16. 375px 宽度下，无嵌套滚动即可访问讨论、议题、参与者和主操作。
17. 关键交互、幂等启动、Brief 版本冲突和失败恢复有自动化测试。

---

## 8. Implementation slices

| # | 切片 | 验证 |
|---|------|------|
| 1 | Room 状态机 + 持久化 + 6×app seat 工厂 + 角色 seed | API 建房、列出 seat |
| 2 | R1 裁判 session 对话 + Brief 确认 | 双人聊通并出站 |
| 3 | R2 五席并行 prompt（隔离）+ 裁判 Summary₂ | 五条正文 + 总结入时间线 |
| 4 | R3 resume + 公开上下文注入 + Summary₃ | 同 session_id 跨轮；可见他人 R2 |
| 5 | 前端时间线壳（阶段条、turn 卡片、折叠过程、席位条）— #256 | 默认无 process 噪音 |
| 5b | 固定底栏裁判嵌入 Chat（历史+流式+Composer）；时间线不嵌裁判 — #260 | design §6.2 / §7.9 |
| 6 | App / 发现中心入口 + 启动向导 | 从应用位一键进入 |
| 7 | 端到端冒烟（创建→R1→R2→R3→终稿） | 验收清单 §7 全绿 |

### 8.1 vNext interaction slices

| # | 切片 | 验证 |
|---|------|------|
| 8 | Brief UI 去重 + system event 紧凑渲染 | 房间内只有一个完整 Brief 正文 |
| 9 | R1 圆桌内裁判 Chat + 对话去重 | 不离开圆桌可完成 R1；无双份消息 |
| 10 | BriefVersion + propose/confirm 契约 | Agent 只能提案；用户确认明确版本 |
| 11 | RoundRun 异步/幂等/进度事件 | 并发启动一次；刷新可恢复真实进度 |
| 12 | R2/R3 阶段化工作台 + Final-first | 每阶段主任务明确；完成态先见结论 |
| 13 | 单席重试/跳过 + 总结恢复 | 部分失败不要求整场重来 |
| 14 | 移动端/a11y/交互自动化验收 | 375px、键盘、读屏和核心测试通过 |

---

## 9. Out of scope (explicit)

- 用户自定义席位数/角色拓扑  
- 非 Grok Build 混排  
- 执行闭环（写码、分支、终端验证）  
- 无限轮辩论、投票、Council 黑盒  

---

## 10. References

- tmp workspace：`backend/internal/agent/oneshot.go`、`meta.KindTmp`  
- 1acp resume：`modules/1acp` bridge `resumeSessionId` / `session/resume`  
- 发现中心：`frontend/src/modules/discovery-manifest.ts`（`apps` 分类）  
- App manifest：`frontend/src/services/appManifestService.ts`  
- 竞品调研纪要：产品讨论（AutoGen 群聊 / MetaGPT 产物 / xAI 有界并行 / Cursor 人收敛）— 本 MVP 取 **真 session + 固定剧本 + 正文时间线**  
