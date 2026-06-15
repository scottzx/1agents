# 1Agents 🚀

### An Army of One — AI-Native Office Software (One Person, Infinite Agents)

**1Agents** is an open-source, self-hosted **AI-native office software for an army of one**. As the sole owner, you command a virtual AI team — PMO, PM, Executor, and Verifier — that collaborates through an **A2A (Agent-to-Agent)** workflow to systematically turn complex information, ideas, and tasks into reality. One person, an entire army.

[简体中文](README.md) | **English**

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 🌌 Vision

> **"One Person, Infinite Agents"** — In the AI era, productivity is no longer limited by headcount, but by how effectively you orchestrate your AI legion.

1Agents is built for Indie Hackers, Solopreneurs, and knowledge workers who refuse to drown in operational overhead. We practice **Loop Engineering** and **Proactive Agents** — shifting from one-shot prompt engineering toward autonomous, self-correcting, event-triggered AI systems.

### 🔄 The A2A Collaboration Model
As the **sole owner**, you set the goals; your AI team coordinates and executes:

* **PMO**: Central command entry point — collects the Inbox, filters noise, facilitates project launches.
* **PM**: Breaks requirements into Projects with goals, milestones, and roadmaps, then schedules tasks.
* **Executor**: Completes assigned tasks, modifies source code, runs local tools/compilers.
* **Verifier**: The quality gate — validates outputs (pass or reject with feedback) for zero-defect delivery.

More than a remote terminal, 1Agents is an **open-source distributed agent collaboration network**, managing heterogeneous nodes from a single browser:

- **💻 macOS**: Daily productivity workspace — iCloud sync, system logs, AppleScript / Shortcuts.
- **🐧 Linux VPS**: Heavy cloud workhorse — 24/7 Docker orchestration, databases, long-running tasks.
- **🪟 Windows**: Specialist operator — UI automation & Win32 RPA for legacy desktop software.
- **🔌 Embedded & IoT**: Extends to the edge — reading sensors and driving signals into physical workflows.

---

## 🧭 Status & Roadmap

**Today**: 1Agents is a lightweight, zero-config, multi-workspace Web dashboard — an ultra-responsive web terminal (`ttyd + tmux`), a full-featured file explorer with HTML/PDF HD preview, native Speech-to-Text, and auto-generated SSL. It hosts and manages your local AI tooling (`Claude Code`, `Codex CLI`, `OpenClaw`, etc.).

**Next**: the **1Agents Distributed Orchestrator** — a complete **"User Request → Intent Analysis → Multi-device Orchestration → Distributed Execution → Unified Synthesis & Feedback"** loop. An LLM breaks high-level goals into sub-tasks, dispatches them to the best node (macOS/Linux/Windows/MCU) by native capability, and the hub verifies and compiles the result. The underlying **Agent Protocol Network** is the secure backbone for this multi-node orchestration.

---

## 🌟 Core Capabilities

