# 1Agents 文档索引

本目录按用途分类。入口优先读 **[名称定义表](./product/名称定义表.md)（术语权威 / §0 冲突裁决）→ [核心产品介绍](./product/1Agents_项目介绍.md)（Agent Infra / 工作 Graph / 任务蓝图）→ 架构总纲 → 设计规范 → 功能 PRD**；运维与排错看 `guides/` / `tips/`。术语与表名/kind 以名称定义表为准，专题文档若冲突应回写定义表或改专题。

```
docs/
├── product/          产品定位、路线图、营销、功能总览
├── architecture/     系统架构、集成骨架、分发与多端
├── design_rules/     Agentic Workbench 设计规范（强制）
├── uiux/             交互重构与竞品参照
├── features/         单功能 PRD / 设计 / 走查
├── guides/           编译打包、OAuth、ACP 环境配置
├── tips/             证书、语音等实操技巧
├── experience/       实战经验沉淀（非规格）
├── insights/         核心观察、心得感悟与架构反思
├── GOAI/            Agent Infra 参赛方案与长期演进记录
├── discussions/      讨论稿与落地蓝图
├── assets/           图示、营销页静态资源
└── archive/          历史会话笔记（可忽略）
```

---

## 1. 产品与路线 · `product/`

| 文档 | 说明 |
|------|------|
| [名称定义表.md](./product/名称定义表.md) | **术语与名称总表**（§0 冲突裁决 · ProjectItem / workforce / executor×assignee / Session×AgentTurn） |
| [术语迁移引用清单-M1.md](./product/术语迁移引用清单-M1.md) | Epic #184 迁移触点盘点（代码 + docs 过时句） |
| [roadmap.md](./product/roadmap.md) | **权威版本路线图**：工作 Graph、反馈回路、1.x→4.x 里程碑与 issue 归位 |
| [1Agents_项目介绍.md](./product/1Agents_项目介绍.md) | **核心产品介绍**：一万-Agent Infra、自循环智能体 Graph、任务蓝图与端到端业务链 |
| [1Agents_营销素材与项目定位白皮书.md](./product/1Agents_营销素材与项目定位白皮书.md) | 对外定位、受众、核心话术与社交传播文案 |
| [frontend-product-features.md](./product/frontend-product-features.md) | 前端产品功能总览（模块 ↔ 入口） |
| [产品命名.md](./product/产品命名.md) | 1Agents / 1Hive 品牌命名（细表见名称定义表） |
| [专家智能体-视觉创作台词.md](./product/专家智能体-视觉创作台词.md) | 视觉创作专家 persona / 系统提示词 |

---

## 2. 架构与系统设计 · `architecture/`

| 文档 | 说明 |
|------|------|
| [enterprise-foundation-v1.0.0.md](./architecture/enterprise-foundation-v1.0.0.md) | **v1.0.0 目标架构**：当前/目标分界、D1–D7、三层边界、WorkCase、Command/Query/Event、能力晋升、同库分域、三 Product Shell 与 C0/C1/C2 闸门 |
| [domain-ownership.md](./architecture/domain-ownership.md) | **领域所有权与依赖门禁（C0 已落地）**：kernel_/enterprise_/presales_/commerce_ 命名空间、表/写 API 唯一所有者、受控执行器与拒绝审计、架构门禁规则清单与新增领域应用操作清单（`make archgate`） |
| [agentsOS-架构设计.md](./architecture/agentsOS-架构设计.md) | **架构总纲**：工作 Graph · 任务内核 · 项目外壳；App Registry 与三类挂载基础设施已落地，领域应用逐项演进 |
| [chat-first-agentteams-interaction.md](./architecture/chat-first-agentteams-interaction.md) | **聊天优先交互架构**：Matrix / 1agents Chat 与工作台双表面、AgentTeams Team Runtime、cc-connect Channel Gateway 边界 |
| [app-sdk-contract.md](./architecture/app-sdk-contract.md) | 完整外部应用 SDK 契约草案；当前内部 Manifest / Registry / 挂载边界见 agentsOS 架构 |
| [ai_collaborative_workbench_design.md](./architecture/ai_collaborative_workbench_design.md) | AI 协作工作台设计（Project→Task→Session） |
| [remote_agents和cc-connect的技术架构设计.md](./architecture/remote_agents和cc-connect的技术架构设计.md) | 工作区 ↔ cc-connect 多通道集成 |
| [ota-architecture.md](./architecture/ota-architecture.md) | OTA 多形态分发更新架构 |
| [分布式多设备架构PRD.md](./architecture/分布式多设备架构PRD.md) | 多设备协同 PRD |
| [agent-convergence-roadmap.md](./architecture/agent-convergence-roadmap.md) | Agent 层收敛（happy 传输 + 引擎热替换） |
| [happy-integration-skeleton.md](./architecture/happy-integration-skeleton.md) | Happy 集成骨架（`adapter/` 接缝基准） |
| [happy-cli-fork-sync.md](./architecture/happy-cli-fork-sync.md) | happy-cli fork 同步 runbook |
| [ccpark设计参考.md](./architecture/ccpark设计参考.md) | ccpark/agentdock 可借鉴模式（非接入方案） |
| [miniapp-skills-webview-plan.md](./architecture/miniapp-skills-webview-plan.md) | 小程序 web-view 嵌入 skills 后端方案 |
| [execution-job-agent-profile-architecture.md](./architecture/execution-job-agent-profile-architecture.md) | **ExecutionJob / Trigger / TaskRun / AgentProfile 当前实施基线与防漂移边界** |
| [automation-baseline.md](./architecture/automation-baseline.md) | **自动任务 / Automation 实施基线**：配方台、Function→ACP 两段管线、`core.script`、侧栏合并 |

