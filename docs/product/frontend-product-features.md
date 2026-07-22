# 1Agents 前端产品功能文档

本文从当前前端实现出发，梳理 1Agents App 的产品功能模块、每个模块承载的用户能力，以及主要前端入口。它用于产品规划、设计改版、功能验收和团队沟通。

当前前端的产品定位可以概括为：

> 面向人机协作的 Agentic Workbench：把项目、会话、终端、文件、任务、数据源、联系人和插件应用组织在同一个远程智能体工作台里。

## 1. 启动、接入与基础门禁

用户进入 App 后，前端首先处理后端连接、访问权限、工作区数据加载和首屏模式。

主要功能：

- 后端连接初始化：支持本机直连、Relay 中转、未连接三种状态。
- Relay 配对门禁：当未选中远程节点或中转节点断开时，引导用户重新配对/选择节点。
- Access Token 门禁：直连模式下根据后端访问控制显示访问令牌验证。
- 首次使用引导：支持欢迎页、初学者/高级模式选择。
- 工作区与会话初始化：加载项目、终端窗口、聊天会话、Agent 目录、远程设备。
- Relay 重连恢复：中转连接恢复后重新拉取工作区、终端、Agent 目录和会话列表。
- PWA 能力：注册 service worker，支持 Web App 形态。

主要前端入口：

- `frontend/src/components/app.tsx`
- `frontend/src/components/auth/AccessTokenGate.tsx`
- `frontend/src/components/welcome/WelcomeOnboarding.tsx`
- `frontend/src/components/welcome/ModeSelectOnboarding.tsx`
- `frontend/src/components/settings/RelayOnboarding.tsx`

## 2. 全局工作台 Shell

全局 Shell 是前端所有核心功能的承载框架。桌面端以左侧工作区树、顶部工作区信息、主工作区画布、右侧内容/详情面板组织；移动端使用单列与底部/菜单式导航。

主要功能：

- 桌面/移动布局切换。
- 左侧项目、助理、会话、远程设备导航。
- 顶部工作区 Header：展示项目/会话上下文、路径、连接状态、主题、语言、终端鼠标模式等。
- 多标签页：支持预览、浏览器等动态标签页。
- 两列工作台：左侧会话/终端主线，右侧项目、文件、Git、渠道、任务等内容面板。
- Focus / Split / Project Overview / Project Detail 等布局模式。
- 可拖拽调整侧栏宽度和左右分栏比例。
- 全局搜索入口。
- 模块化内容渲染：通过 `ContentViewHost` 把终端、聊天、任务、文件、设置、数据源等视图统一挂载。

主要前端入口：

- `frontend/src/components/desktop/DesktopAppLayout.tsx`
- `frontend/src/components/mobile/MobileAppLayout.tsx`
- `frontend/src/components/sidebar/LeftSidebar.tsx`
- `frontend/src/components/header/WorkspaceHeader.tsx`
- `frontend/src/components/stage/ContentViewHost.tsx`
- `frontend/src/components/shared/WorkbenchCanvas.tsx`
- `frontend/src/components/shared/RightPanelHost.tsx`
- `frontend/src/components/search/GlobalSearch.tsx`

## 3. 项目与工作区管理

项目是 1Agents 的主要业务容器。每个项目对应一个工作区路径，承载会话、任务、文件、渠道、数据源、团队、技能、配置和插件页。

主要功能：

- 项目总览：展示当前项目列表、归档项目、项目模板入口。
- 新建项目：通过全局项目创建弹窗创建项目/工作区。
- 项目搜索：在项目首页按名称搜索。
- 项目详情页：进入单个项目后查看会话、团队、任务、文件、渠道、动态、计划、资产和设置。
- 项目标签页：内置标签与业务 App 贡献的 project-tab 共同组成项目内导航。
- 项目归档/恢复：在项目设置中切换活跃/归档状态。
- 项目配置：管理指令、连接器、可见标签页等项目级配置。
- 项目模板：已安装 App 可作为模板入口展示。
- 大屏入口：从项目首页打开 Dashboard 大屏。

主要前端入口：

- `frontend/src/components/platform/ProjectHome.tsx`
- `frontend/src/components/platform/ProjectShell.tsx`
- `frontend/src/components/platform/ProjectDetailShell.tsx`
- `frontend/src/components/platform/ProjectConfigPanel.tsx`
- `frontend/src/components/pages/SettingsTab.tsx`
- `frontend/src/components/modal/WorkspaceModal.tsx`

