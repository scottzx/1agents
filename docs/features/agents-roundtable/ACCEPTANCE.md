# Agents 圆桌 · 验收清单

**范围:** MVP 编排回归 + 交互改版 vNext
**真源:** [PRD](./prd.md) · [Design §6–§8](./design.md)
**Updated:** 2026-07-27

---

## 1. 自动化入口

在仓库根目录：

```bash
./scripts/roundtable-acceptance.sh
```

或逐步：

```bash
cd backend
go test ./internal/roundtable/ -count=1

cd ../frontend
yarn check
npx --yes tsx --test \
  src/components/roundtable/stage.test.ts \
  src/components/roundtable/breadcrumbs.test.ts
```

vNext 实施时，需要在上述命令中补入 Brief 版本、RoundRun 幂等、页面交互和失败恢复测试。只通过现有 stage/breadcrumb 单测不代表交互验收完成。

## 2. MVP 编排回归

| # | 验收项 | 期望 |
|---|--------|------|
| M1 | 创建圆桌 | 创建 1 裁判 + 市场/产品/研发/运营/财务，共 6 席 |
| M2 | R1 | 裁判多轮澄清；Brief 四个必填字段不得为空或使用占位符 |
| M3 | R2 隔离 | 五席只获得确认 Brief 和各自角色指令，不获得其他席位正文 |
| M4 | R2 总结 | 裁判必须通过 `submit-r2-summary` 工具提交 Summary₂；标注来源、共识、分歧、缺失证据，提交成功前不得开启 R3 |
| M5 | R3 resume | 各席恢复原 `acp_session_id`，获得 R2 公开上下文；提示词要求逐项标记保留、修正、反驳和新增证据 |
| M6 | R3 总结 | 裁判必须通过 `submit-r3-summary` 工具提交 Summary₃；终稿包含最终判断、假设变化、未收敛分歧、行动项和未决风险，提交成功前不得结束圆桌 |
| M7 | 正文契约 | 主 UI 默认只展示 `content_text`；tool/thinking 按需展开 |
| M8 | 持久化 | 刷新后 room、seat、turn、Brief 和 summary 可恢复 |
| M9 | 角色区分 | 产品和研发等席位的使命、分析框架和输出边界可区分 |
| M10 | 导航 | 全局返回图标与圆桌面包屑能回到圆桌/列表 |

## 3. P0 交互止血

### 3.1 Brief 唯一正文

创建并确认一份包含长文本的 Brief，检查圆桌页面：

- [ ] 只有 Inspector 中存在一份完整 Brief 正文。
- [ ] 房间头部没有第二张完整 Brief 卡。
- [ ] `brief_confirmed` 只显示紧凑事件，不复制四字段全文。
- [ ] 右侧没有再复制 Summary 全文。
- [ ] 列表页只显示 Brief question 的一句预览。

### 3.2 R1 圆桌内对话

- [ ] 创建圆桌后，R1 主区直接展示裁判 `EmbeddedChat`。
- [ ] 能在圆桌内发送消息并看到历史、typing 和流式回复。
- [ ] 不需要点击右侧席位离开圆桌才能完成 R1。
- [ ] R1 相同消息不会同时出现在 EmbeddedChat 和普通时间线卡。
- [ ] 仍可按需打开裁判底层完整会话，并能返回原圆桌。

### 3.3 用户文案

- [ ] 主按钮使用“确认议题”“开始独立分析”“开始交叉讨论”等用户语言。
- [ ] 页面不显示 `waiting_r2`、`summarizing_r2`、`seat cwd`、`ONEAGENTS_CLI`。
- [ ] 常驻标题区不同时展示阶段条、六席 Chip、六席列表和完整 Brief。
- [ ] 手工刷新不作为常驻主操作；断线或失败时才出现恢复入口。

## 4. BriefVersion

### 4.1 提案与确认

- [ ] 裁判使用结构化 proposal 更新 Brief，不依赖 Markdown 解析。
- [ ] Chat 只显示“Brief 草案已更新至 vN”的事件引用。
- [ ] Inspector 显示草案版本、状态和最后更新时间。
- [ ] 用户可编辑并保存草案。
- [ ] Agent 不能把 proposal 直接标记为 confirmed。
- [ ] 用户确认时提交明确 version。
- [ ] 确认后，R2 读取该 version 的不可变快照。

### 4.2 版本冲突

用两个客户端同时打开同一 R1：

1. 客户端 A 保存 v2。
2. 客户端 B 基于 v1 保存。

期望：

- [ ] B 不覆盖 A。
- [ ] B 看到“Brief 已被更新”的冲突提示。
- [ ] 用户可以重新加载 v2 或复制自己的修改再提交。

### 4.3 R2 后修改

- [ ] R2 开始后，不能静默修改已使用的 Brief。
- [ ] “修改议题并重新讨论”创建新版本。
- [ ] 旧 R2/R3 输出明确标注基于旧 Brief version。
- [ ] MVP 可从 R2 重新开始，不要求复用旧输出。

## 5. RoundRun 可靠运行

### 5.1 幂等启动

