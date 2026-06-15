# 1Agents 🚀

### 一人成军的 AI 原生办公软件 (One Person, Infinite Agents)

**1Agents** 是一款开源、自托管的 **一人成军的 AI 原生办公软件**。你以唯一老板的身份，指挥一支由 PMO、PM、执行者、校验者组成的 AI 团队，通过 **A2A (Agent-to-Agent)** 自动协同，把庞杂的想法、信息与任务流有条不紊地推进落地——一个人，活成一支军队。

**简体中文** | [English](README_EN.md)

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 🌌 愿景 (Vision)

> **“一人成军” (One Person, Infinite Agents)** —— 在 AI 时代，生产力不再受限于人手多少，而取决于你如何编排你的 AI 军团。

1Agents 为独立开发者 (Indie Hackers)、单干创始人 (Solopreneurs) 与所有不想被琐事淹没的知识工作者而生。我们实践 **环路工程 (Loop Engineering)** 与 **主动式智能体 (Proactive Agents)**：从单次提示词交互，转向自主纠偏、闭环反馈、主动运转的 AI 系统。

### 🔄 A2A 协同组织模型
你是 **唯一的老板 (Owner)**，AI 团队按角色分工自主推进：

* **PMO（总裁办总管）**：全局对话入口，收拢散落信息 (Inbox)、过滤并建议立项。
* **PM（项目经理）**：把需求拆解为含目标、里程碑、路线图的 Project，并排期。
* **Executor（执行者）**：执行具体开发任务，直接改文件、跑命令。
* **Verifier（校验者）**：只做质量验收（通过或打回），把关交付标准。

它不止是远程终端，更是一个 **开源分布式智能体协同网络**，通过浏览器统一管理跨平台节点：

* **💻 Mac**：日常办公专家——iCloud 同步、系统日志、AppleScript / 快捷指令。
* **🐧 Linux VPS**：后台重负载专家——24h 在线，容器编排、数据库、长周期任务。
* **🪟 Windows**：专业工具人——RPA 自动化操控传统桌面软件与原生环境。
* **🔌 具身智能 / 单片机**：未来延伸至物理边缘，把智能体带入真实工作现场。

---

## 🧭 项目现状与下一步 (Status & Roadmap)

**现在**：1Agents 是一个免配置、多端协同的轻量级 Web 工作台——零延迟 Web 终端（`ttyd + tmux`）、全功能文件浏览器（HTML/PDF 高清预览）、原生语音输入、全自动 SSL，能统一承载你本地已有的 AI 智能体（Claude Code、Codex CLI、OpenClaw 等）。

**下一步**：实现 **分布式任务与资源编排中枢 (Distributed Orchestrator)**——「用户提需求 → 中枢做编排 → 分布式执行 → 统一验收反馈」的闭环：LLM 把高阶目标拆解为子任务，按各节点（macOS/Linux/Windows/MCU）的原生工具能力做最优派发，结果回传中枢统一合成。底层的 **智能体通讯网络协议 (Agent Protocol Network)** 是支撑这套多端编排的核心基石。

---

## 🌟 核心能力