## 4. 智能体会话与 Chat 工作流

Chat 是用户与智能体协作的核心入口。它不仅是对话窗口，也承载工具调用、权限确认、计划清单、文件 diff、终端命令和过程审计。

主要功能：

- 新建会话首页：选择工作区、Agent 类型、角色、权限模式、初始 Prompt。
- 支持普通会话和 PM 会话等不同角色。
- 支持将会话锁定到某个项目/助理上下文。
- 会话列表：按工作区归属展示聊天会话，支持选择、重命名、删除。
- 消息流：展示用户消息、助理回复、流式输出。
- Agentic 过程块：展示 thinking、tool call、tool result、permission request、diff、命令输出等过程信息。
- Markdown 渲染：支持列表、代码块、链接、表格、Mermaid、任务引用等。
- 文件附件与语音输入：通过输入区能力补充上下文。
- Slash Command：输入命令式快捷操作。
- 权限模式选择：控制 Agent 执行工具/命令时的确认策略。
- 使用量展示：展示 token/usage 等会话元信息。
- 会话接管提示：当会话被其它执行端接管时提示用户。
- 历史会话加载与回放。

主要前端入口：

- `frontend/src/components/chat/NewChatHome.tsx`
- `frontend/src/components/chat/ChatPanel.tsx`
- `frontend/src/components/chat/MessageList.tsx`
- `frontend/src/components/chat/MessageBubble.tsx`
- `frontend/src/components/chat/Composer.tsx`
- `frontend/src/components/chat/ToolDiffView.tsx`
- `frontend/src/components/chat/PlanChecklist.tsx`
- `frontend/src/components/chat/PermissionModePicker.tsx`
- `frontend/src/components/chat/SlashCommandPalette.tsx`

## 5. 终端与远程命令工作台

终端功能为远程工作区提供 shell 操作能力，是 Agent 执行与用户接管的重要界面。

主要功能：

- xterm.js 终端渲染。
- 终端 WebSocket 连接与 token 获取。
- 多终端窗口加载与轮询。
- 新建终端：可指定工作区和 cwd，并支持初始命令。
- 终端会话列表：在侧栏与其它会话一起展示。
- 终端空状态：没有终端时显示明确入口。
- tmux 鼠标模式切换。
- 移动端终端字体、键盘与视口适配。
- 终端主题：根据 light/dark 模式切换。

主要前端入口：

- `frontend/src/components/terminal/index.tsx`
- `frontend/src/components/terminal/terminalConfig.ts`
- `frontend/src/components/shared/TerminalEmptyState.tsx`
- `frontend/src/components/stage/ContentViewHost.tsx`

## 6. PMO、任务与路线图

任务系统是项目执行和 PM 管理的核心。前端把讨论、需求、任务、缺陷、里程碑、会话和 Agent 建议组织成可规划、可追踪、可执行的工作池。

主要功能：

- 任务总览：展示项目任务健康度、会话数、待办、风险等概览信息。
- 讨论区：承载尚未明确为需求/任务的方向和想法。
- 需求池：管理 requirement / bug / agent-suggested 建议，支持采纳或忽略 AI 建议。
- 任务列表：展示可执行 task，支持搜索、筛选、排序、分组、列显隐、内联编辑。
- 看板视图：按状态列展示任务，并支持拖拽到完成/取消等终态。
- 日历视图：按日期展示任务。
- 里程碑视图：创建、编辑、删除里程碑，查看路线图与里程碑下任务。
- 任务详情：查看描述、验收标准、checklist、子任务、关系、评论、会话分支和属性。
- 任务深链：支持 `#编号` 和 URL deep link 打开任务详情。
- PM 会话入口：从讨论区触发 PM 会话，把模糊想法梳理成讨论、需求或任务。
- 会话关联任务：任务详情可展示相关执行会话，侧栏会话可显示任务徽标。

主要前端入口：

