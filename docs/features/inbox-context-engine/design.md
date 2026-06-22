# Inbox 全上下文引擎 + 专家系统底座（RFC 定稿）

**Status:** RFC 定稿（待拆 issue）
**Author:** scott + Claude
**Date:** 2026-06-22
**Scope:** `backend/internal/meta`（inbox_items / roles）、`modules/1skills`、新增角色仓 `.1agents/roles/`、`modules/_upstream/*`（吸收参照）
**关联路线图:** [docs/roadmap.md](../../roadmap.md) 3.x（信息收口→闭环可控）+ 4.x（经验复利）

> 本文是把 Inbox 从「统一收件箱」升级为「**全部外部上下文收口 + 驱动自动运行的引擎**」，并据此搭建「专家系统」（角色 + 技能 + 吸收管线）的单一事实来源。跨 Inbox / 角色 / 技能 / 开源吸收四块，先有定稿再拆 issue，避免各写各的。

---

## 0. 背景与定位

现状缺口：

- **Inbox（#60）只有「来源」一个维度**（manual/im/email/rss/misc），且只做被动收口——它该承载的「全上下文 + 自动运行」还没有。
- **专家/PMO（#139）、角色模板（#137）、开源吸收（#138）三件事各自为政**，没有统一的「角色 ≠ 技能」对象模型和落盘规则。
- 我们想吸收三个成熟开源项目（见 §1），但它们是 Claude Code 插件，不能原样运行。

本 RFC 把这些收敛成一句话：

> **Inbox 是公司的感知器官（雷达）。所有外部上下文从这里进，经分类/深挖/讨论后，要么转成需求驱动项目执行，要么沉淀进知识库。专家（角色）+ 技能 + 吸收管线是支撑这条流水线的底座。**

这与 [roadmap.md](../../roadmap.md) 的「信息收口是入口也是回流口」「三时间尺度叠加，沉淀链是复利护城河」完全一致——本 RFC 是那条理念在 Inbox/专家层的具体落地。

---

## 1. 三个外部项目的吸收定位

三者不冲突，正好各补一层（都 MIT，走 #138 吸收机制）：

| 项目 | 本质 | 补我们哪一层 | 落点 |
|---|---|---|---|
| **kwiki**（卡帕西知识库） | 知识基底：`raw/ → wiki/ → output/`，把知识「编译」一次再查询 | Inbox 的 L1/L2 处理大脑 + 沉淀基底 | §3 知识基底、§7 Phase 4 |
| **superpowers**（obra） | 技能原语：`SKILL.md` 格式 + 过程技能（brainstorm/TDD/plan）+ `writing-skills` 元技能 | 技能层（过程方法） | `modules/1skills` |
| **gstack**（garrytan） | 角色化软件工厂：23 个 slash skill 按角色编排成 Think→Plan→Build→Review→Ship→Reflect | 角色层（新员工：CSO/design reviewer/市场分析师）+ 全周期流水线 | 角色仓 + `modules/1skills` |

**我们的差异化**（三项目都没有，要守住）：多引擎 `engine+model`（#137）、工件/事件驱动的对抗式验证（#130/#131/#132）、PMO 驾驶舱（#123）。

---

## 2. 五条核心决策

**决策 1 · 三层吸收。** kwiki=知识基底，superpowers=技能原语，gstack=角色+流水线。

**决策 2 · Inbox = 收口 + 引擎，加两个维度。** 在 #60 `inbox_items` 上**扩字段、不扩表**：
- `domain`：工作（售前/售后/研发/市场）/ 市场情报 / 个人健康 …
- `depth`/`action`：仅归档 / 需讨论 / 需深挖 / 直接转需求

**决策 3 · 谁能写进哪一层（收口规则）。**
- **外部源**（RSS / IM 群聊 / 爬虫 / 邮件）→ 只能进 **L0 Inbox**（必须先 triage）。
- **boss 会话** → 可写任意层：丢 Inbox（待办）/ 起 Discussion（idea 边聊边深化）/ **快路径**直进需求池或任务池（已决定，跳过 PMO/Discussion）。

