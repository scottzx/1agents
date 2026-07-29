# PRD：功能蓝图与语义化版本里程碑

| 字段 | 内容 |
|---|---|
| 状态 | 已确认，待实施 |
| 日期 | 2026-07-29 |
| 产品名称 | 功能蓝图（功能模块清单） |
| 范围 | 项目设置、功能模块树、需求/任务/里程碑追溯、版本计划视图、AI PM |
| 关联设计 | [项目模型](../project-model/design.md)、[Issue 模型](../issue-model/design.md)、[PM 独立能力](../pm-standalone/prd.md) |

## 1. 背景

当前项目管理已经支持需求、缺陷、任务、子任务、依赖、计划日期和里程碑，但任务主要通过“想到一项就增加一项”的方式积累，缺少需求分析之后的正式范围基线。

售前解决方案和大型交付项目通常会在需求分析后产出“功能模块清单”：

```text
一级模块
  └── 二级模块
        └── 三级模块
              └── 功能点
```

这份清单既是架构和产品范围的共同视图，也是后续任务拆解、版本计划、里程碑和甘特图的上游依据。

本需求在现有项目管理模型中引入一层可选的“功能蓝图”，形成以下链路：

```text
需求 / Bug
   ↓ 来源追溯
功能蓝图：一级模块 → 二级模块 → 三级模块 → 功能点
   ↓ 交付追溯
任务 → 计划日期 / 依赖 / 执行会话 / 验收
   ↓ 版本编排
语义化版本里程碑 x.y.z → 甘特图与发布计划
```

## 2. 产品定义

| 层次 | 回答的问题 | 系统实体 |
|---|---|---|
| 需求 | 为什么做、解决什么问题 | `ProjectItem(type=requirement\|bug)` |
| 功能蓝图 | 系统由哪些业务能力和功能点组成 | `FeatureNode` |
| 任务 | 具体如何实现、由谁执行 | `ProjectItem(type=task)` |
| 版本里程碑 | 哪一批功能在哪个版本交付 | `Milestone(version=x.y.z)` |
| 甘特图 | 任务在时间与依赖上如何展开 | 任务计划日期与依赖的投影 |

功能蓝图不是另一套任务系统。功能节点不可调度，任务仍是唯一的执行单元。

模块和里程碑也不是同一个维度：

- 模块表达产品/系统范围。
- 里程碑表达版本和交付批次。
- 一个版本可以横跨多个模块。
- 一个模块可以跨多个版本逐步交付。

因此系统不把一级模块自动一一转换成里程碑，而是把功能点分配给目标版本，再由模块聚合展示版本覆盖。

## 3. 目标与非目标

### 3.1 目标

1. 在项目设置中按项目开启或关闭功能蓝图。
2. 支持一级、二级、三级模块和功能点的结构化维护。
3. 支持功能点追溯来源需求和交付任务。
4. 支持功能点设置目标版本里程碑。
5. 识别未拆解功能点和未归入功能蓝图的任务。
6. 由关联任务派生模块覆盖率和交付进度。
7. 将里程碑从自由命名改为系统生成的 `x.y.z` 版本。
8. 将里程碑前端改为按版本顺序排列的纵向树。
9. 点击某一版本后继续复用现有任务卡片和展开交互。
10. 让 AI PM 能从需求生成或维护功能蓝图，并从功能点拆解任务。

### 3.2 非目标

- 不建立第二套任务状态机。
- 不让功能节点直接进入调度器。
- 不把模块强制映射为里程碑。
- 不为模块单独维护另一套计划开始/结束日期。
- 不在 MVP 中引入功能蓝图发布基线和版本快照。
- 不批量破坏性重命名历史自定义里程碑。
- 不在 MVP 中提供无确认的递归删除。
- 不在 MVP 中让 AI 覆盖整棵已有功能树。

## 4. 项目级能力开关

### 4.1 设置入口

项目详情 → 设置 → 项目管理能力：

```text
☐ 启用功能蓝图
  按一级、二级、三级模块和功能点规划项目范围，
  并关联需求、任务和版本里程碑。关闭后仅隐藏入口，不删除已有数据。
```

配置保存在项目现有文件：

```text
<workspace>/.1agents/project_config.json
```

字段：

```json
{
  "featureCatalogEnabled": true
}
```

### 4.2 默认值

- 字段缺失时视为 `false`。
- 所有已有项目默认关闭，确保升级后界面零变化。
- 新项目在 MVP 中同样默认关闭。
- 后续项目创建向导可增加“轻量项目 / 规划型项目”模板；规划型项目默认开启。

### 4.3 开关语义

