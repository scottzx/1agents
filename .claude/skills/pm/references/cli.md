# `/Users/scott/Documents/01-开发项目/1agents/1agents_app/build/1agents project-items` — 完整参考

命令行操作项目看板，走的是和 app 内 MCP 工具**同一套后端接口**，所以业务逻辑（编号、归口、issueState 关闭、auto-close、links 解析）完全一致。**用 Bash 调用，在项目目录下运行**（按 cwd 解析项目）。

## 通用

- **`<id>` 参数接受条目 UUID 或 `list` 显示的短号 `#N`**（如 `` `#3` ``；`get`/`graph`/`update`/`close`/`reopen` 都支持）。写 `#N` 时注意：Markdown 里裸写 `#3` 会被当标题，引用条目/给用户看时用反引号包成 `` `#3` ``。
- `--project <id|name|path>`：覆盖 cwd 解析的项目（默认自动按当前目录逐级上找）。
- `--json`：机器可读输出。读命令输出原始 JSON；写命令的 create 也可加。
- 项目解析优先级：`ONEAGENTS_WORKSPACE_ID` 环境变量 → `--project` → 当前目录逐级上找匹配。
- 隔离：跨项目的 id 在 get/update/close 时会被 `not in project` 拒绝。

## 读

| 命令 | 说明 |
|---|---|
| `list [--status S] [--type T] [--json]` | 列出本项目条目。文本输出：`#编号 类型 状态 标题` |
| `get <id> [--json]` | 单条完整详情（含 description / acceptance / dependsOn / issueState 等） |
| `graph <id> [--json]` | 该条的引用关系图：`outgoing`（它引用的）+ `incoming`（引用它的，即归口回链） |

`--status`：pending / queued / running / completed / failed / cancelled / blocked / not_ready …
`--type`：task / requirement / bug / discussion

## 写

### create（建条目）
```
create --title T [flags] [--json '<CreateArgs>']
```
便捷 flag：`--title --type --priority --milestone --assignee --acceptance --description`

- `--type`：task（默认）| requirement | bug。（discussion 用单独的 `discussion` 动词。）
- `--priority`：urgent | high | medium | low。
- `--assignee`：执行 agent 类型（如 claudecode / codex），或 `user`（个人待办，不派单，落日历）。留空默认 claudecode。
- `--acceptance`：**可执行任务必填**，否则挂为 not_ready 不进队列。

嵌套字段用 `--json`（schema 与字段名如下），可与便捷 flag 混用（flag 覆盖同名）：
```json
{
  "title": "...", "description": "...", "acceptanceCriteria": "...",
  "type": "task", "priority": "high", "milestone": "v0.1", "assignee": "claudecode",
  "dependsOn": ["<id1>", "<id2>"],
  "links": [{"target": "<需求id>", "rel": "relates"}],
  "verifier": "claudecode", "verifierCount": 2, "verifyPassThreshold": 2,
  "dueAt": "2026-08-01T09:00:00+08:00",
  "recurrence": {"freq": "weekly", "daysOfWeek": [1], "at": "09:00"},
  "checklist": [{"text": "子步骤A"}, {"text": "子步骤B", "done": false}]
}
```
- **归口**：`links`（rel=relates）或在 description 写 `#需求编号` 二选一即可，二者等价。
- **个人待办 / 提醒**：`--assignee user` + `--json` 里 `dueAt`（截止 / 触发时间）、`recurrence`（重复）。不会派给 agent，落在用户日历上。
- `dependsOn`：先建被依赖项拿到 id，再挂后续项，表达执行顺序。

示例：
```bash
# 顶层需求
/Users/scott/Documents/01-开发项目/1agents/1agents_app/build/1agents project-items create --title "用户登录" --type requirement \
  --description "支持手机号+验证码登录"
# 归口到 #12 的可执行任务，挂依赖
/Users/scott/Documents/01-开发项目/1agents/1agents_app/build/1agents project-items create --title "登录后端接口" --type task --milestone "v0.1" \
  --description "实现 #12：POST /login，签发 JWT" \
  --acceptance "正确凭据返回 token；错误返回 401" \
  --json '{"dependsOn":["<前置任务id>"]}'
```

### discussion（纯讨论）
```
discussion --title T [--description D]
```
建 type=discussion 的条目，落讨论区，不会被调度。

### update（改字段）
```
update <id> [flags] [--json '<patch>']
```
便捷 flag：`--status --issue-state --priority --milestone --type --assignee --acceptance --description`
- `--status`：只能 `completed` 或 `cancelled`（其余运行态由调度器拥有）。**任务收尾用它**。
- `--issue-state`：`open` | `closed`。**需求 / 缺陷收尾用它**（或用下面的 `close`）。
- `--json '<patch>'`：可传 update 白名单内任意字段（含上面这些 + github* 同步锚点）。

### close / reopen（需求/缺陷收尾语法糖）
```
close <id>       # = update <id> --issue-state closed
reopen <id>      # = update <id> --issue-state open
```

## 里程碑

| 命令 | 说明 |
|---|---|
| `milestones list` | 列出本项目里程碑及进度 |
| `milestones create --name N [--description D] [--target-date RFC3339] [--predecessor ID]` | 建阶段。`--predecessor` 置于某阶段之后；共享同一前驱的阶段在路线图上并行 |
| `milestones update <id> [--name] [--description] [--target-date] [--predecessor]` | 改里程碑（改名会级联到归属它的所有条目） |

`--target-date` 用 RFC3339，例如 `2026-08-01T00:00:00Z`。

## status vs issueState（再强调一次）

- `status` = **任务**的生命周期，可执行任务用；手动只可置 completed / cancelled。
- `issueState`（open/closed）= **需求 / 缺陷**的开闭，是它们的「完成」维度。
- 两者解耦：一条需求可以 issueState=closed（收尾了）但其下任务各有各的 status。给需求 / 缺陷收尾**用 `close`，不要用 `--status`**。
