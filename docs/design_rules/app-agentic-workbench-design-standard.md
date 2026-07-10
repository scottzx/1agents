# 1Agents Agentic Workbench Design Standard

本规范是 1Agents 全 APP 的设计总纲。它从 Chat UI 的 Agentic UI 方向抽象而来，适用于工作台、任务、项目、数据源、设置、Agent 管理、Dashboard 等所有产品界面。

一句话原则：

> 主线清晰，过程可审计，界面克制。

## 1. 产品设计定位

1Agents 不是普通聊天软件，也不是营销型 SaaS 后台，而是一个 **智能体协作工作台**。

设计语言应表达：

- 智能体在执行真实工作。
- 用户可以理解、审计、接管关键过程。
- 长时间使用不疲劳。
- 信息密度高，但不嘈杂。
- 视觉克制，内容优先。

推荐命名：

```text
Agentic Workbench
智能体工作台
```

关键词：

- Agentic UI
- Workbench
- Document flow
- Process auditability
- Human-in-the-loop
- Developer-centric aesthetic
- Calm control surface

## 2. 三层信息架构

全 APP 信息展示分三层。

### 2.1 L1 主线内容

用户当前真正关心的东西。

示例：

- Chat 的最终答复
- Task 的目标、状态、验收标准
- Project 的健康状态与当前风险
- Data Source 的连接状态
- File viewer 的正文
- Settings 的当前配置值

设计规则：

- 默认外露。
- 排版最清晰。
- 不被日志、元数据、装饰物抢占注意力。

### 2.2 L2 过程与证据

AI 或系统如何得出结果、做过什么。

示例：

- thinking
- tool calls
- command output
- file diffs
- sync logs
- deployment steps
- permission decisions

设计规则：

- 默认折叠或轻量收纳。
- 必须可展开审计。
- 出错、等待权限、阻塞时可以提升可见性。

### 2.3 L3 系统与元信息

辅助判断但不是主线的信息。

示例：

- token / cost
- session id
- model id
- timestamps
- sync version
- workspace path
- connection details

设计规则：

- 默认弱化。
- 通过 hover、详情面板、折叠区、meta row 展示。
- 不作为页面主视觉。

## 3. 全局布局模式

### 3.1 Workbench Layout

用于 Chat、任务详情、项目执行、文件查看。

结构：

- 主区承载 L1 内容。
- 侧区或折叠区承载 L2/L3。
- 不做 hero。
- 不做营销说明区。

要求：

- 内容流应可连续阅读。
- 执行过程应可展开。
- 工具、文件、任务、结果之间关系要明确。

### 3.2 List-Detail Layout

用于任务列表、会话列表、数据源、联系人、Agent 管理。

结构：

- 左侧或上方是可扫描列表。
- 右侧或下方是详情。

要求：

- 列表行高度稳定。
- 状态点、标题、摘要、时间、操作位置固定。
- 不用大卡片墙承载高频操作对象。

### 3.3 Control Panel Layout

用于 Settings、权限、连接、Provider、数据源配置。

结构：

- 分组表单。
- label + description + control。
- 危险操作独立区域。

要求：

- 控件比说明文字更重要。
- 危险操作可见但不恐吓。
- 配置状态必须清楚。

### 3.4 Operational Dashboard Layout

用于 Dashboard、项目总览、运行状态总览。

结构：

- 紧凑指标。
- 状态列表。
- 风险、阻塞、最近活动。

要求：

- 不做营销 dashboard。
- 不堆砌彩色统计卡。
- 指标必须能指导下一步动作。

## 4. 色彩标准

### 4.1 基调

全 APP 使用柔和白色和中性边框建立工作台质感。

原则：

- 背景偏白，但不刺眼。
- 不使用偏黄乳白作为主背景。
- 不使用明显浅灰造成脏感。
- 不用大面积品牌色。
- 状态色只表达状态，不做装饰。

### 4.2 推荐语义 token

全局 token 应按语义命名，而不是按具体颜色命名。

