# Git 面板性能与体验优化 PRD（P0–P2）

| 字段 | 内容 |
|------|------|
| **Status** | Done — P0–P2 已实现（#136–#148 completed） |
| **Author** | PM (Grok) |
| **Date** | 2026-07-17 |
| **Doc** | `docs/features/git-panel/prd.md` |
| **Scope** | `frontend/src/components/drawer/GitPanel.tsx`；相关 i18n / SCSS；可选 `backend/internal/git/handler.go`；可选 `frontend/src/services/gitService.ts` |
| **相关分析** | Git 前端现状评审（会话 2026-07-17）：轮询过重、render 重算、架构单体、分支 API 与 UI 脱节 |
| **执行人** | human（看板 `assignee=user` / `executor=human`，不派 agent） |
| **看板** | 需求 **#136**；任务 **#137–#148**；里程碑 Git P0 / P1 / P2 |

---

## 1. 背景与问题陈述

### 1.1 现状

应用内版本控制 UI 集中在 **`GitPanel`**（约 1660 行 class 组件），两处挂载：

| 入口 | 文件 |
|------|------|
| 右侧抽屉 `activeDrawerTab === 'git'` | `RightPanel.tsx` |
| 舞台 content view `case 'git'` | `ContentViewHost.tsx` |

**已有能力（保留，不回退）：**

- staged / unstaged / untracked 分区；单文件与全部 stage/unstage；discard
- 工作区 / commit / worktree 内联 diff（共用 `renderDiffPanel`）
- Commit + AI commit message；Push / Pull / Fetch；ahead/behind
- Worktree 切换与只读 peek
- 提交图（lane 布局 + SVG rail + worktree 徽章）
- 15s 自动刷新 + 外部 `onRegisterRefresh`

后端 `backend/internal/git/handler.go` 提供完整 REST（含 **branches / checkout / log**），前端目前主要用 status / worktrees / graph / stage* / commit / push|pull|fetch。

### 1.2 核心矛盾

面板功能已超过「简单状态条」，但体感与工程成本被以下问题拖住：

1. **轮询过重且带 loading 闪烁**：每 15s 对 status + worktrees + graph(limit=100) 全量拉取，且 `loading: true`。
2. **渲染路径偏贵**：`buildGraphLayout` 与 `parseDiffLines` 在每次 render 重算；大 diff 全量 DOM。
3. **架构债**：单体 class、裸 `fetch`、无 Abort、stage/unstage 不检查 `res.ok`、`workdir` prop 未参与请求。
4. **产品缺口**：同 worktree 分支切换/创建 UI 缺失（后端已有）；文件路径不能一键打开编辑器；无乐观更新 / 冲突高亮。

### 1.3 产品目标

在 **不推翻现有 worktree + graph 能力** 的前提下：

1. **P0**：面板空闲时安静、快、无假 loading；失败可感知。
2. **P1**：可维护的 service/组件边界，便于后续功能增量。
3. **P2**：补齐高频缺失（分支、打开文件、乐观更新、冲突可见）。

### 1.4 成功标准（全局）

| ID | 标准 |
|----|------|
| S1 | 空闲打开 Git 面板时，15s 窗口内网络请求以 status 为主；graph 不每次都打 |
| S2 | 后台轮询不置全局 `loading`；首载 / 手动 refresh 仍有明确 loading |
| S3 | 浏览器页签 hidden 或离开 Git 视图后无后台轮询（定时器停） |
| S4 | 输入 commit message / toast 刷新时，不重算 graph layout（graph 数据未变） |
| S5 | stage/unstage 失败有 toast；成功有状态刷新；不再静默吞错 |
| S6 | 无回归：现有 worktree / commit / AI commit / push-pull / graph 展开仍可用 |
| S7（P2） | 用户可在面板内切换/创建分支；点文件路径可打开文件预览/详情 |

**不在本 PRD 范围（明确延后）：**

- Stash UI、hunk/行级 stage、完整 merge 冲突解决器、blame
- 引入第三方 Git 库 / 替换 git CLI 后端
- Graph 虚拟滚动全量重写（P1 可先 limit/缓存；完整虚拟列表另案）

---

## 2. 用户与场景

### 2.1 用户

| 角色 | 诉求 |
|------|------|
| 日常开发者 | 看变更、stage、commit、push；面板别卡、别闪 |
| 多 worktree 用户 | 继续 peek 其它 worktree；与分支能力并存 |
| 无 worktree 用户 | 需要切分支/建分支，不必只靠终端 |
| 执行人（human） | 卡片粒度清晰、验收可勾选 |

### 2.2 关键用户故事

