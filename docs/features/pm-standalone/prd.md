# PRD：PM 插件能力（主入口 = `1agents`，不拆独立 pm 包）

| 字段 | 内容 |
|------|------|
| 状态 | **边界草案 · 待评审** |
| 版本 | **v0.4** |
| 日期 | 2026-07-18 |
| 看板 | requirement **#176**；讨论纪要 **#177** |
| 定位一句话 | **PM 是 1agents 核心能力**；对外以 **`1agents` CLI 为主入口** 接入 Claude Code / Codex（记任务 + 可选 MCP + localhost 看板）；**不**另发用户面 `@1agents/pm` |
| 关联 | [project-model](../project-model/design.md)、[issue-model](../issue-model/design.md)、[npm-package-split](../npm-package-split/prd.md) |

---

## 0. 产品命题（v0.4）

PM **是** 1agents 的核心能力，不是旁路产品。

> **让只用 Claude Code / Codex 的用户，通过安装 `1agents`（不必打开完整工作台 UI），像用插件一样记任务、看看板。**

### 0.0 还要不要拆 `@1agents/pm`？

| 选项 | 结论 |
|------|------|
| 另发用户面包 `@1agents/pm` / bin `1pm` | **不做（MVP 否决）** |
| 主入口 | **`1agents`**（`@1agents/1agents` + `@1agents/core-*` 二进制） |
| 仓库内模块整理 | 实现期可做；**用户无感知**，不增加第二品牌 |

**为什么不拆 pm 包：**

1. 记任务 CLI **已经是** `1agents project-items`，现 PM skill 已在用——再造 `1pm` = 双品牌、双文档、双安装。  
2. npm 已在拆 `@1agents/1agents` / `core-*`；「装插件」= 装主入口即可。  
3. PM 是核心：独立包容易被当成「可选附属」，与定位相反。  
4. 真正缺的不是新包名，而是：  
   - **CLI/MCP 自运行**（不依赖先起完整 daemon）  
   - **轻量看板 URL**（`1agents board` 之类，给内置浏览器）

**能力挂在 `1agents` 下：**

| 能力 | 命令形态 |
|------|----------|
| 任务记录（默认） | `1agents project-items …` |
| 可视化 | `1agents board`（名称可微调：`project-items ui` / `board serve`） |
| 可选 MCP | `1agents project-items` 的 mcp/stdio 模式（现有雏形 → 改为自运行） |

### 接入优先级

| 优先级 | 入口 | 谁用 | 做什么 |
|--------|------|------|--------|
| **P0** | **`1agents project-items …`** | 人 + agent shell | 任务**记录** |
| **P0** | **`1agents board`** | 人 + agent 内置浏览器 | 表 / 看板 / 详情 |
| **P1** | **MCP（`1agents` 拉起）** | 已配 MCP 的 agent | 与 CLI 同构 CRUD |
| — | ~~自动派单~~ | — | **非 MVP** |

**CLI 优先于 MCP**：安装摩擦更低；agent 默认会 bash；与现 skill 一致。MCP 是同构增强，不是入门门槛。

用户旅程：

```text
1. npm i -g @1agents/1agents          # 得到 1agents 命令
2. cd my-repo
   1agents project-items list
   1agents project-items create --title "…" --type task …
3. （可选）配置 MCP → command=1agents，args 指向 project-items mcp 模式
4. 1agents board → 打印 http://127.0.0.1:PORT，内置浏览器打开
5. 同一 ~/.1agents/meta.db；完整工作台以后再开也通
```

Skill / 文档 **只教 `1agents`**。

---

## 0.1 已锁定决策

| # | 决策 | 说明 |
|---|------|------|
| **D0** | **主入口 = `1agents`；不拆用户面 `@1agents/pm`** | v0.4 否决第二 CLI 品牌 |
| D1 | **存储 = `~/.1agents/meta.db`** | 全局库；cwd 锁定项目 |
| D2 | **作用域 = cwd → project** | 默认不跨项目写；`--project` 仅调试 |
| D3 | **MVP = project-items CLI + board URL + 可选 MCP** | 记录优先；无自动派单 |
| D3a | **CLI 优先于 MCP** | 文档/skill/验收默认 CLI |
| D3b | **看板 = loopback URL** | 供 agent 内置浏览器 |
| D4 | **内部实现栈可后定** | 用户面命令已锁定为 `1agents` |
| D5 | **CLI / MCP / board 均自运行** | **禁止**「必须先起完整 daemon/工作台」才能记任务、开轻量看板 |
| D6 | **分发 = 现有 `@1agents/1agents` + core** | 不另发 pm npm 包 |
| D7 | **PM 即核心域** | 无平行任务系统 |
| D8 | **CLI ↔ MCP 动词同构** | 同一 core |

