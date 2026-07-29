# 功能蓝图与语义化版本里程碑：实施计划

| 字段 | 内容 |
|---|---|
| 状态 | 待实施 |
| 日期 | 2026-07-29 |
| 产品规格 | [prd.md](./prd.md) |
| 交付方式 | 五个可独立验收的版本里程碑 |

## 1. 成功标准

本计划完成后：

1. 项目可选择轻量任务管理或功能蓝图工作模式。
2. 需求、功能点、任务、版本里程碑形成可查询的追溯链。
3. 新里程碑由系统按 SemVer 生成，不再自由命名。
4. 里程碑页面按版本顺序显示纵向树。
5. 点击版本继续复用现有任务卡片和任务详情。
6. AI PM 能维护蓝图并从功能点拆解任务。
7. 甘特图只投影任务计划和依赖，不产生第二套排期数据。

## 2. 版本路线图

| 版本 | 目标 | 主要交付 |
|---|---|---|
| `0.1.0` | 数据与 API 基础 | FeatureNode、关联表、SemVer milestone 数据/API、事件与迁移 |
| `0.2.0` | 功能蓝图 UI | 项目开关、模块树、功能点 CRUD、空状态与详情 |
| `0.3.0` | 追溯与版本闭环 | 需求/任务关联、进度、目标版本、纵向版本树 |
| `0.4.0` | AI PM 与批量生成 | CLI/MCP、batch API、PM Skill、AI 拆解与回填 |
| `0.5.0` | 甘特图与导出 | 模块分组甘特图、Markdown/JSON 导出、全链路验收 |

版本按上表顺序串联 predecessor，不设置未经确认的目标日期。

## 3. PR 1 / 版本 0.1.0：领域模型、存储与 API

### 3.1 目标

建立功能蓝图和语义化版本里程碑的后端事实源，不改变现有界面。

### 3.2 主要文件

- `backend/internal/meta/types.go`
- `backend/internal/meta/db.go`
- `backend/internal/meta/feature_catalog.go`（新增）
- `backend/internal/meta/feature_catalog_test.go`（新增）
- `backend/internal/meta/milestones.go`
- `backend/internal/meta/milestones_test.go`
- `backend/internal/meta/project_events.go`
- `backend/internal/meta/activity.go`
- `backend/internal/agent/feature_catalog_handler.go`（新增）
- `backend/internal/agent/handler.go`
- `backend/internal/server/server.go`
- `frontend/packages/core/types/featureCatalog.ts`（新增）
- `frontend/packages/core/services/featureCatalogService.ts`（新增）

### 3.3 工作项

1. 增加 schema 迁移与幂等 reconcile：
   - `feature_nodes`
   - `feature_item_links`
   - `milestones.version`
   - 项目内非空 version 唯一索引
2. 实现 `FeatureCatalogStore`：
   - CRUD
   - 移动和排序
   - 深度校验
   - 循环校验
   - 项目隔离
   - source/delivery 关联
3. 实现功能蓝图 REST API。
4. 注册功能节点和关联事件。
5. 实现里程碑 SemVer：
   - bump minor/patch/major
   - 事务内分配下一版本
   - 自动 predecessor
   - 禁止普通更新修改版本
6. 保留 legacy milestone 读写兼容。
7. 删除任务和里程碑时清理功能关联。
8. 增加 Go 类型、前端共享类型和 service。

### 3.4 验收

- 旧数据库升级后项目项、里程碑和关联任务不变。
- 可以创建一至三级模块和功能点。
- 第四级、循环、跨项目操作被拒绝。
- source/delivery 类型校验正确。
- 并发创建里程碑不会得到重复 SemVer。
- `0.9.0`、`0.10.0`、`1.0.0` 排序正确。
- 历史自由名称里程碑的 version 为空且仍可读取。
- 数据修改和 ProjectEvent 原子提交。

### 3.5 验证

```bash
cd backend
go test ./internal/meta ./internal/agent
go test ./...
```

## 4. PR 2 / 版本 0.2.0：项目开关与功能蓝图 UI

### 4.1 目标

用户可以按项目启用功能蓝图，并人工维护模块与功能点。

### 4.2 主要文件

- `frontend/src/stores/projectTabPrefs.ts`
- `frontend/src/stores/projectViewPrefs.ts`
- `frontend/src/components/pages/SettingsTab.tsx`
- `frontend/src/components/drawer/TaskList/index.tsx`
- `frontend/src/components/drawer/TaskList/FeatureCatalogView.tsx`（新增）
- `frontend/src/components/drawer/TaskList/FeatureNodeForm.tsx`（新增）
- TaskList 相关样式和 i18n