---

## 3. 设计规范 · `design_rules/`

| 文档 | 说明 |
|------|------|
| [app-agentic-workbench-design-standard.md](./design_rules/app-agentic-workbench-design-standard.md) | **全 APP 设计总纲** |
| [chatui-agentic-design-rules.md](./design_rules/chatui-agentic-design-rules.md) | Chat UI 实现约束 |
| [page-patterns.md](./design_rules/page-patterns.md) | 页面类型与信息架构模式 |
| [component-patterns.md](./design_rules/component-patterns.md) | 可复用组件模式 |

补充交互建议（非强制规格）：[uiux/workbuddy参照与交互重构建议.md](./uiux/workbuddy参照与交互重构建议.md)

---

## 4. 功能设计 · `features/`

按功能域拆分；常见文件名：`prd.md` / `design.md` / `walkthrough.md` / `task.md`。

> **状态说明：** 部分专题保留了立项或实施前写稿时的 `Draft / Proposed / Ready` 状态和“现状缺口”，用于追溯设计演进，不再单独代表当前产品是否已经交付。当前对外能力以 [项目介绍](./product/1Agents_项目介绍.md)、[前端产品功能总览](./product/frontend-product-features.md)和代码为准；需要回看设计取舍时再进入对应专题。

| 功能 | 文档 | 状态侧重 |
|------|------|----------|
| Chat UI / 任务看板 Tab | [chat-ui/](./features/chat-ui/) | 实现计划 + 走查 |
| Agent Turn | [Turn 权威 Journal 迁移](./features/turn-model/journal-authority-migration.md) | 方案 A 已实现：1ACP Journal 真源、SQLite 异步投影、重连恢复 |
| 项目模型（Workspace → ProjectItem） | [project-model/](./features/project-model/) | 已落地设计 + 走查；表名迁移见名称定义表 |
| Issue / ProjectItem 话题模型 | [issue-model/](./features/issue-model/) | 设计 + GitHub 字段映射；主实体 = ProjectItem |
| 可验证完成门禁 | [verification-gate/](./features/verification-gate/) | 提案中 |
| 上下文中心 | [context-center/](./features/context-center/) | 设计 |
| Inbox 全上下文引擎 | [inbox-context-engine/](./features/inbox-context-engine/) | RFC 定稿（引擎/角色吸收） |
| Workspace Inbox（项目邮箱 + 派件接力） | [workspace-inbox/](./features/workspace-inbox/) | 定稿·实现中 |
| 统一新建会话 | [unified-session-setup/](./features/unified-session-setup/) | 设计就绪 |
| 右栏 Artifact 多 Tab | [right-panel-tabs/](./features/right-panel-tabs/) | PRD |
| Git 面板 | [git-panel/](./features/git-panel/) | PRD |
| 多维表格 DataGrid | [data-grid/](./features/data-grid/) | PRD |
| 游戏化大屏 Hangar | [gamified-dashboard/](./features/gamified-dashboard/) | 设计评审 / 素材 |
| 多 Agent 圆桌 | [当前运行机制](./features/agents-roundtable/runtime-reference.md) | 已实现：多人讨论、进度、门禁与恢复 |
| npm 多包拆分 | [npm-package-split/](./features/npm-package-split/) | 分发权威 PRD |
| PM 插件能力 | [pm-standalone/](./features/pm-standalone/) | PRD（主入口仍为 1agents） |

---

