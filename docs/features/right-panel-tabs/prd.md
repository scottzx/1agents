# 右栏 Artifact 多 Tab（Right Panel Tabs）PRD

| 字段 | 内容 |
|------|------|
| **Status** | Ready — 看板已落库 |
| **Author** | PM (Grok) + 产品讨论输入（2026-07-18） |
| **Date** | 2026-07-18 |
| **Doc** | `docs/features/right-panel-tabs/prd.md` |
| **Scope** | `frontend/src/components/drawer/RightPanel.tsx`；`RightPanelHost`；`WorkspaceHeader`（header-right）；`tabsStore` / `stageStore`；文件预览 / 浏览器打开路径；相关 i18n / SCSS |
| **相关讨论** | 三栏 Sidebar–Main–Inspector；Chat 产物（任务 / 文件 / 网站）；Mode + Document 两层 Tab |
| **执行人** | human（看板 `assignee=user` / `executor=human`，不派 agent） |
| **看板** | 需求 **#151**；P0 **#152–#159**；P1 **#160–#164**；里程碑 **Right Panel P0：Tab 工作台** / **Right Panel P1：Diff / 多开 / 溢出** |

---

## 1. 背景与问题陈述

### 1.1 现状

桌面工作台已是 **左栏导航 + 中栏 Chat/Terminal + 右栏 Artifact** 两列内容壳（`stageStore`）：

| 区域 | 驱动 | 现状 |
|------|------|------|
| 中栏（primary） | `tabsStore.activeTab` / session | chat / terminal / newChat |
| 右栏（secondary） | `tabsStore.activeDrawerTab` **单选** | `tasks` / `files` / `browser` / `git` / `channels`；再点关闭 |
| Header 快捷键 | 同一 `toggleDrawerTab` | 一排 Mode 图标（任务·文件·浏览器·Git）+ 中栏 rail |
| 旧 workspace tabs | `tabsStore.tabs` | 顶栏已隐藏；`preview-*` / `browser-*` 仍旁路存在 |

Chat / 工具可「推出」任务、文件预览、网站预览，但右栏一次只能装一种内容；文件多开与浏览器会话被拆在两套模型里。

### 1.2 核心矛盾

1. **Mode 与 Document 混层风险**：任务看板 vs `app.tsx` vs localhost 不是同一种 Tab；全塞一层会语义打架。
2. **Header 过载**：Mode 切换堆在 `header-right`，右栏自身只有单 title（`panel-tab-title`），没有可添加的 Tab 条。
3. **打开策略未统一**：Agent 写文件是否抢焦、关栏是否丢栈、换项目/换会话如何隔离——需产品规则，不能靠各入口自行 `openContentTab`。
4. **双路径债**：`openPreviewTab`（全局 tabs）与 `activeDrawerTab === 'files'|'browser'`（抽屉单例）并存，迁移必须收敛。

### 1.3 产品目标

把右栏升级为与中栏 **平级** 的 **`right-panel` 组件**：自带 Tab 条、可添加 Tab；Header 只负责 **开/关侧栏** 与 **添加 Tab**。

核心心智：

```
中栏 = 过程流（Agent 推理 / 工具 / 确认）
右栏 = 产物与上下文（Mode 默认 Tab + 文件/URL 实例 Tab）
```

### 1.4 已锁定产品规则（2026-07-18）

| ID | 规则 |
|----|------|
| R1 | **Agent 写文件时，默认不打开**右栏文件 Tab |
| R2 | **关右栏 → 保留文档栈**；再打开恢复 |
| R3 | **换项目 → 清空**文档栈 |
| R4 | **同项目换会话 → 保留**文档栈 |
| R5 | **Git Diff 与 File Preview 合并进「文件 Mode」**（文件 Tab 内 Preview \| Diff；不把 Git 审查拆成与文件对等的另一套顶层文档语义） |
| R6 | 打开文件 Mode → **右栏出现**，顶部出现 Tab 条 |
| R7 | `right-panel` 与 main **平级**；右栏自带 Tab，可添加 |
| R8 | `header-right` 调整为：**打开/关闭侧边栏** + **添加 Tab**（去掉一排 Mode 快捷为主入口） |
| R9 | Tab 展示名称；**默认 Tab 支持 `⋯` 快速切换** |

### 1.5 成功标准（全局）