1. **作为开发者**，我希望 Git 面板开着写代码时，不会每 15 秒整页闪 loading。
2. **作为开发者**，我希望 graph 折叠时不要白打 100 条 commit 的接口。
3. **作为开发者**，我希望 stage 失败时立刻看到原因，而不是列表「好像没动」。
4. **作为开发者**，我希望切走 Git tab 或最小化浏览器后，后端不再被 git 命令轰炸。
5. **作为开发者（P2）**，我希望在面板里切分支，并点击改动文件直接打开编辑。

---

## 3. 现状架构（实现锚点）

```
frontend/src/components/drawer/GitPanel.tsx   # 单体 UI + 状态 + layout + diff parse + fetch
frontend/src/components/drawer/RightPanel.tsx # 抽屉挂载
frontend/src/components/stage/ContentViewHost.tsx
frontend/src/i18n/dict.ts                    # git.* 文案（含未接线的 branch/log search）
frontend/src/style/index.scss                # .git-panel …
backend/internal/git/handler.go              # /api/git/*
backend/internal/server/server.go            # 路由注册
```

**刷新链路（当前）：**

```
componentDidMount → refresh() every 15s
  → loading=true
  → GET /api/git/status
  → loadWorktrees()  GET /api/git/worktrees
  → loadGraph()      GET /api/git/graph?limit=100
  → (optional) worktree-status
```

**已知结构性问题（摘要）**

| # | 问题 | 优先级 |
|---|------|--------|
| 1 | 轮询 `loading: true` 闪烁 | P0 |
| 2 | status / worktrees / graph 同频全量刷新 | P0 |
| 3 | 页签 hidden / 非 Git 视图仍轮询 | P0 |
| 4 | stage/unstage 不检查 `res.ok` | P0 |
| 5 | `buildGraphLayout` 每 render 重算 | P0 |
| 6 | `parseDiffLines` 每 render 重算 | P0 |
| 7 | 无 `gitService`、裸 fetch | P1 |
| 8 | 单体 1660 行难测难改 | P1 |
| 9 | 无 Abort / workspace 切换竞态 | P1 |
| 10 | graph limit=100 偏重；无 snapshot API | P1 |
| 11 | 分支切换/创建 UI 缺失（后端有） | P2 |
| 12 | 文件路径无法打开编辑器 | P2 |
| 13 | 无乐观 stage/unstage | P2 |
| 14 | 冲突文件（UU/AA 等）无高亮 | P2 |

---

## 4. 范围与优先级总表

### 4.1 阶段划分（里程碑）

| 阶段 | 里程碑名 | 对应优先级 | 目标一句话 | 目标日期 |
|------|----------|------------|------------|----------|
| **P0** | Git P0：立刻可感 | P0 | 安静刷新 + 正确错误 + 缓存 | 2026-07-19 |
| **P1** | Git P1：结构可维护 | P1 | service 拆分 + 竞态防护 + 接口减负 | 2026-07-24 |
| **P2** | Git P2：产品体验 | P2 | 分支 / 打开文件 / 乐观更新 / 冲突可见 | 2026-08-07 |

### 4.2 P0 — 立刻可感

| ID | 项 | 说明 |
|----|----|------|
| P0-1 | Silent poll | 后台轮询不设 `loading: true`；仅首载 / 手动 refresh 显示 loading |
| P0-2 | 拆分刷新频率 | status 保持 ~15s；graph 仅在展开时 60s+ 或 mutation 后刷新；worktrees 可与 status 同批或 30s |
| P0-3 | 页签隐藏停轮询 | `document.visibilityState === 'hidden'` 时 clearInterval；visible 再启并可选立即 silent refresh |
| P0-4 | stage/unstage 错误处理 | 检查 `res.ok`；失败 toast（i18n）；成功再 refresh |
| P0-5 | layout / diff 缓存 | graph 数据未变不重跑 `buildGraphLayout`；diff content 未变不重跑 `parseDiffLines` |

### 4.3 P1 — 结构可维护

| ID | 项 | 说明 |
|----|----|------|
| P1-1 | `gitService.ts` | 统一 API 封装，与 `fsService` 风格对齐 |
| P1-2 | 组件拆分 | 至少拆出 `DiffPanel` + graph layout 纯函数；推荐 `GitChanges` / `GitGraph` / `GitWorktreeSwitcher` |
| P1-3 | Abort + 竞态 | workspace 切换 / unmount 取消 in-flight；toast timer 清理 |
| P1-4 | 后端减负 | graph 默认 limit 下调（如 30）或 `GET /api/git/snapshot`；可选 status 短 TTL |

### 4.4 P2 — 产品体验

