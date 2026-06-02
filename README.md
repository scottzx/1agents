# 一芥智能体协同系统 (1Agents) 🚀

### 万物归一，一生万物 —— 开源分布式智能体与多设备协同网络
**一芥智能体 (1Agents)** 是一个开源、去中心化、自托管的 **分布式智能体调度与任务编排工作空间**。它提供了一个**统一门户（Unified Portal）**，用于管理、编排和协同运行在多个物理或数字设备（macOS、Linux VPS、Windows 以及嵌入式/IoT 系统）上的异构 AI 智能体。

**简体中文** | [English](README_EN.md)

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 🌌 品牌起源与愿景 (Branding & Vision)

> **“一生万物，万物归一” (One is All, All is One)**
> —— 这就是 **1Agents** 命名的禅意内核。在 AI 浪潮的下一阶段，我们不再需要无数个彼此孤立的对话框，而是需要一个能容纳一切、连接物理与数字世界的统一智能中枢。

我们正处在一个**“AI 将兼容一切”**的拐点。随着系统工具 CLI 化、传统软件 API 化、中间件的快速普及，AI 智能体正在成为连接一切系统与设备的通用适配器。

**1Agents** 绝不仅是一个简单的远程终端，而是一个**开源分布式智能体协同网络**。它致力于打破设备与地域的物理界限，通过一个浏览器（Web 界面），将不同硬件平台（Mac、Linux、Windows 甚至未来的单片机/具身智能设备）上的 Agent 统一管理、无缝调度、接力协同：
- **💻 Mac 节点**：化身你的“日常办公专家”，深度调用 iCloud 闪电同步、分析系统日志、执行 AppleScript 或快捷指令。
- **🐧 Linux VPS 节点**：化身你的“后台重负载专家”，24 小时在线提供容器编排、数据库管理及长周期后台任务调度。
- **🪟 Windows 节点**：化身你的“专业工具人”，通过 RPA 自动化操控复杂的传统桌面专业软件与 Windows 原生环境。
- **🔌 具身智能/单片机**：在未来，它还将延伸至物理边缘，通过传感器和物理执行器，将智能体带入真实的工作物理现场。

---

## 🧭 项目现状与核心里程碑 (Current Status & Roadmap)

**1Agents** 并非一蹴而就的抽象网络，而是一个脚踏实地的、面向未来的分布式智能体协同系统。

### 1. 我们现在是什么（What We Are Today）
今天，1Agents 是一个**专为开发者与 AI 协同设计的轻量级、免配置、多端协同 Web 工作台**。它提供了极致响应的零延迟 Web 终端（融合 `ttyd + tmux`）、全功能 Web 文件浏览器（支持 HTML/PDF 高清预览）、原生语音输入以及全自动 SSL 安全通道，能够完美承载并统一管理你本地已有的 AI 智能体（如 Claude Code、Codex CLI、OpenClaw 等）。

### 2. 我们未来去向哪里（Next Major Milestone: 1Agents Distributed Orchestrator）
我们下一步将重点攻克并实现 **1Agents 分布式任务与资源编排中枢 (Distributed Orchestrator)**。这不仅是一个底层的智能体通讯协议，而是一个真正实现**“用户提要求 -> 中枢做编排 -> 分布式执行 -> 统一验收反馈”**的智能调度工作空间：

- **👤 用户意图输入 (Intent Capture)**：用户只需在 Unified Portal（统一门户网页/App）中提交高阶任务目标或想法。
- **📋 任务编排 (Task Orchestration)**：编排中枢利用大语言模型（LLM）将复杂目标智能拆解为多步子任务与执行流。
- **🧩 资源/智能体编排 (Resource/Agent Orchestration)**：编排中枢感知当前网络中所有在线设备（macOS, Linux, Windows, MCU 等）上运行的本地 Agent，并根据其**异构系统原生工具能力**（如 Mac 节点读取本地 iCloud 数据，Linux 节点跑 Docker 容器，Windows 节点控制专业桌面软件 API）进行最优的资源派发。
- **🛡️ 验收执行与反馈 (Execution, Validation & Synthesis)**：各个异构设备节点在本地执行任务，结果回传至中枢统一验收合成，最终无缝反馈给用户。