### 4.3 工作项

1. 读写 `featureCatalogEnabled`。
2. 增加配置加载状态、错误提示和 optimistic rollback。
3. `TaskListView` 增加 `features`。
4. 按开关条件显示“功能蓝图”。
5. 关闭开关时从功能蓝图回退到任务。
6. 实现空状态。
7. 实现模块树、选中状态和详情区。
8. 实现新增、编辑、移动、排序、删除确认。
9. 实现窄屏详情导航。

### 4.4 验收

- 默认关闭，升级后现有项目 UI 无变化。
- 开启后立即出现功能蓝图入口。
- 刷新和重新打开项目后开关保持。
- 关闭不删除数据，重开恢复。
- 保存失败时开关回滚。
- 支持一级模块直接挂功能点。
- 树移动后顺序和深度正确。

### 4.5 验证

```bash
cd frontend
yarn check
yarn build
```

并运行项目现有的相关前端测试。

## 5. PR 3 / 版本 0.3.0：追溯、进度与纵向版本树

### 5.1 目标

把功能蓝图接入需求、任务和里程碑，并完成新版里程碑交互。

### 5.2 主要文件

- `frontend/src/components/drawer/TaskList/FeatureCatalogView.tsx`
- `frontend/src/components/drawer/TaskList/TaskDetail.tsx`
- `frontend/src/components/drawer/TaskList/RequirementPool.tsx`
- `frontend/src/components/drawer/TaskList/TasksView.tsx`
- `frontend/src/components/drawer/TaskList/MilestoneView.tsx`
- `frontend/src/components/drawer/TaskList/MilestoneForm.tsx`
- `frontend/src/components/drawer/TaskList/DataGrid.tsx`
- `frontend/src/stores/projectViewPrefs.ts`
- `frontend/packages/core/types/task.ts`
- `frontend/packages/core/services/taskService.ts`

### 5.3 工作项

1. 功能点关联来源 requirement/bug。
2. 功能点关联已有 task。
3. 任务详情显示所属功能。
4. 需求详情显示影响功能。
5. 计算功能点和模块覆盖/进度。
6. 任务表增加按功能模块分组。
7. 无功能关联任务进入“未归入功能蓝图”。
8. 功能点设置目标版本。
9. 新任务继承目标版本。
10. 修改目标版本时提供显式任务同步，禁止静默重排。
11. 移除里程碑自由名称输入。
12. 创建里程碑改为选择 Patch / Minor / Major。
13. 里程碑页面改为纵向 SemVer 树。
14. 点击版本继续渲染现有任务卡片。
15. 历史自由名称里程碑进入折叠的 legacy 区域。

### 5.4 验收

- 一个需求可关联多个功能点。
- 一个任务可关联多个功能点。
- 未拆解与普通 `0%` 区分。
- 取消任务不进入进度分母。
- 模块同时显示交付进度和未拆解数量。
- 任务可按完整模块路径分组。
- 创建 UI 无自由名称输入。
- SemVer 树排序不受字符串排序错误影响。
- 点击版本后的任务卡片字段和操作与改版前一致。
- 历史里程碑和任务关联无回归。

## 6. PR 4 / 版本 0.4.0：AI PM、CLI/MCP 与批量生成

### 6.1 目标

将需求分析后产出功能蓝图、再从功能点拆任务变成标准 AI PM 工作流。

### 6.2 主要文件

- `backend/internal/projectitems/cli.go`
- `backend/internal/projectitems/tools.go`
- `backend/internal/projectitems/client.go`
- `backend/internal/agent/feature_catalog_handler.go`
- `.agents/skills/pm/SKILL.md`
- `frontend/src/components/drawer/TaskList/FeatureCatalogView.tsx`

### 6.3 工作项

1. 新增 `1agents feature-catalog` CLI。
2. 新增同构 MCP 工具。
3. 新增事务化 batch API 和 `clientRef` 父子引用。
4. PM Skill 读取 `featureCatalogEnabled`。
5. 开启时执行需求 → 蓝图 → 任务流程。
6. 关闭时保持原轻量流程。
7. 实现“与 AI PM 一起生成”。
8. 实现“让 AI PM 拆解为任务”。
9. AI 创建任务时建立 delivery 关联并继承目标版本。
10. AI 写入后复述新增节点、关联和版本。
11. 更新里程碑 CLI 为 `--bump`，正常流程停止使用 `--name`。

