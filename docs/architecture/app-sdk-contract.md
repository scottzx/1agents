# 应用 SDK 契约 — 应用两面契约 · 北向任务 API · 三存储面

> **状态：演进中的外部 SDK 契约草案**（[名称定义表 §0.9](../product/名称定义表.md)）。编译期 App Registry、Manifest 查询与启停、前端视图注册及三类挂载基础设施已经落地；本文描述的是更完整的应用接入目标，不能把所有规范条目都视为当前已实现 API。当前事实以代码和 [agentsOS 架构](./agentsOS-架构设计.md)中的实现边界为准。
>
> 1agents「AI-native 组织 / 可组合多应用平台」的**外部应用接入规范草案**。
> 配套总纲见 [`agentsOS-架构设计.md`](./agentsOS-架构设计.md);本文是其 §10「应用契约两面(SDK)」的展开。
> **术语权威**：[名称定义表 §0](../product/名称定义表.md)。可调度单元 = ProjectItem（`ItemType=task`）；**核心 / 内核** = ②③④ 框架；**应用 / 专业模板** = ① 业务层 + function 实现。
> 当前适用范围：单用户 · 无 RBAC · 编译期模块 + Registry 启停；动态下载安装和第三方市场不在当前实现边界。

演进北极星：**新增注册应用尽量不改内核**。契约验证思路仍可用 #347 闸门语言描述，但本文不是当前版本全部 API 的实现证明。

---

## 目录