```css
--surface-page
--surface-panel
--surface-card
--surface-muted
--surface-process

--border-subtle
--border-strong

--text-primary
--text-secondary
--text-muted

--accent-fg
--success-fg
--warning-fg
--danger-fg

--agent-thinking
--agent-tool
--agent-permission
--agent-terminal
```

现有 token 可以逐步映射，不要求一次性重命名。

### 4.3 状态色规则

- Blue：链接、主操作、当前运行。
- Green：成功、完成、允许。
- Amber：等待、思考、权限待确认。
- Red：错误、拒绝、危险。
- Purple：特殊模式或 agent 类型，不作为主视觉。
- Orange：命令执行、外部动作、运行时事件。

## 5. 排版标准

### 5.1 字体

- 正文使用全局 sans。
- 命令、路径、参数、工具名、日志使用 mono。
- 不使用装饰字体。

### 5.2 字号尺度

推荐范围：

```text
正文：13px-14px
主内容行高：1.55-1.7
紧凑列表行高：1.35-1.45
页面标题：18px-22px
区域标题：14px-16px
卡片标题：13px-15px
辅助说明：12px-13px
Meta 信息：11px-12px
Mono 信息：11px-12.5px
```

规则：

- 不用超大标题制造层级。
- 不在紧凑面板内使用 hero 字号。
- 不使用负 letter-spacing。
- 标题层级应通过间距、字重、位置建立，而不是剧烈字号跳变。

## 6. Markdown 标准

全 APP 使用两套 Markdown 场景。

### 6.1 Document Markdown

用于：

- 文档预览
- README
- 说明文档
- 长文档阅读

规则：

- h1/h2 可以更明显。
- 可使用分割线。
- 更接近 GitHub 文档阅读体验。

### 6.2 Conversation Markdown

用于：

- Chat
- 评论
- 任务讨论
- 用户 query
- 助理回答

规则：

- 标题更像段落标题。
- h1/h2 不使用文档式下划线。
- 正文尺寸稳定。
- 列表、代码、表格保持清晰，但不造成强烈字号跳动。

## 7. Agentic 交互原则

### 7.1 先摘要，后细节

复杂过程先展示摘要，细节折叠。

正确：

```text
已同步 12 个数据源，3 个失败
```

展开后再展示每个步骤日志。

### 7.2 默认可审计，但不默认打扰

过程必须存在，但不应默认淹没主线。

规则：

- 成功过程默认折叠。
- 错误过程提升可见性。
- 权限请求必须显式可见。

### 7.3 Human-in-the-loop 必须明确

需要用户确认的动作不能藏起来。

包括：

- 删除
- 覆盖
- 推送
- 部署
- 外发消息
- 执行命令
- 授权数据源
- 修改权限范围

### 7.4 实时与历史一致

实时执行过程和历史回放应结构一致。

示例：

- Chat turn 实时看到工具过程，loadHistory 后仍折叠为同一过程块。
- 数据源同步实时有步骤，历史同步记录也应以摘要 + 展开步骤展示。
- 部署过程实时有日志，历史部署详情也应可审计。

### 7.5 用户输入与 AI 输出平等

用户输入也可能是结构化内容。

规则：

- 用户 query 支持 Markdown。
- 评论支持 Markdown。
- 任务描述支持 Markdown。
- 长链接、代码块、列表不能撑破容器。

## 8. 视觉禁区

全 APP 避免以下模式：

- 营销页 hero 套进工作台。
- 大量渐变、光斑、背景装饰。
- 一屏很多彩色大卡片。
- 传统 IM 强气泡聊天样式。
- 每个工具调用都是厚重卡片。
- 用大字号 H1/H2 渲染聊天内容。
- 过程日志默认全部展开。
- 权限请求隐藏在最终结果后。
- 用状态色做装饰色。

## 9. 与具体规范的关系

本文件是全 APP 总纲。具体落地参考：

- `chatui-agentic-design-rules.md`：Chat UI 具体规则。
- `component-patterns.md`：可复用组件模式。
- `page-patterns.md`：模块页面模式。

当具体页面需求与本总纲冲突时，优先保持本总纲的三层信息架构与 Agentic 交互原则。