### 6.4 验收

- PM 能读取并增量维护现有蓝图。
- batch 任一操作失败时整批回滚。
- AI 创建的任务都有需求归口和功能归口。
- 功能目标版本正确传递到新任务。
- 关闭功能蓝图时 AI 不创建隐藏蓝图数据。
- CLI、MCP、Web 使用同一数据语义。

## 7. PR 5 / 版本 0.5.0：甘特图、导出与全链路验收

### 7.1 目标

把功能蓝图转化为可交付的作战蓝图和计划报告。

### 7.2 工作项

1. 甘特图按功能模块路径分组。
2. 使用任务 `PlannedStart` / `PlannedEnd`。
3. 使用任务 `DependsOn` 绘制依赖。
4. 使用 SemVer 里程碑作为版本节点。
5. 模块日期和进度从任务聚合。
6. 导出 Markdown 功能模块清单。
7. 导出 JSON 蓝图数据。
8. 报告包含需求、功能点、版本、任务和进度追溯。
9. 完成桌面、窄屏、旧项目和 legacy milestone 回归。
10. 更新产品文档和 walkthrough。

### 7.3 验收

- 甘特图不保存第二套模块计划日期。
- 调整任务日期后甘特图和模块汇总同步变化。
- Markdown 导出保留一级/二级/三级模块结构。
- JSON 导出可稳定重新消费。
- 轻量项目、规划型项目和历史里程碑均通过回归。

## 8. 依赖关系

```text
0.1.0 后端事实源
   ↓
0.2.0 功能蓝图 UI
   ↓
0.3.0 追溯与版本树
   ↓
0.4.0 AI PM 与批量生成
   ↓
0.5.0 甘特图与导出
```

同一版本内也按“先被依赖、后依赖”的顺序建卡。最终全链路验收依赖所有实现任务。

## 9. 测试矩阵

### 9.1 后端

- 全新数据库建表。
- schema v27 升级。
- 已有高 `user_version` 但缺表/缺列的 reconcile。
- 树深度、循环、跨项目和节点类型校验。
- feature item link 类型校验。
- 删除清理。
- ProjectEvent 原子性。
- SemVer bump。
- 并发版本分配。
- legacy milestone 兼容。
- SemVer 数值排序。

### 9.2 前端

- 配置默认值、加载、写入失败回滚。
- 条件导航和非法 activeView 回退。
- 模块树 CRUD、移动和排序。
- 关联选择器。
- 覆盖率和进度边界。
- 目标版本同步确认。
- 纵向版本树。
- 点击版本后的原任务卡片回归。
- legacy milestone 折叠区。
- 桌面和窄屏。

### 9.3 端到端

1. 开启功能蓝图。
2. 从需求生成三级模块和功能点。
3. 给功能点分配目标版本。
4. 从功能点拆解任务。
5. 执行任务并观察进度聚合。
6. 在纵向版本树打开版本并操作原任务卡片。
7. 导出功能蓝图与甘特图。
8. 关闭功能蓝图并确认数据仍在。

## 10. 发布与兼容策略

1. schema 和 API 先落地，UI 后启用。
2. `featureCatalogEnabled` 默认关闭。
3. 历史里程碑不自动改名。
4. 新创建路径禁止自由名称。
5. CLI 先支持 `--bump`，旧 `--name` 标记 deprecated 后再移除。
6. 版本更新不允许普通编辑，防止破坏任务 join key。
7. 每个 PR 独立通过后端测试、前端类型检查和构建。
8. 最终执行：

```bash
git diff --check
cd backend && go test ./...
cd frontend && yarn check && yarn build
```

## 11. 风险与控制

| 风险 | 控制 |
|---|---|
| 功能蓝图和任务层级混淆 | 独立 FeatureNode 表，不复用 `ProjectItem.ParentID` |
| 功能点字符串数组难维护 | 功能点使用稳定 ID 和一等实体 |
| 模块与里程碑一一映射导致错误规划 | 目标版本配置在功能点，模块只聚合 |
| 两套计划日期漂移 | 甘特图只读取任务日期 |
| SemVer 并发重复 | 在数据库事务内分配并建唯一索引 |
| 历史自由名称迁移破坏任务关联 | legacy 保留，不自动重命名 |
| 版本树改版破坏任务交互 | 继续复用现有任务卡片组件 |
| 关闭开关导致数据丢失 | 开关仅控制工作流和可见入口 |
| AI 生成半棵树 | batch 全量校验并原子提交 |

