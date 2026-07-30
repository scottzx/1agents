# HarnessKit 全量切换

1agents 使用 `modules/HarnessKit` 中的受控 Fork 作为唯一扩展管理运行时。
Skills-manager 不再参与启动、API、前端、npm 分发或桌面打包。

## 运行时边界

- 1agents 启动受监督的 `hk serve` 子进程。
- 子进程只监听随机的 loopback 端口，并使用每次启动新生成的 bearer token。
- 浏览器只访问 `/api/harnesskit/*`。1agents 在服务端注入 token，并用显式路由白名单拒绝未审计能力。
- HarnessKit 数据固定在 `~/.1agents/harnesskit`（可通过启动参数覆盖）。
- Agent ID 使用 `config/agent-extension-map.json` 统一映射；未验证的 Agent 明确标记为不支持。

## 一次性数据迁移

正式版本提供以下命令：

```bash
1agents migrate harnesskit --plan
1agents migrate harnesskit --apply
1agents migrate harnesskit --clean-start
1agents migrate harnesskit --data-rollback <backup-id>
```

`--plan` 只读扫描旧数据并输出迁移/冲突/降级清单。`--apply` 必须先：

1. 获取独占迁移锁；
2. 备份旧数据和所有将改写的 Agent 路径；
3. 把指向旧共享母体的 symlink 物化成普通文件或目录；
4. 保留 Skills、Subagent、Slash Command 与 MCP 的现有 Agent 落盘状态；
5. 通过 HarnessKit 的公开 CLI/API 触发扫描，不直接写 `metadata.db`；
6. 写入可 fsync 的操作日志、校验摘要和损失报告。

旧版的历史、来源和分支谱系如果没有 HarnessKit 等价字段，会原样保存在备份的
`legacy-metadata` 中，并在报告里标为 `preserved-not-imported`，不得静默丢弃。
冲突默认不覆盖目标文件；只有内容摘要相同的项目才按幂等成功处理。

`--clean-start` 不读取旧数据，只初始化新的 HarnessKit 数据目录。`--data-rollback`
仅恢复数据与被迁移改写的路径，不回滚应用二进制。

默认路径为：

- Skills-manager：`~/.1agents/skill-manager`
- HarnessKit：`~/.1agents/harnesskit`
- 迁移备份：`~/.1agents/migrations`

需要隔离演练时，可以分别使用 `--home`、`--oneagents-home`、`--legacy-dir`、
`--harnesskit-data-dir`、`--backup-root` 和 `--hk-bin` 覆盖。三个数据目录必须是
互不包含的绝对路径。命令返回可机器读取的 JSON；MCP 环境变量和 header 值不会进入
plan 或损失报告。

### 迁移语义

- 指向旧中央仓库的 Agent-native symlink 会原位物化，并在备份中保存原 symlink。
- 没有 active native binding 的 Skill 会复制到 `~/.agents/skills/<name>`；
  Subagent 会复制到 `~/.claude/agents/<name>.md`。
- 首选目标已存在且内容摘要相同，视为幂等成功；内容不同则进入 conflict，`--apply`
  不会改写任何目标。
- 已经原生落盘的 Skill、Subagent、Slash Command 和 MCP 配置保持原位。
- Slash Command canonical TOML、MCP canonical manifest、历史版本、pending conflict
  以及来源/分支谱系没有安全的一一映射，因此进入受限权限备份的 `legacy-metadata/`
  并在损失报告中明确标为 `preserved-not-imported`。

`--apply` 可在中断后用同一条命令继续：它会复用原 operation journal 和备份，不创建
第二份迁移。备份含 `backup-checksums.json`，恢复前会先校验。数据回滚只恢复原 symlink
并删除由迁移新建、且摘要没有漂移的 orphan 导入；用户修改过的路径会保留并报告
conflict。

## 受控 Fork 与许可证

- Fork 基线和同步规则见 `modules/HarnessKit/UPSTREAM.md`。
- 素材替换策略见 `modules/HarnessKit/ASSET-LICENSES.md`。
- `make harnesskit-compliance` 会扫描源码和构建产物，阻止受保护的上游品牌素材进入发布包。

## 发布门禁

完整切换必须同时通过：

```bash
make harnesskit
make harnesskit-compliance
go test ./...
npm test --prefix modules/HarnessKit
cargo test --manifest-path modules/HarnessKit/Cargo.toml
```

并确认发布包中存在 `bin/hk`（Windows 为 `bin/hk.exe`），且不存在
`skill-manager`、`1skills`、`@1agents/skills` 或 `skills-embed.js`。
