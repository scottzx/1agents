# PM CLI 参考：项目项与执行编排

```bash
BIN=/Users/scott/Documents/01-开发项目/1agents/1agents_app/build/1agents
```

所有命令在本项目目录运行。`project-items` 会从 cwd 解析项目；`execution` 是连接运行中 daemon 的 API 客户端，默认访问 `http://127.0.0.1:38080`，可用 `ONEAGENTS_URL` 覆盖。

`1agents task` 已退休。它既不能作为看板命令，也不能作为 Job、调度或执行命令使用。

## ProjectItem：工作定义

先运行 `$BIN project-items list --json`。顶层 `workspaceId` 是此项目的执行 CLI 所需 `projectId`；不要从标题、路径或其他项目猜测它。

| 命令 | 说明 |
| --- | --- |
| `project-items list [--status S] [--type T] [--json]` | 列出当前项目；JSON 含 `workspaceId` |
| `project-items get <id> [--json]` | 一条工作项详情 |
| `project-items graph <id> [--json]` | links 的入/出关系 |
| `project-items create --title T [flags] [--json '<CreateArgs>']` | 创建 requirement / bug / task |
| `project-items discussion --title T --description D` | 创建纯讨论 |
| `project-items update <id> --status completed\|cancelled` | 完成人工确认或已验收的 task |
| `project-items close\|reopen <id>` | 关闭/重开 requirement 或 bug |

创建 task 的最小可执行定义：

```bash
$BIN project-items create --title "登录后端接口" --type task \
  --description "实现 #12：POST /login，签发 JWT" \
  --acceptance "正确凭据返回 token；错误凭据返回 401" \
  --json '{"dependsOn":["<前置任务-id>"],"links":[{"target":"<requirement-id>","rel":"relates"}]}'
```

- `task` 必须有 `description` 和可检验的 `acceptance`；`links` 或 description 内的 `#编号` 必须归口到 requirement / bug。
- `dependsOn` 只表示工作项依赖，不创建 Trigger，也不会运行 Job。
- 嵌套字段使用 `--json`：`links`、`dependsOn`、`checklist`。不要用项目项的 `assignee`、`recurrence` 或 `status=running` 取代 ExecutionJob/Trigger。
- ProjectItem `status` 与 requirement/bug 的 `issueState` 不同：前者仅在实际完成/取消时更新；后者使用 `close` / `reopen`。

功能蓝图仅当 `.1agents/project_config.json` 的 `featureCatalogEnabled` 为 `true` 时可写：

| 命令 | 说明 |
| --- | --- |
| `$BIN feature-catalog list` | 读取节点树及 source/delivery 关系 |
| `$BIN feature-catalog batch --json '<operations>'` | 事务化 create/update/move/link/unlink |
| `$BIN project-items milestones list` | 读取版本 |
| `$BIN project-items milestones create --bump patch\|minor\|major` | 创建下一个 SemVer 版本 |

已有蓝图先展示拟议差异并取得明确确认；禁止未经确认的整体覆盖、重建、删除、重命名或移动。

## ExecutionJob：执行定义

仅为已创建的 `task` 创建 Job。Job 绑定一个 `workItemId`，但 Job 创建、运行、暂停或归档不会自动改变 ProjectItem 状态。

```bash
PROJECT_ID=<project-items-list-json 中的 workspaceId>

# Agent：明确指定 AgentProfile，Profile 承载运行时/供应商/模型配置
$BIN execution create --project "$PROJECT_ID" --item <task-id> \
  --executor agent --profile <profile-id> --cwd "$PWD" --timeout 30 --max-attempts 1

# 仅兼容存量 agent 配置时使用；不可同时作为新默认方案
$BIN execution create --project "$PROJECT_ID" --item <task-id> \
  --executor agent --legacy-agent codex

# Function / Human
$BIN execution create --project "$PROJECT_ID" --item <task-id> --executor function --function <type>
$BIN execution create --project "$PROJECT_ID" --item <task-id> --executor human
```

- agent Job 优先显式传 `--profile`；没有用户指定或项目默认 Profile 时，先确认，勿擅自挑选模型。
- function Job 必须传 `--function`；human Job 不传 Profile/function。
- Profile 只传 ID，绝不写入或要求提供 API key、token、密码等凭据。
- `--cwd`、`--timeout`、`--max-attempts` 属于执行定义；只按用户需求填写，避免猜测。

## Trigger 和 TaskRun：时间与事实

```bash
# 立即创建一次 Run：仅在用户授权“现在运行”时使用
$BIN execution run <job-id>

# 查看 Job 与每次真实尝试（状态、输出、错误）
$BIN execution get <job-id>
$BIN execution runs <job-id>

# 自动调度：仅在用户明确给出时间/周期时配置
$BIN execution trigger <job-id> --kind at --spec '{"at":"2026-08-12T09:00:00+08:00"}'
$BIN execution trigger <job-id> --kind recurrence --spec '{"everyMinutes":60}' \
  --timezone Asia/Shanghai --misfire run_once --overlap forbid
$BIN execution trigger-delete <job-id>

# 生命周期
$BIN execution pause <job-id>
$BIN execution resume <job-id>
$BIN execution archive <job-id>
```

Trigger 是 Job 的调度定义；TaskRun 是单次事实记录。不要把 Trigger 的存在、Job 的创建或 Run 的发起描述成 task 已完成。agent/function task 只有成功 Run 且验收满足后才可完成；失败时查看 Run，再决定重试、修订、暂停或取消。human Job 以负责人确认作为完成依据。

`at` 的 `spec` 使用 RFC3339 `at` 字段；当前 recurrence 使用正整数 `everyMinutes`。`trigger` 可选 `--timezone`、`--misfire skip|run_once`、`--overlap forbid|allow`。不要沿用旧任务 CLI 的 `--recur` 等参数。
