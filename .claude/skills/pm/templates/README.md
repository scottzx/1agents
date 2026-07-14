# 项目看板条目模板

每种条目（bug / requirement / task / discussion）的 description 骨架。建条目时挑一个对应模板，填好整段作为 `--description` 传给 `create`。

| 类型 | 文件 | 何时用 |
|---|---|---|
| bug | [bug.md](./bug.md) | 已观察到的错误现象，需要修 |
| requirement | [requirement.md](./requirement.md) | 有明确交付物的目标，先对齐再拆任务 |
| task | [task.md](./task.md) | 派给执行 agent 的最小可执行单元 |
| discussion | [discussion.md](./discussion.md) | 方向 / 概念性记录，可能不转化成交付物 |

## 用法

1. 打开对应类型的 `.md`。
2. **第一段（空骨架）**复制出去，按提示填空。
3. 把填好的整段作为 `--description '...'`（或 `--json` 里的 `description`）传给 `project-items create`。
4. 可执行 task 还必须填 `--acceptance`（验收标准），否则会被判为 `not_ready` 不进调度队列。

## 共用原则（与 SKILL.md 一致）

- **不要凭空补用户没给的细节**。不确定就问，或在 description 里明确标 `TBD`。
- **标题**：bug / requirement 一句话说症状或目标，**不写方案**。
- **验收标准必须可检验**——执行 agent 完成后能逐条对照。
- **归口**：可执行 task 在 description 里写 `#需求编号`（或用 `--json links`）追溯到源 requirement / bug。