而底层的**智能体通讯网络协议 (Agent Protocol Network)**，正是支撑起这一套分布式多端编排与调度的最核心基石。我们将通过这个闭环，真正让“万物归一，一生万物”的 AI 协同在工作现场落地。

---

## 🌟 核心能力与架构优势

- ⚡ **零延迟 Web 终端 (ttyd + tmux 深度融合)**：
  - 基于 `xterm.js` 与高性能 WebSocket，继承 `ttyd` 的极致响应速度。
  - **终端会话自动持久化**：内置 `tmux` 状态管理，即便意外断网、浏览器刷新，所有终端进程与会话状态均毫秒级还原，绝不断线。
- 📂 **全功能 Web 文件浏览器 & 编辑器**：
  - **极速浏览**：左侧树形目录 + 核心区平铺视图，支持文件极速检索与类型筛选。
  - **全能预览**：内置文本、图片浏览器，**支持 HTML & PDF 文件的原生高清渲染与 16:9 新窗口/标签页全屏预览**。
  - **在线编辑**：零配置的高亮语法编辑器，支持文件直接重命名、保存与下载。
- 📁 **动态多工作区管理**：
  - 支持创建、切换与删除多个工作区。
  - **系统级 Folder Picker 融合**：深度集成浏览器原生文件/文件夹选择 API，直接导入本地任意开发文件夹。
  - 工作区切换时，**终端会话与文件浏览器上下文瞬间并行秒级同步**。
- 🎙️ **原生 Speech-to-Text 语音输入**：
  - 内置 Web Speech 原生语音识别，支持中英文快捷听写，打造“声控智能体”的流畅交互。
- 🔒 **全自动 SSL/TLS 证书网络安全**：
  - **自签名证书闪电生成**：通过 `--ssl` 启动，系统在没有证书时会自动生成高强度 ECDSA P-256 自签名证书，有效期达 10 年，适配所有局域网 IP。
  - **Tailscale 自动绿锁**：自动识别并匹配 Tailscale 的 Let's Encrypt 官方证书，为跨设备连接提供真正的浏览器绿色安全标志 🔒。