1. [契约一图流:应用提供什么 / 平台提供什么](#1-契约一图流)
2. [App Manifest(应用清单 · 规范形)](#2-app-manifest应用清单--规范形)
3. [平台提供给应用的能力](#3-平台提供给应用的能力)
   - 3.1 [北向任务 API(`backend/internal/taskapi`)](#31-北向任务-apibackendinternaltaskapi)
   - 3.2 [Function 注册表(一份注册、两种消费)](#32-function-注册表一份注册两种消费)
   - 3.3 [领域存储约定(三存储面)](#33-领域存储约定三存储面)
   - 3.4 [项目模板 · 项目配置 · 挂载点 · co-pilot · 共享源](#34-项目模板--项目配置--挂载点--co-pilot--共享源)
4. [应用必须提供的 SDK 表面](#4-应用必须提供的-sdk-表面)
5. [硬规则与保证](#5-硬规则与保证)
6. [新应用接入 checklist](#6-新应用接入-checklist)
7. [可拓展性验收(#347 闸门)](#7-可拓展性验收347-闸门)
8. [契约稳定性与标注约定](#8-契约稳定性与标注约定)

---

## 1. 契约一图流

应用是**一个产物,两种角色**(对内扩展单元 / 对外交付单元)。两面契约的边界:

```
┌─────────────────────── 应用(长出来 · 装上去)────────────────────────┐
│  manifest(id/挂载点/taskTypes/域表)                                   │
│  前端 view bundle      → registerAppView(id, Component)               │
│  后端模块               → 域表 ensureTables · function handler          │
│                          · 完成回写 hook · (可选)项目模板               │
│  business_ref 命名空间  → "<id>:<对象>:<主键>"                          │
└────────────────────────────────┬─────────────────────────────────────┘
                  仅经北向 API ↑  │  ↓ 应用不自带 agent
┌────────────────────────────────┴─────────────────────────────────────┐
│  平台 / 内核(出厂就有 · 保持干净)                                      │
│  北向任务 API   taskapi.{DispatchTask,QueryTask(s),                    │
│                 IssueTasksFromBusiness,ListTasksForBusiness,          │
│                 RegisterApp,RegisterCompletionHook,RegisterFunction}  │
│  执行层三态     agent(柔)/ function(刚)/ human(裁)                │
│  领域存储       meta.db(命名空间域表)· workspace 目录(产物)· 任务总线 │
│  项目外壳       动态/计划/任务/资产 + 项目配置(= agent 上下文)         │
│  挂载点 · co-pilot · 共享源(Inbox / RSS / digest)                    │
└───────────────────────────────────────────────────────────────────────┘
```

| 方向 | 谁出 | 内容 |
|---|---|---|
| **应用 → 平台** | 应用 | manifest · 前端 bundle · 后端模块(域表 + function + 回写 hook + 模板)· `business_ref` 命名空间 |
| **平台 → 应用** | 平台 | 北向任务 API · 领域存储 · 共享源 · 项目外壳 · co-pilot · 挂载点 |

铁律:**应用永远只经北向 API 调用 AI 能力,自己不嵌任何 agent**(总纲 §10 自举生命周期)。

---

## 2. App Manifest(应用清单 · 规范形)

应用 = **模块 + 挂载点**,由 manifest 声明。下面是**规范形**,新应用照抄结构:

```json
{
  "id": "media", "name": "自媒体", "version": "1.0.0", "enabled": true,
  "mountPoints": [
    { "type": "project-tab", "id": "material", "label": "素材", "view": "MediaMaterialTab" },
    { "type": "l1-page",     "id": "crm",      "label": "CRM",  "view": "CrmPage", "icon": "users" },
    { "type": "lens",        "id": "cost",     "label": "成本",  "view": "CostLens", "scope": "project" }
  ],
  "taskTypes": ["media.silence_detect"],
  "domainTables": ["media_content_project", "media_material"]
}
```

### 字段定义

| 字段 | 类型 | 必填 | 含义 |
|---|---|---|---|
| `id` | string | ✓ | **应用唯一标识 = 命名空间**。这一个值同时是:`business_ref` 前缀、域表前缀、`taskTypes` 前缀、`RegisterApp` 的 `Namespace`。全小写、`[a-z][a-z0-9_]*`。 |
| `name` | string | ✓ | 人类可读名(中文),用于挂载点导航与设置页。 |
| `version` | string | ✓ | 语义化版本。Phase 1 编译期模块,版本随发布走;**registry 开关即版本**。 |
| `enabled` | bool | ✓ | 是否启用。电台为 `opt-in`,默认可 `false`。停用即从 registry 摘除挂载点与权限。 |
| `mountPoints` | array | ✓ | 该应用挂哪些视图。可多挂载点(总纲 §8)。 |
| `taskTypes` | string[] | ✓ | 该应用会派发/实现的任务类型(= function type / 权限白名单),**全部以 `<id>.` 前缀**。喂给 `RegisterApp` 的 `AllowedTypes`,也是 `RegisterFunction` 的 key。 |
| `domainTables` | string[] | ✓ | 该应用在 meta.db 里建的域表名,**全部以 `<id>_` 前缀**。仅作清单与审计,真正建表靠后端 `ensureTables`(见 §3.3)。 |

### mountPoint 子字段与三种类型

| 字段 | 含义 |
|---|---|
| `type` | `project-tab` \| `l1-page` \| `lens`,三选一(见下表)。 |
| `id` | 挂载点在本应用内的局部标识(`<id>` 命名空间内唯一)。 |
| `label` | 导航上显示的中文名。 |
| `view` | 前端注册的视图组件键,对应 `registerAppView(view, Component)`(§4)。 |
| `icon` | (可选)L1 页 / tab 的图标名。 |
| `scope` | (仅 `lens`)`project` \| `home`,声明横切视图叠加到哪一层。 |

| 挂载点 `type` | 呈现 | 适用 | 准则 |
|---|---|---|---|
| **`project-tab`** | 项目详情加主标签,与「动态/计划/任务/资产」平级 | 自媒体、视频、研发/Bug | 一个项目 = 一个实例;继承项目外壳 |
| **`l1-page`** | 左侧导航独立全局页 | CRM、AI 电台、财务 | 一个全局实例,跨项目 |
| **`lens`** | 往项目 / Home 叠加横切视图 | 财务(成本)、CRM(关联线索) | 横切,叠加,不独占 |

> 准则(总纲 §8):**项目级应用必须做成 `project-tab`(继承项目外壳);全局级做 `l1-page`**。「专业模板」= 挂在项目里的应用,「全局应用」= 挂在 L1 的应用——同一套 registry,区别只是挂载点。

**`id` 是一根线串起四处命名空间**,这是契约里最该记住的一条:

```
manifest.id = "media"
  ├─ business_ref  → "media:content:123"
  ├─ 域表          → "media_content_project" / "media_material"
  ├─ taskType/fn   → "media.silence_detect"
  └─ RegisterApp   → AppPermissions{ Namespace: "media", AllowedTypes: ["media.silence_detect"], AllowedRefs: ["media:"] }
```

---

## 3. 平台提供给应用的能力

### 3.1 北向任务 API(`backend/internal/taskapi`)

> 这是已落地、稳定的 Wave 1 内核(#320),签名以 `backend/internal/taskapi/{api,binding,function}.go` 为准。应用**只**经此包调度任务,绝不 import runner/scheduler。

#### 服务构造与权限注册

```go
// New 在服务启动时构造一次,接到 server 上。
func New(store *meta.TaskStore) *API

// RegisterApp 在启动时声明应用权限,任何 DispatchTask 之前调用。
// 对同一 Namespace 幂等(后写覆盖)。
func (a *API) RegisterApp(p AppPermissions)

type AppPermissions struct {
    Namespace    string   // 应用 id,如 "media" / "crm" / "radio"
    AllowedTypes []string // 允许派发的任务类型;空 = 不限(内核自己)
    AllowedRefs  []string // 允许的 business_ref 前缀;空 = 任意
}
```

> Phase 1 单用户:权限是**荣誉制白名单**,非加密强制。`checkPermission` 当前对未注册命名空间放行(`AllowedRefs` 为空也放行),交付期再收紧到 RBAC(总纲 §11 留位)。

#### 派发任务

```go
// DispatchTask 创建并入队一个任务,返回新任务 ID。
// namespace = 调用应用 id(空 = 内核)。WorkspacePath 必填。
func (a *API) DispatchTask(namespace string, spec DispatchSpec) (string, error)

type DispatchSpec struct {
    Title              string             // 短标题
    Description        string             // 工作指令(Markdown);agent 任务作 prompt,function 任务仅信息性
    AcceptanceCriteria string             // 注入 agent prompt 的自检闸门
    Executor           meta.TaskExecutor  // "agent" | "function" | "human";默认 "agent"
    FunctionType       string             // executor=function 时的 handler key,如 "media.silence_detect"
    BusinessRef        string             // 绑定缝,如 "crm:lead:42";可空
    Target             *meta.TaskTargetSpec
    DependsOn          []string           // 依赖的任务 ID(依赖与执行者无关)
    Priority           string             // "urgent"|"high"|"medium"|"low";默认 "medium"
    Milestone          string             // 里程碑 / 阶段标签
    WorkspacePath      string             // 项目目录,必填
}
```

**executor 三态**(总纲 §6 刚 / 柔 / 裁):

| `Executor` | 含义 | 派发出口 |
|---|---|---|
| `meta.TaskExecutorAgent`(`"agent"`,默认) | 柔:推理 / 组织 / 变通 | 解析 `Target` → 1acp 在 cwd ensure 会话 → agent cd 进目录执行,需 verifier |
| `meta.TaskExecutorFunction`(`"function"`) | 刚:确定性 worker,token≈0 | 路由进 function 注册表;`FunctionType` 经 `fn:<type>` label 传递(见 §3.2) |
| `meta.TaskExecutorHuman`(`"human"`) | 裁:最终担责 | 入决策 UI / 任务队列,等用户动作,完成解锁下游 |

> **角色是任务的属性,不是工具的属性**:同一个 ffmpeg,时间戳明确时是 function 任务;跨多素材组织取舍时是 agent 手里的工具。

**`Target`(派发覆盖)** — `meta.TaskTargetSpec`,让任务覆盖项目默认值:

```go
type TaskTargetSpec struct {
    AgentType    string   `json:"agent,omitempty"`        // 覆盖派哪个 agent,如 "claudecode"
    Cwd          string   `json:"cwd,omitempty"`          // agent cd 进的绝对目录;空 = 项目 WorkspacePath
    Capabilities []string `json:"capabilities,omitempty"` // 为该任务注入的 MCP server 名
}
```

`Target` 为空时,任务的 agent / cwd / 能力取自**项目配置**(§3.4):`cwd = 项目目录`,1acp 在 cwd 起 agent(项目地址 = agent 的 pwd,总纲 §9)。

#### 查询任务

```go
// QueryTask 按 id 返回当前状态;ok=false 表示未找到。
func (a *API) QueryTask(id string) (meta.Task, bool, error)

// QueryTasks 返回某 workspace 的任务,可按 business_ref / executor 过滤;空串跳过该过滤。
func (a *API) QueryTasks(workspacePath, businessRef, executorFilter string) ([]meta.Task, error)
```

#### 绑定缝:任务 ↔ 业务对象

`business_ref` 是任务到应用业务对象 / 阶段的绑定缝(总纲 §5)。两个方向:

```go
// 正向:从业务对象/阶段派生一批任务,自动把 BusinessRef 设为 ref。
// ref 如 "crm:lead:42";stage 追加到每个 spec 的 Milestone(当其为空时)。
func (a *API) IssueTasksFromBusiness(namespace, ref, stage string, specs []DispatchSpec) ([]string, error)

// 反向:返回 business_ref == ref 的全部任务,供业务 UI 内联展示执行状态。
func (a *API) ListTasksForBusiness(ref string) ([]meta.Task, error)
```

正向示例(Wave 3 CRM):

```go
ids, err := api.IssueTasksFromBusiness("crm", "crm:lead:42", "enrich", []taskapi.DispatchSpec{
    {Title: "富集线索 42", Executor: meta.TaskExecutorAgent, WorkspacePath: ws},
})
```

#### 完成回写 hook

任务到达终态(completed / failed / cancelled)时,平台同步回调应用注册的 hook;应用据此把结果写回自己的域表。

```go
// RegisterCompletionHook 注册一个回调,任意任务到达终态时被触发。Wave 3 应用在此挂回写处理。
// hook 从 finish 路径同步调用 —— 保持快,重活转 goroutine。
func (a *API) RegisterCompletionHook(h CompletionHook)

type CompletionHook func(ev CompletionEvent)

type CompletionEvent struct {
    TaskID      string
    Status      meta.TaskStatus // completed | failed | cancelled
    Result      string          // executor 写入的 JSON 结果
    CostTokens  int64
    CompletedAt time.Time
}
```

回写约定:hook 内**按 `BusinessRef` 前缀认领自己的任务**(非本命名空间的事件直接 return),再把 `Result` 落进应用域表。回写走的就是这条路——结果不会自动进域表,内核只负责通知。

### 3.2 Function 注册表(一份注册、两种消费)

> 签名以 `backend/internal/taskapi/function.go` 为准。这是「专业模板的核心贡献」(总纲 §8):通用内核擅长 agent/human,缺确定性工人,模板把工人补进来。

```go
// RegisterFunction 把 handler 注册到全局注册表;typeName 即 taskType,
// 在任务 Labels 里以 "fn:<type>" 形式承载(如 "fn:media.silence_detect")。
// 并发安全;同 key 后注册覆盖。
func RegisterFunction(typeName string, handler FunctionHandler)

type FunctionHandler func(ctx FunctionContext) (result any, err error)

type FunctionContext struct {
    Task       meta.Task
    CostTokens int64 // 纯内存函数留 0;调外部 LLM/服务产生 token 成本时填
}
```

**一份注册、两种消费**(总纲 §6.1):

1. **独立 function 任务** — `DispatchTask{Executor: function, FunctionType: "media.silence_detect"}`,内核 function-runner 用 `Lookup(typeName)` 取出 handler 直执行,结果 JSON 写回 `task.result` 并标 completed(出错则 failed,走重试预算)。
2. **agent 工具** — 同一个 handler 经 MCP function-call 路径暴露给 agent 调用(Wave 3)。同一段确定性逻辑,既能独立刚跑,也能进 agent 的工具箱。

辅助:

```go
func Lookup(typeName string) FunctionHandler                 // 取 handler,未注册返回 nil
func ExtractFunctionType(labels []string) string             // 从 Labels 取 "fn:<type>" 的 <type>
func RunFunction(task meta.Task, workspacePath string, store *meta.TaskStore, api *API) // 内核 ready loop 调用,应用不碰
```

内核自带样例 handler 证明管线通(`core.noop`、`media.silence_detect` stub),Wave 3 用真实 ffmpeg/silero-vad 实现替换。

> 注册时机:在应用后端模块的 `init()` 或显式 `Register()` 里调 `RegisterFunction`(总纲:插件架构靠 `init()` 注册表)。`typeName` 必须落在 `<id>.` 命名空间内,并与 manifest 的 `taskTypes` 一致。

### 3.3 领域存储约定(三存储面)

三存储面(总纲 §9)——应用的数据分别落在哪:

| 面 | 存什么 | 应用怎么用 |
|---|---|---|
| **meta.db** | 元数据 / 索引 / 状态 | 应用建**命名空间域表**(`<id>_*`),存素材名/路径、阶段状态、业务对象 |
| **项目目录(workspace)** | 字节 / 产物本体 | 口播原片、音频、成片——二进制大产物走文件面 |
| **任务总线** | 只传 `spec + cwd` | "处理 cwd 下 clip2.mp4 的静音段" |

二进制大产物**不进任务 payload、不进 meta.db 字节**。

#### 域表建表约定:幂等 per-app `ensureTables`(契约约定)

应用在 meta.db 里建表,**绝不 bump 全局 `schemaVersion`**(当前 `meta.schemaVersion = 20`,`backend/internal/meta/db.go`)。内核已落地的样板就是这种「独立于 `user_version` 的幂等补列」——参见 `ensureProjectsColumns` / `ensureContactsColumns`:探测已有列/表,缺什么补什么,跑多少次都安全。

应用照抄此模式,提供一个 per-app `ensureTables(db)`:

```go
// 契约约定:应用后端模块导出一个幂等建表函数,启动时由 registry 调用一次。
// 不读、不写、不 bump 全局 schemaVersion;只对自己的 <id>_* 域表用
// CREATE TABLE IF NOT EXISTS / 探测列后 ALTER ADD(参见 meta.ensureProjectsColumns)。
func EnsureTables(db *meta.DB) error {
    // CREATE TABLE IF NOT EXISTS media_content_project (...);
    // CREATE TABLE IF NOT EXISTS media_material (...);
    // 后续加字段:tableColumns("media_material") 探测后 ALTER ADD COLUMN。
}
```

为什么这条是硬契约:全局 `schemaVersion` 是**内核迁移序号**;应用 bump 它会逼所有客户走全量迁移、并与并行未合分支撞号(MEMORY 里 v9/v15 撞号、靠无条件 ensure 共存的教训)。**应用建表必须与全局版本号解耦**,这正是「碰内核 = 0」可达成的关键之一。

### 3.4 项目模板 · 项目配置 · 挂载点 · co-pilot · 共享源

平台还把这些现成能力交给应用:

- **项目模板** — 新建专业项目 = 选模板(= 领域应用 = 固化产物)→ 铺 workspace:子目录骨架 + 域表 + 预置配置(总纲 §7)。应用可注册一个模板,定义建项时铺什么。模板 guide 参考 `backend/internal/workspace/templates/{agents_guide,project_guide}.md`。**契约约定**:模板注册经 registry,内核不为某个应用硬编码模板。
- **项目配置 = 该项目的 agent 上下文** — `指令`(system prompt)+ `连接器`(MCP)+ `专家`(角色)+ `技能` + `自动化`。项目内派任务时,任务的 `Target.Capabilities / 提示词`默认来自项目配置;应用模板可预置这些。
- **挂载点** — 见 §2;应用经 manifest 声明,内核负责把视图挂到项目 tab / L1 / lens。
- **co-pilot 上下文注入** — 随处可唤的对话入口,意图识别 → 落成任务。应用可向当前上下文(项目 / 业务对象)注入 co-pilot 上下文,让对话理解"现在在看哪条线索"。**契约约定**:注入是只读上下文,co-pilot 产出任务仍走北向 `DispatchTask`。
- **共享源(Inbox 收口 / RSS / digest)** — 平台提供的外部上下文入口。CRM 骑 Inbox intake + digest ACP,电台骑 RSS + TTS(总纲 §8 首批应用表)。应用消费共享源,不各自造采集。

---

## 4. 应用必须提供的 SDK 表面

一个应用接入,**完整 SDK 表面 = 四件 + 一个命名空间**:

| # | 交付物 | 形态 | 落点 |
|---|---|---|---|
| 1 | **manifest** | JSON / 结构体(§2 规范形) | registry 注册 |
| 2 | **前端 view bundle** | 组件 + `registerAppView(view, Component)` | 对应 manifest 每个 mountPoint 的 `view` |
| 3 | **后端模块** | 域表 `EnsureTables` + `RegisterFunction` handler + `RegisterCompletionHook` 回写 + (可选)项目模板 | `backend/internal/apps/<id>/`(契约约定路径) |
| 4 | **business_ref 命名空间** | `"<id>:<对象>:<主键>"` 约定 | 贯穿派发 / 查询 / 回写 |

**前端视图注册(契约约定)** — manifest 的 `view` 键映射到前端注册的组件:

```ts
// 契约约定:前端在应用入口注册视图,键与 manifest.mountPoints[].view 一致。
registerAppView('MediaMaterialTab', MediaMaterialTab);
registerAppView('CrmPage', CrmPage);
registerAppView('CostLens', CostLens);
```

> 多端(总纲外:React H5/桌面 + Taro 小程序/App 共享 `@1agents/core`):视图组件应尽量把数据访问放进共享 core,平台特定渲染留在端层。本契约只约定**注册键**与 manifest 对齐,不约束渲染框架。

**铁律重申**:应用后端模块**只**经北向 `taskapi` 调度 AI 能力,**不嵌任何 agent、不 import runner/scheduler**。应用运行时回调任务 API,统一由平台执行(总纲 §10 自举生命周期)。

---

## 5. 硬规则与保证

把"碰内核 = 0"拆成可逐条核对的硬规则(总纲 §4 / §12 纪律):

| # | 硬规则 | 含义 / 怎么违反 |
|---|---|---|
| R1 | **应用不依赖应用** | 应用之间零 import;跨应用协作只经 `business_ref` + 任务 API。CRM 不 import media 包。 |
| R2 | **核心保持干净** | 内核(`taskapi` / `meta` / runner / scheduler)不为任何具体应用硬编码分支。新应用不改内核文件。 |
| R3 | **registry = 版次边界** | 启用/停用经 registry 开关;**开关即版本**。应用挂载、权限、模板、function 全经注册表(`init()` 注册),不写死进内核启动序列。 |
| R4 | **不 bump 全局 `schemaVersion`** | 应用域表用幂等 per-app `EnsureTables`(§3.3),与 `meta.schemaVersion` 解耦。 |
| R5 | **不改既有 task 主流程** | 不动 `meta.Task` 模型、不改派发/调度/verifier 主链路。新能力靠 `business_ref` / `fn:<type>` label / 域表承载(沿用现有 `Labels` 承载 `fn:` 的做法)。 |

平台对应用的**保证**(反过来):

- 北向 API 签名稳定(Wave 1 已落地,见 §3.1/§3.2)。
- 任务到终态必触发 `RegisterCompletionHook`(回写有保障)。
- `business_ref` 双向绑定缝长期可用(`IssueTasksFromBusiness` / `ListTasksForBusiness`)。
- 三存储面边界不变:大产物走 workspace、总线只传 spec+cwd。

---

## 6. 新应用接入 checklist

第三个应用(电台)照此逐步走,每步标出**碰内核 = 0** 的核对点:

```
□ 1. 注册 manifest
     - 写 { id, name, version, enabled, mountPoints[], taskTypes[], domainTables[] }(§2)
     - id 全小写、唯一;它就是 business_ref / 域表 / taskType / Namespace 前缀
     - 核对:只新增 manifest,不改内核文件 ✓ R2/R3

□ 2. 建域表(幂等 EnsureTables)
     - CREATE TABLE IF NOT EXISTS <id>_*(...);后续字段靠 tableColumns 探测 + ALTER ADD
     - 仿 meta.ensureProjectsColumns / ensureContactsColumns
     - 核对:不 bump schemaVersion ✓ R4

□ 3. 注册项目模板(可选)
     - 定义建项时铺的 workspace 子目录骨架 + 预置项目配置(指令/连接器/专家/技能/自动化)
     - 经 registry 注册,不进内核硬编码
     - 核对:模板注册经 registry ✓ R3

□ 4. 注册 function handler + 完成回写 hook
     - RegisterFunction("<id>.<fn>", handler) —— 与 manifest.taskTypes 对齐(§3.2)
     - RegisterCompletionHook(h):h 内按 business_ref 前缀认领,写回自己的域表(§3.1)
     - 核对:只经 taskapi,不嵌 agent、不 import runner ✓ R5

□ 5. 注册前端视图
     - registerAppView(view, Component),键与 manifest.mountPoints[].view 一致(§4)
     - 数据访问尽量进共享 core
     - 核对:不改既有页面主流程,只挂新视图 ✓ R2

□ 6. 声明 taskTypes 权限
     - RegisterApp(AppPermissions{ Namespace:"<id>", AllowedTypes:[...], AllowedRefs:["<id>:"] })
     - 启动时、任何 DispatchTask 之前调用(§3.1)
     - 核对:权限经注册表声明,不改内核鉴权代码 ✓ R3

□ 7. 派发任务一律经北向 API
     - DispatchTask / IssueTasksFromBusiness;Executor 三态按"刚柔裁"选(§3.1)
     - 二进制产物走 workspace 目录,总线只传 spec+cwd(§3.3)
     - 核对:应用不依赖其它应用 ✓ R1
```

---

## 7. 可拓展性验收(#347 闸门)

#347「点播页 + SDK 契约完备性验证」拿 **AI 电台作为第三个应用**验收本契约。验收闸门:

> **接入第三个应用,必须做到:碰既有表 = 0、碰任务主流程代码 = 0。**

逐条对应硬规则,验收即逐条勾验:

| 验收项 | 硬指标 | 对应 |
|---|---|---|
| 电台可独立开关 | manifest `enabled` + registry 开关,停用即摘挂载点/权限 | R3 |
| 接入零内核改动 | 新增文件全在 `apps/radio/`(契约约定路径)+ manifest;`diff` 内核目录 = 空 | R2/R5 |
| 不碰既有表 | 只新增 `radio_*` 域表,经幂等 `EnsureTables`;`schemaVersion` 不变 | R4 |
| 不碰任务主流程 | 三段任务(agent + function TTS)全经 `DispatchTask` / `IssueTasksFromBusiness`;不改派发/调度/verifier | R5 |
| 不依赖其它应用 | radio 不 import media/crm 包 | R1 |

落地形态(总纲 §8 首批应用):电台挂 **L1 页**(opt-in)· 骑 **RSS + TTS** 共享源 · 新增 `RadioEpisode` 域 · 三段任务(agent 选题/写稿 + function TTS)· 音频走 workspace 文件面串流 · 点播页(player:剧集列表 + 播放器 + 逐字稿)。

**验收产出**:一份「碰内核 = 0」记录——列出电台接入的全部新增/改动文件,证明内核目录 diff 为空。这份记录就是 Phase 1 唯一成功标准(做第三个应用几乎零内核改动)的客观证据。

---

## 8. 契约稳定性与标注约定

本文混合两类内容,读者据此判断稳定度:

- **已落地(签名稳定)** — `backend/internal/taskapi/{api,binding,function}.go` 与 `backend/internal/meta/{types,db,tasks}.go`。§3.1、§3.2 的所有签名、`AppPermissions` / `DispatchSpec` / `TaskTargetSpec` / `CompletionEvent` 结构、executor 三态常量、`meta.schemaVersion = 20` 与 `ensureProjectsColumns` 幂等模式,均**直接来自已提交代码**。
- **实现核对说明** — `appregistry` / `domainstore` / `templateregistry`、前端 `registerAppView` 与三类挂载已有实现，但具体签名和应用目录布局以当前代码为准。文中显式标「契约约定」的内容仍是外部 SDK 目标，不应反向覆盖已经落地的内部接口。

> 落地纪律(总纲 §12):先立任务 API 边界、应用只经 API;`task` / `pm_*` 物理表先共存、延后拆;别第一步重写 `meta.Task`。本契约就是那条边界的成文版本。
