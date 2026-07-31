# 1agents

**1agents** 是一个开源、自托管的 AI-native 工作操作系统，也是一套面向多智能体协作的 **Agent Infra**。它把数据、需求、任务、上下文、执行者、验证和状态回写连接成一张持续运转的工作 Graph，让一个人也能组织一支持续工作的 AI 团队。

传统 AI 助手解决的是一次对话里的单点效率；1agents 解决的是一件工作从进入系统到被验证完成的完整链路。它把 Inbox、IM、数据源、项目、任务蓝图、Agent 会话、终端、文件、日程和扩展能力组织在一起，不是为了堆功能，而是为了让工作可以被接住、被理解、被拆解、被调度、被执行、被验收，并让结果成为下一轮行动的上下文。

**一万-Agent Infra** 描述的是基础设施目标，而不是同时运行数量的机械承诺：执行者可以增加、模型可以替换，但承载目标、状态、依赖和验收标准的工作 Graph 保持稳定。自循环智能体 Graph 是工作方式，“一人成军”是最终结果。

**简体中文** | [English](README_EN.md)

[![NPM](https://img.shields.io/badge/npm-@1agents%2Fcli-blue?logo=npm)](https://www.npmjs.com/org/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 产品方向：搭建自循环的智能体 Graph

1agents 的核心不是一个更大的 Chat 面板，也不只是让 Agent 在单条 Loop 中重复执行，而是用 Graph 表达真实工作：

- **节点（Node）**承载数据、需求、缺陷、任务、里程碑、会话、产物和验收记录；
- **状态（State）**描述工作处于待规划、执行中、阻塞、待验收、完成或返工；
- **边（Edge）**表达来源、归属、依赖、拆解、分发、验收和回写关系；
- **判断（Decision）**决定依赖是否满足、由谁执行、验收是否通过以及下一步走向；
- **回流（Feedback）**把执行结果和失败原因写回 Graph，更新计划并触发后续节点。

一条典型的业务 Graph：

```text
数据源
  → 数据治理与热更新
  → Inbox 分类与意图识别
  → 需求 / 反馈
  → 任务蓝图与里程碑
  → Schedule 调度
  → 1ACP / ACPX 分发
  → Agent / function / human 执行
  → Check 验收
  → 关闭或返工
  → 状态与结果回写任务蓝图
```

目标是让 AI 不只停留在“帮人做一步”，而是进入协作链路：工作进来后，系统根据节点、状态和判断持续推进，显式暴露卡点，把结果交给人确认，并把经验沉淀成可复用的能力。Loop 是 Graph 中的一条回路；Graph 才是完整工作的组织方式。

## 主要功能

### 智能体工作台

- Web 前端访问本机或远程 1agents 后端。
- 支持本机直连和 Relay 中转连接。
- 支持 Access Token 门禁和 Relay 配对门禁。
- 桌面端提供左侧工作区树、顶部上下文栏、主工作区画布和右侧内容面板。
- 移动端提供单列布局、触控导航和移动端 Chat/终端/设置入口。
- 支持多标签页、内置浏览器、文件预览、全局搜索和可拖拽分栏。
- 桌面端右侧副面板支持按会话持久化的多 Tab，可在任务、文件、浏览器、Git 与终端之间并行切换。

### 项目与工作区管理

- 创建、切换、删除和归档工作区。
- 一个工作区对应一个目录，也是终端和 Agent 执行任务的工作目录。
- 项目首页展示项目列表、归档项目和项目模板入口。
- 项目详情页包含会话、团队、任务、文件、渠道、动态、计划、资产和设置等标签。
- 项目配置支持指令、连接器、可见标签页和项目级设置。

### AI 会话

- 新建 Chat 会话时可选择工作区、Agent 类型、角色、权限模式和初始 Prompt。
- 支持普通会话和 PM 会话。
- 支持会话列表、会话重命名、删除、任务关联和项目上下文锁定。
- 消息区支持 Markdown、代码块、表格、链接、Mermaid 和任务引用。
- 展示 Agent 的工具调用、命令输出、文件 diff、计划清单和权限确认。
- 支持附件、语音输入和 Slash Command。

### Web 终端

- 基于 `ttyd`、`xterm.js` 和 WebSocket 提供浏览器终端。
- 使用 `tmux` 维护终端会话，刷新或断线后可恢复。
- 支持多终端窗口、指定工作区和 cwd 创建终端。
- 副面板终端 Tab 空闲后会卸载前端连接并回收对应 `tmux` window，避免长期遗留进程。
- 支持初始命令、tmux 鼠标模式、浅色/深色终端主题和移动端终端适配。

### PMO、任务与路线图

- 项目内置讨论、需求、任务、缺陷、AI 建议和里程碑管理。
- 功能蓝图展示模块、功能点、来源需求和交付任务，并与任务、版本和交付状态联动。
- 任务蓝图将需求、功能点、交付任务、依赖、执行计划和里程碑组织成可导航、可追踪、可回写的工作地图。
- 任务视图支持列表、看板、日历和里程碑路线图。
- 任务表格支持搜索、筛选、排序、分组、列显隐和内联编辑。
- 任务详情展示描述、验收标准、checklist、子任务、关系、评论、会话分支和属性。
- 支持 `#编号` 任务引用和任务深链。
- Inbox 内容可以分发到项目需求池。

### 文件、预览与 Git

- 文件浏览器支持工作区文件树、平铺视图和文件搜索。
- 支持文本、图片、Markdown、HTML、PDF 等内容预览。
- Chat 和 Git 相关流程支持 diff 展示。
- Git 面板围绕当前工作区展示版本控制相关操作。
- 内置浏览器可打开 URL、Dashboard 或外部页面。

### 数据源与数据治理

- 数据源模块支持外部账号接入、授权、配置和数据查看。
- 数据接入层按 source/account 管理原始数据。
- 数据治理层展示清洗、融合后的治理表。
- 支持 Python 脚本或确定性任务执行数据处理与热更新。
- 支持治理表详情、依赖关系和执行记录。

### 联系人、消息、Inbox 与日程

- 联系人模块可聚合外部渠道身份，支持联系人表格、搜索、分组、详情和编辑。
- 消息模块展示外部渠道消息和群聊内容。
- Inbox 支持手动捕获文本或 URL、信息分类与意图识别、已读/未读、归档，以及将需求或反馈分发到项目。
- 日程模块聚合个人提醒、项目任务和里程碑日期，支持月历视图和提醒创建。

### 助理、技能与 Agent 能力管理

- 助理页管理 Assistant 工作区。
- 助理详情包含会话、团队、任务、文件、渠道和设置。
- 团队与技能页用于管理项目/助理关联的技能配置。
- 集成受控 HarnessKit Fork，用统一 Extensions 面板管理 Skills、Subagents、Commands、MCP、Hooks、Plugins、CLI 和 Kits。
- Agent Catalog 展示可用 Agent 类型和安装命令。

### 插件式应用

- 已实现编译期 App Registry、Manifest 查询与启停，以及前端 Mount Point 渲染。
- 应用可以贡献一级页面、项目标签页和 Lens overlay；当前生产注册应用包括“圆桌讨论”。
- 完整外部应用 SDK、运行时热安装和第三方应用市场仍在演进。
- 发现中心展示应用、精选推荐和开源项目。

### cc-connect、Provider 与渠道

- 集成 `cc-connect`，用于连接 Feishu、Telegram、Discord、Slack、DingTalk 等消息平台。
- Provider 面板用于管理通信和 Agent 执行平台配置。
- 项目渠道页展示项目相关渠道状态。
- Relay 模式下嵌入模块请求会走统一中转。

### 系统设置与更新

- 支持语言、主题、初学者/高级模式设置。
- 支持 Relay 配对、设备状态、账户/订阅和本机模式设置。
- 支持前端和后端版本检查与更新。
- 支持清空浏览器缓存和重置本地 App 数据。

### Dashboard

- Dashboard 用于查看项目和团队运行状态。
- 包含项目列表、进度、最近活动、全局任务板和大屏视图。
- 可作为独立页面或内置浏览器标签打开。

---

## 架构概览

1agents 的四层共同维护同一张工作 Graph：

- **入口与数据层**：Inbox、IM 渠道、数据源、数据治理、手动任务、日程和应用业务对象，把外部信息转成可处理的工作信号。
- **组织与 Graph 层**：项目 / 工作区、ProjectItem、依赖、任务蓝图、计划、优先级、里程碑和验收标准，负责保存上下文、状态和关系。
- **调度与执行层**：Schedule 根据状态和依赖触发任务，通过 1ACP / ACPX 接入不同 Agent，并与确定性 function、human 在同一任务内核下分流执行。
- **验证与回写层**：Check、任务时间线、TaskRun 和 ProjectEvent 记录产出、验证、返工和状态变化，决定下一条边并把结果写回 Graph。

核心对象：

- **工作 Graph**：由节点、状态、边、判断和回流组成的工作事实与执行关系。
- **项目 / 工作区**：承载一个目录、一组会话、一组任务和项目配置。
- **ProjectItem / 工作项**：承载需求、缺陷、讨论或可执行任务；task 型工作项进入调度。
- **任务蓝图**：连接来源需求、功能点、交付任务、依赖、里程碑、目标版本和验收状态。
- **执行者**：包括 AI Agent、确定性程序和用户本人。
- **时间线与执行记录**：承载用户反馈、Agent 回复、会话分支、TaskRun、验证结果和状态变更。

主要代码结构：

```text
frontend/              Web 前端，包含工作台、Chat、任务、文件、数据源、设置等界面
backend/               1agents Go 后端服务
modules/ttyd/          Web 终端服务
modules/cc-connect/    消息平台和 Agent 桥接
modules/cc-switch-cli/ Agent provider / 模型配置切换 sidecar
modules/HarnessKit/    受控 Fork；Extensions 清单、审计、市场、Agent Adapter 和 Kits
modules/1acp/          Agent Client Protocol 适配、示例和一致性测试
modules/happy-cli/     Happy agent CLI 及本地 launcher 打包来源
modules/gstack/        项目内置工程技能、QA、发布和浏览器自动化工作流
modules/grok-build/    Grok 相关 agent、CLI 和构建组件
build/                 本地构建产物
docs/                  产品、设计和架构文档
```

仓库按“主产品 + 可替换执行组件 + 可分发模块”组织：前端和后端提供 1agents 主工作台；`ttyd` 提供终端能力；`cc-connect`、`cc-switch`、`happy`、`HarnessKit`、`1acp` 等子模块提供 Agent 接入、扩展管理、协议适配和 CLI sidecar；npm 分发层会把 core / web / cc-connect / cc-switch 等拆成平台包或功能包发布。

更多设计文档：

- [docs/README.md](docs/README.md) — 文档分类索引
- [docs/product/1Agents_项目介绍.md](docs/product/1Agents_项目介绍.md) — Agent Infra、工作 Graph 与任务蓝图的核心产品介绍
- [docs/product/名称定义表.md](docs/product/名称定义表.md) — **术语权威**（§0 冲突裁决 · ProjectItem / workforce）
- [docs/product/frontend-product-features.md](docs/product/frontend-product-features.md)
- [docs/design_rules/app-agentic-workbench-design-standard.md](docs/design_rules/app-agentic-workbench-design-standard.md)
- [docs/architecture/agentsOS-架构设计.md](docs/architecture/agentsOS-架构设计.md) — 工作 Graph、任务内核、项目外壳与当前 App Registry 边界

---

## 前置依赖

终端会话恢复依赖 `tmux`：

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Ubuntu / Debian
sudo dnf install -y tmux                       # Fedora / CentOS / RHEL
```

---

## 安装

### NPM（推荐 · 默认分发）

```bash
# 组织 scope：@1agents（与 @1agents/wire 相同组织）
npm install -g @1agents/1agents
# 国内镜像示例
npm install -g @1agents/1agents --registry=https://registry.npmmirror.com

1agents [参数]
```

要求：

- Node.js >= 22
- macOS arm64 / Linux x64 / Linux arm64（Windows 请用 WSL2 或源码）

**分发方式（重要）：**

- 采用 **多包拆分**：`@1agents/1agents` 为入口；**`@1agents/core-<plat>` 等平台包直接上传 npm，包内即二进制**（`1agents` + `ttyd` + `hk`）。
- 安装时从 **npm registry** 拉取当前架构的 core / web / cc-connect / cc-switch 等，**不需要**再访问 GitHub 下载大包。
- 设计说明见 [`docs/features/npm-package-split/prd.md`](docs/features/npm-package-split/prd.md)。
- `cloudflared` 可选（`-tunnel` 时按需）；HarnessKit 的 `hk` 随 core 平台包分发，不依赖本机 Python。

> 历史包名 `@scottzx/1agents` 与「薄安装器 + GitHub Release 下载」方案 **已废弃**，请改用 `@1agents/1agents`。

### 预编译二进制（可选旁路，非 npm 用户）

若不用 npm，可从 [GitHub Releases](https://github.com/scottzx/1agents/releases) 下载整包 tar 解压运行。**npm 用户无需走此路径。**

### Docker

```bash
docker run -d -p 8080:8080 \
  -v /path/to/your/workspaces:/workspace \
  --name 1agents scottzx/1Agents:latest
```

### 从源码构建

```bash
git clone --recursive https://github.com/scottzx/1Agents.git
cd 1agents_app
make all
```

常用构建命令：

```bash
make help
make all
make frontend
make ttyd
make cc-connect
make cc-connect-noweb
make cc-switch
make happy
make backend
make package
```

---

## 开发

前端：

```bash
cd frontend
yarn install
yarn start
yarn build
yarn check
```

后端：

```bash
cd backend
go build ./cmd/backend
```

cc-connect：

```bash
cd modules/cc-connect
make build
go test ./...
```

子模块：

```bash
git submodule update --init --recursive
make submodules
```

---

## 关联项目

- [cc-connect](https://github.com/scottzx/cc-connect)：多平台消息桥接和 Agent 集成。
- [1Hive](https://00claw.com/)：用于长期运行 AI Agent 的配套硬件方案。

---

## 许可证

本项目基于 [MIT License](LICENSE) 开源。

---

## 鸣谢

1agents 使用了多个开源项目和库。主要依赖包括：

**终端与前端**

- [ttyd](https://github.com/tsl0922/ttyd) (MIT)
- [xterm.js](https://github.com/xtermjs/xterm.js) (MIT)
- [Preact](https://github.com/preactjs/preact) (MIT)
- [Marked](https://github.com/markedjs/marked) (MIT)
- [trzsz](https://github.com/trzsz/trzsz.js) (MIT)
- [webpack](https://github.com/webpack/webpack) (MIT)

**AI / 消息平台桥接**

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) (MIT)
- [discordgo](https://github.com/bwmarrin/discordgo) (BSD-3)
- [go-telegram/bot](https://github.com/go-telegram/bot) (MIT)
- [slack-go](https://github.com/slack-go/slack) (BSD-2)
- [line-bot-sdk-go](https://github.com/line/line-bot-sdk-go) (Apache-2.0)
- [larksuite/oapi-sdk-go](https://github.com/larksuite/oapi-sdk-go) (MIT)
- [dingtalk-stream-sdk-go](https://github.com/open-dingtalk/dingtalk-stream-sdk-go) (Apache-2.0)
- [gorilla/websocket](https://github.com/gorilla/websocket) (BSD-2)

**Agent 工具链与数据基础设施**

- [cc-switch](https://github.com/farion1231/cc-switch) (MIT)
- [cc-switch-cli](https://github.com/SaladDay/cc-switch-cli) (MIT)
- [HarnessKit](https://github.com/RealZST/HarnessKit) (Apache-2.0；1agents 使用受控 Fork)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3)
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT)
- [creack/pty](https://github.com/creack/pty) (MIT)
- [robfig/cron](https://github.com/robfig/cron) (MIT)

如需完整第三方依赖说明，请查看 [THIRD_PARTY.md](THIRD_PARTY.md)。