| 场景 | 行为 |
|---|---|
| 开启 | 任务管理内显示“功能蓝图”，AI PM 使用系统化蓝图流程 |
| 关闭 | 隐藏入口，AI PM 沿用需求 → 任务轻量流程 |
| 关闭后重开 | 恢复已有功能节点和关联 |
| 当前停在功能蓝图时关闭 | 自动回退到任务视图 |
| 配置保存失败 | UI 回滚开关并显示错误提示 |

开关是项目工作模式，不是权限边界。关闭不会删除功能数据。

## 5. 功能蓝图信息架构

### 5.1 入口

未启用时：

```text
总览 | 讨论 | 需求 | 任务 | 里程碑
```

启用后：

```text
总览 | 讨论 | 需求 | 功能蓝图 | 任务 | 里程碑
```

### 5.2 页面布局

桌面端采用模块树与详情区：

```text
功能蓝图                                  [让 AI 生成] [新增一级模块]

▾ 用户与权限                                覆盖 8/10 · 交付 63%
  ▾ 用户认证
    ▾ 登录
      · 密码登录                            0.1.0 · 3/3
      · 验证码登录                          0.2.0 · 1/3
      · 第三方登录                          未拆解
    ▸ 密码管理
  ▸ 组织与成员

右侧详情：
- 名称与说明
- 完整模块路径
- 来源需求
- 目标版本
- 交付任务
- 覆盖与进度
- 关联已有任务
- 让 AI PM 拆解为任务
```

窄屏使用单列树；点击节点进入详情，再通过全局返回操作回到树。

### 5.3 空状态

```text
尚未建立功能蓝图

从需求和架构设计出发，整理一级、二级、三级模块及功能点，
再将功能点分解为可执行任务。

[与 AI PM 一起生成] [手动新增一级模块]
```

### 5.4 人工操作

用户可以：

- 新增、修改、移动和排序模块。
- 新增、修改、移动和排序功能点。
- 关联来源需求。
- 关联已有任务。
- 设置目标版本。

创建新任务继续遵守现有产品原则：由 Agent/PM 创建。功能点提供“让 AI PM 拆解为任务”，不另造完整人工任务表单。

## 6. 功能蓝图领域模型

### 6.1 FeatureNode

模块与功能点统一为树节点：

```go
type FeatureNodeKind string

const (
    FeatureNodeModule FeatureNodeKind = "module"
    FeatureNodePoint  FeatureNodeKind = "feature"
)

type FeatureNode struct {
    ID                string          `json:"id"`
    ProjectID         string          `json:"-"`
    ParentID          string          `json:"parentId,omitempty"`
    Kind              FeatureNodeKind `json:"kind"`
    Title             string          `json:"title"`
    Description       string          `json:"description,omitempty"`
    TargetMilestoneID string          `json:"targetMilestoneId,omitempty"`
    Position          int             `json:"position"`
    CreatedAt         time.Time       `json:"createdAt"`
    UpdatedAt         time.Time       `json:"updatedAt"`
}
```

以下字段不落库：

- `level`：由父链派生。
- `path`：由父链派生。
- `progress`：由关联任务派生。
- 模块的版本列表：由后代功能点聚合。

### 6.2 树约束

- 模块最多三级。
- 一级模块是根节点。
- 模块的父节点必须是模块。
- 功能点必须挂在模块下，但可直接挂在任意级模块下。
- 功能点不能拥有子节点。
- 移动模块后整棵子树仍不得超过三级。
- 禁止循环。
- 父子节点必须属于同一项目。

允许轻量结构：

```text
一级模块 → 功能点
```

也允许完整结构：

```text
一级模块 → 二级模块 → 三级模块 → 功能点
```

### 6.3 表结构

```sql
CREATE TABLE feature_nodes (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL,
    parent_id           TEXT NOT NULL DEFAULT '',
    kind                TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    target_milestone_id TEXT NOT NULL DEFAULT '',
    position            INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX idx_feature_nodes_project_parent
    ON feature_nodes(project_id, parent_id, position);
```

功能点与项目项使用通用关联表：

```sql
CREATE TABLE feature_item_links (
    feature_id TEXT NOT NULL,
    item_id    TEXT NOT NULL,
    relation   TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (feature_id, item_id, relation)
);

CREATE INDEX idx_feature_item_links_item
    ON feature_item_links(item_id, relation);
```

关联类型：

| relation | 允许的项目项类型 | 含义 |
|---|---|---|
| `source` | requirement、bug | 功能点的来源 |
| `delivery` | task | 功能点的交付任务 |

使用多对多关系：

- 一个需求可影响多个功能点。
- 一个功能点可来源于多个需求。
- 一个功能点可由多个任务交付。
- 一个跨模块任务可支撑多个功能点。

## 7. 覆盖率与进度

### 7.1 覆盖率