- ⚡ **零延迟 Web 终端 (ttyd + tmux)**：基于 `xterm.js` + WebSocket，内置 `tmux` 状态管理，断网/刷新后终端会话毫秒级还原，绝不断线。
- 📂 **全功能文件浏览器 & 编辑器**：树形目录 + 平铺视图，极速检索；内置文本/图片预览，**支持 HTML & PDF 原生高清渲染与 16:9 全屏预览**；零配置高亮编辑器，支持重命名、保存、下载。
- 📁 **动态多工作区**：创建/切换/删除多工作区，深度集成浏览器原生 **Folder Picker** 直接导入本地文件夹；切换时终端与文件上下文秒级同步。
- 🎙️ **原生语音输入 (Speech-to-Text)**：内置 Web Speech 识别，中英文快捷听写。
- 🔒 **全自动 SSL/TLS**：`--ssl` 启动时无证书则自动生成 10 年期 ECDSA P-256 自签名证书；自动识别 Tailscale 的 Let's Encrypt 官方证书，实现跨设备浏览器绿锁 🔒。
- 🤖 **CC-Connect 多渠道消息桥接**：集成 [cc-connect](https://github.com/scottzx/cc-connect)，把工作区注册为项目，与飞书、Telegram、Discord、Slack 等平台双向通信，主题/语言多维同步。
- 🌐 **按需公网安全通道**：`--tunnel` 或 cc-connect 对话即可拉起 Cloudflare 安全隧道，免端口映射/公网 IP；本地无 `cloudflared` 时自动下载（约 30MB），并生成动态会话 Token 与终端二维码，扫码即连。

---

## ⚙️ 前置依赖 (Prerequisites)

终端会话自动持久化（断线重连）依赖 **`tmux`**。由于 `tmux` 是动态链接的 C 程序，不便预包进 NPM，请先用系统包管理器安装：

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Linux (Ubuntu/Debian)
sudo dnf install -y tmux                       # Linux (CentOS/RHEL/Fedora)
```

---

## 🚀 安装 (Installation)

### 方法一：NPM（推荐 ⚡）

预编译 NPM 包会自动检测系统架构并下载匹配的平台二进制。为解决国内/部分服务器无法访问 GitHub 的问题，**`ttyd` 与 `cloudflared` 官方二进制已直接预包进 `@scottzx/1agents`**，无需后置下载，安装即 100% 离线开箱即用。

```bash
npm install -g @scottzx/1agents   # 全局安装（含守护进程、ttyd、Web 前端、cloudflared）
npx @scottzx/1agents [参数]        # 或免安装直接运行
```

> **要求**：Node.js >= 22（兼容 Node 24）｜**架构**：macOS (x64/arm64)、Linux (x64/arm64)、Windows (x64/arm64)

### 方法二：手动下载预编译二进制
访问 [GitHub Releases](https://github.com/scottzx/1Agents/releases) 下载对应架构的静态包，解压即用。

### 方法三：Docker

```bash
docker run -d -p 8080:8080 \
  -v /path/to/your/workspaces:/workspace \
  --name 1agents scottzx/1Agents:latest
```

### 方法四：从源码构建

```bash
# 1. 编译 C 终端后端 (ttyd)
git clone --recursive https://github.com/scottzx/1Agents.git
cd 1agents && mkdir build && cd build && cmake .. && make

# 2. 构建前端静态资源（生成嵌入式 html.h）
cd ../html && corepack enable && yarn install && yarn build

# 3. 编译 Go 守护进程
cd ../agent && go build -o 1agents ./cmd/agent/main.go
```

---

## 🛠️ 使用与命令行参数

```bash
1agents                                                  # 默认监听 :8080，工作目录 ~
1agents -listen 0.0.0.0:9000 -workdir /Users/scott/Projects   # 指定监听与工作目录
```

启动后在浏览器打开 `http://localhost:8080`（或对应端口）即可进入工作台。

| 命令行参数 | 类型 | 默认值 | 说明 |
| :--- | :---: | :---: | :--- |
| `-listen` | `string` | `":8080"` | 对外监听地址与端口 (如 `0.0.0.0:8080` / `:9000`) |
| `-workdir` | `string` | `"~"` | 暴露的文件系统根目录，目录外文件不可访问 |
| `-tmux-session` | `string` | `"1agents"` | 绑定的 tmux 会话名，用于断线重连与持久化 |
| `-ssl` | `bool` | `false` | 开启 HTTPS；无证书时自动生成 10 年期自签名证书 |
| `-ssl-cert` | `string` | `""` | 外部 SSL/TLS 证书路径 (PEM) |
| `-ssl-key` | `string` | `""` | 外部 SSL/TLS 私钥路径 (PEM) |
| `-no-ttyd` | `bool` | `false` | 跳过自动拉起 ttyd（开发调试用） |
| `-ttyd-bin` | `string` | `"./ttyd"` | 外部 `ttyd` 二进制路径 |
| `-ttyd-addr` | `string` | `"127.0.0.1:7681"` | ttyd 与守护进程的本地回环地址 |
| `-restart-delay` | `duration` | `"3s"` | ttyd 异常退出后重启的等待间隔 |
| `-max-restarts` | `int` | `5` | 连续异常重启上限，防止崩溃锁死 |
| `-tunnel` | `bool` | `false` | 开启 Cloudflare 按需公网隧道，启动输出公网链接与二维码 |

---

## 💡 高级配置

### 1. HTTPS 权威绿锁 (Tailscale + Let's Encrypt)
浏览器对麦克风、剪贴板等高级 API 强制要求安全上下文（`localhost` 或 HTTPS），局域网手机/平板访问时需配置 SSL。推荐结合 Tailscale 获取官方证书：

```bash
tailscale cert <你的节点域名.ts.net>   # 生成 .crt / .key
```

把证书放入 `~/.1agents/certs/`，再 `1agents --ssl` 即可——守护进程会自动扫描匹配，全球访问呈现绿锁 🔒。详见 [SSL 证书配置指南](docs/tips/ssl-certificate-guide.md)。

### 2. 语音识别浏览器兼容性
- **桌面端推荐 Safari (macOS)**：对接系统本地离线听写，无网络限制、秒级解析、中文精准。
- **Chrome / Edge**：依赖 Google 云端解析，国内无全局代理会报 `Speech recognition error: network`。
- **移动端**：强制 HTTPS，否则无法申请麦克风权限。

详见 [语音识别与麦克风权限兼容性指南](docs/tips/voice-recognition.md)。

### 3. 免配置公网安全隧道
在无公网 IP / 无证书的内网环境（居家宽带、公司内网、咖啡厅 Wi-Fi）下，加 `-tunnel` 参数即可一键发布：

```bash
1agents -tunnel
```

- **智能复用**：已装 `cloudflared`（如 `brew install`）则 0.1 秒直接复用，无需下载。
- **一次性自动下载**：无缓存时从 GitHub 官方源安全下载（约 30MB，海外 2~5s、国内 15~30s），控制台实时打印进度；二次启动秒开。
- **动态安全认证**：生成单次会话 Token，终端渲染高对比度二维码，手机扫码即连。

进一步地，启用 [cc-connect](https://github.com/scottzx/cc-connect) 后，只需在飞书/Telegram/Slack/微信等向智能体发一句「开启公网访问」，它就会在后台完成拉起并回传一个临时的互联网安全 URL。

---

## 🔗 关联项目

**1Hive**（曾用名 iClaw虾窝）—— 保障 AI Agent 7×24 长时间在线运行的配套硬件方案，让你的「一人成军」永不掉线：[https://00claw.com/](https://00claw.com/)。

---

## 📄 许可证 (License)

本项目基于 [MIT License](LICENSE) 协议开源。

---

## 🙏 鸣谢 (Acknowledgements)

`1agents` 站在巨人的肩膀上,下列开源项目共同支撑了本应用,在此向所有作者与维护者表达诚挚感谢:

**终端与前端**
- [ttyd](https://github.com/tsl0922/ttyd) (MIT) — 基于 Web 的终端服务,本项目前端及 `modules/ttyd` 子模块均 fork 自此项目
- [xterm.js](https://github.com/xtermjs/xterm.js) (MIT) — 高性能 Web 终端模拟器
- [Preact](https://github.com/preactjs/preact) (MIT) — 3KB 体积的 React 兼容运行时,驱动整个 UI
- [Marked](https://github.com/markedjs/marked) (MIT) — AI 输出消息的 Markdown 渲染器
- [trzsz](https://github.com/trzsz/trzsz.js) (MIT) — 基于 Web 的 trzsz 文件传输实现
- [webpack](https://github.com/webpack/webpack) (MIT) — 前端打包与 dev-server

**AI / 消息平台桥接 (cc-connect)**
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) (MIT) — Charm 团队出品的 Go TUI 框架
- [discordgo](https://github.com/bwmarrin/discordgo) (BSD-3) · [go-telegram/bot](https://github.com/go-telegram/bot) (MIT) · [slack-go](https://github.com/slack-go/slack) (BSD-2) · [line-bot-sdk-go](https://github.com/line/line-bot-sdk-go) (Apache-2.0)
- [larksuite/oapi-sdk-go](https://github.com/larksuite/oapi-sdk-go) (MIT) — 飞书开放平台官方 SDK
- [dingtalk-stream-sdk-go](https://github.com/open-dingtalk/dingtalk-stream-sdk-go) (Apache-2.0) — 钉钉流式接入 SDK
- [gorilla/websocket](https://github.com/gorilla/websocket) (BSD-2) — WebSocket 传输基础

**AI Agent 工具链**
- [cc-switch](https://github.com/farion1231/cc-switch) (MIT) — farion1231 出品的 Claude Code / Codex / Gemini 多账号配置切换原型
- [cc-switch-cli](https://github.com/SaladDay/cc-switch-cli) (MIT) — cc-switch 的 Rust TUI/CLI 衍生版,作为 sidecar 被编译并随发行版分发,负责供应商与会话管理
- [skill-manager](https://github.com/mode-io/skill-manager) (MIT) — 本地优先的 Skill / MCP / Slash 命令管理面板,以 git submodule 形式集成在 `modules/1skills`

**数据与基础设施**
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3) — 纯 Go 实现的 SQLite,支撑工作空间与会话持久化
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT) — TOML 配置解析
- [creack/pty](https://github.com/creack/pty) (MIT) — Unix 伪终端绑定,远端终端会话的核心依赖
- [robfig/cron](https://github.com/robfig/cron) (MIT) — Go 的 cron 表达式调度器

跨渠道 AI 消息桥接与智能体集成方案由子模块 [cc-connect](https://github.com/scottzx/cc-connect) 驱动。如有遗漏,欢迎通过 issue 告知补充。