| ID | 项 | 说明 |
|----|----|------|
| P2-1 | 分支切换 / 创建 | 接线 `/api/git/branches` + `/checkout`；与 worktree 切换器并存 |
| P2-2 | 点文件打开 | 变更列表 / commit 文件行 → 打开 FileDetail / preview |
| P2-3 | 乐观 stage/unstage | 本地立刻挪文件行，失败回滚 + toast |
| P2-4 | 冲突状态可见 | porcelain 冲突码高亮（至少 UU/AA/DD 等） |

---

## 5. 功能需求详述

### 5.1 P0 — 立刻可感

#### P0-1 Silent poll

- **行为**：
  - `refresh({ silent?: boolean })`：`silent=true` 时不改 `loading`。
  - `setInterval` 调用 `refresh({ silent: true })`。
  - `componentDidMount` 首次、`onRegisterRefresh` 手动刷新、workspace 切换：`silent=false`（或等价「show loading」）。
- **验收**：
  - 打开面板首载可见 loading 文案/spinner。
  - 15s 后列表数据可更新，但无明显整板 loading 闪烁。
  - 手动点刷新（若 UI 有）仍有 loading 反馈。

#### P0-2 拆分刷新频率

- **行为**：
  - **Status**：默认 15s silent 轮询。
  - **Graph**：仅当 `graphExpanded === true` 时定时刷新（建议 ≥60s），或在 commit / push / pull / fetch / checkout 等 mutation 后刷新一次。
  - **Worktrees**：可与 status 同频，或合并进 mutation 后刷新；禁止「status 一次、graph 无脑 100 条」同频。
- **验收**：
  - graph **折叠**时：DevTools 中不应周期性出现 `/api/git/graph`。
  - graph **展开**时：graph 请求频率明显低于 status。
  - commit 成功后 graph 与 status 均更新。

#### P0-3 页签隐藏停轮询

- **行为**：
  - 监听 `visibilitychange`：hidden → 停 timer；visible → 启 timer + 一次 silent refresh。
  - 组件 unmount 必须清理 timer 与 listener。
  - （推荐）若父级能感知「当前不是 git 视图」，也可在 unmount 时自然停止（已满足则无需额外全局 store）。
- **验收**：
  - 切到其它浏览器页签 ≥30s，回到时再请求；hidden 期间无 `/api/git/*` 周期性请求。
  - 关闭 Git 面板/切 tab 卸载组件后无 timer 泄漏。

#### P0-4 stage/unstage 错误处理

- **行为**：
  - `stage` / `unstage`（含 all）读取 `res.ok`；失败读取 `res.text()` 进 toast。
  - 成功再 `refresh({ silent: true })` 或局部更新。
  - 新增 i18n key（中/英）：如 `git.toast.stageFailed` / `unstageFailed`。
- **验收**：
  - 模拟后端 500：出现失败 toast，文件不错误搬家。
  - 正常 stage：列表分区正确更新。

#### P0-5 layout / diff 缓存

- **行为**：
  - `graph` 数组引用或内容签名未变时，复用上次 `buildGraphLayout` 结果（instance 字段或 state）。
  - `diffContent`（及可选 file key）未变时，复用 `parseDiffLines` 结果。
  - 输入 commit message、toast 显隐不得触发上述重算。
- **验收**：
  - 在展开 graph + 打开大 diff 时输入 commit 文案，主线程无明显卡顿回归（可用 performance mark 或人工对比）。
  - graph 数据更新后 layout 仍正确。

---

### 5.2 P1 — 结构可维护

#### P1-1 gitService

- 新建 `frontend/src/services/gitService.ts`（或等价路径），封装现有 `/api/git/*`。
- GitPanel（及拆出的子组件）不再散落裸 `fetch`。
- **验收**：面板行为无回归；服务层函数可单测 mock。

#### P1-2 组件拆分

- 最低交付：`parseDiff` + `buildGraphLayout` 纯函数文件；`DiffPanel` 展示组件。
- 推荐：Changes / Graph / WorktreeSwitcher 分文件，GitPanel 只做编排。
- **验收**：功能无回归；单文件行数显著下降（GitPanel 壳建议 &lt; 600 行）。

#### P1-3 Abort 与清理

- fetch 带 `AbortSignal`；workspace 切换 / unmount abort。
- toast `setTimeout` 在 unmount 清理。
- **验收**：快速切换 workspace 不出现「旧仓库数据闪一下」；无 React 卸载后 setState 告警。

#### P1-4 后端减负

- 二选一或组合：
  - graph 默认 `limit` 降为 30，UI 可「加载更多」；或
  - `GET /api/git/snapshot?includeGraph=0|1` 合并 status(+worktrees)。
- **验收**：空闲轮询时 git 进程压力下降（定性：请求次数与 payload 变小）；功能无回归。

---

### 5.3 P2 — 产品体验

#### P2-1 分支切换 / 创建