**决策 4 · 角色 ≠ 技能（两种对象，别混）。**
- **技能** = 提示词 + 工具 → 可调用能力（#143），落 `modules/1skills`。
- **角色/员工** = 引擎/模型 + 提示词 + 工具 + **绑定的 skills** + 权限（#137），落角色仓。
- PMO/PM/CSO/市场分析师是**角色**，不归口进 `1skills`。gstack 吸收时**拆成「角色 + 技能」两份**分别归口；角色用 `skills:[]` 按名引用技能。后端两个 loader。

**决策 5 · 三级解析 + 内置 embed。**
- 内置级 = `//go:embed` 进二进制（删不掉，gstack 吸收的角色随产品发布）。
- 用户级 = `~/.1agents/roles|skills/`；项目级 = `<repo>/.1agents/roles|skills/`。
- **按名覆盖、永不删、删了回落内置、改内置 = fork 到用户级（删 fork 即还原）。** 角色仓与技能仓同一套规则。照搬 Claude Code user/project 两级 + 项目隔离（#137 已拍板）。

---

## 3. Inbox 引擎架构

### 3.1 分级管线（借 kwiki）

```
驱动源:
  push  ── big boss 会话框输入 ────────────┐
  pull  ── RSS / IM群聊 / 定时爬虫(Top50) ─┤
                                          ▼
  ┌───────────────── INBOX 引擎 ─────────────────┐
  │ L0 capture   原始落库 (raw)              #60   │
  │ L1 ingest    自动分类(domain)+摘要+标签   kwiki │
  │ L2 deep      值得深挖→二次调研/深度爬取         │ ← gstack /browse 重映射 + 专家角色
  │ L3 card      生成 Discussion 卡(=Agent建议卡)  │ ← 合并 #47
  └───────────────────────────────────────────────┘
                  ▼ 人(boss)在 Discussion 讨论/决策
                  ▼ 确定事件 → 按 domain 路由
       ┌──────────┼───────────────┬──────────────┐
   工作域          市场情报域        个人/健康域
   →requirement    →沉淀进 wiki      →个人 wiki + 提醒
   →#67/#68 执行   →(可触发新需求)    →不进项目链
```

- **外部源只进 L0**；boss 可在任意层写入（决策 3）。
- **Discussion = 对话线程，可选挂 0~N 张卡**。有卡（L2 深挖产物 / Agent 建议卡 #47）讨论更顺；无卡就是 idea 对话，讨论本身在细化它。**卡是上下文增强，不是前置条件。**
- 自动运行靠 **事件驱动编排（#133）**：定时爬虫/RSS → 自动 ingest → 自动出卡 → 等人拍板 → 派工，正是 roadmap「雷达：既入口又回流口」的闭环。

### 3.2 域路由

| domain | 下游 | 说明 |
|---|---|---|
| 工作（售前/售后/研发/市场需求） | requirement → #67/#68 执行链 → 需求池 → #49/#50 排期 | 走既有立项/任务链 |
| 市场情报（榜单、竞品） | 沉淀进 knowledge wiki | 出现「大厂已做同类」可触发砍项目（#141）或新需求 |
| 个人 / 健康体检 | 个人 wiki（按时间沉淀趋势）+ 提醒 | **不进项目链**；要变行动项时降级为无 `project_id` 的临时 Task（#67） |

### 3.3 知识基底（kwiki 模型）

`raw/wiki/output` 直接做 Inbox 的存储约定：raw=收口原文，wiki=分类后知识（市场情报、健康趋势），output=转出的需求/报告。`.ingested.json` 记 ingest 历史。市场情报与个人健康都落 wiki；wiki 也是沉淀链（#143/#144）的载体。

---

## 4. 角色仓 / 技能仓（专家系统底座）

### 4.1 落点与三级解析（决策 4 + 5）

