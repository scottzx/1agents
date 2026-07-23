# 1Agents 文档索引

本目录按用途分类。入口优先读 **[名称定义表](./product/名称定义表.md)（术语权威 / §0 冲突裁决）→ 产品定位 → 架构总纲 → 设计规范 → 功能 PRD**；运维与排错看 `guides/` / `tips/`。术语与表名/kind 以名称定义表为准，专题文档若冲突应回写定义表或改专题。

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
├── discussions/      讨论稿与落地蓝图
├── assets/           图示、营销页静态资源
└── archive/          历史会话笔记（可忽略）
```

---

## 1. 产品与路线 · `product/`

| 文档 | 说明 |
|------|------|
| [名称定义表.md](./product/名称定义表.md) | **术语与名称总表**（§0 冲突裁决 · ProjectItem / workforce / executor×assignee） |
| [术语迁移引用清单-M1.md](./product/术语迁移引用清单-M1.md) | Epic #184 迁移触点盘点（代码 + docs 过时句） |
| [roadmap.md](./product/roadmap.md) | **权威版本路线图**（1.x→4.x 里程碑与 issue 归位） |
| [1Agents_项目介绍.md](./product/1Agents_项目介绍.md) | 对外项目介绍 / 关于我们 |
| [1Agents_营销素材与项目定位白皮书.md](./product/1Agents_营销素材与项目定位白皮书.md) | 定位、受众、营销素材 |
| [frontend-product-features.md](./product/frontend-product-features.md) | 前端产品功能总览（模块 ↔ 入口） |
| [产品命名.md](./product/产品命名.md) | 1Agents / 1Hive 品牌命名（细表见名称定义表） |
| [专家智能体-视觉创作台词.md](./product/专家智能体-视觉创作台词.md) | 视觉创作专家 persona / 系统提示词 |

---

## 2. 架构与系统设计 · `architecture/`

| 文档 | 说明 |
|------|------|
| [agentsOS-架构设计.md](./architecture/agentsOS-架构设计.md) | **架构总纲**：任务内核 · 项目外壳；App Registry / 多应用为**远期** |
| [app-sdk-contract.md](./architecture/app-sdk-contract.md) | 应用 SDK 契约草案（**远期** registry；北向任务 API / 三存储面） |
| [ai_collaborative_workbench_design.md](./architecture/ai_collaborative_workbench_design.md) | AI 协作工作台设计（Project→Task→Session） |
| [remote_agents和cc-connect的技术架构设计.md](./architecture/remote_agents和cc-connect的技术架构设计.md) | 工作区 ↔ cc-connect 多通道集成 |
| [ota-architecture.md](./architecture/ota-architecture.md) | OTA 多形态分发更新架构 |
| [分布式多设备架构PRD.md](./architecture/分布式多设备架构PRD.md) | 多设备协同 PRD |
| [agent-convergence-roadmap.md](./architecture/agent-convergence-roadmap.md) | Agent 层收敛（happy 传输 + 引擎热替换） |
| [happy-integration-skeleton.md](./architecture/happy-integration-skeleton.md) | Happy 集成骨架（`adapter/` 接缝基准） |
| [happy-cli-fork-sync.md](./architecture/happy-cli-fork-sync.md) | happy-cli fork 同步 runbook |
| [ccpark设计参考.md](./architecture/ccpark设计参考.md) | ccpark/agentdock 可借鉴模式（非接入方案） |
| [miniapp-skills-webview-plan.md](./architecture/miniapp-skills-webview-plan.md) | 小程序 web-view 嵌入 skills 后端方案 |

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

| 功能 | 文档 | 状态侧重 |
|------|------|----------|
| Chat UI / 任务看板 Tab | [chat-ui/](./features/chat-ui/) | 实现计划 + 走查 |
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

## 6. 经验、讨论与归档

| 目录 | 文档 | 说明 |
|------|------|------|
| `experience/` | [三执行者实战案例.md](./experience/三执行者实战案例.md) | agent / function / human 经验（认知层） |
| `experience/` | [builtin-browser-webproxy-remotion.md](./experience/builtin-browser-webproxy-remotion.md) | 内置浏览器 path 反代 + Remotion composition 路由（成功经验） |
| `discussions/` | [一芥智能体落地技术实现蓝图_V1.1.md](./discussions/一芥智能体落地技术实现蓝图_V1.1.md) | 落地技术蓝图讨论稿 |
| `assets/` | 设计图、业务流程图、`index.html` | 图示与静态页 |
| `archive/` | ORIGINAL_REQUEST / PROJECT / model-role-notes | 历史会话与个人笔记，默认不作为规格引用 |

---

## 阅读路径建议

1. **新同学上手**：`product/名称定义表` → `product/1Agents_项目介绍` → `product/frontend-product-features` → `architecture/agentsOS-架构设计` → `design_rules/app-agentic-workbench-design-standard`
2. **做功能 / 改 UI**：`design_rules/*` → 对应 `features/<域>/` → 必要时 `uiux/`
3. **接 agent / 改传输层**：`architecture/agent-convergence-roadmap` → `happy-integration-skeleton` → `happy-cli-fork-sync` → `adapter/README`
4. **发版 / 部署**：`guides/编译与打包指南` → `architecture/ota-architecture` → `features/npm-package-split/prd`
5. **排错**：`guides/acp-agent-env-config` · `guides/microsoft-oauth-setup` · `tips/*`

---

## 约定

- **权威优先级**：`product/名称定义表.md`（**术语 / 实体名 / kind**）> `product/roadmap.md`（版本）> `architecture/agentsOS-架构设计.md`（架构；registry 段为远期）> `design_rules/`（UI 规范）> 单功能 `features/*`。
- **状态字段**：功能文档文首的 Status（Implemented / Draft / RFC 等）以该文件为准；索引表只做导航。
- **新增文档**：按上表选目录；功能级一律放 `features/<kebab-name>/`，避免继续堆在 `docs/` 根目录。
- **过时术语**：`tasks` 表名、`kind=assistant`、`SessionTier`/`professional-project` 落库、`任务可脱离 project` 等——见名称定义表 §0 否决与 §0.0 落地状态；docs 中出现须带标注，不得当现行断言。
