# 深度反思：1agents 现存系统断层与 Event-Driven 自动闭环的缩短差距路径

> **记录时间**：2026-08-06  
> **归档位置**：`docs/insights/02-1agents系统闭环反思与事件驱动缩短差距路径.md`  
> **分类**：系统反思 / 架构诊断 / 演进路线 (Insights & Retrospective)

---

## 1. 反思背景：强大的底层架构与实际运行的“不闭环”困境

目前 1agents 已经在代码与架构设计上建立了非常超前且强大的基础设施：
* 拥有统一的 `ProjectItem` 看板数据实体与多维关系；
* 拥有 `1ACP` 无头 agent 调度能力与 `SessionRoleAuto` 自动化运行身份；
* 架构上明确划分了 `agent`（柔性推理）、`function`（刚性程序）、`human`（决策拍板）三角分工模型；
* 拥有 `Scheduler` 扫描与 Workspace 锁控制机制。

然而，在**实际项目调度与执行过程**中，使用者会明显感受到系统**不够闭环**，存在多处致命的**环节节点断层**，导致设想中的“事件驱动 (Event-Driven) 自动化 Agent 流水线”始终无法顺畅运转。随着项目规模膨胀，现状与理想目标的差距逐渐变得模糊。本文旨在精准诊断断层所在，并给出明确的缩短差距路径。

---

## 2. 现存的 5 大关键节点断层（核心诊断）

系统目前之所以“跑不闭环”，主要在于以下 5 个维度的执行断层：

### 💔 断层 1：伪事件驱动 —— 依赖 5 秒定时轮询 (Polling vs. Real Event)
* **现状**：后端的 `Scheduler` 底层采用 `time.NewTicker(5 * time.Second)` 定时扫盘（`tickWorkspace`）。
* **断层表现**：
  * 卡片 A 完成后，卡片 B 无法被秒级实时触发，必须死板等待下一个轮询周期；
  * 一旦扫描过程中遇到 WorkspaceLock 竞争或锁超时，整个接力链条就会静默停滞；
  * 缺少实时内存事件总线（Real-time Event Reactor），系统无法对卡片状态迁移、Artifact 产出做出即时反应。

### 💔 断层 2：Spec 与上下文在节点接力间“被动截断”（Context Lost in Handoff）
* **现状**：`TaskRunner.Execute()` 在调度 Agent 时，拼接的 `instruction` 仅包含当前 Task 的 `Title` + `Description` + `AcceptanceCriteria`。
* **断层表现**：
  * 上游需求/架构 Agent 产出了详尽的架构 Spec Markdown 或 DB Schema 产物；
  * 但当下游前端/后端 Agent 被唤醒时，系统**没有把上游产出的结构化 Spec/Artifacts 自动作为 Context 挂载输入**；
  * 下游 Agent 如同“失忆者”，仅凭卡片上简短的一句话 Description 瞎猜盲写，导致前后端产出严重脱节。

### 💔 断层 3：Executor 三态中 Human 与 Function 链条断裂
* **现状**：系统定义了 `agent` / `function` / `human` 三条轨道，但实际落地极度失衡。
* **断层表现**：
  * **Human 节点阻塞后无提醒**：一旦任务流转到 `assignee=user`（人工决策卡片）或等待 Review，系统缺乏桌面前端强弹窗或多通道（微信/飞书）主动推卡片机制。人类根本不知道系统在等他拍板，整个自动流水线直接死卡在那里。
  * **Function 节点能力僵化**：用户无法在看板上自由给卡片绑定指定的 Shell 脚本或 Webhook URL，导致 `executor=function` 的刚性快道无法在日常敏捷配置中发挥作用。

### 💔 断层 4：双表面（Chat 与 看板）的数据与状态感知割裂
* **现状**：用户在 Chat 面板里命令 PM Agent 拆解任务，PM Agent 答复“已为你创建 5 个子卡片进行开发”。
* **断层表现**：
  * Chat 面板与任务看板（DataGrid）的数据呈现是异步脱节的；
  * 界面缺乏**“流水线实时执行大屏 (Pipeline Inspector / Pulse)”**，用户看不见后台无头 Agent 跑到了哪一步、日志是什么、报错了还是假死壳了；
  * 由于缺乏可视化掌控感，人类不得不重新在 Chat 框发 Prompt 催问，被迫倒退回“单步被动对话”模式。

### 💔 断层 5：缺乏自愈与失败降级回路 (No Self-Healing Loop)
* **现状**：当无头 Agent 在后台编译报错、依赖缺失或连接超时时，`TaskRunner` 仅简单记录 `TaskStatusFailed` 并终止。
* **断层表现**：
  * 缺乏**自动重试/自愈回路**（如自动把报错日志回喂给 Agent 让其自我修正一次）；
  * 缺乏**降级路由**（Fail 后自动把卡片转为 Human 提示卡片并通知人类）。流水线断裂后静默无声，用户体验极差。

