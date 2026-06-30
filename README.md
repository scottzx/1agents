# 一万 1agents 🚀

### A2A 智能体自主协作项目管理系统 · One Person, Infinite Agents

**一万(1agents)** 是一套开源、自托管的 **A2A 智能体自主协作项目管理系统**。你把工作拆成**任务**,交给一支会协作的数字班底——**智能体(agent)、确定性程序(function)、你自己(human)**——在一套**三层内核**上自动编排、调度、执行,关键节点由你拍板。

一个人,指挥一支智能体军团,活成一支军队。

**简体中文** | [English](README_EN.md)

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 🧠 方案 / 架构

1agents 的内核是**三层**(自下而上),应用就跑在这三层之上:

```
   应用层    内嵌/扩展应用(CRM · 视觉创作 · 自媒体 …)= 跑在内核上的"专业项目"
  ───────────────────────────────────────────────────────────
   ③ 项目层   把一摊任务组织成一个项目,有目标、有节奏地推进
   ② 任务层   把活编排成任务:定时 / 周期 / 依赖 / 重试 / 调度
   ① 执行层   把活真正做掉:agent(智能体)· function(确定性程序)· human(人)
```

- **A2A 自主协作**:多个智能体围绕同一个项目,按**任务依赖**自主分工、交接、验收;`function` 处理确定的活(0 token、可靠),`human` 在关键节点拍板。
- **三种执行者,刚 / 柔 / 裁**:确定的步骤交给 `function`(刚),要判断的交给 `agent`(柔),要担责的留给 `human`(裁)。依赖与调度执行者无关,只有"谁来干"那一步分流。
- **固化(柔 → 刚)**:摸索过的活,从对话 / agent 逐步固化成 **skill → 定时任务 → function 代码**——越用越快、越省、越可靠。

> 完整架构设计见 **[docs/agentsOS-架构设计.md](docs/agentsOS-架构设计.md)**;实施进度见 Epic [#317](https://github.com/scottzx/1Agents/issues/317)。

---

## 🎯 内嵌应用:CRM(已落地)

1agents 内置的第一个应用,直接演示这套底座能干嘛:

- **定时把每个飞书群的人和关键信息抓下来**(`function`,准、省、不掉链子);不只你的好友,连群里没加过的人也自动入库——瞬间织出一张几千人的关系网(**一度 / 二度**,飞书 + 微信按手机号合并成**同一个人**)。
- 再让 **AI 把几万字聊天嚼成一张张决策卡**(`agent`):谁在找什么、哪条像商机、值不值得跟。
- 你看汇总后的卡,点**跟**还是**弃**(`human`)。

**抓取 = function、分析 = agent、决策 = 你**——一条龙就是三层内核的活样板。

> **已上线**:飞书联系人聚合(一/二度、跨渠道按手机号合并、公司表、多维表格、群聊气泡)+ 单批价值提取卡(经任务层 + agent)。
> **在建**:微信 / 钉钉 / 邮箱 / Mac 通讯录扩展、通讯录导入导出、按线索自动起市场调研、视觉创作合成画布。

---

## 🧱 功能特性(运行底座)

- ⚡ **零延迟 Web 终端 (ttyd + tmux)**:`xterm.js` + WebSocket,内置 `tmux` 状态管理,断网/刷新后终端会话毫秒级还原。
- 📂 **全功能文件浏览器 & 编辑器**:树形 + 平铺视图,极速检索;文本/图片预览,**HTML & PDF 原生高清渲染 + 16:9 全屏预览**;零配置高亮编辑器。
- 📁 **动态多工作区 (workspace)**:创建/切换/删除多工作区,集成浏览器原生 **Folder Picker** 导入本地文件夹。**一个工作区就是一个目录**,也是智能体执行任务的工作目录(`cwd`)。
- 🎙️ **原生语音输入 (Speech-to-Text)**:内置 Web Speech 识别,中英文快捷听写。
- 🔒 **全自动 SSL/TLS**:`--ssl` 无证书时自动生成 10 年期 ECDSA P-256 自签名证书;自动识别 Tailscale 官方证书。
- 🤖 **CC-Connect 多渠道桥接**:集成 [cc-connect](https://github.com/scottzx/cc-connect),与飞书、Telegram、Discord、Slack 等双向通信。
- 🌐 **按需公网安全通道**:`--tunnel` 一键拉起 Cloudflare 隧道,免端口映射/公网 IP,生成会话 Token 与扫码二维码。

---

## ⚙️ 前置依赖 (Prerequisites)

终端会话自动持久化(断线重连)依赖 **`tmux`**,请先用系统包管理器安装:

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Linux (Ubuntu/Debian)
sudo dnf install -y tmux                       # Linux (CentOS/RHEL/Fedora)
```

---

## 🚀 安装 (Installation)

### 方法一:NPM(推荐 ⚡)

预编译 NPM 包自动检测架构并下载匹配的平台二进制。**`ttyd` 与 `cloudflared` 官方二进制已直接预包进 `@scottzx/1agents`**,安装即 100% 离线开箱即用。

```bash
npm install -g @scottzx/1agents   # 全局安装（含守护进程、ttyd、Web 前端、cloudflared）
npx @scottzx/1agents [参数]        # 或免安装直接运行
```

> **要求**:Node.js >= 22(兼容 Node 24)｜**架构**:macOS (x64/arm64)、Linux (x64/arm64)、Windows (x64/arm64)

### 方法二:手动下载预编译二进制
访问 [GitHub Releases](https://github.com/scottzx/1Agents/releases) 下载对应架构的静态包,解压即用。

### 方法三:Docker

```bash
docker run -d -p 8080:8080 \
  -v /path/to/your/workspaces:/workspace \
  --name 1agents scottzx/1Agents:latest
```

### 方法四:从源码构建

```bash
git clone --recursive https://github.com/scottzx/1Agents.git
cd 1agents_app
make all      # 一键构建 frontend、ttyd、cc-connect、backend（详见 Makefile / CLAUDE.md）
```

---

## 🔗 关联项目

**1Hive**(曾用名 iClaw虾窝)—— 保障 AI Agent 7×24 长时间在线运行的配套硬件方案:[https://00claw.com/](https://00claw.com/)。

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
