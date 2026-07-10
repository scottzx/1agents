# 1Agents Component Patterns

本规范定义全 APP 可复用的核心组件模式。组件应服务于 Agentic Workbench 的三层信息架构：主线内容、过程与证据、系统元信息。

## 1. Process Block

用于承载智能体或系统执行过程。

适用场景：

- Chat 工具过程
- 任务执行日志
- 数据源同步步骤
- Git 操作记录
- 部署流程
- QA / canary / benchmark 运行记录

### 1.1 结构

```text
[caret] 摘要标题                         [状态]
  step / note / tool call / output / diff / permission
```

### 1.2 标题

标题必须是执行摘要，不应只写“详情”或“日志”。

推荐：

```text
读取 5 个文件，运行 8 条命令，使用 6 个工具
同步 12 个数据源，3 个失败
部署到 staging，执行 4 个检查
```

### 1.3 默认状态

- 成功完成：默认折叠。
- 正在运行：可以展开。
- 等待权限：必须可见。
- 错误：可展开，并突出错误摘要。

### 1.4 展开内容

允许包含：

- Process note
- Tool row
- Terminal output
- File diff
- Permission decision
- Error block

不应包含：

- 营销说明。
- 大面积装饰。
- 与本过程无关的对象卡片。

## 2. Status Row

用于列表和活动流中的单行状态对象。

适用场景：

- 会话列表
- 任务列表
- 数据源列表
- 设备连接
- 最近活动
- Agent 运行状态

### 2.1 结构

```text
[icon/status dot] title
                  summary/meta
                                      [status] [time] [action]
```

### 2.2 规则

- 行高稳定。
- 左侧识别对象，右侧展示状态。
- title 必须可扫描。
- summary 不超过一行，必要时省略。
- 操作按钮只放高频动作。

### 2.3 状态表达

优先级：

1. 小状态点
2. 短 badge
3. 短文本
4. 轻量图标

避免：

- 大色块。
- 多个强按钮。
- 大面积红/绿背景。

## 3. Inline Badge

用于行内元信息。

适用场景：

- 文件路径
- 工具名
- Agent 类型
- 模型
- 权限模式
- 语言
- 状态
- tag

### 3.1 样式

推荐：

```text
font-size: 10.5px-12px
border-radius: 4px-6px
padding: 1px-6px
border: subtle
background: very light
```

### 3.2 规则

- badge 不应看起来像按钮，除非真的可点击。
- 一行内多个 badge 要保持低对比度。
- 路径、工具、命令类 badge 使用 mono。

## 4. Terminal Lite Block

用于命令与日志输出。

适用场景：

- shell command
- stdout/stderr
- build logs
- sync logs
- agent runtime logs

### 4.1 结构

```text
$ command
output...
```

### 4.2 规则

- 使用 mono。
- 支持横向滚动。
- 支持复制。
- 长输出限制高度并滚动。
- error 输出可使用红色文本或边框，但不要整块强红。

### 4.3 风格

可以使用暗底终端块，也可以使用浅底日志块。一个页面内应统一，不要混用过多终端风格。

## 5. Permission Decision

用于 Human-in-the-loop 确认。

适用场景：

- 工具权限
- 文件覆盖
- 删除
- 外发消息
- 部署
- 访问敏感数据

### 5.1 必须展示

- AI 想做什么。
- 影响范围。
- 可选决策。
- 决策结果回执。

### 5.2 按钮语义

推荐顺序：

```text
总是拒绝 / 拒绝 / 允许 / 总是允许
```

危险操作可以把拒绝动作放得更容易选择，但不要让危险允许动作误触。

### 5.3 展示位置

权限请求必须出现在相关过程块内，且 pending 状态不能被隐藏到用户难以发现的位置。

## 6. Markdown Content

用于结构化文本。

适用场景：

- User query
- Assistant answer
- 任务描述
- 评论
- 文档片段

### 6.1 场景模式

使用两类样式：

- Document Markdown
- Conversation Markdown

### 6.2 规则

- 长链接必须断行。
- 代码块不能撑破容器。
- 表格可横向滚动。
- Mermaid 或图表渲染失败时保留源码 fallback。
- Conversation Markdown 标题要克制。

## 7. Detail Section

用于详情页中的信息分组。

适用场景：

- 任务详情
- 数据源配置
- Agent 配置
- 设置页

### 7.1 结构

```text
Section title
Description
[fields / controls / rows]
```

### 7.2 规则

- 分组标题短。
- 说明文字弱化。
- 控件对齐。
- 危险区独立。
- 不把每个 section 都做成浮动大卡片。

## 8. Action Toolbar

用于页面级或对象级操作。

### 8.1 规则

- 高频主操作最多一个。
- 低频操作进菜单。
- 图标按钮必须有 title/tooltip。
- 工具类按钮优先使用图标。
- 不用一排多个同权重文字按钮。

## 9. Empty State

用于空列表、无会话、无数据。

### 9.1 规则

- 说明当前为什么为空。
- 给出一个明确下一步。
- 不使用大插画或营销文案。
- 不在工作台页面使用 hero。

## 10. Error State

用于错误、失败、连接不可用。

### 10.1 规则

- 先说发生了什么。
- 再说用户能做什么。
- 技术细节可折叠。
- 可恢复错误给出 retry。
- 不用大面积红色背景。