- 使用已有 `/api/git/branches`、`/api/git/checkout`。
- UI 与 worktree 切换并存（分支管当前 worktree HEAD；worktree 管目录）。
- 可复用 i18n 中已有 `git.branch.*` 文案（若仍匹配）。
- **验收**：可列出分支、切换、创建并切换；失败 toast；成功后 status/graph 更新。

#### P2-2 点文件打开

- 变更列表 / commit 文件行点击（或明确按钮）打开文件预览/详情（复用现有 fs / stage store 能力）。
- **验收**：存在文件可打开；已删除文件有合理提示。

#### P2-3 乐观 stage/unstage

- 点击后立即更新本地分区；请求失败回滚 + toast。
- **验收**：弱网下操作仍跟手；失败不丢数据一致性。

#### P2-4 冲突可见

- porcelain 冲突类状态高亮 + 文案（UU/AA/DD/AU/UA 等至少覆盖常见项）。
- **验收**：构造冲突仓库时列表可见冲突标记。

---

## 6. 非目标与约束

| 约束 | 说明 |
|------|------|
| 外科手术 | 不顺便重写 SCSS 设计体系；graph 色可继续硬编码 lane 色（P2 末可选 token） |
| 兼容 | 不破坏 RightPanel / ContentViewHost 现有 props 契约（可扩展 optional） |
| 执行 | 全部任务 **assignee=user（human）**，不派 agent 自动改代码 |
| 测试 | P0 以手工验收为主；P1 service/纯函数优先补单测 |

---

## 7. 里程碑与任务映射

| 里程碑 | 看板 | 任务 | PRD ID |
|--------|------|------|--------|
| Git P0：立刻可感 | #137 | Silent poll + 页签隐藏停轮询 | P0-1, P0-3 |
| Git P0：立刻可感 | #138 | 拆分 status / graph 刷新频率（建议先 #137） | P0-2 |
| Git P0：立刻可感 | #139 | stage/unstage 错误处理与 i18n toast | P0-4 |
| Git P0：立刻可感 | #140 | graph layout 与 diff 解析缓存 | P0-5 |
| Git P1：结构可维护 | #141 | 抽取 gitService（依赖 P0 四卡） | P1-1 |
| Git P1：结构可维护 | #142 | 拆分 DiffPanel / layout 与面板子模块 | P1-2 |
| Git P1：结构可维护 | #143 | Abort、竞态与 timer 清理 | P1-3 |
| Git P1：结构可维护 | #144 | graph limit / snapshot 后端减负 | P1-4 |
| Git P2：产品体验 | #145 | 分支列表切换与创建 | P2-1 |
| Git P2：产品体验 | #146 | 变更文件一键打开 | P2-2 |
| Git P2：产品体验 | #147 | 乐观 stage/unstage | P2-3 |
| Git P2：产品体验 | #148 | 冲突状态高亮 | P2-4 |

依赖建议：

```
P0 四卡可并行（建议先做 refresh 策略两卡）
  └─→ P1-1 gitService
        ├─→ P1-2 拆分
        ├─→ P1-3 Abort（可与 P1-1 同 PR）
        └─→ P1-4 后端（可并行）
              └─→ P2 四卡（分支/打开文件优先）
```

---

## 8. 验收清单（Epic 级）

### P0 Done

- [ ] Silent poll 行为符合 5.1 P0-1
- [ ] 折叠 graph 时无周期 graph 请求
- [ ] hidden 页签停止轮询
- [ ] stage 失败可见
- [ ] 输入 commit 文案不重算 graph/diff
- [ ] 既有 Git 流程手工回归通过

### P1 Done

- [ ] gitService 接入
- [ ] 主文件瘦身 / 模块边界清晰
- [ ] 快速切 workspace 无脏数据
- [ ] 轮询 payload 或次数下降

### P2 Done

- [ ] 分支切换/创建可用
- [ ] 点文件可打开
- [ ] 乐观更新失败可回滚
- [ ] 冲突文件可见

---

## 9. 风险

| 风险 | 缓解 |
|------|------|
| silent refresh 与用户正在看的 diff 打架 | mutation 后关闭过期 diff 或按 path 重载 |
| graph 降频导致「刚 push 图不变」 | mutation 后强制 graph refresh |
| 拆分 PR 过大 | P0 不拆文件；P1 再拆 |
| 分支 UI 与 worktree 混淆 | 文案分区：当前 worktree 的分支 vs 其它 worktree |

---

## 10. 参考

- 实现主文件：`frontend/src/components/drawer/GitPanel.tsx`
- 后端：`backend/internal/git/handler.go`
- 路由：`backend/internal/server/server.go`（`/api/git/*`）
- 设计风格：`Claude.md` / `Agents.md` 中 Frontend Design Language
- 同类 PRD 体例：`docs/features/data-grid/prd.md`