- `frontend/src/components/drawer/TaskList/index.tsx`
- `frontend/src/components/drawer/TaskList/Overview.tsx`
- `frontend/src/components/drawer/TaskList/DiscussionView.tsx`
- `frontend/src/components/drawer/TaskList/RequirementPool.tsx`
- `frontend/src/components/drawer/TaskList/TasksView.tsx`
- `frontend/src/components/drawer/TaskList/TaskTable.tsx`
- `frontend/src/components/drawer/TaskList/DataGrid.tsx`
- `frontend/src/components/drawer/TaskList/KanbanBoard.tsx`
- `frontend/src/components/drawer/TaskList/CalendarBoard.tsx`
- `frontend/src/components/drawer/TaskList/MilestoneView.tsx`
- `frontend/src/components/drawer/TaskList/TaskDetail.tsx`

## 7. 文件、预览与 Git

文件与 Git 模块用于查看项目文件、预览内容、管理代码变更和执行版本控制动作。

主要功能：

- 文件浏览：浏览当前工作区文件树。
- 文件预览：打开文件预览标签页。
- Markdown/文档内容渲染：用于查看文本、文档片段和 Markdown 文件。
- 工作区文件分栏：在项目详情中查看文件。
- Git 面板：面向当前工作区展示 Git 操作和状态。
- Diff 展示：Chat 工具结果和 Git/文件相关流程可展示 diff。
- 内置浏览器：打开 URL 或 Dashboard 等 Web 页面。

主要前端入口：

- `frontend/src/components/drawer/FlatFileBrowser.tsx`
- `frontend/src/components/shared/FilePreviewContent.tsx`
- `frontend/src/components/shared/WorkspacePanes.tsx`
- `frontend/src/components/drawer/GitPanel.tsx`
- `frontend/src/components/chat/ToolDiffView.tsx`
- `frontend/src/components/browser/BuiltinBrowser.tsx`

## 8. 数据源管理与数据治理

数据源模块用于把外部系统接入工作台，并围绕接入数据建立治理视图。

主要功能：

- 数据接入层：以外部数据源账号为中心管理连接。
- 添加数据源：选择 vendor，完成授权/配置并创建账号。
- 数据源账号详情：查看账号配置、授权状态、原始数据区。
- 数据源分区标签：按 source capability 展示授权、配置、同步、数据等区域。
- 原始数据详情：打开指定 source/kind 的 raw table。
- 数据治理层：展示清洗/融合后的治理表。
- 治理表详情：查看某个治理输出表。
- 依赖图与执行记录：用于理解治理链路和运行结果。

主要前端入口：

- `frontend/src/components/drawer/DataSources/index.tsx`
- `frontend/src/components/drawer/DataSources/SourceHome.tsx`
- `frontend/src/components/drawer/DataSources/AddSource.tsx`
- `frontend/src/components/drawer/DataSources/SourcePanel.tsx`
- `frontend/src/components/drawer/DataSources/SourceDetail.tsx`
- `frontend/src/components/drawer/DataSources/GovernanceView.tsx`
- `frontend/src/components/drawer/DataSources/TaskRunsGrid.tsx`

## 9. 联系人、消息、收件箱与日程

这组功能把外部上下文和个人待办收束到工作台中，用于后续分发到项目或任务。

### 9.1 联系人与消息

主要功能：

- 联系人聚合：从同步消息中发现联系人身份。
- 联系人表格：支持搜索、分组、排序、联系人详情。
- 一度/二度关系过滤。
- 新建/编辑联系人。
- 关联或解绑渠道身份。
- 消息视图：查看外部渠道消息。
- 群聊气泡展示。

主要前端入口：

- `frontend/src/components/drawer/Contacts/index.tsx`
- `frontend/src/components/drawer/Contacts/contactGrid.tsx`
- `frontend/src/components/drawer/Contacts/GroupChatBubbles.tsx`

### 9.2 Inbox 信息收口层

主要功能：

- 手动捕获文本或 URL。
- 展示未读/已归档收件项。
- 标记已读/未读。
- 归档/取消归档。
- 将 Inbox 项分发到某个项目的需求池。

主要前端入口：

- `frontend/src/components/drawer/Inbox/index.tsx`

### 9.3 个人日程与提醒

主要功能：

- 月历聚合个人提醒、项目任务、里程碑日期。
- 新建提醒。
- 点击任务日程深链到任务详情。
- 点击提醒或里程碑打开轻量弹窗。
- 刷新日程数据。

主要前端入口：

