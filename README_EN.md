# 1Agents 🚀

### An Army of One — AI-Native Work Platform (One Person, Infinite Agents)

**1Agents** is an open-source, self-hosted **AI-native work platform**. It breaks any work into **tasks** and dispatches them to three kinds of executor — **digital employees (agent)**, **deterministic programs (function)**, and **you (human)** — coordinated on one unified scheduling spine.

More importantly, it is a **crystallization engine**: you use an agent to explore what you "can't do yet" into something you "can do", then settle it into a more rigid, cheaper, and more reliable capability. So apps like **accounting, CRM, self-media, AI radio** can grow on the same agent substrate like plugins — **install or uninstall at will, write only business logic, call AI on demand as tasks, and never rebuild the agent layer**.

One person, commanding a team of AI and deterministic programs — living as an entire army.

[简体中文](README.md) | **English**

[![NPM Version](https://img.shields.io/npm/v/@scottzx/1agents?color=blue&logo=npm)](https://www.npmjs.com/package/@scottzx/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## 🧠 Core Philosophy

> Full architecture design: **[docs/agentsOS-架构设计.md](docs/agentsOS-架构设计.md)**; implementation tracked in Epic [#317](https://github.com/scottzx/1Agents/issues/317).

### Everything is a task — three executors: rigid / flexible / judgment

The test is simple — **is the "how" of this work already fixed?**

| Executor | Rigid/Flexible | When | Examples | Cost |
| :--- | :---: | :--- | :--- | :---: |
| **function** | rigid | inputs and steps are **all fixed** | ffmpeg (given timestamps), polling an API, fetching chat messages, cleaning, scheduled triggers | ~0 tokens |
| **agent** | flexible | needs **judgment / orchestration / adaptation** | organizing a multi-clip edit, mining leads from data, drafting plans, building | tokens |
| **human** | judgment | needs **final accountability / a call** | whether to pursue a lead, content selection | human time |

> Principle: **push work down to the cheapest role that can do it** — fixed work to `function`, judgment work to `agent`, accountable work to `human`. Dependencies and scheduling are executor-agnostic; only the "who does it" step branches.

### Layered architecture (grown bottom-up)

```
① Business    Apps / professional templates (self-media · CRM · AI radio · dev/bug)
② Project     Generic project shell: workspace (dir) · activity/plan/tasks/assets · project config
③ Task        Task engine (the atom): task model · scheduling/deps/verify · north-bound Task API
④ Execution   agent (ACP/1acp in a dir) · function (deterministic) · human (decision)
```

A task is the atom and can run without a project; **the business layer is not designed up front — it grows and settles out of the task/execution layers below.**

### The core loop: crystallization (diverge → converge → settle)

The platform explores with an agent, then **crystallizes** the stable parts into more reliable forms; the deeper the crystallization, the cheaper (tokens) and the more rigidly reliable:

`ad-hoc (do it in chat each time) → tool (skill/script the agent calls faster next time) → scheduled (recurring) → code (pure function, no agent at runtime)`

A real example: explore `lark-cli` parameters in chat to fetch group messages → converge on a "full pagination + incremental fetch" plan → crystallize into a script/code/module. From then on it runs rigidly on its own, no longer bothering the agent.

### Pluggable apps

An app is a module registered into the platform; its `manifest` declares a **mount point**:
- **In-project tab** (project-scoped, e.g. self-media — inherits the project shell and adds custom views);
- **Standalone page** (global, e.g. CRM / AI radio);
- **Project lens** (cross-cutting overlay, e.g. finance cost).

At runtime an app **does not embed an agent** — it only calls back into the platform's Task API. **Building the third app barely touches the kernel** — that is the hard metric for "extensible".

---

## 🧭 Status & Roadmap

**Now (available)**: a zero-config, multi-device, lightweight Web workbench — a zero-latency Web terminal (`ttyd + tmux`), a full-featured file browser, native voice input, fully automatic SSL, and an on-demand public tunnel. It hosts the AI agents you already run locally (Claude Code, Codex CLI, OpenClaw, etc.) and bridges to Feishu/Telegram/Slack and more via [1acp](https://github.com/scottzx/1acp) (an ACP-protocol client).

**Building (Phase 1 · Epic [#317](https://github.com/scottzx/1Agents/issues/317))**: converging the above into agentsOS — a **task kernel + project shell + pluggable apps**. The task model gains the `agent/function/human` three-state, a north-bound Task API, and an app mount mechanism; the first apps land: **self-media, CRM** (plus **AI radio** as an extensibility check). Single-user first; no multi-tenancy.

---

## 🧱 Foundation Capabilities (today)

- ⚡ **Zero-latency Web terminal (ttyd + tmux)**: `xterm.js` + WebSocket with built-in `tmux` state management; terminal sessions restore in milliseconds after a disconnect/refresh — never dropped.
- 📂 **Full-featured file browser & editor**: tree + flat views with instant search; built-in text/image preview, **native high-fidelity HTML & PDF rendering with 16:9 fullscreen preview**; zero-config syntax editor.
- 📁 **Dynamic multi-workspace**: create/switch/delete workspaces, with deep integration of the browser-native **Folder Picker** to import local folders directly; terminal and file context sync in seconds on switch. **A workspace is a directory** — and the working directory (`cwd`) where agents execute tasks.
- 🎙️ **Native voice input (Speech-to-Text)**: built-in Web Speech recognition for quick Chinese/English dictation.
- 🔒 **Fully automatic SSL/TLS**: with `--ssl`, a 10-year ECDSA P-256 self-signed cert is generated when none exists; Tailscale's official Let's Encrypt certs are auto-detected for a cross-device green lock 🔒.
- 🤖 **CC-Connect multi-channel bridging**: integrates [cc-connect](https://github.com/scottzx/cc-connect) to register workspaces as projects and communicate bidirectionally with Feishu, Telegram, Discord, Slack, and more.
- 🌐 **On-demand secure public tunnel**: `--tunnel` (or a chat command) spins up a Cloudflare tunnel — no port mapping or public IP; `cloudflared` is auto-downloaded if missing, with a dynamic session token and a terminal QR code to scan and connect.

---

## ⚙️ Prerequisites

Automatic terminal session persistence (reconnect on drop) depends on **`tmux`**. Since `tmux` is a dynamically linked C program and inconvenient to pre-bundle in NPM, install it first via your system package manager:

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Linux (Ubuntu/Debian)
sudo dnf install -y tmux                       # Linux (CentOS/RHEL/Fedora)
```

---

## 🚀 Installation

### Option 1: NPM (recommended ⚡)

The prebuilt NPM package auto-detects your architecture and downloads the matching platform binary. To work around GitHub access issues in some regions/servers, **the official `ttyd` and `cloudflared` binaries are bundled directly into `@scottzx/1agents`** — 100% offline, ready out of the box.

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

## 🛠️ Usage & CLI Flags

```bash
1agents                                                       # listen on :8080, workdir ~
1agents -listen 0.0.0.0:9000 -workdir /Users/scott/Projects   # custom listen + workdir
```

Open `http://localhost:8080` (or your port) in a browser to enter the workbench.

| Flag | Type | Default | Description |
| :--- | :---: | :---: | :--- |
| `-listen` | `string` | `":8080"` | Listen address and port (e.g. `0.0.0.0:8080` / `:9000`) |
| `-workdir` | `string` | `"~"` | Exposed filesystem root; files outside it are inaccessible |
| `-tmux-session` | `string` | `"1agents"` | Bound tmux session name, for reconnect and persistence |
| `-ssl` | `bool` | `false` | Enable HTTPS; auto-generate a 10-year self-signed cert if none |
| `-ssl-cert` | `string` | `""` | External SSL/TLS certificate path (PEM) |
| `-ssl-key` | `string` | `""` | External SSL/TLS private key path (PEM) |
| `-no-ttyd` | `bool` | `false` | Skip auto-launching ttyd (for dev/debug) |
| `-ttyd-bin` | `string` | `"./ttyd"` | External `ttyd` binary path |
| `-ttyd-addr` | `string` | `"127.0.0.1:7681"` | Loopback address between ttyd and the daemon |
| `-restart-delay` | `duration` | `"3s"` | Wait interval before restarting ttyd after a crash |
| `-max-restarts` | `int` | `5` | Max consecutive restart attempts, to avoid crash lock |
| `-tunnel` | `bool` | `false` | Enable the Cloudflare on-demand tunnel; prints a public URL and QR code |

---

## 💡 Advanced Configuration

### 1. Trusted green lock over HTTPS (Tailscale + Let's Encrypt)
Browsers require a secure context (`localhost` or HTTPS) for advanced APIs like microphone and clipboard, so LAN access from phones/tablets needs SSL. We recommend pairing with Tailscale for official certs:

```bash
tailscale cert <your-node.ts.net>   # generates .crt / .key
```

Put the certs in `~/.1agents/certs/`, then run `1agents --ssl` — the daemon auto-scans and matches, presenting a green lock 🔒 from anywhere. See the [SSL certificate guide](docs/tips/ssl-certificate-guide.md).

### 2. Speech recognition browser compatibility
- **Desktop: Safari (macOS) recommended** — uses the system's local offline dictation: no network limits, sub-second, accurate Chinese.
- **Chrome / Edge**: depend on Google's cloud recognition; without a global proxy in some regions you'll hit `Speech recognition error: network`.
- **Mobile**: HTTPS is mandatory, otherwise the microphone permission can't be requested.

See the [voice recognition & microphone permission guide](docs/tips/voice-recognition.md).

### 3. Zero-config secure public tunnel
On networks without a public IP / cert (home broadband, office intranet, café Wi-Fi), add `-tunnel` to publish in one shot:

```bash
1agents -tunnel
```

- **Smart reuse**: if `cloudflared` is installed (e.g. via `brew`), it's reused in ~0.1s with no download.
- **One-time auto-download**: when not cached, it downloads securely from the official GitHub source (~30MB) with live progress; subsequent starts are instant.
- **Dynamic auth**: a single-use session token is generated and a high-contrast QR code rendered in the terminal — scan to connect.

Further, with [cc-connect](https://github.com/scottzx/cc-connect) enabled, just message the agent "open public access" from Feishu/Telegram/Slack/WeChat and it will spin up the tunnel in the background and return a temporary secure URL.

---

## 🔗 Related Projects

**1Hive** (formerly iClaw) — companion hardware that keeps your AI agents running 24/7 so your "army of one" never goes offline: [https://00claw.com/](https://00claw.com/).

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