| ID | 标准 |
|----|------|
| S1 | 桌面 split 下，右栏顶部为真实 Tab 条（默认 Tab + 实例 Tab），不再只有单一 `panel-tab-title` |
| S2 | Header 右侧仅「开/关右栏」+「添加 Tab」；不再用一排 Mode 图标作为主切换 |
| S3 | 关右栏后再开，先前打开的 Tab 与 active 仍在（同项目） |
| S4 | 切换 workspace/项目后文档栈清空；同项目切换 chat session 栈仍在 |
| S5 | Agent 写文件不自动 `panelVisible` / 不自动新建文件 Tab |
| S6 | 用户显式点击文件路径 /「在右侧打开」→ 打开右栏 + 文件 Mode + 对应实例 Tab 前台聚焦 |
| S7 | 默认 Tab（至少：任务 / 文件 / 浏览器）可通过 `⋯` 快速切换到其它默认面板 |
| S8 | 无回归：任务看板、文件树/预览、内置浏览器、Git 面板能力在迁移后仍可到达（Git 完整面板可作为默认 Tab 或文件内二级，见范围） |

### 1.6 不在本 PRD 范围（明确延后）

- 移动端完整复刻桌面双层 Tab（可复用 store，chrome 另案）
- Tab 拖拽排序、分屏同一右栏双文档
- Agent 写文件的「未读点 / chip 后台打开」（P1 可选，默认仍不打开）
- 第三方 `react-resizable-panels` 替换现有 split（现有 resizer 保留即可）
- 全量 localStorage 跨刷新持久化文档栈（P1）

---

## 2. 用户与场景

### 2.1 用户

| 角色 | 诉求 |
|------|------|
| AI 结对开发者 | 中栏看过程，右栏看代码/预览；多文件可切换不丢 |
| 任务驱动用户 | 右栏开任务看板，与会话并存 |
| 预览调试用户 | 打开本地 URL / 静态预览，可与文件 Tab 并存 |
| 执行人（human） | 卡片粒度清晰、验收可勾选 |

### 2.2 关键用户故事

1. **作为开发者**，我打开「文件」后右栏出现，顶部有 Tab；再打开多个文件可切换。
2. **作为开发者**，我关掉右栏写对话，再开右栏时文件仍在。
3. **作为开发者**，Agent 改了很多文件时，右栏不会被自动刷屏。
4. **作为用户**，我只想在 header 点「加 Tab / 开关侧栏」，不在 header 找四个 Mode 图标。
5. **作为用户**，默认 Tab 旁的 `⋯` 能快速切到任务/文件/浏览器。

---

## 3. 信息架构与交互

### 3.1 组件层级

```
Desktop shell
├── left-sidebar
├── main (chat / terminal)          ← 中栏
└── right-panel                     ← 与 main 平级
      ├── tab strip
      │     默认 Tab（Mode）+ 实例 Tab（Document）+ ⋯ + [+]
      └── active tab body
```

### 3.2 Tab 两类

| 类型 | 例子 | 关闭 | 说明 |
|------|------|------|------|
| **默认 Tab（Mode）** | 任务 · 文件 · 浏览器（P0）；可选 Git / 渠道 | 可从栈移除；定义仍在「添加」菜单 | 同一 Mode 在栈中最多 1 个 |
| **实例 Tab（Document）** | `app.tsx` · hostname | ✅ | 文件 path / URL 去重 |

**P0 默认可添加集合：** `任务 | 文件 | 浏览器`  
**P0 可选：** Git 完整面板仍可通过「添加 Tab」加入（与 R5 不冲突：Diff 审阅走文件实例；Git 面板管 status/commit）。

### 3.3 Tab 条行为

| 行为 | 规则 |
|------|------|
| 单击 Tab 标题 | 激活 |
| 实例 Tab `×` | 关闭；MRU 切到上一 Tab；关光实例后留在该 Mode empty state，**不自动关右栏** |
| 默认 Tab `⋯` | **先激活该 Tab**，再弹出菜单：快速切换到其它默认面板 +（文件）最近打开列表入口 |
| 条内 / Header `[+]` | 打开「添加 Tab」菜单（与 header 同源） |
| 关右栏（Header 或栏内 ×） | `panelVisible=false`，**不清栈** |

### 3.4 header-right

```
header-right
├── [可选] 中栏相关控件（如 tmux mouse）
├── [打开/关闭侧边栏]   → panelVisible toggle
└── [添加 Tab]          → 菜单 → 确保 Tab 入栈 + 聚焦 + panelVisible=true
```

移除（或降级不再作为主入口）：任务 / 文件 / 浏览器 / Git 一排 `shortcut-btn` Mode 切换。

### 3.5 打开协议（Open Artifact）

