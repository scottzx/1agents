# 1Agents Page Patterns

本规范定义 1Agents 各类页面如何应用 Agentic Workbench Design Standard。页面设计应遵守三层信息架构：主线内容、过程与证据、系统元信息。

## 1. Chat Page

参考：`chatui-agentic-design-rules.md`

### 主线内容

- User query
- Assistant final answer

### 过程与证据

- thinking
- tool calls
- tool results
- permission request
- 中间助理过程说明

### 页面规则

- 一轮 turn 只外露最后一条正式回答。
- 过程内容合并进一个可展开过程块。
- User 与 Assistant 都支持 Conversation Markdown。
- 排队 user query 保持单行省略。
- 权限请求必须可发现。

## 2. Task Detail Page

### 主线内容

- 任务标题
- 当前状态
- 目标描述
- 验收标准
- owner / agent
- checklist

### 过程与证据

- AI 执行记录
- 评论与讨论
- 文件变更
- 命令记录
- 状态迁移历史

### 页面规则

- 任务目标和下一步动作优先。
- 活动流按摘要展示，细节折叠。
- checklist 使用稳定紧凑布局。
- 评论支持 Conversation Markdown。
- 文件 diff 使用可展开证据区，不直接淹没任务正文。

### 禁止

- 把任务详情做成营销式大卡。
- 一屏铺满统计装饰。
- 把活动日志默认全部展开。

## 3. Project Overview Page

### 主线内容

- 项目健康状态
- 当前重点任务
- 风险与阻塞
- 最近进展

### 过程与证据

- 最近 agent sessions
- 关键任务执行记录
- 数据源同步状态
- 代码/部署活动

### 页面规则

- 使用 Operational Dashboard Layout。
- 指标要服务下一步判断。
- 风险和阻塞优先于装饰性统计。
- 最近活动用 Status Row 或 Process Block。
- 卡片数量要克制，避免 dashboard 噪音。

## 4. Task List / Session List Page

### 主线内容

- 可扫描对象列表
- 筛选与排序
- 当前选中对象详情

### 过程与证据

- 最近状态变化
- 最近执行结果
- 运行中/等待权限/错误状态

### 页面规则

- 使用 List-Detail Layout。
- 列表行高度稳定。
- 每行只展示一个主状态。
- 次要 meta 弱化。
- 批量操作和低频操作放 toolbar 或 menu。

### 禁止

- 高频列表使用大卡片墙。
- 每行塞入多个强按钮。
- 状态色大面积铺底。

## 5. Data Sources Page

### 主线内容

- 数据源连接状态
- 授权状态
- 同步范围
- 最近同步结果

### 过程与证据

- 同步日志
- 失败原因
- 权限记录
- 推送/订阅记录

### 页面规则

- 配置使用 Control Panel Layout。
- 同步历史使用 Process Block。
- 授权状态必须清楚。
- 失败状态给出 retry 或修复建议。
- 敏感权限变更需要 Permission Decision。

## 6. Agent / Assistant Management Page

### 主线内容

- Agent 名称
- 类型/来源
- 能力描述
- 当前配置
- 可用状态

### 过程与证据

- 最近会话
- 工具权限
- 运行记录
- 安装/更新记录

### 页面规则

- 使用 Registry + Detail 模式。
- Agent 卡片简洁，不社交化。
- 能力描述短，详细说明可展开。
- 工具权限用结构化列表。
- 运行记录使用 Status Row / Process Block。

## 7. File / Diff Page

### 主线内容

- 文件内容
- 当前 diff
- 路径与分支上下文

### 过程与证据

- 谁修改了文件
- 相关工具调用
- 命令输出
- commit / patch 记录

### 页面规则

- 文件正文使用 Document Markdown 或代码查看器。
- 路径使用 mono。
- diff 语义色要稳定。
- 大文件/大 diff 必须可滚动、可折叠。

## 8. Git / Deployment / QA Page

### 主线内容

- 当前结果
- 是否可继续
- 失败项和阻塞项

### 过程与证据

- 命令记录
- 检查结果
- 测试输出
- 部署日志

### 页面规则

- 使用 Process Block 展示 pipeline。
- 成功步骤默认折叠。
- 失败步骤展开或突出。
- 日志使用 Terminal Lite Block。
- 最终结果必须比日志更醒目。

## 9. Settings Page

### 主线内容

- 当前配置值
- 可修改控件
- 连接状态

### 过程与证据

- 检测结果
- 保存结果
- 权限说明
- 错误诊断

### 页面规则

- 使用 Control Panel Layout。
- 每个设置组有短标题和弱说明。
- 危险操作独立分区。
- 保存/测试/重连操作反馈明确。
- 不用大面积提示框。

## 10. Dashboard Page

### 主线内容

- 当前工作健康度
- 需要用户关注的事项
- 正在运行的 agent / task
- 风险和阻塞

### 过程与证据

- 最近运行记录
- 最近错误
- 最近完成项
- 系统连接状态

### 页面规则

- Dashboard 是工作台，不是营销页。
- 用紧凑指标 + 状态列表。
- 每个指标必须可解释、可点击或可行动。
- 不堆砌彩色统计卡。
- 最近活动用低噪音列表。

## 11. Mobile Page Rules

移动端优先保留主线内容。

规则：

- L1 优先显示。
- L2 过程进入折叠区、底部 sheet 或详情页。
- L3 meta 默认隐藏。
- 按钮尺寸适合触控。
- 列表行保持稳定高度。
- 不把桌面双栏硬塞到移动端。

## 12. 全页面验收清单

每个页面交付前检查：

- 是否明确区分 L1/L2/L3。
- L1 是否默认可见。
- L2 是否可审计但不扰乱主线。
- L3 是否弱化。
- 页面是否使用合适布局模式。
- 状态色是否只表达状态。
- 是否避免大面积装饰、渐变、营销式 hero。
- 列表行、工具行、按钮尺寸是否稳定。
- 关键权限/危险操作是否显式确认。
- 历史记录是否与实时状态结构一致。
