# 一万 1agents 🚀

### A2A Agent-Collaboration Project Management System · One Person, Infinite Agents

**1agents** is an open-source, self-hosted **A2A (Agent-to-Agent) collaboration project management system**. You break work into **tasks** and hand them to a collaborating digital crew — **agents, deterministic programs (function), and you (human)** — orchestrated, scheduled, and executed on a **three-layer kernel**, with you making the call at key nodes.

One person, commanding a legion of agents — living as an entire army.

[简体中文](README.md) | **English**

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 🧠 Approach / Architecture

The kernel is **three layers** (bottom-up); apps run on top of them:

```
   App layer    Built-in / extended apps (CRM · visual creation · self-media …) = "projects" on the kernel
  ────────────────────────────────────────────────────────────────────────────
   ③ Project    Organize a pile of tasks into a project — with goals and cadence
   ② Task       Orchestrate work into tasks: scheduled / recurring / deps / retry / scheduling
   ① Execution  Get work actually done: agent · function (deterministic program) · human
```

- **A2A collaboration**: multiple agents work around one project, dividing/handing-off/verifying autonomously along **task dependencies**; `function` handles the fixed work (0 tokens, reliable), and `human` makes the call at key nodes.
- **Three executors — rigid / flexible / judgment**: fixed steps go to `function` (rigid), judgment work to `agent` (flexible), accountable calls to `human` (judgment). Dependencies and scheduling are executor-agnostic; only the "who does it" step branches.
- **Crystallization (flexible → rigid)**: explored work gradually settles from chat / agent into **skill → scheduled task → function code** — faster, cheaper, and more reliable the more you use it.