> 非 MVP：schedule 自动派单；executor adapter；完整工作台作为最强宿主。见 §8。

---

## 1. 问题与动机

### 1.1 现状

```text
Agent ──stdio──► project-items MCP ──HTTP──► 1agents daemon ──► meta.db
前端 TaskList ──────────────────HTTP──────────────────────────┘
```

缺口（对 Claude Code / Codex 用户）：

1. 不想开完整工作台时，记任务/看板路径不清晰。  
2. MCP 依赖 daemon，装「插件」摩擦大。  
3. 没有干净的 localhost 看板 URL 给内置浏览器。  
4. **任务记录**是最小公倍数；自动执行可后置。

### 1.2 目标

- 装 **`@1agents/1agents`** 即可用 **`1agents project-items`** 记任务（人/agent shell）。  
- **`1agents board`** 给出 loopback 看板。  
- 可选 MCP 与 CLI 同构、自运行。  
- 与完整工作台 **同一 meta.db**。  
- 不强制自动执行。

### 1.3 非目标（MVP）

| 非目标 | 原因 |
|--------|------|
| 用户面 `@1agents/pm` / `1pm` | D0 |
| 自动派单 / TaskRunner | 记录优先 |
| 验证门 / GitHub sync / IM | 主产品重能力 |
| 项目本地 db 替代全局库 | D1 |
| 替代全站 Chat UI | 轻量看板即可 |

---

## 2. 用户与场景

| 角色 | 场景 | 成功标准 |
|------|------|----------|
| Claude Code / Codex（默认） | shell 调 `1agents project-items …` | **无 MCP**；无完整工作台进程也能记任务 |
| 可选 MCP | mcpServers 指向 `1agents` | 同库同语义 |
| 内置浏览器 | 打开 board URL | 见当前 cwd 项目看板 |
| 人类 | 终端 + 浏览器 | 一致 |
| 已有工作台用户 | 并行 | 同一 meta.db |
| CI | `--json` | 可脚本化 |

---

## 3. 包与进程边界

### 3.1 命名（已收敛）

| 项 | 选择 |
|----|------|
| 用户安装 | `npm i -g @1agents/1agents`（沿用 [npm-package-split](../npm-package-split/prd.md)） |
| 用户命令 | **`1agents`** |
| 任务子命令 | **`1agents project-items …`**（已有） |
| 看板子命令 | **`1agents board`**（推荐名；实现期可别名） |
| 另发 `@1agents/pm` | **否** |

### 3.2 进程模型（MVP）

```text
  人 / Agent shell ──► 1agents project-items … ──┐  一次性
                                                   │
  Agent MCP（可选）──► 1agents … mcp ─────────────┼──► meta core ──► ~/.1agents/meta.db
                                                   │
  浏览器 / 内置浏览器 ◄── 1agents board ──────────┘  短驻 HTTP+UI
```

- **无**「必须先起 daemon」；board 仅在用户/agent 需要可视化时短驻。  
- 完整 `1agents` 工作台 daemon 仍是**最强宿主**，不是插件前提。

### 3.3 与 npm 多包关系

| 包 | 角色 |
|----|------|
| `@1agents/1agents` | 安装入口、路径 resolve |
| `@1agents/core-*` | 含 `1agents` 二进制（project-items / board / 可选 mcp 都在此或同版本发布） |
| `@1agents/web` 等 | 完整工作台用；**轻量 board UI 可内嵌 core 或极小静态资源**，实现期定 |
| `@1agents/pm` | **不发布** |

体积：若担心「只想记任务却装很重」，优先靠 **core 已瘦身**（npm 拆分已把 cc-connect/skills 等拆出），而不是再发明 pm 包。若未来仍嫌重，再议「core 最小 profile」，仍不引入第二品牌。

---

## 4. 存储与项目解析

### 4.1 库

- **`~/.1agents/meta.db`**，WAL，多进程（CLI / board / 完整 daemon）安全。  
- Schema 权威：`internal/meta`（单源，禁止平行实现漂移）。

### 4.2 cwd → project

1. cwd 向上匹配已注册 `workspace_path`。  
2. 未注册时（评审二选一，默认建议 **B**）：  
   - A 报错并提示注册  
   - B 惰性注册（repo 根或 cwd）  
3. 锁定后只读写该 project。  
4. MCP 默认无跨项目参数；CLI 可保留 `--project` 调试。

### 4.3 Schema 版本

读写前检查 version；迁移与主产品同源；禁止静默破坏可读性。

---

## 5. 领域模型（沿用）

讨论 / 需求 / 缺陷 / 任务 + 里程碑；验收与归口 → `not_ready` 语义保留；`#N` 引用与 graph 保留。  
MVP 不实现 verifier 面板、github 同步等重字段的写入路径（可读可忽略）。

