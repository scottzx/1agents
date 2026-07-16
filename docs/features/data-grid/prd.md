# 多维表格（DataGrid）基础 UI 组件优化 PRD

| 字段 | 内容 |
|------|------|
| **Status** | Ready — A+B epic on board (#125) |
| **Author** | PM (Grok) + 设计评审输入 |
| **Date** | 2026-07-16 |
| **Doc** | `docs/features/data-grid/prd.md` |
| **Scope** | `frontend/src/components/drawer/TaskList/DataGrid.tsx` 及消费方；`frontend/src/style/index.scss` 中 `.task-grid*` 体系 |
| **相关分析** | 任务列表多维表格作为基础 UI 组件的可用性与实现评审（会话 2026-07-16） |

---

## 1. 背景与问题陈述

### 1.1 现状

应用内已将「多维表格」抽象为通用 **`DataGrid`**，被多处复用：

| 消费方 | 配置文件 | 能力侧重 |
|--------|----------|----------|
| 任务列表 `TaskTable` | `gridConfig` + `TaskGridCell` | 可编辑、父子层级、分组排序 |
| 会话归档 `SessionsView` | `sessionGrid` | 只读 + 跳转任务 |
| 联系人 | `contactGrid` | 只读 + 详情弹窗 |
| 数据源 / 治理 | `sourceGrid` | schema-free 动态列 + `persistKey` |

内核能力：**列显隐/排序、表头点击排序、分组折叠、可选父子层级、可选行内编辑、可选操作列**。筛选搜索在各业务的 FilterBar，表格只负责「维度展示」——职责边界正确。

### 1.2 核心矛盾

组件已服务多业务，但：

1. **命名与样式仍绑在「任务」**（`.task-grid` / `.task-table`），作为基础 primitive 语义不清。
2. **交互距离「多维表」心智还有明显差距**（列操作工程向、双击编辑、无固定列、误删风险等）。
3. **体验不一致**：数据源有列持久化，任务主表没有；错误用 `alert`；工具栏部分文案未 i18n。
4. **布局有结构性债务**：`TasksView` 与 `DataGrid` 双重 `.task-grid` 嵌套，flex 高度与间距难控。

### 1.3 产品目标

把 `DataGrid` 升级为全产品可复用的 **多维表格基础 UI 组件**：

- 各业务只提供列配置 + 单元格渲染，共享同一套交互与视觉语言。
- 符合 codex-minimal / Bento 设计系统（扁平、ink hairline、无装饰阴影）。
- 在不引入第三方表格库的前提下，先做高 ROI 的体验与工程债清理，再考虑虚拟滚动等规模化能力。

### 1.4 成功标准（全局）

| ID | 标准 |
|----|------|
| S1 | 任务 / 会话 / 联系人 / 数据源四条路径共用同一套 class 与工具栏行为，无业务分叉的交互逻辑 |
| S2 | 用户列显隐、列顺序、列宽在刷新后仍可恢复（按业务/项目维度的 `persistKey`） |
| S3 | 桌面端列操作（显隐、排序、关闭弹层）无需猜；删除等高风险操作有确认 |
| S4 | 横向滚动时主识别列与操作列仍可见（sticky） |
| S5 | 工具栏与表格 chrome 文案 100% 走 i18n |
| S6 | 无回归：现有分组、排序、层级、行内编辑、动态列仍可用 |

**不在本 PRD 范围（后续阶段）：** 真实 Airtable 公式列、多视图保存为「视图」、协作光标、服务端分页协议大改。

---

## 2. 用户与场景

### 2.1 用户

| 角色 | 诉求 |
|------|------|
| 项目执行者 / PM | 在任务表快速改字段、找父子关系、避免误删；列布局要记住 |
| 数据治理用户 | 在动态列宽表中扫读字段，固定序号列，调整列序后不丢失 |
| 联系人 / 会话浏览者 | 只读表：稳定排序分组，点击打开详情 |
| 前端开发 | 新业务 1 天内可接入 DataGrid，不必复制一份 task-table SCSS |

### 2.2 关键用户故事

1. **作为 PM**，我希望关掉不需要的列并调好顺序后，下次打开同一项目仍是这个布局。
2. **作为用户**，我希望横向滚动时仍能看到任务标题/ID 与右侧打开按钮。
3. **作为用户**，我希望点「列」面板时点空白处能关掉，不必再点一次按钮。
4. **作为用户**，我不希望误点垃圾桶就删掉任务。
5. **作为开发**，我希望 CSS 叫 `data-grid` 而不是 `task-grid`，文档与设计规范能直接引用。

---

## 3. 现状架构（实现锚点）

```
DataGrid.tsx          # 通用内核：列状态 / sort / group / hierarchy / edit cell key
GridToolbar.tsx       # 分组 + 层级开关 + 列显隐/顺序弹层
gridConfig.ts         # 任务列模型 + compare + groupValue
TaskGridCell.tsx      # 任务单元格（display + inline edit）
TaskTable.tsx         # 任务装配层 → DataGrid
TasksView.tsx         # FilterBar + 列表/看板/日历切换（外层也有 .task-grid）
sessionGrid.tsx / contactGrid.tsx / sourceGrid.tsx
index.scss            # .task-grid / .task-table / .task-group-header ...
```

**已有能力**

- `persistKey`：列显隐 + 顺序 localStorage，与 live schema reconcile
- Header sort：off → asc → desc → off
- Group collapse、optional hierarchy
- `renderCell` / `renderActions` 注入

**已知结构性问题（摘要）**

| # | 问题 | 严重度 |
|---|------|--------|
| 1 | 类名 task 绑定 | P0 组件化 |
| 2 | TasksView + DataGrid 双重 `.task-grid` | P0 布局 |
| 3 | 任务表未传 `persistKey` | P0 体验 |
| 4 | `allColumns` 变化时列 state 不自动 reconcile（靠 remount） | P0 工程 |
| 5 | 列弹层无 outside click / Esc | P0 交互 |
| 6 | 删除无确认；patch 失败用 `alert` | P0 安全/一致性 |
| 7 | 列宽固定 minWidth，无拖拽；列序靠 ↑↓ | P1 交互 |
| 8 | locked 列逻辑固定，横向不 sticky | P1 可读性 |
| 9 | 双击编辑 / 触控差；分组与层级互斥无说明 | P1 交互 |
| 10 | 排序/分组/折叠不持久 | P1 一致性 |
| 11 | 工具栏硬编码中文 | P1 i18n |
| 12 | group header `rgba(var(--bg-panel), 0.7)` 可能失效 | P0 视觉 |
| 13 | 无虚拟滚动；hierarchy O(n²) | P2 性能 |
| 14 | 无批量选择；键盘/a11y 薄弱 | P2 效率/无障碍 |
| 15 | 无内置 cell 类型 registry | P2 扩展 |

---

## 4. 范围与优先级总表

### 4.1 阶段划分（落地批次）

| 阶段 | 名称 | 对应优先级 | 目标一句话 |
|------|------|------------|------------|
| **A** | 立刻可感 | P0 子集 | 小改动、体感明显、低风险 |
| **B** | 基础组件化 | P0 余量 + P1 组件层 | 真正变成可复用 primitive |
| **C** | 表格效率 | P1 交互深化 | 接近多维表心智（编辑手势、视图状态、批量） |
| **D** | 规模化 | P2 | 大数据量与扩展 API |

本 Epic **只交付 A+B**；C/D 写入本 PRD 作路线图，另立里程碑时再拆卡。

### 4.2 P0 — 结构性 / 正确性 / 高 ROI 体验

| ID | 项 | 阶段 | 说明 |
|----|----|------|------|
| P0-1 | 任务主表启用 `persistKey` | A | 建议 `tasks:cols:{projectId}` |
| P0-2 | 列弹层 outside click + Esc 关闭 | A | 所有 DataGrid 受益 |
| P0-3 | 删除确认（至少任务操作列） | A | 确认弹层或二次确认；避免直接删 |
| P0-4 | 去掉双重 `.task-grid` 嵌套 | A | DataGrid 根节点改语义 class；外层只承载 FilterBar |
| P0-5 | 修复分组头背景色 token | A | 禁止无效 `rgba(var(--bg-panel), …)` |
| P0-6 | `allColumns` 与 col state reconcile | B | 语言切换 / 动态列不依赖隐式 remount |
| P0-7 | 错误反馈去 `alert` | A | 用现有 toast / 错误条；失败可取消编辑态 |
| P0-8 | 类名组件化（`data-grid` 体系） | B | 保留过渡兼容或一次性迁移所有消费方 |

### 4.3 P1 — 交互与设计语言（组件化）

| ID | 项 | 阶段 | 说明 |
|----|----|------|------|
| P1-1 | 工具栏与 chrome 全量 i18n | B | 分组/列/固定/上移下移/排序 title |
| P1-2 | Sticky 首列（locked） | B | 横向滚动时识别列可见 |
| P1-3 | Sticky 操作列（右侧） | B | 打开/删除始终可达 |
| P1-4 | 列宽拖拽 + 写入 persist | B | 表头右缘拖拽；width 进 localStorage |
| P1-5 | 列顺序拖拽（替代/补充 ↑↓） | B* | *可与 P1-4 同卡或紧随；A+B 至少保留 ↑↓ 并可用 |
| P1-6 | 「恢复默认列」 | B | 清空 persist 中列配置并重置 |
| P1-7 | 可编辑 affordance 对齐 Bento | B | hover ink hairline，避免无效装饰阴影 |
| P1-8 | 分组时层级开关禁用 + 说明 | C | 互斥可理解 |
| P1-9 | 编辑手势（单击选中 / F2 / 触控） | C | 降低双击门槛 |
| P1-10 | 排序 + 分组 (+ 可选折叠) 持久化 | C | 与列配置同命名空间或扩展 view state |
| P1-11 | 操作列：删除 hover 显示 / 权重降级 | C | 降低视觉噪音与误点 |
| P1-12 | 表格密度与主列 2 行省略 | C | compact/default；对齐 tabular 数字列 |
| P1-13 | Loading 有数据时的刷新指示 | C | 非空表刷新时 subtle progress |

### 4.4 P2 — 规模化与无障碍

| ID | 项 | 阶段 | 说明 |
|----|----|------|------|
| P2-1 | 行虚拟滚动（阈值阈值） | D | 联系人/数据源上百行 |
| P2-2 | 排序/层级算法优化 | D | Set/Map；memo 派生 |
| P2-3 | 多选 + 批量操作 API | D | checkbox 列；业务注入 bulk actions |
| P2-4 | 键盘导航与 a11y | D | `aria-sort`、`aria-expanded`、行列焦点 |
| P2-5 | 内置 cell 类型 registry | D | text/badge/date/link 减少业务样板 |
| P2-6 | `index` 语义：view vs group | D | 数据源「#」列在分组下不歧义 |
| P2-7 | 接入样板 `createGridModel` 薄封装 | D | 降低 props 样板代码 |

---

## 5. 功能需求详述（A+B 交付范围）

### 5.1 A — 立刻可感

#### A1. 任务表列配置持久化（P0-1）

- **行为**：`TaskTable` 传入 `persistKey`，按项目隔离。
- **Key 约定**：`tasks:cols:{projectId}`（无 projectId 时用 `tasks:cols:global` 并文档说明）。
- **内容**：与现有一致——`ColState[]`（key + visible + order）。
- **验收**：刷新页面后列显隐与顺序保持；新列 key 默认可见并追加在末尾；删除的列 key 被 drop。

#### A2. 列弹层关闭（P0-2）

- **行为**：打开「列」popover 后：
  - 点击组件外关闭；
  - 按 `Escape` 关闭；
  - 再次点击「列」按钮切换关闭。
- **验收**：任务 / 数据源 / 联系人任一 DataGrid 行为一致；不误关内部 checkbox 点击。

#### A3. 删除确认（P0-3）

- **范围**：任务表 `renderActions` 删除按钮（会话若有破坏性操作同样原则）。
- **行为**：点击删除 → 确认（Modal 或项目既有确认组件）→ 确认后才调用 `onDeleteTask`。
- **验收**：取消不删；确认后删除；文案 i18n。

#### A4. 布局去重（P0-4）

- **问题**：`TasksView` 与 `DataGrid` 均渲染 `.task-grid`。
- **方案**：
  - 外层（FilterBar 宿主）保留 flex 列容器（可改名 `task-view-shell` 或保留一层）。
  - `DataGrid` 根节点使用 `data-grid`（或本阶段先用 `task-grid-inner`，B 阶段统一 `data-grid`）。
- **验收**：列表视图高度正确铺满；无双重 gap 导致工具栏间距异常；看板/日历不受损。

#### A5. 分组头样式修复（P0-5）

- **行为**：分组行背景使用有效 token（如 `var(--bg-panel)` 或 `--*-rgb` 三元组）。
- **验收**：亮/暗主题下分组头与 hover 均可见、无透明失效。

#### A6. 错误反馈（P0-7）

- **行为**：`onPatchRow` 失败不再 `alert`；使用项目既有 toast / banner。
- **验收**：模拟 patch 失败时有非阻塞提示；编辑态正确退出（推荐：失败 toast + 退出编辑）。

---

### 5.2 B — 基础组件化

#### B1. Class 体系重命名（P0-8）

| 旧 | 新 |
|----|-----|
| `.task-grid`（DataGrid 根） | `.data-grid` |
| `.task-grid-toolbar` | `.data-grid-toolbar` |
| `.task-table` / scroller | `.data-grid-table` / `.data-grid-scroller` |
| `.task-group-header` | `.data-grid-group-header` |
| 业务特有（`.task-row` status 等） | **保留**在任务侧 |

- **策略**：一次性替换所有 DataGrid 消费方 class；SCSS 同步；避免长期双轨。
- **验收**：四条业务路径视觉无回归；全局搜 `.task-grid` 仅剩业务壳（若有）。

#### B2. 列状态与 columns 同步（P0-6）

- **行为**：`allColumns` 引用变化时 reconcile（与 `initColState` 相同算法），不丢用户可见性；或文档化强制 `key={colSig}` + 单测保证。
- **验收**：切换语言后列 label 更新且用户隐藏列仍隐藏；数据源动态增列无需整页刷新逻辑分叉。

#### B3. 工具栏 i18n（P1-1）

- 所有 GridToolbar 用户可见字符串进 `i18n/dict.ts`（中/英）。
- 表头 sort title、列 pinned 标记、空状态已有的继续保持。
- **验收**：切到 English 后工具栏无中文硬编码。

#### B4. Sticky 列（P1-2 / P1-3）

- **左侧**：`locked` 列（任务 ID、数据源序号、会话名等）`position: sticky; left: 0; z-index` 高于 body。
- **右侧**：存在 `renderActions` 时操作列 `sticky; right: 0`。
- **验收**：横向滚动时首列与操作列不消失；与 sticky thead 不打架；暗色主题底色不透出。

#### B5. 列宽拖拽 + persist（P1-4）

- 表头右缘拖拽调整 `width`；最小值建议 48–64px。
- Persist 结构扩展为：

```ts
type PersistedColState = {
  key: string;
  visible: boolean;
  width?: number; // 用户拖过的宽度
};
```

- 与 `GridColumn.width` 默认合并：用户 width 优先。
- **验收**：拖宽刷新后保持；重置默认后恢复配置宽度。

#### B6. 恢复默认列（P1-6）

- 列弹层底部「恢复默认」：可见性全开、顺序恢复 `allColumns`、清除用户 width。
- **验收**：一点即恢复；persist 同步。

#### B7. 可编辑单元格视觉（P1-7）

- hover 使用 ink hairline（`border-color: var(--text-main)` 或 inset 与 Bento 一致），去掉无效/装饰性阴影依赖。
- **验收**：亮暗主题下可编辑格可感知；只读格无 text cursor。

#### B8.（可选同里程碑）列序拖拽（P1-5）

- 若工期紧：A+B 可仅保证 ↑↓ + i18n；拖拽列为 B 加分项，可拆子任务依赖 B1。

---

## 6. 非功能需求

| 类别 | 要求 |
|------|------|
| **性能 A+B** | 不引入虚拟列表；禁止因 sticky/拖拽导致每帧 layout thrash；拖拽用 pointer event + rAF 可选 |
| **兼容** | 旧 localStorage 仅 `{key,visible}[]` 必须仍能读；缺 width 用默认 |
| **无障碍（最低）** | 列按钮可键盘聚焦；Esc 关弹层；排序 th 在 B 可暂不 ARIA，D 阶段补 `aria-sort` |
| **i18n** | 新增 key 中英双语 |
| **设计** | 遵循 Frontend Design Language：扁平、无 resting shadow、accent 仅用于 active |
| **测试** | 关键逻辑（initColState reconcile、persist merge）建议单测或纯函数抽出测；UI 手测清单见 §9 |

---

## 7. 明确不做什么（本 Epic）

- 不换 AG Grid / TanStack Table 等重型库（可在 D 评估）。
- 不做多视图命名保存（「我的视图」）。
- 不做单元格协作、公式列、关联 rollup。
- 不做批量编辑与虚拟滚动（C/D）。
- 不重写 TaskGridCell 类型系统（registry 属 D）。

---

## 8. 里程碑与看板映射

| 里程碑 | 内容 | PRD 阶段 |
|--------|------|----------|
| **DataGrid A+B：可感修复 + 基础组件化** | §5 全部 | A + B |
| （后续）DataGrid C：表格效率 | P1-8…P1-13 | C |
| （后续）DataGrid D：规模化 | P2-* | D |

### 8.1 A+B 看板条目（已落库 2026-07-16）

| # | 类型 | 标题 | 依赖 |
|---|------|------|------|
| **#125** | requirement | 多维表格 DataGrid 基础组件 A+B | — |
| **#126** | task | [A] 任务表 persistKey + 布局去重 + 分组头色 + toast 错误 | — |
| **#127** | task | [A] 列弹层 outside/Esc 关闭 + 删除确认 | — |
| **#128** | task | [B] class 重命名 data-grid 全量迁移 | #126 |
| **#129** | task | [B] 列 state reconcile + persist 扩展 width | — |
| **#130** | task | [B] 工具栏 i18n + 恢复默认列 | #127 |
| **#131** | task | [B] sticky 首列/操作列 + 列宽拖拽 + 可编辑 affordance | #128, #129 |

**里程碑：** `DataGrid A+B：可感修复 + 基础组件化`（target 2026-08-01）

依赖建议：布局去重/重命名 → sticky/列宽；A 两卡可并行；i18n 可与 B 视觉并行。

---

## 9. 验收清单（Epic 完成定义）

### 9.1 功能

- [ ] 任务表：列显隐/顺序刷新后保持（分项目 key）
- [ ] 列弹层：外点与 Esc 关闭
- [ ] 删除任务：需确认
- [ ] 列表布局：无双重工具栏间距/高度异常
- [ ] 分组头：亮暗主题正常
- [ ] Patch 失败：非 alert 提示
- [ ] DOM/CSS：DataGrid 使用 `data-grid*` 命名
- [ ] 语言切换：工具栏英文完整；隐藏列不丢
- [ ] 横向滚动：locked 列与操作列 sticky
- [ ] 列宽拖拽：刷新保持；恢复默认可用
- [ ] 可编辑格 hover 符合 Bento

### 9.2 回归手测路径

1. 项目 → 任务 → 列表：排序、分组、层级、双击改标题/状态
2. 会话归档表
3. 联系人表
4. 数据源详情动态列 + 已有 `persistKey`
5. 暗色主题扫一眼表格与 sticky 底色

### 9.3 代码卫生

- [ ] 无新增无效 CSS 变量 fallback 破坏暗色
- [ ] 旧 persist JSON 兼容
- [ ] 不引入与本需求无关的重构

---

## 10. 风险与缓解

| 风险 | 缓解 |
|------|------|
| class 重命名漏改导致样式全丢 | 全局 rg 校验；改完四条路径手测 |
| sticky + border-collapse/overflow 兼容性 | scroller 上 overflow；table `border-separate` 已有基础；真机 Chrome/Safari |
| persist 结构扩展破坏旧数据 | 读取时 optional width；写回渐进 |
| 列宽拖拽与排序点击冲突 | 仅边缘 4px 命中拖拽；点击空白处仍排序 |
| 任务 delete 确认组件选型 | 优先复用现有 Modal；避免新依赖 |

---

## 11. 路线图（C/D 摘要，本 Epic 不拆卡）

### C — 表格效率

- 分组 ↔ 层级互斥 UX
- 编辑手势升级（键鼠 + 触控）
- sort/group 持久化
- 操作列降噪、密度、loading overlay

### D — 规模化

- 虚拟列表、算法优化、多选批量、a11y、cell registry、`createGridModel`

---

## 12. 参考实现位置

| 模块 | 路径 |
|------|------|
| 内核 | `frontend/src/components/drawer/TaskList/DataGrid.tsx` |
| 工具栏 | `frontend/src/components/drawer/TaskList/GridToolbar.tsx` |
| 任务装配 | `frontend/src/components/drawer/TaskList/TaskTable.tsx` |
| 样式 | `frontend/src/style/index.scss`（Multi-dimensional grid 段） |
| 设计语言 | Frontend Design Language；`docs/design_rules/` |

---

## 13. 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-16 | 初稿：完整 P0–P2 + A/B/C/D 阶段；A+B 为第一 Epic |
| 2026-07-16 | 看板落库：里程碑 + #125 Epic + #126–#131 任务；§8.1 回填编号 |