| 触发 | 行为 |
|------|------|
| Header「添加 Tab」/ 右栏 `+` 选「文件」 | `panelVisible=true`；确保文件默认 Tab；active=文件 |
| 用户显式点文件路径 / 打开预览 | 同上 + path 去重实例 Tab 前台聚焦 |
| 用户显式点 URL / 打开浏览器 | 浏览器默认 Tab + URL（P0 单会话即可） |
| Agent 写文件 / tool 完成 | **不**改 `panelVisible`，**不**新建文件 Tab（R1） |
| Chat 内嵌小 Diff | 保留；大 diff「在右侧打开」→ 文件实例 + Diff 子视图（P1 可加强） |

### 3.6 生命周期状态机

```
panelVisible: boolean
tabs: ArtifactTab[]      // 按 projectId 隔离
activeTabId: string
```

| 事件 | panelVisible | tabs |
|------|--------------|------|
| 关侧栏 | false | 不变 |
| 开侧栏 | true | 不变，恢复 active |
| 添加/打开 Mode 或实例 | true | 入栈/聚焦 |
| Agent 写文件 | 不变 | 不变 |
| 换项目 | 可保持或 false | **清空** |
| 同项目换会话 | 不变 | **保留** |

### 3.7 文件 Mode 与 Diff（R5）

```
文件默认 Tab body：
├── empty / 树：FlatFileBrowser
└── 实例 Tab 激活：
      FileDetailView +（P1）Preview | Diff 子视图
Git 变更点文件 → 打开同 path 实例 Tab（+ Diff 若有）
完整 Git 面板 → 可选默认 Tab「Git」，不替代文件 Diff 语义
```

---

## 4. 现状架构（实现锚点）

| 文件 | 职责 |
|------|------|
| `frontend/src/stores/tabsStore.ts` | `activeDrawerTab` 单选；`tabs[]` preview/browser；`openPreviewTab` / `openBrowserTab` / `toggleDrawerTab` |
| `frontend/src/stores/stageStore.ts` | `panes` / `hasContent` / `collapsed` / split；`SECONDARY_TABS` |
| `frontend/src/components/drawer/RightPanel.tsx` | 右栏 chrome（`panel-tabs-header` 单 title）+ body 按 drawer 切换 |
| `frontend/src/components/shared/RightPanelHost.tsx` | 桌面/移动共用 wiring |
| `frontend/src/components/header/WorkspaceHeader.tsx` | `header-right` Mode 快捷键集群 |
| `frontend/src/components/desktop/DesktopAppLayout.tsx` | 两列 shell：chat + RightPanelHost |
| `frontend/src/components/shared/FilePreviewContent.tsx` | 旧 preview tab 内容 |
| `frontend/src/components/browser/BuiltinBrowser.tsx` | 内置浏览器 |
| `frontend/src/style/index.scss` | `.right-panel` / `.panel-tabs-header` / `.header-right` |

**目标收敛方向：**

- 引入（或演进）`artifact` 状态：`panelVisible` + `tabs[]` + `activeTabId`（建议按 `workspaceId` 分桶）。
- `stageStore.hasContent` 改为「右栏可见且有内容」而非仅 `activeDrawerTab ∈ SECONDARY_TABS`。
- `openPreviewTab` / 全局 workspace preview 路径 **并入** 右栏文件实例 Tab。
- `openBrowserTab` **并入** 右栏浏览器 Tab。

---

## 5. 分阶段交付

### 5.1 P0 — 可用的右栏 Tab 工作台（立刻可感）

目标：桌面用户能用 Tab 条 + Header 精简完成「开关 / 添加 / 切换 / 文件多开」，并满足 R1–R4、R6–R9。

| 序号 | 交付 | 验收要点 |
|------|------|----------|
| P0-1 | Artifact 状态模型与生命周期 | 关栏保留；换项目清空；同项目换会话保留；API：`openPanel` / `closePanel` / `addTab` / `closeTab` / `selectTab` |
| P0-2 | RightPanel Tab 条 UI | 默认+实例 Tab 名称；active；实例 ×；默认 `⋯` 快切；视觉贴合 codex-minimal / 现有 panel header |
| P0-3 | header-right 精简 | 仅开/关侧栏 + 添加 Tab；Mode 一排入口移除或不再主路径 |
| P0-4 | 文件 Mode + 实例 Tab | 打开文件 Mode 出右栏；path 去重；Chat/文件树显式打开聚焦 |
| P0-5 | 打开协议 R1 | Agent 写文件路径不自动开栏/加 Tab；显式点击会 |
| P0-6 | stage 接线 | `hasContent` / split / rail 与 `panelVisible` 一致；无双开闪烁 |
| P0-7 | 浏览器 + 任务默认 Tab | 可通过添加/⋯ 打开；浏览器 P0 单会话即可 |
| P0-8 | 清理旁路 | 桌面主路径不再依赖顶栏 hidden workspace-tabs 打开 preview |

### 5.2 P1 — 体验加深