```text
已关联至少一个有效交付任务的功能点数 / 功能点总数
```

没有关联任务的功能点显示“未拆解”，不显示为普通 `0%`。

### 7.2 交付进度

功能点：

```text
已完成交付任务数 / 非取消交付任务总数
```

规则：

- `completed` 计为完成。
- `cancelled` 不进入分母。
- 没有关联任务为“未拆解”。
- 所有关联任务都取消为“需要重新规划”。

模块聚合后代功能点所有有效任务，并同时显示未拆解功能点数量，避免已有任务完成后出现虚假的 `100%`。

### 7.3 派生状态

| 状态 | 条件 |
|---|---|
| 未拆解 | 无交付任务 |
| 待开始 | 有交付任务，但均未开始/完成 |
| 进行中 | 部分任务开始或完成 |
| 已交付 | 所有非取消任务完成 |
| 需要重新规划 | 关联任务全部取消 |

MVP 不增加人工维护的功能状态，避免任务状态与功能状态形成双重真相。

## 8. 语义化版本里程碑

### 8.1 核心要求

新里程碑不再由用户自由命名。系统必须生成严格的：

```text
x.y.z
```

例如：

```text
0.1.0
0.2.0
0.2.1
1.0.0
```

不使用 `v0.1.0` 前缀，持久化值和 UI 主标题统一为纯 `x.y.z`。

### 8.2 自增规则

创建里程碑时不显示名称输入框，只选择版本变更类型：

| 类型 | 规则 | 用途 |
|---|---|---|
| Patch | `x.y.(z+1)` | 修复、补充、小范围 follow-up |
| Minor | `x.(y+1).0` | 向后兼容的新功能阶段，默认选择 |
| Major | `(x+1).0.0` | 不兼容或重大平台阶段，必须显式选择 |

如果项目不存在任何语义化版本：

- 默认 Minor 创建 `0.1.0`。
- Patch 创建 `0.0.1`。
- Major 创建 `1.0.0`。

版本分配必须在 SQLite 事务中完成：

1. 查询项目内最高有效 SemVer。
2. 按 bump 类型计算下一版本。
3. 校验未被占用。
4. 创建里程碑。
5. 自动把前一语义化版本设为 predecessor。
6. 提交事务。

并发创建不得得到重复版本。

### 8.3 数据兼容

当前任务通过 `ProjectItem.Milestone` 名称字符串关联里程碑。为避免一次性破坏历史数据：

- `milestones` 增加 `version` 字段。
- 新里程碑的 `version` 和现有 `name` 都写入同一个 `x.y.z`。
- 新 API 以 `version` 为权威字段。
- 现有 `name` 保留为兼容 join key。
- 历史自由命名里程碑的 `version` 为空，标记为 legacy。
- 不自动重命名历史里程碑。
- 不改变历史任务的 `milestone` 字符串。

建议结构：

```sql
ALTER TABLE milestones
    ADD COLUMN version TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_milestones_project_version
    ON milestones(project_id, version)
    WHERE version != '';
```

Milestone 对外类型增加：

```go
Version  string `json:"version,omitempty"`
IsLegacy bool   `json:"isLegacy,omitempty"`
```

`Name` 在兼容期继续返回；新代码不得用自由名称创建版本里程碑。

### 8.4 API 变化

新创建请求：

```http
POST /api/agent/milestones
Content-Type: application/json

{
  "workspace_id": "project-1",
  "bump": "minor",
  "description": "交付功能蓝图基础 UI"
}
```

响应：

```json
{
  "id": "milestone-id",
  "version": "0.2.0",
  "name": "0.2.0",
  "description": "交付功能蓝图基础 UI",
  "predecessorId": "previous-version-id"
}
```

更新里程碑：

- 允许更新说明和目标日期。
- 不允许直接修改版本。
- 不允许修改 predecessor 破坏版本链。
- 如将来需要修正错误版本，必须设计专门的管理员迁移操作，不在普通编辑中开放。

CLI 目标形态：

```bash
1agents project-items milestones create --bump minor --description "..."
1agents project-items milestones create --bump patch --description "..."
```

废弃：

```bash
--name "任意名称"
--predecessor <任意里程碑>
```

兼容期可保留旧参数只用于 legacy 导入，但正常 UI、PM Skill 和文档不得继续使用。

### 8.5 版本排序

必须按 SemVer 数值排序：

```text
0.9.0 < 0.10.0 < 1.0.0
```

禁止按字符串排序，否则 `0.10.0` 会错误地排在 `0.9.0` 前。

### 8.6 纵向版本树

里程碑页面改为纵向版本树：

