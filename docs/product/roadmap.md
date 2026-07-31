# 1Agents 产品里程碑路线图

> 本文是 1Agents 的**唯一权威版本路线图**。大版本固定 1/2/3/4，内部子版本连续递增、不跳号。
> GitHub 用[原生 Milestones](https://github.com/scottzx/1agents/milestones) 承载，看板见 [Project #3](https://github.com/users/scottzx/projects/3)。
> issue 引用形如 [#60](https://github.com/scottzx/1agents/issues/60)，点击直达。

---

## 设计理念：一张工作 Graph + 多条反馈回路 + 三时间尺度 + 两观察高度

1Agents 是面向多智能体协作的 Agent Infra。“一人成军”是用户获得的结果，**工作 Graph** 是产品组织数据、需求、任务、执行、验收和回写的方式。

Loop 只能描述一次重复；路线图围绕一张包含节点、状态、依赖、判断和回流的工作 Graph 展开：

```
数据源 → 数据治理 → Inbox → 需求 / 缺陷 → 任务蓝图 → Schedule → 1ACP
  ↑                                                            ↓
经验沉淀 ← 状态与结果回写 ← 关闭或返工 ← Check ← Agent/function/human
```

**三个关键判断（理念基石，Web 与大屏都据此而建）：**

1. **Inbox 是入口，任务蓝图是索引，验收回写是回流。** Inbox 接住新机会、用户反馈和会改变在跑项目的外部信号；任务蓝图把来源、功能点、交付任务、依赖和里程碑连接起来；Check 的结果回到 Graph，决定关闭、返工或后续节点。三者共同构成工作系统，不是三个孤立页面。

2. **三个时间尺度叠加，越往下越是复利护城河。**
   - 短（单任务）：报名填个表就完。
   - 中（单项目）：立项 → 交付 → 发布。
   - 长（沉淀）：提示词+工具 → 技能卡 → 任务模板 → 项目模板/架构。
   最值钱的是沉淀链——把每个项目的经验固化成可复用的「项目架构（带技能+依赖+检查项）」，让一个人越干越强。

3. **痛点根因：落地难 = 数据进不了工作流，执行结果回不到计划。** 1Agents 通过数据治理、Schedule、1ACP 和三类执行者推动节点，通过 Check 和状态回写维持 Graph。人不需要照看每一步，但仍在目标、授权、风险和最终验收节点承担责任。

**一致性原则（Web ↔ 大屏相呼应）：** 靠的不是长得像，而是共享同一套 **Graph 对象语言**（ProjectItem、任务蓝图、里程碑、执行者、状态和产物）以及两个**观察高度**：
- **大屏 = 俯瞰**：系统级驾驶舱，看 Graph 全貌、发现阻塞、做跨项目决策。
- **Web = 下钻**：进入单项目的需求、任务蓝图、里程碑、会话和执行环境，完成具体工作。
- 点大屏一个项目卡 → 落回 Web 单项目视图。同一批对象，换个海拔看。开罗暖色办公风是贯穿两高度的统一气质。

---

## 大版本主题

| 大版本 | 主题 | 一句话 |
|--------|------|--------|
| **1.x** | 单机好用 → 远程可达 | 桌面智能体打磨 + 轻量化远程控制（小程序/APP） |
| **2.x** | 任务自动化 → 数据可见 | 任务管理自动化 + 大屏只读「看」+ 小版本视频自动化 |
| **3.x** | 信息收口 → 闭环可控 | PMO/Inbox 收口 + 大屏可交互「控」 + 正反馈闭环 |
| **4.x** | 经验复利 | 复盘归档 → 提示词/工具 → 技能卡 → 任务模板 → 项目模板/架构 |
| **分支线** | 多设备集群 | Remote Control 之上的另一主线：无中心多设备 Mesh |

> **当前产品能力：** 已形成从数据 / Inbox、ProjectItem、任务蓝图、Schedule、1ACP 执行到 Check 与状态回写的工作 Graph。大版本、Milestone 和 issue 的历史归位仍以 GitHub Milestones 与 Project #3 为准。

---

## 子版本细分 + issue 归位

> ⬤ = 已有 Milestone；◯ = 预留槽位（仅文档，后续拆子 issue 时连续编号）。

### 1.x — 单机好用 → 远程可达
- ⬤ **1.0 桌面智能体基线**（已发布）：对话框+ACP 多智能体、文件系统、**Markdown/HTML 预览**（亮点）、Web 终端。本版仅加固 → [#52](https://github.com/scottzx/1agents/issues/52)
- ⬤ **1.1 前端多端地基 + 角色化外壳**：[#119](https://github.com/scottzx/1agents/issues/119)（core/+PlatformBridge 地基）、[#118](https://github.com/scottzx/1agents/issues/118)（多端重构总纲）、[#72](https://github.com/scottzx/1agents/issues/72)（角色化外壳：双栏+三层 sidebar，为 Inbox/PMO 预埋导航骨架）
- ⬤ **1.2 轻量化远程控制 · 小程序**：[#122](https://github.com/scottzx/1agents/issues/122)（小程序 WS AI 聊天）+ happy/survey H5 打底 + 小程序文件上传/管理
- ⬤ **1.3 远程控制 · 独立 APP**：[#121](https://github.com/scottzx/1agents/issues/121)（Tauri Mobile 壳 + 原生文件上传）、[#29](https://github.com/scottzx/1agents/issues/29)（桌面端 Tauri 语音输入）

### 2.x — 任务自动化 → 数据可见
- ⬤ **2.0 任务模型底座**：[#68](https://github.com/scottzx/1agents/issues/68)（归口原则）、[#74](https://github.com/scottzx/1agents/issues/74)（Task 字段+MCP 对齐 GitHub）、[#64](https://github.com/scottzx/1agents/issues/64)（链式 append-only）、[#130](https://github.com/scottzx/1agents/issues/130)（AI 原生任务引擎 T1–T4 总纲）、[#134](https://github.com/scottzx/1agents/issues/134)（Label/字段作机器可读触发器）、[#135](https://github.com/scottzx/1agents/issues/135)（任务模板强制 acceptance criteria）、[#14](https://github.com/scottzx/1agents/issues/14)（Verifiable Completion·CI-gate+审计时间线）
- ⬤ **2.1 自动化执行内核**：[#50](https://github.com/scottzx/1agents/issues/50)（三角色 PM/执行/校验）、[#47](https://github.com/scottzx/1agents/issues/47)（建议任务）、[#62](https://github.com/scottzx/1agents/issues/62)（权限演进）、[#63](https://github.com/scottzx/1agents/issues/63)（auto 权限）、[#128](https://github.com/scottzx/1agents/issues/128)（[P3] Dev→Review 验收闭环）、[#131](https://github.com/scottzx/1agents/issues/131)（[T1] 执行结果即提案·对抗式校验）、[#132](https://github.com/scottzx/1agents/issues/132)（[T1] 状态机由工件事件驱动）、[#133](https://github.com/scottzx/1agents/issues/133)（[T2] 事件驱动编排引擎）
- ⬤ **2.2 跨项目 + 集成**：[#91](https://github.com/scottzx/1agents/issues/91)（全局看板）、[#101](https://github.com/scottzx/1agents/issues/101)（飞书同步）、[#37](https://github.com/scottzx/1agents/issues/37)（PM 路线图补完）、[#129](https://github.com/scottzx/1agents/issues/129)（[P4] cc-connect 交互卡片·IM 一键批准/打回）、[#136](https://github.com/scottzx/1agents/issues/136)（[T3] 任务交叉引用+backlink 知识图谱）
- ⬤ **2.3 动态大屏 · 看**：[#120](https://github.com/scottzx/1agents/issues/120)（大屏 build target）、[#126](https://github.com/scottzx/1agents/issues/126)（开罗暖色换皮）、[#127](https://github.com/scottzx/1agents/issues/127)（接真实数据+阻塞显著性）、[#123](https://github.com/scottzx/1agents/issues/123)（[RFC] V3 愿景 umbrella，spans 2.3→3.3）
- ⬤ **2.4 小版本产品视频自动化**：[#145](https://github.com/scottzx/1agents/issues/145)（小版本发布即自动生成产品视频，类 hyperframe；**占位记录，需求回头细化**）

### 3.x — 信息收口 → 闭环可控

> **Inbox 全上下文引擎 + 专家系统底座定稿** → [docs/features/inbox-context-engine/design.md](../features/inbox-context-engine/design.md)。把 Inbox 升级为「全部外部上下文收口 + 自动运行引擎」，并据此搭角色仓/技能仓/开源吸收管线（吸收 kwiki/superpowers/gstack）。下列 ◯ 槽位拆 issue 时连续编号。

- ⬤ **3.0 Inbox 信息收口 + 立项**：[#60](https://github.com/scottzx/1agents/issues/60)（统一收口层 → 扩 domain+depth 维度 + 收口规则，见 RFC §3）、[#61](https://github.com/scottzx/1agents/issues/61)（PMO 需求分发）、[#67](https://github.com/scottzx/1agents/issues/67)（下游分叉·临时任务 vs 立项；个人 Task PhaseA 为本期地基）
  - ⬤ [#189](https://github.com/scottzx/1agents/issues/189) **Discussion 决策层**（合并 [#47](https://github.com/scottzx/1agents/issues/47) 建议卡）：对话线程 + 可选挂卡 + 拍板转需求/任务（RFC §3.1）
- ⬤ **3.1 专家细化 + 开源吸收**：[#139](https://github.com/scottzx/1agents/issues/139)（PMO 员工层细化·专属提示词+工具）、[#138](https://github.com/scottzx/1agents/issues/138)（开源项目吸收·MIT 可合并 → superpowers/gstack 吸收管线，RFC §5）、[#137](https://github.com/scottzx/1agents/issues/137)（智能体角色模板·YAML frontmatter+markdown）
  - ⬤ [#187](https://github.com/scottzx/1agents/issues/187) **三级解析 loader + 角色仓落点**（内置 embed + 用户/项目 `.1agents/roles|skills/` + 按名覆盖，RFC §4）
  - ⬤ [#188](https://github.com/scottzx/1agents/issues/188) **superpowers/gstack 吸收管线**（submodule 参照 + 转化器 + `.absorbed.json` 增量同步，RFC §5）
  - ⬤ [#190](https://github.com/scottzx/1agents/issues/190) **Inbox 自动调研管线**（定时爬虫 + L2 深度调研，gstack `/browse` 重映射 + 专家角色，接 [#133](https://github.com/scottzx/1agents/issues/133) 编排，RFC §3）
- ⬤ **3.2 大屏 · 控**：[#142](https://github.com/scottzx/1agents/issues/142)（大屏交互层 Phase2·卡片直接下指令/派工）
- ⬤ **3.3 闭环正反馈**：[#140](https://github.com/scottzx/1agents/issues/140)（社交数据回流→Inbox）、[#141](https://github.com/scottzx/1agents/issues/141)（项目阶段性归档/关闭：完成归档 or 竞品砍掉）

### 4.x — 经验复利（技能沉淀链）
- ⬤ **4.0 复盘归档 + 技能沉淀链总纲**：[#144](https://github.com/scottzx/1agents/issues/144)（自动化流程复盘+归档机制）、[#143](https://github.com/scottzx/1agents/issues/143)（技能沉淀链 epic）
  - ⬤ [#191](https://github.com/scottzx/1agents/issues/191) **kwiki 知识基底**（`raw/wiki/output` + ingest）：Inbox 沉淀 + 市场情报/个人健康落 wiki 的统一载体，是沉淀链的存储底座（RFC §3.3）
  - ⬤ [#192](https://github.com/scottzx/1agents/issues/192) **提醒层**（复用 Scheduled Tasks）+ ◯ **个人 wiki**：个人/健康域下游（RFC §3.2 / §7 Phase 4）
- ◯ **4.1 技能卡封装**（复用 skills 模块 `html/src/modules/registry.ts`）→ ◯ **4.2 任务模板** → ◯ **4.3 项目模板/项目架构（带技能+前后依赖+检查项）**：落在 [#143](https://github.com/scottzx/1agents/issues/143) 的 checklist，后续拆独立 issue 时连续编号

### 分支线 · 多设备集群
- ⬤ **分支线**（独立 Milestone，不占主版本序）：[#109](https://github.com/scottzx/1agents/issues/109)（主密钥持久化）、[#110](https://github.com/scottzx/1agents/issues/110)（设备注册+心跳+Tailscale）、[#111](https://github.com/scottzx/1agents/issues/111)（代理路由层）、[#112](https://github.com/scottzx/1agents/issues/112)（凭据迁移）、[#113](https://github.com/scottzx/1agents/issues/113)（设备管理 UI）、[#114](https://github.com/scottzx/1agents/issues/114)（多设备项目视图）、[#115](https://github.com/scottzx/1agents/issues/115)（Mesh 总计划 epic）

---

## 归位取舍记录

- **[#72](https://github.com/scottzx/1agents/issues/72) 角色化外壳** → 归 **1.1**（前端地基/导航骨架先行）；若想随小程序出可移 1.2。
- **[#120](https://github.com/scottzx/1agents/issues/120) 大屏 build target** → title 写「前端四端重构 Phase1」，但产品上随大屏内容落 **2.3**（refactor 的 Phase 号是该 epic 内部技术顺序，不绑产品版本序）。
- **[#123](https://github.com/scottzx/1agents/issues/123) RFC** → umbrella 锚定 **2.3**，实际 spans 到 3.3。
- **技能沉淀 Epic**：[#143](https://github.com/scottzx/1agents/issues/143)/[#144](https://github.com/scottzx/1agents/issues/144) 暂置于 Epic「其他」——建议在 Project #3 web UI 新增 Epic 选项「技能沉淀」后重新归类（API 改单选字段会重建选项 ID、风险波及存量条目，故未自动改）。
- **AI 原生任务引擎 T 系列**（[#130](https://github.com/scottzx/1agents/issues/130) 总纲 + [#131](https://github.com/scottzx/1agents/issues/131)/[#132](https://github.com/scottzx/1agents/issues/132)/[#133](https://github.com/scottzx/1agents/issues/133)/[#134](https://github.com/scottzx/1agents/issues/134)/[#135](https://github.com/scottzx/1agents/issues/135)/[#136](https://github.com/scottzx/1agents/issues/136)）是一块**自成体系的硬骨头**，现按数据层/执行层/知识层分散进 2.0/2.1/2.2。若希望集中追踪，可单独抽一个子版本（如插入「2.x AI 原生任务引擎」），届时其后子版本顺延重排。