| 序号 | 交付 | 验收要点 |
|------|------|----------|
| P1-1 | 文件 Tab 内 Preview \| Diff | Git 点文件进同 path + Diff 子视图 |
| P1-2 | 浏览器多实例 Tab | 多 origin/port 可并存；URL 去重策略明确 |
| P1-3 | Tab 溢出 `⋯` | 条太窄时 overflow 列表 |
| P1-4 | 栈持久化 | localStorage 按 project 恢复（可选） |
| P1-5 | Chat chip / 未读点 | 后台打开可发现（仍默认不抢焦） |

### 5.3 P2 — 延后（本 PRD 不建卡）

- 移动端 Tab chrome
- Pin、拖拽排序
- 右栏内嵌双文档 split

---

## 6. 任务拆解与看板映射

> 落库后把本节 #编号回填；执行人一律 **human**（`assignee=user`，`executor=human`）。

### 6.1 顶层需求

| # | 标题 |
|---|------|
| **#151** | 右栏 Artifact 多 Tab（Right Panel Tabs） |

### 6.2 里程碑

| 里程碑 | 范围 | target |
|--------|------|--------|
| **Right Panel P0：Tab 工作台** | §5.1 | 2026-07-28 |
| **Right Panel P1：Diff / 多开 / 溢出** | §5.2（predecessor = P0） | 2026-08-11 |

### 6.3 P0 任务

| # | 任务 | 依赖 | 说明 |
|---|------|------|------|
| **#152** | P0-1 状态模型与生命周期 | — | store + 项目隔离 |
| **#153** | P0-2 RightPanel Tab 条 UI | #152（文案） | chrome + 默认 Tab ⋯ |
| **#154** | P0-3 header-right 精简 | #152（文案） | 开/关 + 添加 |
| **#155** | P0-6 stageStore 接线 | #152（文案） | split/rail |
| **#156** | P0-4 文件 Mode + 实例 Tab | #152 #153 | 核心路径 |
| **#157** | P0-7 任务 / 浏览器默认 Tab | #153 | body 复用 |
| **#158** | P0-5 打开协议 R1 | #156 | Agent 不抢焦 |
| **#159** | P0-8 收敛 preview 旁路 | #156 #155 | 桌面主路径 |

### 6.4 P1 任务

| # | 任务 | 依赖 | 说明 |
|---|------|------|------|
| **#160** | P1-1 文件 Preview\|Diff | #156 | R5 深化 |
| **#161** | P1-2 浏览器多 Tab | #157 | |
| **#162** | P1-3 溢出菜单 | #153 | |
| **#163** | P1-4 持久化 | #152 | |
| **#164** | P1-5 未读/chip | #156 | 可选 |

---

## 7. 设计与实现约束

1. **外科手术式改动**：不顺手重构 TaskList / GitPanel 业务；只改挂载与打开路径。
2. **设计系统**：Tab 条用扁平 hairline、pill 交互控件；无装饰阴影；token 来自 `index.scss`。
3. **i18n**：Tab 名、添加菜单、`⋯` 菜单、header 按钮全部走 `dict.ts`。
4. **移动端**：P0 不破坏现有 drawer；若 store 共用，mobile 仍可 single-select 全屏。
5. **Beginner mode**：若隐藏任务入口，添加菜单与默认集合需尊重 `isBeginnerMode`。

---

## 8. 验收清单（Epic 级）

- [ ] 桌面：仅 header「开/关」+「添加」可完成右栏生命周期
- [ ] 打开文件 Mode → 右栏 + Tab 条可见
- [ ] 多文件实例 Tab 切换 / 关闭正确
- [ ] 关栏再开栈仍在；换项目栈清空；同项目换会话栈仍在
- [ ] Agent 写文件不自动开栏
- [ ] 默认 Tab `⋯` 可快切任务/文件/浏览器
- [ ] split 布局与 rail 行为正常
- [ ] 中英 i18n 无硬编码主路径文案

---

## 9. 开放决策（已取默认，可复议）

| 议题 | 本 PRD 默认 | 备注 |
|------|-------------|------|
| P0 默认 Tab 集合 | 任务 · 文件 · 浏览器 | Git/渠道走「添加」 |
| 默认 Tab `⋯` P0 | 跨默认面板快切 + 文件最近列表入口 | 不做满操作菜单 |
| 关栏控件 | Header 主 + 栏内 × 可保留 | 栏内 × = closePanel |
| Git 完整面板 | P0 可通过添加加入 | Diff 走文件 Mode（P1 子视图） |

---

## 10. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-07-18 | 初稿：产品讨论锁定 R1–R9；P0/P1 拆解；执行人 human |
)
