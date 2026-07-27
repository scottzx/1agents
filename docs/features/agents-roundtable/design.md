# Agents 圆桌脑暴 MVP

**Status:** 已定稿（待实现）  
**Author:** scott + Grok  
**Date:** 2026-07-23  
**Scope:** 真多 session 编排（路线 B）、tmp 席位、1acp resume、发现中心/应用入口、固定 3 轮剧本  
**成功标准:** 功能上线可跑通 3 轮（有这个功能）  
**执行人:** grok-build  

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

---

## 6. UI

> **布局定稿（2026-07-23，#260）**：**底栏 = 裁判嵌入 Chat**；**时间线不嵌裁判**。  
> 与 #256 关系：#256 提供时间线壳（阶段条 / 席位条 / turn 卡 / 侧栏）；本节省 **固定底栏裁判会话**（历史 + 实时流 + Composer）。

```
┌─────────────────────────────────────┐
│  阶段条 / 席位条 / 时间线 turn 卡    │  ← 主区（#256）：R2/R3 职能发言等
│  （裁判席位 **不** 再挂一张嵌入卡）  │
├─────────────────────────────────────┤
│  【固定底部】ChatUI 嵌入卡           │  ← **唯一**裁判会话组件（#260）
│  = 裁判 seat.session_id              │
│  历史记录 + 实时流式 + typing        │
│  + R1 输入（Composer，复用 ChatUI）  │
└─────────────────────────────────────┘
```

### 6.1 Main timeline（#256 壳）

- 阶段条：R1 命题 · R2 首轮 · R3 次轮 · 终稿  
- Turn 卡片：职能名 + 正文 only（`content_text`）  
- 「查看过程」展开 process（可选）  
- 进行中：席位条 `speaking / done / error`  
- **裁判不在时间线重复嵌卡**：时间线 / 席位「发言卡」区**不得**再挂一份裁判 `EmbeddedChat`（避免与底栏双份 UI）  
- 职能席（panelist）发言过程：可在时间线用各自嵌入或正文卡展示实时（与底栏正交；P0 可先保证裁判底栏）

### 6.2 固定底栏 · 裁判嵌入 Chat（#260）

- **底栏 = 裁判嵌入 Chat**：页面主栏底部 **sticky/fixed** 一条嵌入 `EmbeddedChat`（或等价），绑定 **裁判** `seat.session_id`  
- 能力：会话**历史** + **实时流式** + **typing** + R1 **Composer**（复用 #261 Chat 嵌入组件，非自绘气泡）  
- **无自定义简易底栏**：去掉 `chatText` 类纯文本输入；R1 用户输入只走底栏裁判 Composer / bridge  
- 底栏不随时间线滚动消失  
- 非目标：底栏再绑 panelist session；开放式 GroupChat

### 6.3 Side (minimal)

- 当前 Brief、最新 Summary、席位列表  
- 不接 Diff / 文件树（app seat 纯对话）

### 6.4 Global breadcrumb

- 使用全局 `WorkspaceHeader` 导航，左侧固定为独立、无外边框的「返回上一级」图标按钮，不在内容区重复放返回按钮。
- 图标右侧才是可点击的路径面包屑：`圆桌列表 › <具体圆桌> › <具体会话>`；路径中不出现「返回」文字节点。
- 返回图标始终回到当前层的上一级；在列表层则回到发现中心的应用列表。
- 从席位打开完整 ChatUI 后保留圆桌上下文，可经面包屑回到原圆桌或圆桌列表。

### 6.5 Entry

| 入口 | 行为 |
|------|------|
| 发现中心 → 应用 | 卡片「Agents 圆桌」→ 启动 |
| 更多应用 | 同一 App |
| App manifest | 如 `id: agents-roundtable`，mount 进应用中心 |

启动向导 MVP：议题草稿（可空）→ 固定编制展示 → 「开始」。

---

## 7. Acceptance (ship criteria)

1. 从 **发现中心 → 应用**（或更多）能打开圆桌并创建 room。  
2. 一局创建 **6** 个 Grok Build session（1 裁判 + 5 职能）。  
3. R1：与裁判多轮后确认 Brief（输入走 **底栏裁判嵌入 Composer**，非自定义简易底栏）。  
4. R2：五席在 **隔离上下文** 下各一条发言；裁判 Summary₂。  
5. R3：各席 **resume** 后再发；上下文含 **R2 全部正文 + Summary₂**；裁判 Summary₃。  
6. 主 UI **默认只见正文**；过程可折叠。  
7. 刷新后 room 可恢复；未结束席位可继续 resume。  
8. Summary 能区分职能来源；研发与产品观点可区分。  
9. **布局（#260）**：底栏固定裁判 `EmbeddedChat`（历史 + 实时 + typing + Composer）；时间线/席位区**不**再渲染裁判第二份嵌入卡；无旧版 `chatText` 简易底栏。
10. **导航**：全局标题栏同时提供独立的返回图标和「圆桌列表 › 具体圆桌 › 具体会话」路径面包屑；图标回上一级，路径节点按名称跳转。
11. **工作空间**：裁判与五个职能席位全部持久化为 `kind=app`。
12. **角色提示词**：六个席位在创建时都写入完整 role prompt，包含使命、职能分析框架、明确行为设置、边界与输出结构；首次 ACP 调用注入同份完整契约。

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
