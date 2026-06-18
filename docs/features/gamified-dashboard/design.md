# Feature Design: 一芥像素星际工坊 (1Agents Hangar - Gamified Dashboard V3)

**Status:** Draft / Proposed (V3 Design Concept, 2026-06-19)  
**Author:** scottzx + Claude  
**Scope:** `docs/features/gamified-dashboard/`, `backend/internal/meta/`, `html/src/`  
**Relation:** 建立在 `project-model` (SQLite) 和 `issue-model` (时间线) 之上的游戏化上层建筑。

---

## 1. 核心设计哲学 (Design Philosophy)

1agents 作为一个“一人成军的 AI 原生办公软件”，旨在通过 AI 智能体编排帮助超级个体（独立开发者、微型创始团队）完成复杂的开发和任务接力。

为了消除传统研发管理看板（如 Jira、GitHub Board）的枯燥与信息过载，V3 版本引入**开罗游戏式（Kairosoft）模拟经营 + 异星工厂（Factorio）式自动化流水线**的机制。将真实的算力成本、API 速率限制、最佳实践积累和社区交付，转化为可视化的游戏机制，让开发过程本身变得富有心流和乐趣。

---

## 2. 雇员系统 (AI Employee & Shadow Clone)

与传统经营游戏不同，AI 员工不需要按物理实体招募，而是遵循“配置模板”与“影分身（Shadow Clone）”的 AI 特性。

### 2.1 员工分类
- **基础员工 (Basic Employees)**：直接映射为纯净的底座模型（如 Claude 3.5 Sonnet, GPT-4o）。没有固定的专长与个性台词，仅受限于基础速率限制（Stamina/精力值）。
- **专才员工 (Specialist Employees)**：从真实聊天会话（Session）的**最佳实践 (Best Practice)** 中提炼封装。
  - **封装逻辑**：当一个开发 Session 极其成功地解决了一个难题（如写出了优秀的 SVG 渲染器），用户可以将该会话的配置（底座模型 + ACP 框架 + 装配的特定版本 Skills 技能包 + System Prompt Persona + 上下文参考）一键保存为“专家卡牌模板”。
  - **专家库**：固化后的专才员工拥有独特的 8-bit 花名、头像、战绩表（参与项目数/主观评分）和专属吐槽气泡。

### 2.2 影分身与全局精力池
- **无限分身**：同一个雇员配置模板可以指派给任意多个项目的 Task 并行执行。
- **精力值 (Stamina) 映射**：
  - **充值余额**：每次运行消耗 API 资金（从公司 Funds 中扣除）。
  - **速率限制 (Rate Limits)**：大模型提供商的请求频次限制（如每 3 小时 50 次）直接映射为该雇员的 **Stamina 最大值**。
- **共享池**：该雇员配置下的所有并发分身共同扣减全局的 Stamina 电池。当全局 Stamina 耗尽，该雇员在所有项目工位上的分身将同步倒地睡觉。

---

## 3. 推理投入度 (Effort Level) 与状态映射

推理投入度直接影响任务的成功率与算力消耗。为了保持大屏纯粹的监视台定位，其控制与显示进行了解耦：

- **工作台控制端 (Workbench Slider)**：在主会话聊天区域（Workbench UI），用户可以拉动滑块调节当前的投入度档位：
  - `Low (省电模式)`：快速推理，Stamina 消耗少，Token 成本极低，但技术力降级，适合简单任务。
  - `Middle (默认推理)`：标准智能体模式。
  - `High (极致/Thinking 模式)`：开启长推理（Thinking tokens），Stamina 扣减速度和 API 成本极高，但技术力极高，适合攻坚。
- **大屏显示端 (Dashboard Visualizer)**：大屏端仅负责读取 `sessions.effort_level` 字段，并在工位卡片边缘以不同的 LED 旋转动效和颜色（Low = 绿，Middle = 蓝，High = 橙/金）以及小人敲键盘的动画帧速度来进行直观渲染。

---

## 4. 公开交付 (Building in Public)

项目开发完成后（所有 tasks 均 completed），大屏端的金色火箭就绪。点击发射将启动“公开交付”系统：

- **交付评级**：根据项目规模、用到的 Skill 迭代版本、任务报错/Bug 比例，生成本次交付的评定指数。
- **社区声誉指标**：
  - **Views (观看量)** & **Stars (收藏点赞)**：模拟发布到 Github/小红书后获得的围观和点赞数。
  - **Feedback (内测反馈)**：生成趣味性的 8-bit 社区评论。
- **成长反馈**：声誉指标将推动公司 **Reputation (声望/粉丝数)** 的提升。声望提高可以用来在人才市场授权导入更高版本、更复杂的 CLI 工具及 Skills 技能卡。

---

## 5. 异星工厂式成就 (Factorio-style Achievements)

在局外设置成就大屏，鼓励玩家实现“无人值守的自动化任务流”：
- **最大吞吐量**：累计自动交付的任务总量。
- **自动化奇迹**：在 0 人工干预（时间线上没有任何 User comment 回复）下，通过 DAG 依赖链自动流转完工的连续子任务个数。
- **并发负载上限**：在精力值额度内同时运转的最大 Session 数量。

---

## 6. 数据表设计 (SQLite Delta Schema)

为实现此系统，需在 `meta.db` 元数据库中新增及修改以下表结构：

```sql
-- 1. sessions 表加字段：推理投入度
ALTER TABLE sessions ADD COLUMN effort_level TEXT NOT NULL DEFAULT 'middle';

-- 2. 雇员配置主表
CREATE TABLE employees (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,              -- 雇员花名
    kind           TEXT NOT NULL DEFAULT 'basic', -- basic | specialist
    model_type     TEXT NOT NULL,              -- 底座模型 (e.g. claude-3-5-sonnet)
    framework      TEXT NOT NULL,              -- 运行框架 (e.g. acpx)
    skills_json    TEXT NOT NULL,              -- 装配的 Skills 及版本列表
    system_prompt  TEXT,                       -- 专才员工的 System Prompt 台词模板
    persona        TEXT DEFAULT 'normal',      -- 吐槽个性 ID
    rating_good    INTEGER DEFAULT 0,          -- 主观评价-超乎期望累计
    rating_normal  INTEGER DEFAULT 0,          -- 主观评价-正常累计
    rating_poor    INTEGER DEFAULT 0,          -- 主观评价-有待提升累计
    stamina        INTEGER DEFAULT 100,        -- 全局精力条当前值
    created_at     TEXT NOT NULL
);

-- 3. 雇员战绩日志表（记录履历与评分）
CREATE TABLE employee_history (
    employee_id    TEXT NOT NULL REFERENCES employees(id),
    task_id        TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    performance    TEXT,                       -- excellent | normal | poor
    completed_at   TEXT NOT NULL,
    PRIMARY KEY (employee_id, task_id)
);

-- 4. 交付公测/上线指标表
CREATE TABLE project_public_metrics (
    project_id     TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    views          INTEGER DEFAULT 0,          -- 浏览量
    stars          INTEGER DEFAULT 0,          -- 点赞/收藏
    phase          TEXT NOT NULL DEFAULT 'beta', -- alpha | beta | stable
    last_shipped   TEXT NOT NULL
);
```