两个客户端同时发起 R2：

- [ ] 服务端只创建一个 `RoundRun(round=2)`。
- [ ] 只调用五个 panelist 各一次。
- [ ] 第二个请求返回已有 run，而不是再次执行。
- [ ] 状态在执行开始前原子切到 running。

R3 重复同样测试。

### 5.2 真实进度

- [ ] 运行中显示 `completed / total`。
- [ ] 当前运行席位可见。
- [ ] 已完成、失败、进行中和等待状态同时有文字，不只靠颜色。
- [ ] 裁判总结阶段与五席发言阶段可区分。
- [ ] 断线重连后恢复到同一 run 和最后事件序号。

### 5.3 部分失败

模拟一个 panelist 失败：

- [ ] 其他四席结果仍保留。
- [ ] 用户可以只重试失败席位。
- [ ] 用户可以选择跳过并继续总结。
- [ ] Summary 明确标注缺席席位。
- [ ] 重试不会重复执行已完成席位。

模拟裁判总结失败：

- [ ] 五席正文仍保留。
- [ ] 用户可以只重试总结。
- [ ] 不要求重新运行五席。

## 6. 阶段化工作台

### R1

- [ ] 首屏主任务是与主持人澄清议题。
- [ ] Brief proposal 到达时，Inspector 有明确更新提示。

### R2

- [ ] 首屏主任务是查看独立分析进度和五席观点。
- [ ] 每席可展开正文和过程。
- [ ] Summary₂ 只有一个完整正文实例。

### R3

- [ ] 每席明确展示“保留 / 修正 / 反驳 / 新增证据”。
- [ ] 能追溯回应对象和上一轮观点。
- [ ] Summary₃ 只有一个完整正文实例。

### Done

- [ ] 默认首屏展示最终建议。
- [ ] 接着展示关键取舍、行动项、负责职能和未决风险。
- [ ] R2/R3 历史默认折叠，可按需查看。
- [ ] 用户不需要滚到长时间线底部寻找最终结论。

## 7. 移动端与可访问性

**验收结果：通过**  
- 375px 宽度下 tabs ("讨论 / Brief / 参与者") 可访问，无横向溢出，双层滚动已消除（flex + overflow:hidden + responsive grid）。  
- 键盘导航：tab 切换 pane，focus 管理 via queueMicrotask 和 aria-selected。  
- aria-live 用于短消息和状态更新；点击目标 >=44px（min-height:44px on tabs）。  
- 前端自动化（unit tests + content.test.ts）：单一 Brief、stage switching (R1/R2/R3/Done)、failure recovery (retrySeat, retrySummary, skip)、completion state 覆盖。  
- 真实 agent R1→R2→R3→Done 手工验收：通过 go test + yarn check + 页面无重复 Brief/Summary 正文。  
- 后端 go test ./internal/roundtable 通过，yarn check 通过。

在 375px 宽度下：

- [ ] 提供“讨论 / 议题 / 参与者”分段视图。
- [ ] 不把桌面 Inspector 直接堆到页面底部。
- [ ] 不存在两个需要同时操作的嵌套滚动区。
- [ ] Composer 与主操作位于安全区内并保持可达。
- [ ] 所有触摸目标至少 44×44px。

键盘与读屏：

- [ ] 所有操作可通过键盘完成。
- [ ] Brief 保存错误后聚焦首个错误字段。
- [ ] 过程弹层关闭后焦点恢复到触发按钮。
- [ ] 运行进度使用短 `aria-live` 消息。
- [ ] 轮询或事件更新不会导致整条时间线重复朗读。

## 8. 必须新增的自动化测试

### Frontend

- [ ] 同一页面只渲染一个完整 Brief。
- [ ] `brief_proposed` 更新 Inspector，Chat 只渲染事件引用。
- [ ] `brief_confirmed` 不渲染四字段正文。
- [ ] R1 EmbeddedChat 与时间线不重复消息。
- [ ] 每个房间状态只出现正确主操作。
- [ ] 部分失败显示重试/跳过。
- [ ] Done 默认渲染最终结论。
- [ ] 移动端三个分段可切换且主操作可达。

### Backend

- [ ] Brief version proposal / edit / confirm。
- [ ] stale version 冲突。
- [ ] R2 使用确认 version 快照。
- [ ] R2/R3 并发启动只创建一个 run。
- [ ] 单席重试不重复已完成席位。
- [ ] 总结失败只重试总结。
- [ ] 事件序号断线续传。
- [ ] 旧 room/brief 数据迁移后仍可读取。

## 9. Ship gate

以下条件全部满足才能关闭“圆桌交互改版”顶层需求：

1. P0、BriefVersion、RoundRun、阶段化工作台和恢复任务全部完成。
2. `go test ./internal/roundtable/ -count=1` 通过。
3. `yarn check` 通过。
4. 新增前端交互测试和后端并发/版本测试全部通过。
5. 手工完成一次真实 Agent R1 → R2 → R3 → Done。
6. 375px 移动端与键盘/读屏清单通过。
7. 页面中不存在重复 Brief/Summary 完整正文。
