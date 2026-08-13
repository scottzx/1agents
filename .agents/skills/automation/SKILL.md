---
name: automation
description: 创建、调度、立即运行并取证 1agents 自动任务（轻量 Function→ACP 配方）。规划/拆任务走 /pm，本技能只做执行层。Triggers: 自动任务、Automation、定时任务、调度、立即执行、Function preamble、core.script、轻量 n8n、execution job、Run、Trigger。Use when the user runs /automation.
---

# 自动任务执行调度

你只负责**怎么跑、何时跑、跑得怎样**。不要拆需求、不要建蓝图、不要改里程碑。那些交给 `/pm`。

```bash
BIN=/Users/scott/Documents/01-开发项目/1agents/1agents_app/build/1agents
```

命令在目标项目目录运行。默认打运行中的 daemon：`http://127.0.0.1:38080`，用 `ONEAGENTS_URL` 覆盖。开工先确认 daemon 活着：

```bash
$BIN automation help
```

## 三种合法配方

| 形态 | 怎么建 |
| --- | --- |
| 只 ACP | 默认。`create` 不带 `--preamble` |
| Function → ACP | `--preamble` 或 `--script relpath`。脚本在 `--cwd` 里跑，stdout 必须是 JSON，失败则 ACP 不启动 |
| 只 Function | 不要用本命令。固化脚本仍走 `1agents execution create --executor function` |

禁止：ACP 后再接 Function；把 n8n 节点图画进 Job。多节点写进那一个 Python 脚本。

一条配方 = 一条 `task` + 一条 Job，`businessRef=automation:<itemId>`。不要拆成两条 Job。

## 创建

```bash
# 只 ACP，挂到当前目录对应项目
$BIN automation create --title "未读邮件摘要" \
  --instructions "阅读未读邮件，只根据事实写摘要，不要编造。" \
  --profile <profile-id> --cwd "$PWD"

# Function → ACP，每 60 分钟
$BIN automation create --title "群消息摘要" \
  --instructions "根据 function_context 写今日要点。" \
  --profile <profile-id> --cwd "$PWD" \
  --script digest.py --every-minutes 60

# 指定时间跑一次
$BIN automation create --title "发布前检查" \
  --instructions "对照验收跑测试并汇报失败项。" \
  --profile <profile-id> --at "2026-08-14T09:00:00+08:00"

# 建完立刻跑一次（仍要用户明确说“现在跑”）
$BIN automation create --title "…" --instructions "…" --profile <id> --run --json
```

规则：

- `--title` 和 `--instructions` 必填。`--project` 可省略，按 cwd 匹配项目。
- agent 配方必须有 `--profile`，或项目/系统已有默认 Profile。不要猜模型，不要向用户要 API key。
- `--script` 是相对 `--cwd` 的路径，默认 `automation.py`。禁止绝对路径和 `..`。
- `--every-minutes` 和 `--at` 互斥。`--at` 必须 RFC3339。
- `--run` 只在用户明确要求立即执行时加。创建默认不跑。
- 创建后用 `--json` 或 `list` / `get` 回读，向用户报告 `itemId`、`jobId`、Trigger、是否已请求 Run。

## 查询与控制

```bash
$BIN automation list [--project <id|name|path>] [--json]
$BIN automation get <job-id>
$BIN automation runs <job-id>
$BIN automation run <job-id>          # 仅用户要求立即执行
$BIN automation pause <job-id>
$BIN automation resume <job-id>
```

`list` 只显示 `automation:` 配方。看板里给普通 task 配的 Job 不在这里。

完成证据是成功的 `TaskRun`，不是 Job 已创建，也不是 Trigger 已设置。失败时读 `runs`，再决定重试、改 Instructions/脚本、暂停。

## 和 PM 的边界

| 谁 | 做什么 |
| --- | --- |
| `/pm` | 需求/缺陷/任务、蓝图、依赖、验收、里程碑 |
| `/automation` | 配方创建、Profile/PWD/preamble、Trigger、立即 Run、Run 取证 |

用户只说“做这个功能”→ `/pm`。用户说“做成定时自动跑 / 先用脚本取数再让 ACP 写 / 现在跑这条配方”→ 本技能。

既有看板 task 要挂执行时：先确认该 task 已存在，再用本技能的 `create`（会新建一条配方 task）。不要把执行字段写回 ProjectItem 的 `assignee` / `recurrence` / `status=running`。不要调用已下线的 `1agents task`。