---

## 6. 接口契约

### 6.1 CLI（P0 · 默认）

```text
1agents project-items list|get|graph|create|discussion|update|close|reopen|milestones …
1agents board [--port] [--host 127.0.0.1]
1agents project-items …   # mcp 模式：实现期定 flag/子命令，自运行
1agents doctor            # 可选：store / cwd / 版本
1agents -version
```

要求：

| 要求 | 说明 |
|------|------|
| 自运行 | list/create/update **不依赖**已启动的工作台 daemon |
| Agent 友好 | 稳定退出码；`--json`；stdout 干净 |
| 与现 skill 同构 | 现 `.agents/skills/pm` 路径可继续用（最多改绝对路径安装说明） |
| 帮助自洽 | `project-items help` 足够自学 |

### 6.2 MCP（P1 · 可选）

- 由 **`1agents`** 拉起（非独立 bin）。  
- Tool 名与现网兼容：`list_project_items` / `create_project_item` / …  
- **默认 cwd 解析**；`ONEAGENTS_BASE_URL` **不再必需**。  
- 与 CLI 同构；不配 MCP 不影响 MVP 成功。

### 6.3 Board HTTP + UI（P0）

- 启动打印 URL：`Board: http://127.0.0.1:…`  
- 仅 loopback；最小 API：project / project-items / milestones / health  
- UI：多维表 + 看板 + 详情（轻量 SPA；可先薄后与 TaskList 收敛）  
- 不做完整 Chat / 跨项目 Dashboard

### 6.4 安装契约（文档置顶）

```bash
npm i -g @1agents/1agents
1agents project-items list
1agents board
```

Skill 片段只含 `1agents project-items` / `1agents board`。  
可选 MCP 示例：`command: "1agents"`, `args: ["…", "mcp 模式"]`。

---

## 7. 并发

| 场景 | 期望 |
|------|------|
| 仅 CLI / 仅 board | 正常读写 |
| CLI + board + 完整 daemon | WAL 可见；MVP 轮询刷新即可 |
| 同 task 并发写 | 不损坏库 |
| schema 迁移 | 单方迁移 + version 检测 |

---

## 8. 阶段路线

| Phase | 内容 | 出口 |
|-------|------|------|
| **0** | 本 PRD 评审 | D0–D8 冻结 |
| **1 MVP** | project-items **自运行** + **board** URL + 可选自运行 MCP | 无完整工作台也能记任务、看轻量看板 |
| **2** | Scheduler / 派单（可选） | 自动执行 |
| **3** | 去掉 MCP 纯 HTTP 代理遗留 | 单实现 |
| **4** | 验证门等重能力 | 工作台宿主 |

---

## 9. 成功指标（MVP）

1. `npm i -g @1agents/1agents` 后，**无**工作台 daemon、**无** MCP，即可 `project-items` CRUD。  
2. Agent 仅靠 shell + skill 完成记任务闭环。  
3. `1agents board` 给出 loopback URL，内置浏览器可看板。  
4. 可选 MCP 与 CLI 同库同语义。  
5. 与完整工作台数据互通。  

---

## 10. 开放问题

| # | 问题 | 选项 |
|---|------|------|
| Q1 | ~~包名 pm？~~ | **已关闭：不发 `@1agents/pm`** |
| Q2 | 未注册目录 | 报错 vs 惰性注册 |
| Q3 | board 子命令最终名 | `board` / `project-items ui` / … |
| Q4 | 轻量 board UI 静态资源放哪 | 嵌 core / 小包 / 复用 web 子集 |
| Q5 | UI 是否对齐主站 token | MVP 否 / 是 |
| Q6 | pdf 导出进 MVP？ | 建议否 |

---

## 11. 验收清单（#176）

- [ ] 确认 **D0：主入口 1agents、不拆 pm 包**  
- [ ] 确认自运行 + board URL 为 Phase 1 范围  
- [ ] Q2/Q3 有结论或「实现期再定」  
- [ ] 再拆 Phase 1 实现任务  

---

## 12. 附录：代码锚点

| 能力 | 位置 |
|------|------|
| MCP（现代理） | `backend/internal/projectitems/` |
| meta / SQLite | `backend/internal/meta/` |
| HTTP project-items | `backend/internal/agent/handler.go` |
| Scheduler / Runner | `backend/internal/agent/scheduler.go`, `runner.go` |
| 前端看板 | `frontend/src/components/drawer/TaskList/` |
| PM skill | `.agents/skills/pm/SKILL.md` |
| project-model | `docs/features/project-model/design.md` |
| npm 拆分 | `docs/features/npm-package-split/prd.md` |