## 5. 运维与配置 · `guides/` + `tips/`

| 文档 | 说明 |
|------|------|
| [编译与打包指南.md](./guides/编译与打包指南.md) | Makefile 统一构建 / 打包 / 部署 |
| [acp-agent-env-config.md](./guides/acp-agent-env-config.md) | ACP agent 环境变量与排错 |
| [microsoft-oauth-setup.md](./guides/microsoft-oauth-setup.md) | Microsoft Graph 授权（含世纪互联） |
| [tips/ssl-certificate-guide.md](./tips/ssl-certificate-guide.md) | HTTPS / 证书与高级 Web API |
| [tips/voice-recognition.md](./tips/voice-recognition.md) | 语音识别与麦克风权限 |

---

## 6. 经验、心得与讨论

| 目录 | 文档 | 说明 |
|------|------|------|
| `insights/` | [01-spec驱动编程与Agent自动化执行模式思考.md](./insights/01-spec驱动编程与Agent自动化执行模式思考.md) | **核心观察**：被动对话模式的极限与 Spec 驱动编程在任务看板+自动触发下的重塑 |
| `insights/` | [02-1agents系统闭环反思与事件驱动缩短差距路径.md](./insights/02-1agents系统闭环反思与事件驱动缩短差距路径.md) | **系统深度反思**：1agents 现存 5 大节点断层诊断与 Event-Driven 闭环演进路径 |
| `experience/` | [三执行者实战案例.md](./experience/三执行者实战案例.md) | agent / function / human 经验（认知层） |
| `experience/` | [builtin-browser-webproxy-remotion.md](./experience/builtin-browser-webproxy-remotion.md) | 内置浏览器 path 反代 + Remotion composition 路由（成功经验） |
| `GOAI/` | [技术方案v1.md](./GOAI/技术方案v1.md) | AgentTeams 粗粒度 Team Runtime 参赛方案（当前决策） |
| `GOAI/` | [技术方案（长期）-v1.md](./GOAI/技术方案（长期）-v1.md) | Story Studio 到单人/多人多 Agent 平台的长期路线 |
| `discussions/` | [一芥智能体落地技术实现蓝图_V1.1.md](./discussions/一芥智能体落地技术实现蓝图_V1.1.md) | 落地技术蓝图讨论稿 |
| `assets/` | 设计图、业务流程图、`index.html` | 图示与静态页 |
| `archive/` | ORIGINAL_REQUEST / PROJECT / model-role-notes | 历史会话与个人笔记，默认不作为规格引用 |

---

## 阅读路径建议

1. **新同学上手**：`product/名称定义表` → `product/1Agents_项目介绍` → `product/frontend-product-features` → `architecture/agentsOS-架构设计` → `design_rules/app-agentic-workbench-design-standard`
2. **规划 / 实现 v1.0.0 企业底座**：`architecture/enterprise-foundation-v1.0.0` → `product/roadmap` → 对应功能蓝图与 `features/<域>/`
3. **做功能 / 改 UI**：`design_rules/*` → 对应 `features/<域>/` → 必要时 `uiux/`
4. **接 agent / 改传输层**：`architecture/chat-first-agentteams-interaction` → `architecture/agent-convergence-roadmap` → `happy-integration-skeleton` → `happy-cli-fork-sync` → `adapter/README`
5. **发版 / 部署**：`guides/编译与打包指南` → `architecture/ota-architecture` → `features/npm-package-split/prd`
6. **排错**：`guides/acp-agent-env-config` · `guides/microsoft-oauth-setup` · `tips/*`

---

## 约定

- **权威优先级**：`product/名称定义表.md`（**现行术语 / 实体名 / kind**）> `product/roadmap.md`（版本）> `architecture/enterprise-foundation-v1.0.0.md`（v1.0.0 目标架构）> `architecture/agentsOS-架构设计.md`（当前架构与 App Registry 边界）> `design_rules/`（UI 规范）> 单功能 `features/*`。目标文档不能覆盖代码尚未实现的当前事实。
- **状态字段**：功能文档文首的 Status（Implemented / Draft / RFC 等）表示该专题文档在写作时的阶段；它可以作为设计历史，但不覆盖当前产品入口材料和代码事实。索引表只做导航。
- **新增文档**：按上表选目录；功能级一律放 `features/<kebab-name>/`，避免继续堆在 `docs/` 根目录。
- **过时术语**：`tasks` 表名、`kind=assistant`、`SessionTier`/`professional-project` 落库、`任务可脱离 project` 等——见名称定义表 §0 否决与 §0.0 落地状态；docs 中出现须带标注，不得当现行断言。
