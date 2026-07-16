# 统一新建会话（Session Setup）PRD / 设计

**Status:** Draft → Ready for implementation  
**Author:** PM (AI) + product owner  
**Date:** 2026-07-16  
**Milestone:** `统一新建会话`  
**Scope:** 主要 `frontend/`（弹窗、入口、NewChatHome、Task 详情、i18n、样式）；无后端 API 变更  

**关联实现计划：** 会话内已批准的 plan（统一 SessionSetupModal + defaults + 先建会话再进 ChatPanel）

---

## 1. 背景与动机

### 1.1 问题

前端存在**多条互不一致的「新建对话 / 新建会话」路径**，以及**多套聊天输入**各自携带 mode / agent / role / permission 选择逻辑：

| 入口 | 当前行为 | 痛点 |
|------|----------|------|
| 侧栏「新对话」 | 打开 `NewChatHome`，首条消息才 `createChatSession` | 无 slash；参数与弹窗路径不一致 |
| 工作区 `…`「新建会话」 | `SessionCreateModal`（名/agent/**role/permission**）→ 空会话 | 暴露执行者/权限；与 NewChatHome 字段不同 |
| 助理 / 项目详情「新建对话」 | lock workspace → NewChatHome | 同上 |
| Mobile FAB | NewChatHome 覆盖层 | 同上 |
| Task 详情「启动/开启新会话」 | 直接 `agentService.index`，agent=`assignee` | 无模式/智能体选择 |

### 1.2 目标

1. **统一入口**：所有通用「新建会话」→ 同一弹窗（或默认配置跳过弹窗）。  
2. **精简创建参数**：对话 / 终端模式 → 智能体（或终端 preset）→ 可选会话名；**隐藏角色/执行者、权限**。  
3. **先建会话再聊天**：确认后立即 `createChatSession` / `createTerminal`，进入 ChatPanel / 终端，使 **slash-command 在 `session_ready` 后可用**。  
4. **默认配置**：New Conversation 区域配置按钮；全局 localStorage；可选 `skipModal` 避免重复弹窗。  
5. **Task 详情通用路径**一并统一；**PM 专用路径**（`createPMSession` / 与 AI 讨论）保留。

### 1.3 非目标（本 Epic 不做）

- 后端会话 API 协议变更  
- 创建时选 PM / executor / verifier 角色（PM 仅走 Task 专用流）  
- 按 workspace 记忆默认（本轮全局一份）  
- `SessionTierPicker`（#328）接入主路径  
- 创建时选 team expert（`agentRef`）— 记为 P2  

---

## 2. 用户故事

### US-1 一键新建（主路径）

> 作为用户，我从侧栏 / 工作区 / 助理 / 项目 / 手机点「新建会话」，看到统一弹窗，选对话或终端与智能体，可选填名称后进入可用的聊天或终端界面。

### US-2 创建后即可用 Slash

> 作为用户，我进入新建的对话后，在 Composer 输入 `/`，能看到该智能体广告的 slash 命令列表（会话就绪后）。

### US-3 默认配置与跳过

> 作为用户，我在 New Conversation 配置默认 mode/agent，并勾选「跳过弹窗」后，再次新建直接按默认创建，不再每次点确认。

### US-4 Task 开启新会话

> 作为用户，我在任务详情选择「开启新会话」或点「启动新会话」，走同一弹窗（agent 预填 assignee），确认后会话绑定该 task；有输入文本则作为首条 auto-send。

### US-5 PM 讨论不受影响

> 作为用户，我点「与 AI 讨论 / 讨论需求」时，仍直接进入 PM 会话，不经过通用弹窗。

---

## 3. 核心交互流

```
[任意通用「新建会话」入口]
        │
        ▼
  defaults.skipModal === true ?
     │                │
    是               否
     │                ▼
     │         SessionSetupModal
     │         · 模式：对话 | 终端
     │         · 智能体 | 终端 preset
     │         · 会话名称（可选；终端可弱化）
     │         · 无 lock 时：workspace 选择
     │         · 无 role / permission
     │                │
     └───────┬────────┘
             ▼
   chat → createChatSession(ws, name, agent, …)   // 默认无 initialMessage
   term → createTerminal(ws, cwd, presetCmd?)
             │
             ▼
   ChatPanel（slash 就绪后可用） / Terminal
```

**配置按钮（New Conversation / 精简落地页）**

- 打开与弹窗共用的表单 + `skipModal` 开关  
- 存储 key：`1agents_session_setup_defaults`  
- 字段：`{ mode: 'chat'|'terminal', agentType, terminalPreset?, skipModal: boolean }`

---

## 4. 功能范围（P0–P2）

### P0 — MVP：统一弹窗 + 主入口 + 先建会话（必须）

| ID | 交付物 | 验收要点 |
|----|--------|----------|
| P0-1 | `sessionSetupDefaults` 模块（localStorage load/save） | 刷新后默认仍在；非法 JSON 回落安全默认 |
| P0-2 | `SessionSetupModal`（升级/替换 `SessionCreateModal`） | 仅 mode + agent/preset + name；无 role/permission |
| P0-3 | `modalStore.openSessionSetup` + `ModalHost` 提交分支 | chat → `createChatSession`；term → `createTerminal`；无 initial 时进空 ChatPanel |
| P0-4 | 统一入口：侧栏「新对话」、工作区 `…`、助理详情、项目详情 | 全部走 setup；lock workspace 时隐藏 picker |
| P0-5 | Mobile FAB 走同一 setup | 确认后关闭覆盖层并进入会话 |
| P0-6 | i18n（zh/en）+ 基础样式（mode 切换对齐现有 design tokens） | 无硬编码中文主文案 |
| P0-7 | 手动验收：对话创建后 `/` 出现 slash（agent 已广告时） | 与 Composer 现有 palette 一致 |

**P0 成功标准：** 主入口只剩一种创建 UI；创建后 slash 可用；创建 UI 无执行者/权限。

### P1 — 落地页精简 + 默认跳过 + Task 统一

| ID | 交付物 | 验收要点 |
|----|--------|----------|
| P1-1 | 精简 `NewChatHome`：配置齿轮 + CTA「新建会话」 | 不再 mode/agent/role/perm + 首条 create-on-submit 主路径 |
| P1-2 | 配置面板：编辑 defaults + `skipModal` | 开启 skip 后入口直接建会话不弹窗；关闭后恢复弹窗 |
| P1-3 | Task 详情 `openNewSession` / composer `replyMode=new` 走 `openSessionSetup` | 预填 agent=assignee；带 `taskId`；有文本则 `pendingInitialMessage` auto-send |
| P1-4 | `ContentViewHost.renderNewChat` 对接精简落地 | 取消时/空态仍可打开配置与 CTA |
| P1-5 | 回归：`createPMSession` / 与 AI 讨论 / 讨论需求 | **不经**通用弹窗，行为与现网一致 |

**P1 成功标准：** NewChatHome 不再是第二套创建台；Task 通用新会话与主路径一致；skip 可用；PM 专用流完好。

### P2 — 体验与边界扩展（可排期）

| ID | 交付物 | 验收要点 |
|----|--------|----------|
| P2-1 | `TerminalEmptyState`「新建终端」纳入 SessionSetup（mode 预填 terminal） | 与侧栏终端创建一致 |
| P2-2 | 弹窗/默认中可选 team expert（`agentRef`） | 有 team 时展示；connect 时注入与现 NewChatHome 一致 |
| P2-3 | 弹窗内「记住本次选择」快捷写 defaults | 不必先打开配置页 |
| P2-4 | 无默认 agent / catalog 未就绪时的降级文案与禁用确认 | 不静默失败 |
| P2-5 | （可选）创建后空态引导：提示「输入 / 查看命令」 | 仅文案，不改协议 |

---

## 5. 信息架构与组件

### 5.1 新建 / 重构

| 模块 | 职责 |
|------|------|
| `frontend/src/stores/sessionSetupDefaults.ts` | 全局 defaults 读写 |
| `SessionSetupForm`（建议） | 弹窗与配置共用表单 |
| `SessionSetupModal` | 替换 `SessionCreateModal` UI 契约 |

### 5.2 改动面（入口）

- `sessionStore.onStartNewChat` → 触发 setup 或 skip 直建  
- `modalStore` / `ModalHost`  
- `LeftSidebar` / `DesktopAppLayout` / `MobileAppLayout`  
- `AssistantDetail` / `ProjectShell`  
- `NewChatHome` / `ContentViewHost`  
- `TaskDetail`  
- `i18n/dict.ts`、`style/index.scss`  

### 5.3 复用、不改协议

- `createChatSession` / `createTerminal` / `pendingInitialMessage`  
- `Composer` + `SlashCommandPalette` + `useBridge.availableCommands`  
- `AgentTypePicker`  
- `createPMSession`  

### 5.4 权限与角色策略

| 时机 | 行为 |
|------|------|
| 创建 | 不传 role（general）；不传 permissionMode（后端/会话默认） |
| 进行中 | Composer 保留 SessionMode / Permission 运行时切换 |
| PM | 仅 `createPMSession` 写 `pm`/`pmo` |

---

## 6. 数据与存储

```ts
// localStorage: 1agents_session_setup_defaults
interface SessionSetupDefaults {
  mode: 'chat' | 'terminal';
  agentType: AgentType;           // chat
  terminalPreset?: 'claude' | 'codex' | 'gemini' | 'shell';
  skipModal: boolean;
}
```

- 作用域：**全局**（非 per-workspace）  
- workspace：入口上下文或弹窗选择；lock 时固定  
- 会话名空：沿用现逻辑自动生成（如 `{Agent} 会话`）

---

## 7. 入口矩阵（实现核对表）

| 入口 | P0 | 参数 |
|------|----|------|
| 侧栏「新对话」 | ✅ | 可选 ws；无 lock |
| 工作区 `…` 新建会话 | ✅ | lock 到该 ws |
| 助理详情新建 | ✅ | lock assistant ws |
| 项目详情新建 | ✅ | lock project ws |
| Mobile FAB | ✅ | 同桌面 |
| Task 启动/开启新会话 | P1 | taskId + 可选 initialMessage |
| PM 讨论 | 不改 | createPMSession |
| Terminal 空态 | P2 | mode=terminal |

---

## 8. 风险与决策记录

| 风险 | 缓解 |
|------|------|
| `onStartNewChat` 语义从「打开落地页」变为「setup/直建」 | 全调用点审计；空态保留精简 NewChatHome |
| PM 无法从通用弹窗创建 | 产品确认：本轮仅 Task 专用流 |
| 无 session 则无 slash | 强制「先建会话」；验收依赖 agent 广告 commands |
| Mobile 弹窗与覆盖层叠层 | 优先弹窗；确认后关 showNewChat |
| expert 缺失 | P2；P0 不传 agentRef |

**已确认产品决策（2026-07-16）**

1. 弹窗确认 → **先建会话再进 ChatPanel**  
2. Task 详情通用路径 **一并统一**  
3. 默认配置 **全局 localStorage**  

---

## 9. 测试与验收清单

### P0

- [ ] 无 skip：侧栏新建 → 弹窗 → 对话+agent → ChatPanel，`/` 有 slash（若 agent 支持）  
- [ ] 弹窗终端模式 → 打开终端  
- [ ] 工作区 `…` 同一弹窗，无 role/permission  
- [ ] 助理/项目 lock：无跨项目 picker  
- [ ] Mobile FAB 与桌面一致  
- [ ] 进行中 Composer 权限/模式仍可用  

### P1

- [ ] 配置 skip 后直接建会话  
- [ ] 关 skip 恢复弹窗  
- [ ] Task 新会话带 taskId；有文本 auto-send  
- [ ] PM「与 AI 讨论」不弹通用窗  

### P2

- [ ] TerminalEmptyState 走 setup  
- [ ] expert 可选且生效  

---

## 10. 里程碑与看板映射

**Epic 里程碑：** `统一新建会话`（id `4c573c47d5557e129688b600e895923e`，position 12）

| 优先级 | 需求 | 任务 |
|--------|------|------|
| **P0** | #108 统一 SessionSetup 弹窗与主入口 | #111 defaults · #112 Modal/Form · #113 modalStore/Host · #114 桌面入口 · #115 Mobile · #116 i18n/样式 · #117 验收 |
| **P1** | #109 NewChatHome 精简 + skip + Task | #118 NewChatHome · #119 skipModal · #120 TaskDetail · #121 P1 回归 |
| **P2** | #110 SessionSetup 体验扩展 | #122 TerminalEmptyState · #123 expert · #124 记住选择/catalog |

**推荐执行顺序（P0）：** `#111 → #112 → #113 → (#114 ∥ #115 ∥ #116) → #117`  
**P1** 依赖 #108 闭环后：`#118 ∥ #119 ∥ #120 → #121`  
**P2** 依赖 #109 或至少 #108：`#122 ∥ #123 ∥ #124`

看板条目 description 已用 `#108` / `#109` / `#110` 归口。

---

## 11. 实现顺序建议（给执行 agent）

1. defaults 模块  
2. SessionSetupForm + Modal + modalStore/ModalHost  
3. 改写 `onStartNewChat` / openChatCreate 及桌面/移动/助理/项目入口  
4. i18n + 样式  
5. 精简 NewChatHome + 配置按钮（P1）  
6. TaskDetail（P1）  
7. P2 按需  

**文件级细节**见实现 plan；本 PRD 为产品与验收真源。

---

## 12. 附录：现状关键文件

- `frontend/src/components/chat/NewChatHome.tsx`  
- `frontend/src/components/chat/SessionCreateModal.tsx`  
- `frontend/src/components/chat/Composer.tsx` / `ChatPanel.tsx`  
- `frontend/src/stores/sessionStore.ts` / `modalStore.ts`  
- `frontend/src/components/modal/ModalHost.tsx`  
- `frontend/src/components/drawer/TaskList/TaskDetail.tsx`  
- `frontend/src/components/stage/ContentViewHost.tsx`  
