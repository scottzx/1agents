# 1Agents 🚀

### 一人成军的 AI 原生工作平台 (One Person, Infinite Agents)

**1Agents** 是一套开源、自托管的 **AI 原生工作平台**。它把任何工作拆成**任务**,派给三种执行者协同完成——**数字员工(agent)**、**确定性程序(function)**、**你自己(human)**——在一条统一的调度脊柱上跑起来。

更关键的是,它是一台 **固化引擎**:用 agent 把"不会做的"探索成"会做的",再沉淀成更刚、更便宜、更可靠的能力。于是 **记账、CRM、自媒体、AI 电台** 这类应用,都能像插件一样长在同一套智能体底座上——**可装可卸,只写业务逻辑,AI 能力按需以任务调用,不重复造 agent**。

一个人,指挥一支由 AI 与确定性程序组成的队伍,活成一支军队。

**简体中文** | [English](README_EN.md)

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 🧠 核心理念

> 完整架构设计见 **[docs/agentsOS-架构设计.md](docs/agentsOS-架构设计.md)**;实施进度见 Epic [#317](https://github.com/scottzx/1Agents/issues/317)。

### 一切皆任务,三种执行者:刚 / 柔 / 裁

判据很简单——**这件事的"怎么做"定死了没有?**

| 执行者 | 刚柔 | 何时用 | 例 | 成本 |
| :--- | :---: | :--- | :--- | :---: |
| **function** | 刚 | 输入与步骤**都定死** | ffmpeg(给定时间戳)、轮询 API、抓群消息、清洗、定时触发 | ~0 token |
| **agent** | 柔 | 需要**判断 / 组织 / 变通** | 多素材剪辑取舍、分析数据找商机线索、写方案、开发 | token |
| **human** | 裁 | 需要**最终担责 / 拍板** | 是否跟进某条线索、内容取舍 | 人力 |

> 原则:**下沉到能胜任的最便宜角色**——确定的交给 function,要判断的才上 agent,要担责的才上 human。依赖与调度执行者无关,只有"谁来干"那一步分流。

### 分层架构(自底向上生长)

```
① 业务层    应用 / 专业模板(自媒体 · CRM · AI电台 · 研发/Bug)
② 项目管理层  通用项目外壳:workspace(目录)· 动态/计划/任务/资产 · 项目配置
③ 任务层    任务引擎(基本要素):任务模型 · 调度/依赖/校验 · 北向任务 API
④ 执行层    agent(ACP/1acp 在目录起智能体)· function(确定性程序)· human(决策)
```

任务是原子,可脱离项目独立跑;**业务层不是预先设计的,是从下面的任务/执行层"长"出来、沉淀出来的**。

### 核心循环:固化(发散 → 收敛 → 沉淀)

平台用 agent 探索,再把稳定下来的部分**固化**成更可靠的形态;固化越深,越省 token、越刚性可靠:

`即兴(每次对话现做)→ 工具(skill/脚本,下次 agent 调更快)→ 定时(recurring)→ 代码(纯 function,运行时不过 agent)`

一个真实例子:用对话探索 `lark-cli` 抓群消息的参数 → 收敛出"翻页全量 + 增量获取"方案 → 固化成脚本/代码/模块。从此它刚性自治,不再每次劳烦 agent。

### 可装可卸的应用

应用 = 注册进平台的模块,`manifest` 声明**挂载点**:
- **项目内标签**(项目级,如自媒体——继承项目外壳,加自定义视图);
- **独立页面**(全局,如 CRM / AI 电台);
- **项目透视**(横切叠加,如财务成本)。

应用运行时**不内嵌 agent**,只回调平台的任务 API。**做第三个应用几乎不碰内核**——这就是"可拓展"成立的硬指标。

---

## 🧭 现状与路线 (Status & Roadmap)

**现在(已可用)**:一个免配置、多端协同的轻量级 Web 工作台——零延迟 Web 终端(`ttyd + tmux`)、全功能文件浏览器、原生语音输入、全自动 SSL、按需公网隧道,统一承载你本地已有的 AI 智能体(Claude Code、Codex CLI、OpenClaw 等),并通过 [1acp](https://github.com/scottzx/1acp)(ACP 协议客户端)与飞书/Telegram/Slack 等渠道双向打通。

**在建(Phase 1 · Epic [#317](https://github.com/scottzx/1Agents/issues/317))**:把上面的能力收敛成 agentsOS 的**任务内核 + 项目外壳 + 可装可卸应用**——任务模型补齐 `agent/function/human` 三态、北向任务 API、应用挂载机制,并落地首批应用:**自媒体、CRM**(+ **AI 电台** 作可拓展性验证)。单用户优先,不做多租户。

---

## 🧱 底座能力(现状)

- ⚡ **零延迟 Web 终端 (ttyd + tmux)**:基于 `xterm.js` + WebSocket,内置 `tmux` 状态管理,断网/刷新后终端会话毫秒级还原,绝不断线。
- 📂 **全功能文件浏览器 & 编辑器**:树形 + 平铺视图,极速检索;内置文本/图片预览,**支持 HTML & PDF 原生高清渲染与 16:9 全屏预览**;零配置高亮编辑器。
- 📁 **动态多工作区 (workspace)**:创建/切换/删除多工作区,深度集成浏览器原生 **Folder Picker** 直接导入本地文件夹;切换时终端与文件上下文秒级同步。**每个工作区就是一个目录**,也是智能体执行任务的工作目录(`cwd`)。
- 🎙️ **原生语音输入 (Speech-to-Text)**:内置 Web Speech 识别,中英文快捷听写。
- 🔒 **全自动 SSL/TLS**:`--ssl` 启动时无证书则自动生成 10 年期 ECDSA P-256 自签名证书;自动识别 Tailscale 的 Let's Encrypt 官方证书,跨设备浏览器绿锁 🔒。
- 🤖 **CC-Connect 多渠道消息桥接**:集成 [cc-connect](https://github.com/scottzx/cc-connect),把工作区注册为项目,与飞书、Telegram、Discord、Slack 等平台双向通信。
- 🌐 **按需公网安全通道**:`--tunnel` 或对话即可拉起 Cloudflare 安全隧道,免端口映射/公网 IP;本地无 `cloudflared` 时自动下载,并生成动态会话 Token 与终端二维码,扫码即连。

---

## ⚙️ 前置依赖 (Prerequisites)

终端会话自动持久化(断线重连)依赖 **`tmux`**。由于 `tmux` 是动态链接的 C 程序,不便预包进 NPM,请先用系统包管理器安装:

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Linux (Ubuntu/Debian)
sudo dnf install -y tmux                       # Linux (CentOS/RHEL/Fedora)
```

---

## 🚀 安装 (Installation)

### 方法一:NPM(推荐 ⚡）

预编译 NPM 包会自动检测系统架构并下载匹配的平台二进制。为解决国内/部分服务器无法访问 GitHub 的问题,**`ttyd` 与 `cloudflared` 官方二进制已直接预包进 `@scottzx/1agents`**,安装即 100% 离线开箱即用。

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

## 🛠️ 使用与命令行参数

```bash
1agents                                                       # 默认监听 :8080，工作目录 ~
1agents -listen 0.0.0.0:9000 -workdir /Users/scott/Projects   # 指定监听与工作目录
```

启动后在浏览器打开 `http://localhost:8080`(或对应端口)即可进入工作台。

| 命令行参数 | 类型 | 默认值 | 说明 |
| :--- | :---: | :---: | :--- |
| `-listen` | `string` | `":8080"` | 对外监听地址与端口 (如 `0.0.0.0:8080` / `:9000`) |
| `-workdir` | `string` | `"~"` | 暴露的文件系统根目录,目录外文件不可访问 |
| `-tmux-session` | `string` | `"1agents"` | 绑定的 tmux 会话名,用于断线重连与持久化 |
| `-ssl` | `bool` | `false` | 开启 HTTPS;无证书时自动生成 10 年期自签名证书 |
| `-ssl-cert` | `string` | `""` | 外部 SSL/TLS 证书路径 (PEM) |
| `-ssl-key` | `string` | `""` | 外部 SSL/TLS 私钥路径 (PEM) |
| `-no-ttyd` | `bool` | `false` | 跳过自动拉起 ttyd(开发调试用) |
| `-ttyd-bin` | `string` | `"./ttyd"` | 外部 `ttyd` 二进制路径 |
| `-ttyd-addr` | `string` | `"127.0.0.1:7681"` | ttyd 与守护进程的本地回环地址 |
| `-restart-delay` | `duration` | `"3s"` | ttyd 异常退出后重启的等待间隔 |
| `-max-restarts` | `int` | `5` | 连续异常重启上限,防止崩溃锁死 |
| `-tunnel` | `bool` | `false` | 开启 Cloudflare 按需公网隧道,启动输出公网链接与二维码 |

---

## 💡 高级配置

### 1. HTTPS 权威绿锁 (Tailscale + Let's Encrypt)
浏览器对麦克风、剪贴板等高级 API 强制要求安全上下文(`localhost` 或 HTTPS),局域网手机/平板访问时需配置 SSL。推荐结合 Tailscale 获取官方证书:

```bash
tailscale cert <你的节点域名.ts.net>   # 生成 .crt / .key
```

把证书放入 `~/.1agents/certs/`,再 `1agents --ssl` 即可——守护进程会自动扫描匹配,全球访问呈现绿锁 🔒。详见 [SSL 证书配置指南](docs/tips/ssl-certificate-guide.md)。

### 2. 语音识别浏览器兼容性
- **桌面端推荐 Safari (macOS)**:对接系统本地离线听写,无网络限制、秒级解析、中文精准。
- **Chrome / Edge**:依赖 Google 云端解析,国内无全局代理会报 `Speech recognition error: network`。
- **移动端**:强制 HTTPS,否则无法申请麦克风权限。

详见 [语音识别与麦克风权限兼容性指南](docs/tips/voice-recognition.md)。

### 3. 免配置公网安全隧道
在无公网 IP / 无证书的内网环境(居家宽带、公司内网、咖啡厅 Wi-Fi)下,加 `-tunnel` 参数即可一键发布:

```bash
1agents -tunnel
```

- **智能复用**:已装 `cloudflared`(如 `brew install`)则 0.1 秒直接复用,无需下载。
- **一次性自动下载**:无缓存时从 GitHub 官方源安全下载(约 30MB),控制台实时打印进度;二次启动秒开。
- **动态安全认证**:生成单次会话 Token,终端渲染高对比度二维码,手机扫码即连。

进一步地,启用 [cc-connect](https://github.com/scottzx/cc-connect) 后,只需在飞书/Telegram/Slack/微信等向智能体发一句「开启公网访问」,它就会在后台完成拉起并回传一个临时的互联网安全 URL。

---

## 🔗 关联项目

**1Hive**(曾用名 iClaw虾窝)—— 保障 AI Agent 7×24 长时间在线运行的配套硬件方案,让你的「一人成军」永不掉线:[https://00claw.com/](https://00claw.com/)。

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