- ⚡ **Zero-Latency Web Terminal (ttyd + tmux)**: `xterm.js` + WebSockets with integrated `tmux` state — sessions restore in milliseconds after a drop or refresh.
- 📂 **Full-Featured File Browser & Editor**: tree + tile navigation with quick search; native text/image preview, **HD HTML & PDF rendering with 16:9 fullscreen popout**; zero-config syntax editor with rename/save/download.
- 📁 **Dynamic Multi-Workspace**: create/switch/delete workspaces; native **Folder Picker** to load any local directory; terminal & file context sync instantly on switch.
- 🎙️ **Native Speech-to-Text**: built-in Web Speech API for instant English/Chinese transcription.
- 🔒 **Zero-Config SSL/TLS**: `--ssl` auto-generates a 10-year ECDSA P-256 self-signed cert; auto-matches Tailscale's Let's Encrypt cert for the browser green lock 🔒.
- 🤖 **CC-Connect Multi-Channel Bridge**: [cc-connect](https://github.com/scottzx/cc-connect) registers workspaces as projects and bridges Feishu, Telegram, Discord, and Slack, syncing theme & language.
- 🌐 **On-Demand Web Tunnel**: `--tunnel` or a cc-connect chat spins up a secure Cloudflare tunnel — no port forwarding or public IP. Missing `cloudflared` is auto-downloaded (~30MB); a dynamic token and terminal QR code let you connect by scanning.

---

## ⚙️ Prerequisites

Persistent terminal sessions (auto-reconnect) rely on **`tmux`**. Since `tmux` is a dynamically linked C binary, it isn't bundled into the NPM package — install it first via your package manager:

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Linux (Ubuntu/Debian)
sudo dnf install -y tmux                       # Linux (CentOS/RHEL/Fedora)
```

---

## 🚀 Installation

### Method 1: NPM (Recommended ⚡)

The wrapper auto-detects your architecture and downloads the matching binary. To bypass GitHub accessibility issues, **`ttyd` and the official `cloudflared` are pre-bundled inside `@scottzx/1agents`** — no post-install downloads, 100% offline out-of-the-box.

```bash
npm install -g @scottzx/1agents   # global install (daemon, ttyd, Web assets, cloudflared)
npx @scottzx/1agents [options]    # or run on-demand without installing
```

> **Requirements**: Node.js >= 22 (supports Node 24) | **Platforms**: macOS (x64/arm64), Linux (x64/arm64), Windows (x64/arm64)

### Method 2: Manual Binary Release
Grab pre-compiled executables from the [GitHub Releases page](https://github.com/scottzx/1Agents/releases), unzip, and run.

### Method 3: Docker

```bash
docker run -d -p 8080:8080 \
  -v /path/to/your/workspaces:/workspace \
  --name 1agents scottzx/1Agents:latest
```

### Method 4: Compile from Source

```bash
# 1. Compile the native C terminal backend (ttyd)
git clone --recursive https://github.com/scottzx/1Agents.git
cd 1agents && mkdir build && cd build && cmake .. && make

# 2. Build frontend assets (generates embedded html.h)
cd ../html && corepack enable && yarn install && yarn build

# 3. Compile the Go daemon
cd ../agent && go build -o 1agents ./cmd/agent/main.go
```

---

## 🛠️ CLI Flags & Usage

```bash
1agents                                                  # default :8080, mounts home (~)
1agents -listen 0.0.0.0:9000 -workdir /Users/scott/Projects   # custom listen & workdir
```

Open `http://localhost:8080` (or your port) in a browser to access your workspace.

| Flag | Type | Default | Description |
| :--- | :---: | :---: | :--- |
| `-listen` | `string` | `":8080"` | Address and port to serve on (e.g. `0.0.0.0:8080` or `:9000`) |
| `-workdir` | `string` | `"~"` | Root workspace folder mounted; files outside are inaccessible |
| `-tmux-session` | `string` | `"1agents"` | tmux session name for state restoration and auto-reconnects |
| `-ssl` | `bool` | `false` | Enable HTTPS; auto-generates a 10-year cert if none is configured |
| `-ssl-cert` | `string` | `""` | Path to custom SSL/TLS certificate (PEM) |
| `-ssl-key` | `string` | `""` | Path to custom SSL/TLS private key (PEM) |
| `-no-ttyd` | `bool` | `false` | Don't auto-start the ttyd backend (for dev testing) |
| `-ttyd-bin` | `string` | `"./ttyd"` | Path to custom `ttyd` binary |
| `-ttyd-addr` | `string` | `"127.0.0.1:7681"` | Loopback address for ttyd ↔ daemon communication |
| `-restart-delay` | `duration` | `"3s"` | Cooldown before respawning a crashed ttyd |
| `-max-restarts` | `int` | `5` | Consecutive-crash threshold before locking to avoid loops |
| `-tunnel` | `bool` | `false` | Launch a Cloudflare public tunnel; prints access link and QR code |

---

## 💡 Advanced Setup

### 1. HTTPS Green Lock (Tailscale + Let's Encrypt)
Browsers enforce a **Secure Context** (`localhost` or HTTPS) for Microphone, Clipboard, and Service Worker APIs, so LAN access from phones/tablets needs SSL. The easiest path is Tailscale's official cert:

```bash
tailscale cert <your-node-domain.ts.net>   # generates .crt / .key
```

Place them in `~/.1agents/certs/` and run `1agents --ssl` — the daemon auto-indexes the cert, giving you a green padlock 🔒 anywhere. See the [SSL Certificate Setup Guide](docs/tips/ssl-certificate-guide.md).

### 2. Speech-to-Text Compatibility
- **Safari (macOS) recommended**: uses native offline transcription — fast, offline, accurate.
- **Chrome / Edge**: rely on Google Cloud; restricted networks see `Speech recognition error: network`.
- **Mobile**: requires HTTPS to grant microphone access.

See [Speech Recognition Compatibility Notes](docs/tips/voice-recognition.md).

### 3. Zero-Config Public Tunnel
In environments with no public IP or firewall access (home routers, corporate networks, cafe Wi-Fi), add `-tunnel` to publish in one step:

```bash
1agents -tunnel
```

- **Smart reuse**: if `cloudflared` is already installed (e.g. via Homebrew), it boots in 0.1s with no download.
- **On-demand download**: otherwise it's fetched securely from the official GitHub release (~30MB; 2–5s internationally, 15–30s in China) with live progress; subsequent launches are instant.
- **Secure connect**: a single-use token plus a high-contrast terminal QR code let any phone scan in.

With [cc-connect](https://github.com/scottzx/cc-connect) active, just message your Feishu/Telegram/Slack bot "start tunnel" and it pulls up the tunnel and replies with a temporary secure HTTPS link.

---

## 🔗 Related Project

**1Hive** (formerly iClaw / 虾窝) — the companion hardware that keeps your AI agents running 24/7 so your "army of one" never goes offline: [https://00claw.com/](https://00claw.com/).

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).

---

## 🙏 Acknowledgements

`1agents` stands on the shoulders of giants. The open-source projects listed below collectively power this application — heartfelt thanks to every author and maintainer:

**Terminal & Frontend**
- [ttyd](https://github.com/tsl0922/ttyd) (MIT) — Web-based terminal server; both the frontend and `modules/ttyd` are forked from this project
- [xterm.js](https://github.com/xtermjs/xterm.js) (MIT) — High-performance web terminal emulator
- [Preact](https://github.com/preactjs/preact) (MIT) — 3 KB React-compatible runtime powering the UI
- [Marked](https://github.com/markedjs/marked) (MIT) — Markdown renderer for AI assistant messages
- [trzsz](https://github.com/trzsz/trzsz.js) (MIT) — Web-based trzsz file transfer
- [webpack](https://github.com/webpack/webpack) (MIT) — Frontend bundler and dev-server

**AI / Messaging Bridge (cc-connect)**
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) (MIT) — Go TUI framework by Charm
- [discordgo](https://github.com/bwmarrin/discordgo) (BSD-3) · [go-telegram/bot](https://github.com/go-telegram/bot) (MIT) · [slack-go](https://github.com/slack-go/slack) (BSD-2) · [line-bot-sdk-go](https://github.com/line/line-bot-sdk-go) (Apache-2.0)
- [larksuite/oapi-sdk-go](https://github.com/larksuite/oapi-sdk-go) (MIT) — Official Feishu open-platform SDK
- [dingtalk-stream-sdk-go](https://github.com/open-dingtalk/dingtalk-stream-sdk-go) (Apache-2.0) — DingTalk stream-connection SDK
- [gorilla/websocket](https://github.com/gorilla/websocket) (BSD-2) — WebSocket transport foundation

**AI Agent Toolchain**
- [cc-switch](https://github.com/farion1231/cc-switch) (MIT) — farion1231's original prototype for multi-account configuration switching across Claude Code / Codex / Gemini
- [cc-switch-cli](https://github.com/SaladDay/cc-switch-cli) (MIT) — Rust TUI/CLI derivative of cc-switch — compiled and shipped as a sidecar binary for provider & session management
- [skill-manager](https://github.com/mode-io/skill-manager) (MIT) — Local-first control panel for Skills / MCP servers / slash commands, integrated as the `modules/1skills` git submodule

**Data & Infrastructure**
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3) — Pure-Go SQLite (no CGO) backing workspace and session persistence
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT) — TOML configuration parser
- [creack/pty](https://github.com/creack/pty) (MIT) — Unix pseudo-terminal bindings, core dependency of remote terminal sessions
- [robfig/cron](https://github.com/robfig/cron) (MIT) — Go cron-expression scheduler

Remote platform chat bridges and project sync are driven by the [cc-connect](https://github.com/scottzx/cc-connect) module. Anything missing? Please open an issue — we'd love to add it.
