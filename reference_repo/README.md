# reference_repo/

只读参考代码。这里的每个目录都是一个**独立的 git 仓库**，克隆下来只为查阅和借鉴，
**不属于本项目、不参与任何构建**。

与 `modules/` 的区别：

| | `modules/` | `reference_repo/` |
|---|---|---|
| 身份 | git submodule 或项目自有代码 | 各自独立的 git 仓库 |
| 父仓库跟踪 | 是（submodule 固定 commit） | 否（整个目录已 gitignore） |
| 参与构建 | 是（Makefile / Go / 脚本引用） | 否 |

## 当前内容

- `gstack` — 工程技能 / QA / 发布工作流。被 `.claude/settings.json` 的 3 个 hook
  和 `backend/cmd/absorb-upstream` 按路径读取（只读吸收，不编译进产物）。
- `superpowers` — 方法论 skills，同样被 `absorb-upstream` 只读吸收。
- `AgentTeams`、`gbrain`、`grok-build`、`grok-bot-0.18-reconstructed`、
  `myContext`、`transcribe.cpp` — 纯查阅参考。

## 注意

因为整个目录不被跟踪，换机器时不会自动出现，需要自己 clone。
`gstack` 和 `superpowers` 缺失会让上面提到的 hook 和 `absorb-upstream` 找不到路径。