```
角色 (agent-role)                          技能 (skill)
 ├ 内置  → backend embed (//go:embed)        ├ 内置  → backend embed
 ├ 用户  → ~/.1agents/roles/                 ├ 用户  → ~/.1agents/skills/
 └ 项目  → <repo>/.1agents/roles/            └ 项目  → <repo>/.1agents/skills/  (现 modules/1skills 为编写源)

解析: 内置 ← 用户 ← 项目, 同名向下覆盖; 删了回落内置; 改内置=fork到用户级
后端: 角色 loader 读角色仓; 技能 loader 读 1skills; 角色.skills:[] 按名绑定技能
```

内置源文件放后端仓（如 `backend/internal/roles/builtin/*.md`）`//go:embed`；可让 `1skills` / 新 `1roles` submodule 当编写源，构建时拷进 embed 目录（类比现有「前端 → `html.h`」的 make 流程，不是新机制）。

### 4.2 角色 schema（沿用 #137）

```markdown
---
name: 市场分析师
description: 对榜单/竞品做深度调研，产出 why-分析
engine: claude-code
model: opus-4.8
permission_mode: auto
effort_level: medium
tools: { allow: [Read, WebSearch], deny: [Bash] }
skills: [deep-research, design-shotgun]   # 按名引用 1skills
mcp_servers: [mcp-tasks]
source: gstack@<sha>                       # 吸收来源（§5）
license: MIT
---
（system prompt, markdown 正文）
```

---

## 5. 吸收管线（superpowers / gstack）

**双轨，不原样运行**（它们是 CC 插件，带 hooks/代码）：

```
轨道 A · 上游只读参照(用来 diff/拉更新)
  modules/_upstream/superpowers   ← submodule, 钉 SHA
  modules/_upstream/gstack        ← submodule, 钉 SHA
        │  git submodule update --remote
        ▼
轨道 B · 转化后的成品(我们真正加载)
  1skills/<skill>/SKILL.md   + 角色仓/<role>.md   ← 我们格式, frontmatter 记 source/license/上游SHA
```

**增量同步 = 吸收管线（= #138 + #137 importer + kwiki ingest，同一件事）：**

```
1. git submodule update --remote          # 拉上游
2. diff 变更的 SKILL.md vs .absorbed.json 记录的上次 SHA
3. 过转化器导入我们格式 (复用 #137 loader 的 import 能力)
4. 人在 Discussion 卡里 review/拍板        # 复用 §3 的 L3 决策动作
5. commit, .absorbed.json 记 source@<sha>  # 增量、可审计 (= kwiki .ingested.json)
```

**两项目处理方式不同：**
- **superpowers ≈ 格式同构**（我们已用 `SKILL.md`）→ 转化轻：策展 + 翻译 + 补 frontmatter 字段。几乎全进 `1skills`。
- **gstack = markdown 方法论 + 真实代码**（`/browse` Chromium、`qa` 框架）→ 拆：方法→技能进 `1skills`，角色身份→角色仓；**code-backed 技能（`/browse`）不 vendor 代码，重映射到我们已有浏览器/computer-use 能力**。

**合规（都 MIT）：** 轨道 A submodule 自带 LICENSE 即满足；轨道 B 派生文件 frontmatter 写 `source/license/SHA`，加一个 `THIRD_PARTY.md` 汇总归属。

---

## 6. 数据模型草案

**Inbox（#60 扩字段，不扩表）：**

| 字段 | 新增? | 说明 |
|---|---|---|
| `source` | 既有 | manual/im/email/rss/crawler/misc |
| `domain` | 🆕 | work/market/personal… |
| `depth` | 🆕 | archive/discuss/deep/to-requirement |
| `summary`/`tags` | 既有 | L1 ingest 产物 |
| `discussion_id` | 🆕 | 关联到 Discussion 线程（可空） |
| `routed_to` | 🆕 | requirement/wiki/personal-task（域路由结果） |

**Discussion（合并 #47 建议卡）：** 一条对话线程，`cards: []`（可空，挂 inbox_items / agent 建议卡），`decision`（拍板结论）→ 转 requirement / task。