- 🤖 **CC-Connect 多渠道 AI 消息桥接**：
  - 深度集成 [cc-connect](https://github.com/scottzx/cc-connect) 模块，支持将工作区动态注册为项目。
  - 通过反向代理，实现与飞书、Telegram、Discord、Slack 等主流平台的无缝双向消息通信。
  - 智能体与远程控制台主题、语言等配置多维无缝同步。
- 🌐 **极简公网安全通道 (On-Demand Web Tunnel)**：
  - **一键极速发布**：支持通过 `--tunnel` 选项在启动时自动拉起安全公网隧道，或直接通过 `cc-connect` 智能体对话拉起临时公网地址，免去任何路由器端口映射、防火墙或公网 IP 配置。
  - **云端免安装/极速体验**：Go 后端智能感知，若本地无 `cloudflared`，将以极佳的实时进度日志自动安全拉取下载（约 30MB，国外 2~5 秒，国内 15~30 秒，二次启动 0.1 秒秒开）；若系统已有安装（如 Homebrew），则闪电复用、直接秒开。
  - **动态安全认证**：自动生成高强度动态会话 Token 并支持终端渲染高对比度二维码，移动端扫码即连。

---

## 📆 近一周 Git 更新亮点 (Weekly Features)

本周项目经历了重大重构和体验升级，以下是最新加入的核心功能：

1. **📄 HTML/PDF 高清预览**：文件管理系统新增 HTML 和 PDF 文件预览支持，且支持新开独立窗口/标签页以 16:9 黄金比例全屏浏览。
2. **🎙️ 原生语音输入与兼容优化**：全新集成 Speech-to-Text 功能并提供多语言切换。同时编写了兼容性指南，明确了 macOS Safari 本地离线引擎的秒级响应优势及 Chrome/Edge 依赖 Google 云端识别易在内网报 `network` 错误的原理。
3. **🔒 零配置安全证书与 Tailscale 融合**：新增 `--ssl` 证书自动生成器，支持零配置生成 ECDSA P-256 10 年期证书；新增 Tailscale 官方证书的自动扫描与发现逻辑，实现真正的跨设备“绿色安全绿锁”访问。
4. **📱 移动端终端快捷键与交互大优化**：
   - 彻底移除了臃肿的终端顶部栏，将 tmux 鼠标滚动/选择模式切换按钮移入顶部全局工作区标题栏。
   - 优化了手机端 Quick Keys 虚拟键盘布局，折叠了子命令菜单，并整合了方向键和退格键，新增 direct `claude` 极速命令。
   - 手机端切换侧边栏会话时，侧边栏支持自动优雅收起，最大化终端可视面积。
5. **⚙️ 快速工作区 Folder Picker**：抛弃了陈旧的文本路径输入框，在新建工作区时直接唤起操作系统原生的 Folder Picker 文件夹选择器。
6. **🚀 并行加载与启动优化**：优化了组件的并发初始化逻辑，工作区与终端会话实现并行异步加载，大幅缩短首屏白屏时间，并支持智能记忆/默认工作区选择。
7. **🤖 CC-Connect 跨域/反向代理升级**：支持工作区动态项目注册和 API POST 请求语言/主题同步，极大提升了 iframe 嵌入的整体感。
8. **📦 自动化 NPM 发布与 Node 24 适配**：推出了 multi-platform release CI/CD 自动化构建与统一的 NPM 包装包 `@scottzx/1agents`。适配了最新的 Node 24 运行环境，并优化了 Github Actions 上的 Yarn 3 缓存。
9. **🌐 按需公网 Web 隧道与智能下载优化**：新增 `--tunnel` 启动参数与 cc-connect 聊天智能唤醒，支持在无公网 IP/无证书环境下瞬间发布临时互联网安全访问链接；内置 Go 自动下载云端引擎（带有细致的进度百分比展示与系统级 `PATH` 智能复用），首次拉起极速自动适配，二次闪电秒开。

---

## 🚀 安装指南 (Installation)

### 方法一：通过 NPM 安装 (最简便 ⚡ 推荐)

我们提供了预编译的 NPM 包包装器，会自动检测您的系统架构并从 GitHub 镜像高速下载最匹配的平台二进制程序。

**🚀 离线开箱即用（全内置二进制）**：
为了解决国内或部分服务器因网络限制无法访问 GitHub 的问题，**我们已经将 `ttyd` 以及 `cloudflared`（Cloudflare Tunnel）官方最新二进制文件直接完整预包（Pre-bundled）在 `@scottzx/1agents` NPM 包体中！**
无需任何后置下载，无论是国内 NPM 镜像源还是官方源，直接安装即享 100% 离线开箱即用，免配置公网安全隧道秒开！

```bash
# 全局安装 1agents 主程序 (自动包含 1agents 守护进程、ttyd 静态后端、Web 前端以及平台专属的 cloudflared 二进制程序)
npm install -g @scottzx/1agents

# 也可以直接免安装直接通过 npx 运行：
npx @scottzx/1agents [参数]
```

> **系统要求**：Node.js >= 22 (完美兼容 Node 24)
> **支持架构**：macOS (Darwin x64/arm64), Linux (x64/arm64), Windows (x64/arm64)

### 方法二：手动下载预编译二进制

您也可以直接访问 [GitHub Releases 页面](https://github.com/scottzx/1Agents/releases) 下载适合您系统架构的静态二进制包，解压后即可开箱即用。

### 方法三：使用 Docker 部署

```bash
docker run -d \
  -p 8080:8080 \
  -v /path/to/your/workspaces:/workspace \
  --name 1agents \
  scottzx/1Agents:latest
```

### 方法四：从源码编译构建

如果您需要进行本地开发调试，请确保本地已启用 Yarn 3：

1. **编译 C 终端后端 (ttyd)**：
   ```bash
   git clone --recursive https://github.com/scottzx/1Agents.git
   cd 1agents
   mkdir build && cd build
   cmake ..
   make  # 产出 ttyd 二进制文件
   ```
2. **构建前端静态资源**：
   ```bash
   cd ../html
   corepack enable  # 确保启用 Yarn 3.6.3 
   yarn install     # 安装依赖
   yarn build       # 编译打包，并调用 gulp 生成嵌入式 html.h 文件
   ```
3. **编译 Go 守护进程**：
   ```bash
   cd ../agent
   go build -o 1agents ./cmd/agent/main.go
   ```

---

## 🛠️ 使用与命令行参数

启动服务非常简单，直接在终端中运行 `1agents` 即可：

```bash
# 启动服务，默认监听 8080 端口，工作目录为用户根目录 (~)
1agents

# 指定监听地址与暴露的工作目录
1agents -listen 0.0.0.0:9000 -workdir /Users/scott/Projects
```

服务启动后，在本地或局域网浏览器中打开 `http://localhost:8080` (或对应的监听端口) 即可进入您的智能体工作台！

### 完整命令行 flags 参数说明

| 命令行参数 | 参数类型 | 默认值 | 详细功能说明 |
| :--- | :---: | :---: | :--- |
| `-listen` | `string` | `":8080"` | 服务对外监听的地址与端口 (例: `0.0.0.0:8080` / `:9000`) |
| `-workdir` | `string` | `"~"` | 工作台默认暴露的文件系统根目录。非在此目录下的文件不可被访问 |
| `-tmux-session` | `string` | `"1agents"` | 默认绑定的 tmux 会话名称，用于实现 Web 终端断线重连与持久化运行 |
| `-ssl` | `bool` | `false` | 是否开启 HTTPS 协议。若为 true 且无证书，系统会自动生成 10 年期自签名证书 |
| `-ssl-cert` | `string` | `""` | 外部指定的高级 SSL/TLS 证书路径 (PEM 格式) |
| `-ssl-key` | `string` | `""` | 外部指定的高级 SSL/TLS 证书私钥路径 (PEM 格式) |
| `-no-ttyd` | `bool` | `false` | 跳过由 Go 守护进程自动拉起 ttyd 进程的步骤 (用于开发调试) |
| `-ttyd-bin` | `string` | `"./ttyd"` | 外部指定的 `ttyd` 二进制执行程序文件路径 |
| `-ttyd-addr` | `string` | `"127.0.0.1:7681"`| 内置 ttyd 与 Go 守护进程的本地通信回环地址 |
| `-restart-delay`| `duration` | `"3s"` | 当 ttyd 意外退出后，守护进程尝试自动重新拉起的等待间隔 |
| `-max-restarts` | `int` | `5` | 最大连续异常重启 ttyd 进程的上限次数，防止循环崩溃锁死 |
| `-tunnel` | `bool` | `false` | 是否开启基于 Cloudflare 的按需安全公网隧道 (Web Tunnel)，启动时自动输出公网链接与二维码 |

---

## 💡 高级配置与技术指南

### 1. 开启局域网/互联网 HTTPS 权威绿锁 (Tailscale Let's Encrypt 方案)

由于浏览器对高级 API (如麦克风权限、Service Worker 缓存、剪贴板等) 强制要求 **安全上下文** (即必须是 `localhost` 或 `HTTPS`)，因此在局域网内使用手机或平板访问时，必须配置 SSL。

最完美的免费方案是结合 **Tailscale** 自动获取官方 Let's Encrypt 证书：

1. **启用 HTTPS**：在 Tailscale Admin Console Settings 中激活 **HTTPS Certificates**。
2. **下载证书**：在宿主机终端中执行：
   ```bash
   tailscale cert <您的Tailscale节点域名.ts.net>
   ```
3. **一键适配**：将生成的 `.crt` 和 `.key` 文件移入 `~/.1agents/certs/` 目录下。
4. **启动服务**：直接运行：
   ```bash
   1agents --ssl
   ```
   *Go 守护进程会自动扫描并匹配 Tailscale 官方证书，全球任何设备访问您的节点域名时都将呈现安全的绿锁标识 🔒！*
   *(更多细节请查阅：[Tailscale 证书配置指南](docs/tips/ssl-certificate-guide.md))*

### 2. 语音识别 (Speech-to-Text) 浏览器兼容性避坑

语音输入对多媒体输入权限及底层的语音解析引擎有较高要求：

- **桌面端推荐使用 Safari**：Safari (macOS) 的语音识别完美对接系统本地离线听写模块，**无任何网络限制，秒级瞬时解析**，且中文普通话极其精准。
- **Chrome / Edge 的 Network 报错**：由于 Chrome/Edge 的 Web Speech 强依赖于 Google 云端服务器解析，若国内网络未配置全局系统代理，会报 `Speech recognition error: network`。
- **移动端 (手机/平板)**：强制要求使用 HTTPS 协议，否则网页端无法申请麦克风录音权限。
- *(更多细节请查阅：[语音识别与麦克风权限兼容性指南](docs/tips/voice-recognition.md))*

### 3. 免配置公网安全隧道 (Cloudflare Tunnel & cc-connect 智能拉起)

当您在**无公网 IP、无证书配置且处于复杂内网环境**（例如居家宽带、公司内网、咖啡厅 Wi-Fi）中时，`1Agents` 提供了最极致的“零配置公网访问”方案。

#### 💡 工作原理与首次下载体验
当您在启动命令中加上 `-tunnel` 参数，例如：
```bash
1agents -tunnel
```
系统会触发按需公网通道的智能拉起流程：
1. **环境变量优先复用（智能跳过）**：
   Go 后端在拉起前会智能检测系统环境。如果您的系统中已经安装过 `cloudflared`（例如 macOS 用户通过 `brew install` 安装，或 Linux 用户使用 `apt` 安装），Go 后端会**瞬间识别并直接复用**系统已有程序，**实现 0.1 秒闪电启动，首次运行也完全无需下载**！
2. **一次性自动下载（进度日志完全可见）**：
   如果您的系统没有任何 `cloudflared` 缓存，Go 后端会从 GitHub Releases 官方源执行一次性安全下载（文件大小约为 30MB）。为防止出现“程序卡死”的疑虑，控制台会**实时打印下载进度日志**：
   ```text
   [tunnel] cloudflared not found in PATH or local bin. Starting automatic download...
   [tunnel] Downloading cloudflared binary from: https://github.com/...
   [tunnel] Successfully downloaded and installed cloudflared to: ~/.1agents/bin/cloudflared
   ```
   - **海外网络/高速宽带**：下载 30MB 仅需 **2 ~ 5 秒**，直接输出二维码。
   - **国内普通网络**：受 GitHub 国际带宽限制，下载时间约 **15 ~ 30 秒**。
   - **二次运行**：本地已缓存，**启动速度为 0.1 秒，瞬间秒开**！
3. **高强度动态安全认证**：
   公网隧道启动后，会自动生成高强度的单次会话 Token，并在终端控制台直接渲染**高对比度连接二维码**，手机/平板只需扫码即可直接通过互联网安全通道秒连！

#### 🤖 CC-Connect 智能体对话拉起 (极致低门槛)
更进一步，如果您启用了 [cc-connect](https://github.com/scottzx/cc-connect)，您甚至连命令行都不需要接触。**只需在飞书、Telegram、Slack 或微信等聊天软件中向您的 CC-Connect 智能体发送一句对话（如“帮我拉起远程工作台”或“开启公网访问”），智能体就会在后台自动完成上述下载与拉起流程，并在聊天窗口中为您返回一个临时的、互联网端可直接访问的安全加密 URL 链接。**

---

## 📄 许可证 (License)

本项目基于 [MIT License](LICENSE) 协议开源。

---

**致谢与关联项目**：
- 终端底层核心能力基于优秀的开源项目 [ttyd](https://github.com/tsl0922/ttyd) 构建，衷心感谢其作者的杰出贡献。
- 跨渠道 AI 消息桥接与智能体集成方案由子模块 [cc-connect](https://github.com/scottzx/cc-connect) 驱动。