> Full architecture: **[docs/agentsOS-架构设计.md](docs/agentsOS-架构设计.md)**; progress in Epic [#317](https://github.com/scottzx/1Agents/issues/317).

---

## 🎯 Built-in app: CRM (shipped)

The first app built into 1agents — a direct demo of what the kernel can do:

- **Fetch each Feishu group's people and key info on a schedule** (`function` — accurate, cheap, never drops the ball); not just your contacts but even people you've never added are ingested — instantly weaving a thousands-strong network (**degree-1 / degree-2**, with Feishu + WeChat merged into **one person** by phone number).
- Let **AI chew tens of thousands of chat words into decision cards** (`agent`): who's looking for what, which line is a lead, whether it's worth pursuing.
- You read the aggregated card and click **pursue** or **drop** (`human`).

**Fetch = function, analysis = agent, decision = you** — the whole chain is a live demo of the three-layer kernel.

> **Shipped**: Feishu contact aggregation (degree-1/2, cross-channel merge by phone, company table, data grid, group-chat bubbles) + single-batch value-extraction cards (via the task layer + agent).
> **In progress**: WeChat / DingTalk / email / Mac contacts, contact import-export, auto-spawned market research per lead, the visual-creation compositing canvas.

---

## 🧱 Features (runtime foundation)

- ⚡ **Zero-latency Web terminal (ttyd + tmux)**: `xterm.js` + WebSocket with built-in `tmux` state; sessions restore in milliseconds after a disconnect/refresh.
- 📂 **Full-featured file browser & editor**: tree + flat views with instant search; text/image preview, **native high-fidelity HTML & PDF rendering + 16:9 fullscreen**; zero-config syntax editor.
- 📁 **Dynamic multi-workspace**: create/switch/delete workspaces, with the browser-native **Folder Picker** to import local folders. **A workspace is a directory** — the working directory (`cwd`) where agents execute tasks.
- 🎙️ **Native voice input (Speech-to-Text)**: built-in Web Speech recognition for quick Chinese/English dictation.
- 🔒 **Fully automatic SSL/TLS**: `--ssl` generates a 10-year ECDSA P-256 self-signed cert when none exists; auto-detects Tailscale official certs.
- 🤖 **CC-Connect multi-channel bridging**: integrates [cc-connect](https://github.com/scottzx/cc-connect) for two-way communication with Feishu, Telegram, Discord, Slack, and more.
- 🌐 **On-demand secure public tunnel**: `--tunnel` spins up a Cloudflare tunnel — no port mapping or public IP — with a session token and a scannable QR code.

---

## ⚙️ Prerequisites

Automatic terminal session persistence (reconnect on drop) depends on **`tmux`** — install it first via your system package manager:

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Linux (Ubuntu/Debian)
sudo dnf install -y tmux                       # Linux (CentOS/RHEL/Fedora)
```

---

## 🚀 Installation

### Option 1: NPM (recommended ⚡)

The prebuilt NPM package auto-detects your architecture and downloads the matching binary. **The official `ttyd` and `cloudflared` binaries are bundled directly into `@scottzx/1agents`** — 100% offline, ready out of the box.

```bash
npm install -g @scottzx/1agents   # global install (daemon, ttyd, Web frontend, cloudflared)
npx @scottzx/1agents [args]        # or run directly without installing
```

> **Requires**: Node.js >= 22 (Node 24 compatible) | **Arch**: macOS (x64/arm64), Linux (x64/arm64), Windows (x64/arm64)

### Option 2: Prebuilt binaries
Download the static package for your architecture from [GitHub Releases](https://github.com/scottzx/1Agents/releases) and extract.

### Option 3: Docker

```bash
docker run -d -p 8080:8080 \
  -v /path/to/your/workspaces:/workspace \
  --name 1agents scottzx/1Agents:latest
```

### Option 4: Build from source

```bash
git clone --recursive https://github.com/scottzx/1Agents.git
cd 1agents_app
make all      # build frontend, ttyd, cc-connect, and backend in one shot (see Makefile / CLAUDE.md)
```

---

## 🔗 Related Projects

**1Hive** (formerly iClaw) — companion hardware that keeps your AI agents running 24/7: [https://00claw.com/](https://00claw.com/).

---

## 📄 License

Released under the [MIT License](LICENSE).

---

## 🙏 Acknowledgements

`1agents` stands on the shoulders of giants. The following open-source projects power this application — sincere thanks to all authors and maintainers:

**Terminal & Frontend**
- [ttyd](https://github.com/tsl0922/ttyd) (MIT) — the Web-based terminal server; both our frontend and the `modules/ttyd` submodule are forked from it
- [xterm.js](https://github.com/xtermjs/xterm.js) (MIT) — high-performance Web terminal emulator
- [Preact](https://github.com/preactjs/preact) (MIT) — the 3KB React-compatible runtime that drives the entire UI
- [Marked](https://github.com/markedjs/marked) (MIT) — Markdown renderer for AI output
- [trzsz](https://github.com/trzsz/trzsz.js) (MIT) — Web-based trzsz file transfer
- [webpack](https://github.com/webpack/webpack) (MIT) — frontend bundling and dev-server

**AI / Messaging Bridges (cc-connect)**
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) (MIT) — Charm's Go TUI framework
- [discordgo](https://github.com/bwmarrin/discordgo) (BSD-3) · [go-telegram/bot](https://github.com/go-telegram/bot) (MIT) · [slack-go](https://github.com/slack-go/slack) (BSD-2) · [line-bot-sdk-go](https://github.com/line/line-bot-sdk-go) (Apache-2.0)
- [larksuite/oapi-sdk-go](https://github.com/larksuite/oapi-sdk-go) (MIT) — official Feishu open-platform SDK
- [dingtalk-stream-sdk-go](https://github.com/open-dingtalk/dingtalk-stream-sdk-go) (Apache-2.0) — DingTalk streaming SDK
- [gorilla/websocket](https://github.com/gorilla/websocket) (BSD-2) — WebSocket transport foundation

**AI Agent Toolchain**
- [cc-switch](https://github.com/farion1231/cc-switch) (MIT) — farion1231's multi-account config switcher for Claude Code / Codex / Gemini
- [cc-switch-cli](https://github.com/SaladDay/cc-switch-cli) (MIT) — the Rust TUI/CLI derivative of cc-switch, compiled and shipped as a sidecar for provider and session management
- [skill-manager](https://github.com/mode-io/skill-manager) (MIT) — a local-first Skill / MCP / Slash command panel, integrated as the `modules/1skills` git submodule

**Data & Infrastructure**
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3) — pure-Go SQLite backing workspace and session persistence
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT) — TOML config parsing
- [creack/pty](https://github.com/creack/pty) (MIT) — Unix pseudo-terminal bindings, core to remote terminal sessions
- [robfig/cron](https://github.com/robfig/cron) (MIT) — Go cron-expression scheduler

Cross-channel AI messaging and agent integration are driven by the [cc-connect](https://github.com/scottzx/cc-connect) submodule. Spotted something missing? Please open an issue.