---

## 3. 现状与理想目标的差距度量 (Gap Analysis)

将 1agents 系统从静态工具到全自动闭环划分 4 个成熟度阶段：

```
[L1: 纯 Chat 辅助] ───> [L2: 静态看板+轮询调度] ───> [L3: 实时 Event 流+Spec接力] ───> [L4: 闭环自治+自愈自举]
  (✅ 100% 完成)            (🟡 当前位置: 约 45%)             (❌ 关键断层所在)                (❌ 目标终局)
```

* **已完成 (L1 - 100%)**：强大的 Terminal、xterm.js、1ACP 基础会话、单步 Agent 对话。
* **部分落地 (L2 - 45%)**：`ProjectItem` 模型、JSON/SQLite 存储、`TaskRunner` 无头启动、基础 Scheduler 定时扫描。
* **核心缺口 (L3 - 0%)**：实时内存 Event Bus 响应、Task 上下文/Artifact 自动继承流动、Human 节点通知闭环、流水线可视化大屏。

---

## 4. 缩短差距的 4 步演进实施路径 (Roadmap)

为了彻底打破断层、建立闭环，建议分 4 个阶段按优先级攻坚：

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 阶段 1：构建物理级 Event Bus（秒级响应替换死板轮询）                       │
│ └── 引入内存事件订阅发布机制，卡片 Status / Artifact 变动秒级唤醒 Runner   │
├─────────────────────────────────────────────────────────────────────────┤
│ 阶段 2：打通 Context / Spec 自动接力管道 (Artifact Handoff)               │
│ └── 改造 TaskRunner，自动追溯并挂载上游父节点/关联卡片的 Spec 代码与 Markdown│
├─────────────────────────────────────────────────────────────────────────┤
│ 阶段 3：Human 节点强提醒 + Function 自由绑定 (Human-in-the-loop)          │
│ └── 卡片卡在 Human 节点时强弹窗/飞书推流；支持卡片快捷绑定 Shell/Webhook  │
├─────────────────────────────────────────────────────────────────────────┤
│ 阶段 4：双表面流水线大屏 (Pipeline Inspector & Self-healing)           │
│ └── Chat/看板界面提供实时 Pulse 脉搏，展示后台 Agent 进度；失败自动投递降级│
└─────────────────────────────────────────────────────────────────────────┘
```

### 战役 1：构建物理级 Event Bus（替换 5 秒死轮询）
* **目标**：实现真正的 Event-Driven。
* **落地**：在后端 `internal/commandbus` 或 `internal/agent` 中建立极简内存 Event Reactor。每当 `ProjectItem` 发生更新、`TaskRun` 状态终止或新的 `Artifact` 创建时，立即 Emit 广播事件，调度器直接响应处理，彻底摆脱 5 秒死轮询导致的等待延迟与死锁。

### 战役 2：打通 Context / Spec 自动接力管道 (Artifact Handoff)
* **目标**：让无头 Agent 不再“失忆”。
* **落地**：增强 `TaskRunner.buildTaskInstruction` 逻辑。当调度卡片 X 时，系统自动向向上递归查找其 Parent/Requirement 卡片挂载的所有文件路径（如 `docs/specs/...`、`schema.sql`），自动将其全文或关键 Diff 注入到 Agent 的系统 Prompt 与首条消息中，确保规范无缝传递。

### 战役 3：Human 节点强提醒 + Function 自由绑定
* **目标**：解决人工节点死卡问题。
* **落地**：
  * 当任务 `Executor == human` 或等待人拍板时，通过 WebSocket 给前端发送全局 Alert，并通过 `cc-connect` 自动向用户的微信/飞书发送一键操作卡片。
  * 在 DataGrid 上开放 `function` 配置列，允许用户为简单的自动化步骤指定 Shell 命令或 HTTP 请求，避开 LLM 成本与不确定性。

### 战役 4：双表面流水线大屏 (Pipeline Inspector & Self-healing)
* **目标**：提供掌控感与自愈保障。
* **落地**：
  * 在 Web UI 增加 **Pipeline Inspector (流水线脉搏)** 视图，直观呈现当前项目下各个 Agent 的运行状态、日志流与产物。
  * 增加错误自愈尝试（Fail 后自动携带 Error Log 重新发起 1 次 Repair Turn），若再次失败则自动降级标记为 `Human Review Needed` 并提醒人类介入。

---

## 5. 结语

1agents 并不缺少宏大的架构框架，缺少的是将各个强大模块连接在一起的“事件黏胶”与“上下文接力棒”。通过实施以上 4 步路径，补齐事件驱动、Spec 流转、Human 通知与可视化大屏，1agents 才能真正从“一个强大的 Agent 工具”跨越为“能够自动运转的 AI-native 组织操作系统”。
