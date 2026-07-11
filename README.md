# 1agents

**1agents** 是一个开源、自托管的 AI-native 工作操作系统。它把任务、上下文、执行者、验证和回写放进同一个闭环，让一个人也能组织一支持续工作的 AI 团队。

传统 AI 助手解决的是一次对话里的单点效率；1agents 解决的是一件工作从进入系统到被验证完成的完整链路。它把 Inbox、IM、数据源、项目、任务、Agent 会话、终端、文件、日程和插件式应用组织在一起，不是为了堆功能，而是为了让工作可以被接住、被拆解、被执行、被验收，并在过程中留下可追溯的上下文。

**简体中文** | [English](README_EN.md)

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 产品方向

1agents 的核心不是一个更大的 Chat 面板，而是一套围绕工作的闭环系统：

1. **工作进入系统**：来自 Inbox、IM、数据源、手动录入、日程提醒或应用业务对象的想法、需求、缺陷和待办，都可以沉淀为项目里的议题或任务。
2. **系统组织工作**：Project / Task / Issue 承载上下文、依赖、计划、优先级、验收标准、子任务和模板，让系统知道下一步应该怎么走。
3. **执行者完成工作**：AI Agent、确定性程序和用户本人按任务属性分流。需要判断和组织的交给 Agent，步骤固定的交给 function，需要最终担责的交给 human。
4. **结果形成闭环**：每次讨论、会话、工具调用、代码 diff、验收、返工和状态变化都回到任务时间线，成为下一轮执行可读取的上下文和后续指标沉淀。

目标是让 AI 不只停留在“帮人做一步”，而是进入协作链路：工作进来后，系统持续推进、显式暴露卡点、把结果交给人确认，并把经验沉淀成可复用的能力。

## 主要功能

### 智能体工作台

- Web 前端访问本机或远程 1agents 后端。
- 支持本机直连和 Relay 中转连接。
- 支持 Access Token 门禁和 Relay 配对门禁。
- 桌面端提供左侧工作区树、顶部上下文栏、主工作区画布和右侧内容面板。
- 移动端提供单列布局、触控导航和移动端 Chat/终端/设置入口。
- 支持多标签页、内置浏览器、文件预览、全局搜索和可拖拽分栏。

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
- 支持初始命令、tmux 鼠标模式、浅色/深色终端主题和移动端终端适配。

### PMO、任务与路线图

- 项目内置讨论、需求、任务、缺陷、AI 建议和里程碑管理。
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
- 支持治理表详情、依赖关系和执行记录。

### 联系人、消息、Inbox 与日程

- 联系人模块可聚合外部渠道身份，支持联系人表格、搜索、分组、详情和编辑。
- 消息模块展示外部渠道消息和群聊内容。
- Inbox 支持手动捕获文本或 URL，标记已读/未读、归档，以及分发到项目。
- 日程模块聚合个人提醒、项目任务和里程碑日期，支持月历视图和提醒创建。

### 助理、技能与 Agent 能力管理

- 助理页管理 Assistant 工作区。
- 助理详情包含会话、团队、任务、文件、渠道和设置。
- 团队与技能页用于管理项目/助理关联的技能配置。
- 集成 1skills 面板，用于管理 Skills、Agents、Slash Commands、MCP 和 Marketplace。
- Agent Catalog 展示可用 Agent 类型和安装命令。

### 插件式应用

- 支持 App Manifest 和 Mount Point。
- 应用可以贡献一级页面、项目标签页和 Lens overlay。
- 发现中心展示应用、精选推荐和开源项目。
- Vlog & Clip 内容工作室作为项目标签页扩展，合并项目演示录制、素材导入、转录、金句提取、纠错和后续混剪成片工作流。

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

1agents 的工作流按四层闭环组织：

- **入口层**：Inbox、IM 渠道、数据源、手动任务、日程和应用业务对象，把外部信息转成可处理的工作。
- **组织层**：项目 / 工作区、Task / Issue、依赖、计划、优先级、里程碑和项目配置，负责保存上下文并决定下一步。
- **执行层**：AI Agent、确定性 function 和 human 三类执行者在同一任务内核下分流执行。
- **闭环层**：任务时间线记录讨论、会话、产出、验证、返工、完成状态和指标，让结果可追溯、可恢复、可优化。

核心对象：

- **项目 / 工作区**：承载一个目录、一组会话、一组任务和项目配置。
- **任务**：描述可执行工作、依赖关系、计划时间、状态和验收标准。
- **执行者**：包括 AI Agent、确定性程序和用户本人。
- **时间线**：承载用户反馈、Agent 回复、会话分支、验证结果和状态变更。

主要代码结构：

```text
frontend/        Web 前端，包含工作台、Chat、任务、文件、数据源、设置等界面
backend/         1agents 后端服务
modules/ttyd/    Web 终端服务
modules/cc-connect/  消息平台和 Agent 桥接
build/           本地构建产物
docs/            产品、设计和架构文档
```

更多设计文档：

- [docs/frontend-product-features.md](docs/frontend-product-features.md)
- [docs/design_rules/app-agentic-workbench-design-standard.md](docs/design_rules/app-agentic-workbench-design-standard.md)
- [docs/agentsOS-架构设计.md](docs/agentsOS-架构设计.md)

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

### NPM

```bash
npm install -g @scottzx/1agents
npx @scottzx/1agents [参数]
```

要求：

- Node.js >= 22
- macOS x64/arm64
- Linux x64/arm64
- Windows x64/arm64

NPM 包包含后端、Web 前端、`ttyd` 和 `cloudflared` 等运行所需组件。

### 预编译二进制

从 [GitHub Releases](https://github.com/scottzx/1Agents/releases) 下载对应平台的压缩包，解压后运行。

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
make frontend
make ttyd
make cc-connect
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
- [skill-manager](https://github.com/mode-io/skill-manager) (MIT)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3)
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT)
- [creack/pty](https://github.com/creack/pty) (MIT)
- [robfig/cron](https://github.com/robfig/cron) (MIT)

如需完整第三方依赖说明，请查看 [THIRD_PARTY.md](THIRD_PARTY.md)。