```text
● 0.5.0  甘特图与导出
│  目标日期：—
│  进度：0 / 2
│
● 0.4.0  AI PM 与批量生成
│  进度：0 / 2
│
● 0.3.0  追溯与版本闭环
│  进度：0 / 2
│
● 0.2.0  功能蓝图 UI
│  进度：0 / 1
│
● 0.1.0  数据与 API 基础
   进度：0 / 1
```

默认建议最新版本在上，沿竖线向下查看历史。可提供“最早优先 / 最新优先”切换，但持久化顺序始终由 SemVer 决定。

每个版本节点展示：

- 版本号。
- 说明。
- 目标日期。
- 完成任务数 / 总任务数。
- 进度状态。
- 是否为当前最近计划版本。

点击某一个版本：

- 展开或进入当前已有的任务卡片列表。
- 任务卡片组件、字段、操作和点击任务进入详情的行为保持不变。
- 只改变版本导航和编排方式，不重做任务卡片。

历史自由命名里程碑放入单独的“历史里程碑”折叠区：

```text
历史里程碑（30）
  ▸ Agents 圆桌交互改版
  ▸ Workspace Inbox
  ▸ Git P0：立刻可感
```

历史区继续复用原有任务卡片，但不参与 SemVer 自动计算和版本树连线。

## 9. 功能蓝图 API

### 9.1 节点

```http
GET    /api/agent/feature-catalog?workspace_id={id}
POST   /api/agent/feature-catalog
PATCH  /api/agent/feature-catalog/{featureId}
DELETE /api/agent/feature-catalog/{featureId}?workspace_id={id}
```

移动节点时由后端原子完成：

- 同项目校验。
- 节点类型校验。
- 循环校验。
- 最大三级校验。
- 同级位置重排。
- ProjectEvent 写入。

### 9.2 项目项关联

```http
POST   /api/agent/feature-catalog/{featureId}/items
DELETE /api/agent/feature-catalog/{featureId}/items/{itemId}?relation=delivery
```

后端校验：

- 只有功能点能关联项目项。
- `source` 只接受 requirement/bug。
- `delivery` 只接受 task。
- 功能点和项目项属于同一项目。
- 重复关联幂等。

### 9.3 删除

- 有子节点的模块返回 `409 has_children`。
- 删除功能点会删除关联，但不删除需求或任务。
- 删除有关联任务的功能点需要前端二次确认。
- 删除任务时清理 `feature_item_links`。
- 删除里程碑时清空对应功能点的 `target_milestone_id`。

### 9.4 项目事件

新增事件：

```text
feature_node.create
feature_node.update
feature_node.move
feature_node.delete
feature_link.link
feature_link.unlink
```

数据修改和事件写入必须位于同一事务。

## 10. AI PM 工作流

功能蓝图开启时：

```text
1. 澄清需求
2. 创建/确认 requirement 或 bug
3. 读取现有功能蓝图
4. 新建或调整模块和功能点
5. 建立 source 关联
6. 用户确认范围
7. 从功能点拆解任务
8. 建立 delivery 关联
9. 新任务继承功能点目标版本
10. 用任务日期和依赖形成执行计划
```

功能蓝图关闭时，继续当前 requirement → task 的轻量流程。

需要提供：

- `1agents feature-catalog` CLI。
- 与 CLI 同构的 MCP 工具。
- 事务化 batch API，供 AI 一次提交整棵草稿。
- “与 AI PM 一起生成”入口。
- “让 AI PM 拆解为任务”入口。

批量写入必须先校验整批数据，任一节点失败时整批不落库。

## 11. 甘特图原则

甘特图只使用现有任务事实：

- 时间：`PlannedStart` / `PlannedEnd`。
- 依赖：`DependsOn`。
- 版本：`Milestone`。
- 分组：功能模块路径。
- 进度：任务完成率。

模块计划时间由后代任务聚合：

```text
模块计划开始 = 后代任务最早 PlannedStart
模块计划结束 = 后代任务最晚 PlannedEnd
```

不在模块节点上再维护一套日期。

## 12. 验收摘要

1. 旧项目默认不显示功能蓝图。
2. 开关关闭不删除数据，重开后恢复。
3. 支持一至三级模块和功能点。
4. 拒绝第四级模块、循环和跨项目关联。
5. 功能点能关联来源需求、交付任务和目标版本。
6. 未拆解功能点和未归入功能蓝图的任务可被识别。
7. 模块覆盖率、进度和未拆解数量计算正确。
8. 新里程碑只能通过 bump 自动产生 `x.y.z`。
9. 并发创建不会产生重复版本。
10. 版本按 SemVer 数值顺序展示。
11. 里程碑前端为纵向版本树。
12. 点击版本继续显示和操作原有任务卡片。
13. 历史自由命名里程碑和任务关联保持不变。
14. AI PM 能维护功能蓝图并把任务关联回功能点。