**.absorbed.json：** `{ "<skill-or-role>": { "source": "superpowers", "sha": "<上游commit>", "absorbed_at": "..." } }`。

> 字段名最终以落地 issue 为准；本表定语义边界。

---

## 7. 四期实施计划

每期可独立交付。关键依赖：**A1/A2 + C1 是地基**；**Discussion(C3) 与吸收评审(B) 共用「人拍板」动作**；**爬虫深挖(C4) 必须等专家角色(B4) 到位**。

| Phase | 工作项 | 依赖 | 里程碑 |
|---|---|---|---|
| **P1 双地基（并行）** | **C1** 临时任务/个人 Task 汇总层（#67 PhaseA，无 project_id）<br>**A1+A2** 三级 loader（embed+用户/项目+按名覆盖+fork/restore）+ 角色仓 schema（#137） | — | 3.0 / 3.1 |
| **P2 Inbox MVP + 吸收管线** | **C2** Inbox 扩 domain+depth + 收口规则（#60）<br>**C3** Discussion 决策层（合并 #47，对话线程+可选卡+拍板转需求/任务）<br>**B1+B2+B3** 吸收管线（submodule 参照 + 转化器 + .absorbed.json 增量同步） | C1,C2 / A1 | 3.0 / 3.1 |
| **P3 首批吸收 + 真·自动运行** | **B4** 吸收 superpowers/gstack → 1skills + 角色仓（/browse 重映射）<br>**C4** 定时爬虫 + L2 深挖（专家角色 + 事件驱动 #133） | A2,B2 / B4,C2 | 3.1 / 3.2 |
| **P4 沉淀 + 提醒** | **C5** 知识基底 wiki（kwiki raw/wiki/output）<br>**C6** 提醒层（复用 Scheduled Tasks）<br>**C7** 个人 wiki + 沉淀链对接（#143/#144） | C2 / C1 | 3.3 / 4.0 |

---

## 8. issue 映射

| 工作项 | 复用/扩写既有 | 新建 issue |
|---|---|---|
| A1/A2/A3 角色仓 + 三级 loader | 扩写 #137 | **#187** 三级解析 loader + 角色仓落点 |
| B1–B4 吸收管线 + 首批吸收 | 挂 #138 | **#188** superpowers/gstack 吸收管线 |
| C1 临时任务层 | #67 PhaseA 细化 | — |
| C2 Inbox 引擎化 | 扩写 #60 | — |
| C3 Discussion 决策层 | 合并 #47 / 接 #61 | **#189** Discussion 决策层 |
| C4 自动调研 | 接 #133 编排 | **#190** 自动调研管线 |
| C5 知识基底 | 喂 #143/#144 | **#191** kwiki 知识基底 |
| C6 提醒层 | 复用 Scheduled Tasks，依赖 #67 | **#192** 提醒层 |
| C7 个人 wiki + 沉淀 | #143/#144 | （个人 wiki 待 #191 后拆） |

---

## 9. Open Questions（不阻塞）

1. **角色仓 submodule 还是仅 `.1agents/roles/` 目录？** 倾向先目录（embed 内置 + 磁盘两级），需要分享/版本化再抽 `1roles` submodule。
2. **L2 深挖的触发与配额。** 哪些 domain/规则自动触发深挖（如「榜单出现未见过的产品形态」），如何限流/控成本——交 #133 编排 + 后续细化。
3. **域路由表是否需要可配置 UI**，还是先内置规则。倾向先内置。
4. **转化器复用 #137 loader 还是单写。** 倾向复用 import 能力，仅加 frontmatter 补全 + provenance 记录。

---

## 10. Out of Scope（明确）

- 原样运行 superpowers/gstack 的 hooks/slash 运行时（我们不是 Claude Code）。
- vendor gstack 的 `/browse` 等 code-backed 引擎（重映射到自有能力）。
- 提醒层的复杂日程/重复规则引擎（先复用 Scheduled Tasks 最小可用）。
- 重人机协同（沿用 roadmap：先做一人 + 一群 AI）。