- `frontend/src/components/drawer/Reminders/index.tsx`
- `frontend/src/components/drawer/Reminders/AgendaCalendar.tsx`
- `frontend/src/components/drawer/Reminders/AgendaItemPopover.tsx`

## 10. 助理、团队、技能与 Agent 能力管理

助理模块用于管理可协作的 Agent/Assistant 工作区；技能模块和团队页用于组织 Agent 的能力来源。

主要功能：

- 助理总览：展示所有助理工作区。
- 助理详情：进入单个助理后查看会话、团队、任务、文件、渠道、设置等。
- 新建助理。
- 默认助理标识。
- 归档助理展示与恢复入口。
- 团队页：管理助理/项目内的技能或成员配置。
- 技能页：查看、添加、移除技能，支持版本和描述展示。
- Agent Catalog：加载可用 Agent 类型及安装命令。
- 1skills 嵌入模块：通过 `<skills-panel>` 管理 skills、agents、slash commands、MCP、marketplace 等。

主要前端入口：

- `frontend/src/components/pages/AssistantsPage.tsx`
- `frontend/src/components/pages/AssistantDetail.tsx`
- `frontend/src/components/pages/TeamTab.tsx`
- `frontend/src/components/pages/SkillsTab.tsx`
- `frontend/src/modules/registry.ts`
- `frontend/src/stores/agentCatalogStore.ts`

## 11. 插件式应用、发现中心与 Studio

前端支持 App Manifest / Mount Point 模型，让业务应用把页面挂载到 L1、项目标签页或 Lens overlay 中。

主要功能：

- App Manifest 加载：加载已安装业务应用的声明。
- L1 App 页面：业务应用可贡献一级页面。
- Project Tab：业务应用可贡献项目内标签页。
- Lens Overlay：业务应用可叠加项目/首页维度的辅助视图。
- 发现中心：展示应用、精选推荐、开源项目。

主要前端入口：

- `frontend/src/modules/appViewRegistry.ts`
- `frontend/src/modules/registry.ts`
- `frontend/src/modules/discovery-manifest.ts`
- `frontend/src/components/platform/L1Shell.tsx`
- `frontend/src/components/platform/MountPointRenderer.tsx`
- `frontend/src/components/drawer/DiscoveryPanel.tsx`

## 12. Provider、渠道与 cc-connect

这组功能连接外部通信平台和 Agent 执行平台，是会话、渠道和消息流转的底层配置界面。

主要功能：

- cc-connect Provider 面板嵌入。
- 项目渠道页：查看当前项目的渠道/连接信息。
- Relay fetch 安装：让嵌入模块在 relay 模式下也走统一中转。
- 会话与渠道上下文联动：进入项目或 App 页面时注入 co-pilot / connector 上下文。

主要前端入口：

- `frontend/src/components/shared/CcProvidersPanel.tsx`
- `frontend/src/components/shared/WorkspacePanes.tsx`
- `frontend/packages/core/services/relay/installRelayFetch.ts`
- `frontend/src/stores/appManifestStore.ts`
- `frontend/src/components/platform/ProjectShell.tsx`

## 13. 系统设置

系统设置负责 App 全局偏好、账户/Relay、设备、更新、关于和危险操作。

主要功能：

- 通用设置：语言切换、初学者/高级模式。
- 外观设置：浅色/深色主题。
- 账户/订阅：在 Relay client host 下展示账户相关设置。
- Agent 设置：展示 Agent 类型和安装命令。
- Relay 配对：配置远程控制和账号级配对。
- 设备：查看本机/远程设备状态。
- 更新：检查前端 manifest、后端版本，并触发更新。
- 本地缓存重置：清空浏览器 localStorage。
- 服务端数据重置：清空本地 App 数据但保留 Relay 配对身份。
- 关于：展示版本和产品信息。

主要前端入口：

- `frontend/src/components/settings/SystemSettings.tsx`
- `frontend/src/components/settings/RelayPairingPanel.tsx`
- `frontend/src/components/settings/SubscriptionPanel.tsx`
- `frontend/src/components/settings/DevicesPanel.tsx`
- `frontend/src/components/settings/LocalMachinePanel.tsx`
- `frontend/src/components/shared/SystemSettingsHost.tsx`
- `frontend/src/modules/settings-manifest.ts`

## 14. Dashboard 大屏

Dashboard 是项目和团队运行状态的大屏入口，目前包含项目 cockpit、工作坊视图、全局任务板和像素风大屏样式。

