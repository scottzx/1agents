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

`AgentTeams`、`gbrain`、`grok-bot-0.18-reconstructed`、`grok-build`、`gstack`、
`myContext`、`superpowers`、`transcribe.cpp`

全部是纯查阅参考，**没有任何一个被代码、配置或构建引用**。

gstack 和 superpowers 曾经有引用（`.claude/settings.json` 的 3 个 hook、
`backend/cmd/absorb-upstream`），都已删除。当年从它们吸收来的 skills/roles 已经
定稿在 `backend/internal/agent/{skills,roles}/` 并 embed 进二进制，出处记在各文件
frontmatter 里，不再需要回读上游。

## 注意

整个目录不被跟踪，换机器时不会自动出现，需要自己 clone。
删掉或不 clone 都不会影响构建和运行。