主要功能：

- 从项目首页打开 Dashboard。
- 展示项目列表、进度、最近活动和刷新动作。
- 展示全局任务板。
- 展示运行状态、成员/技能/发布等大屏区域。
- 支持作为内置浏览器标签页或独立浏览器页面打开。

主要前端入口：

- `frontend/src/dashboard.tsx`
- `frontend/src/components/desktop/DashboardApp.tsx`
- `frontend/src/components/desktop/DashboardCockpit.tsx`
- `frontend/src/components/desktop/DashboardWorkshop.tsx`
- `frontend/src/components/desktop/GlobalTaskBoard.tsx`
- `frontend/src/style/dashboard.scss`

## 15. 移动端体验

移动端不是桌面端的简单缩放，而是复用同一套数据和核心能力，改用适合触控的导航与单列布局。

主要功能：

- 自动检测移动端并切换 `MobileAppLayout`。
- 移动端工作区、会话、任务和设置入口。
- 移动端 Chat 与终端。
- 移动端键盘可见状态处理。
- 移动端日程、设置、更多菜单。
- 移动端卡片式会话列表。
- 移动端触控按钮和底部/菜单式导航。

主要前端入口：

- `frontend/src/components/mobile/MobileAppLayout.tsx`
- `frontend/src/components/mobile/MobileAppLayout.scss`
- `frontend/src/components/shared/SystemSettingsHost.tsx`
- `frontend/src/components/shared/WorkbenchCanvas.tsx`

## 16. 当前功能分层总览

从产品能力上，当前前端可以归为以下几块：

| 功能域 | 用户目标 | 代表功能 |
|---|---|---|
| 接入与门禁 | 连接本机或远程 1Agents 后端 | Relay 配对、Access Token、初始化加载 |
| 工作台 Shell | 在一个界面中切换项目、会话、终端和工具 | 左栏、Header、双栏画布、多标签、搜索 |
| 项目管理 | 管理工作区和项目执行上下文 | 项目总览、项目详情、归档、配置、模板 |
| 智能体会话 | 与 Agent 协作并审计过程 | Chat、工具调用、权限确认、Markdown、附件、语音 |
| 终端 | 接管远程命令行 | xterm、多终端、tmux 鼠标、初始命令 |
| PMO 任务 | 把想法变成需求和可执行任务 | 讨论、需求池、任务、看板、里程碑、详情 |
| 文件与 Git | 查看和管理项目产物 | 文件树、预览、diff、Git 面板、浏览器 |
| 数据源 | 接入和治理外部数据 | 数据接入、账号配置、raw table、治理表 |
| 外部上下文 | 收束联系人、消息、资料和日程 | 联系人、消息、Inbox、提醒、日历 |
| 助理与技能 | 管理 Agent 能力和协作对象 | 助理、团队、技能、1skills、Agent Catalog |
| 插件应用 | 扩展业务工作流 | App Manifest、Discovery |
| Provider/渠道 | 管理通信和执行平台连接 | cc-connect providers、渠道、Relay fetch |
| 系统设置 | 管理 App 偏好和运维动作 | 主题、语言、模式、设备、更新、重置 |
| Dashboard | 观察整体运行状态 | 项目 cockpit、任务板、大屏 |
| 移动端 | 在手机上使用核心工作流 | 移动布局、触控导航、移动 Chat/终端/设置 |

## 17. 与设计标准体系的关系

当前功能模块基本覆盖 `docs/design_rules/page-patterns.md` 中定义的页面类型：

- Chat Page：智能体会话模块。
- Task Detail Page：PMO 任务详情。
- Project Overview Page：项目首页和 Dashboard 的项目 cockpit。
- Task / Session List Page：任务列表、会话列表、联系人表格。
- Data Sources Page：数据源管理。
- Agent / Assistant Management Page：助理、团队、技能。
- File / Diff Page：文件预览、Git、Chat diff。
- Git / Deployment / QA Page：GitPanel 和相关过程输出区域。
- Settings Page：系统设置与项目设置。
- Dashboard Page：Dashboard 大屏。
- Mobile Page Rules：MobileAppLayout。

后续设计改版可以按这个功能拆分推进：先统一全局 Shell 和共享组件，再逐个功能域迁移页面模式，最后做桌面/移动端回归。
