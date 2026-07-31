# 1agents

**1agents** is an open-source, self-hosted AI-native work operating system and an **Agent Infra** for multi-agent collaboration. It connects data, requirements, tasks, context, executors, verification, and state write-back into a continuously evolving work Graph, so one person can organize an AI team that keeps work moving.

Traditional AI assistants improve one prompt or one chat turn. 1agents focuses on the full path from work entering the system to work being verified as done. It brings Inbox, IM, data sources, projects, task blueprints, agent sessions, terminals, files, agenda, and extensions together not as a pile of features, but as a way to capture, understand, structure, schedule, execute, and verify work while turning each result into context for the next action.

**10K-Agent Infra** describes an infrastructure goal, not a mechanical promise about simultaneous runtime count: executors can grow and models can be replaced while the work Graph that carries goals, states, dependencies, and acceptance criteria remains stable. The self-cycling agent Graph is the operating model; "One Person, Infinite Agents" is the outcome.

[简体中文](README.md) | **English**

[![NPM](https://img.shields.io/badge/npm-@1agents%2Fcli-blue?logo=npm)](https://www.npmjs.com/org/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## Product Direction: Building a Self-Cycling Agent Graph

1agents is not just a larger Chat panel, and it is not limited to repeating one agent Loop. It represents real work as a Graph:

- **Nodes** carry data, requirements, bugs, tasks, milestones, sessions, artifacts, and verification records.
- **State** records whether work is unplanned, running, blocked, awaiting verification, completed, or returning for rework.
- **Edges** express source, ownership, dependency, decomposition, dispatch, verification, and write-back.
- **Decisions** determine whether dependencies are ready, who should execute, whether verification passed, and which node comes next.
- **Feedback** writes results and failure reasons back into the Graph, updates the plan, and activates downstream nodes.

A typical business Graph:

```text
Data sources
  → governance and hot updates
  → Inbox classification and intent detection
  → requirements / feedback
  → task blueprint and milestones
  → Schedule
  → 1ACP / ACPX dispatch
  → agent / function / human execution
  → Check and verification
  → close or rework
  → state and results written back to the task blueprint
```

The goal is to move AI beyond "help me with this step" and into the collaboration chain: once work enters the system, nodes, state, and decisions keep it moving, expose blockers explicitly, hand results back for human confirmation, and turn repeated experience into reusable capability. A Loop is one cycle inside the Graph; the Graph organizes the whole body of work.

---

## Main Features

### Agent Workbench

- Access a local or remote 1agents backend from the Web frontend.
- Support direct local connection and Relay-based remote connection.
- Support Access Token gate and Relay pairing gate.
- Desktop layout with workspace tree, context header, main workbench canvas, and side content panel.
- Mobile layout with single-column navigation and mobile Chat, terminal, and settings views.
- Support tabs, built-in browser, file preview, global search, and resizable panes.
- Desktop right-side secondary panels support session-persisted tabs for tasks, files, browser, Git, and terminals.

### Projects and Workspaces

- Create, switch, delete, and archive workspaces.
- A workspace maps to a directory and is used as the working directory for terminals and agents.
- Project home shows active projects, archived projects, and project template entries.
- Project detail pages include sessions, team, tasks, files, channels, activity, plan, assets, and settings.
- Project configuration includes instructions, connectors, visible tabs, and project-level settings.

### AI Chat Sessions

- Create Chat sessions with workspace, agent type, role, permission mode, and initial prompt.
- Support general sessions and PM sessions.
- Support session list, rename, delete, task association, and project-context locking.
- Render Markdown, code blocks, tables, links, Mermaid diagrams, and task references.
- Show agent tool calls, command output, file diffs, plan checklists, and permission requests.
- Support attachments, voice input, and slash commands.

### Web Terminal

- Browser terminal based on `ttyd`, `xterm.js`, and WebSocket.
- Use `tmux` to keep terminal sessions recoverable after refresh or disconnect.
- Support multiple terminal windows and terminal creation with workspace and cwd.
- Secondary-panel terminal tabs unload idle frontend connections and reclaim the corresponding `tmux` window to avoid orphaned processes.
- Support initial command, tmux mouse mode, light/dark terminal themes, and mobile terminal adaptation.

### PMO, Tasks, and Roadmap

- Manage discussions, requirements, tasks, bugs, AI suggestions, and milestones inside each project.
- The Feature Blueprint connects modules, feature nodes, source requirements, delivery tasks, target versions, and delivery state.
- The Task Blueprint organizes requirements, feature nodes, delivery tasks, dependencies, execution plans, and milestones into a navigable, traceable, writable work map.
- Task views include list, kanban, calendar, and milestone roadmap.
- Task tables support search, filters, sorting, grouping, column visibility, and inline editing.
- Task detail shows description, acceptance criteria, checklist, subtasks, relations, comments, session branches, and properties.
- Support `#number` task references and task deep links.
- Inbox items can be dispatched into a project's requirement pool.

### Files, Preview, and Git

- File browser supports workspace file tree, flat view, and search.
- Preview text, images, Markdown, HTML, PDF, and other supported content.
- Chat and Git-related flows can show diffs.
- Git panel exposes version-control actions around the current workspace.
- Built-in browser opens URLs, Dashboard, or external pages.

### Data Sources and Governance

- Connect external source accounts, manage authorization and configuration, and inspect data.
- Data ingestion layer manages raw data by source and account.
- Governance layer shows cleaned and merged output tables.
- Run Python scripts or deterministic tasks for data processing and hot updates.
- Support governance table detail, dependency relations, and execution records.

### Contacts, Messages, Inbox, and Agenda

- Aggregate contact identities from external channels.
- Contact table supports search, grouping, detail view, and editing.
- Message view shows external-channel messages and group-chat content.
- Inbox captures text or URLs, classifies information and detects intent, tracks read/archive state, and dispatches requirements or feedback into projects.
- Agenda aggregates reminders, project tasks, and milestone dates in a calendar view.

### Assistants, Skills, and Agent Capabilities

- Assistant pages manage Assistant workspaces.
- Assistant detail includes sessions, team, tasks, files, channels, and settings.
- Team and skill pages manage skills attached to a project or assistant.
- A controlled HarnessKit fork provides one Extensions surface for Skills, Subagents, Commands, MCP, Hooks, Plugins, CLIs, and Kits.
- Agent Catalog shows available agent types and install commands.

### App Extensions

- Compile-time App Registry, manifest discovery and enable/disable APIs, and frontend mount-point rendering are implemented.
- Apps can contribute top-level pages, project tabs, and lens overlays; the currently confirmed production registration includes Agents Roundtable.
- A complete external app SDK, runtime hot installation, and a third-party marketplace remain under development.
- Discovery center shows apps, featured recommendations, and open-source projects.

### cc-connect, Providers, and Channels

- Integrates `cc-connect` for Feishu, Telegram, Discord, Slack, DingTalk, and other messaging platforms.
- Provider panel manages messaging and agent-execution platform settings.
- Project channel pages show project-related channel state.
- In Relay mode, embedded module requests go through the same relay transport.

### System Settings and Updates

- Configure language, theme, and beginner/advanced mode.
- Configure Relay pairing, device status, account/subscription, and local-machine mode.
- Check and apply frontend and backend updates.
- Clear browser cache or reset local app data.

### Dashboard

- Dashboard shows project and team operating status.
- Includes project list, progress, recent activity, global task board, and large-screen views.
- Can open as a standalone page or an in-app browser tab.

---

## Architecture Overview

Four layers maintain the same work Graph:

- **Intake and data layer**: Inbox, IM channels, data sources, governance, manual tasks, agenda, and business objects turn external information into actionable work signals.
- **Organization and Graph layer**: projects / workspaces, ProjectItems, dependencies, task blueprints, schedule, priority, milestones, and acceptance criteria preserve context, state, and relationships.
- **Scheduling and execution layer**: Schedule activates ready work, 1ACP / ACPX connects replaceable agents, and deterministic functions and humans run through the same task kernel.
- **Verification and write-back layer**: Check, task timelines, TaskRuns, and ProjectEvents record outputs, verification, rework, and state changes, choose the next edge, and write results back into the Graph.

Core objects:

- **Work Graph**: the work facts and execution relationships represented by nodes, states, edges, decisions, and feedback.
- **Project / workspace**: a directory, sessions, tasks, and project configuration.
- **ProjectItem / work item**: a requirement, bug, discussion, or executable task; only task-type items enter scheduling.
- **Task Blueprint**: source requirements, feature nodes, delivery tasks, dependencies, milestones, target versions, and verification state.
- **Executor**: AI agent, deterministic program, or human user.
- **Timeline and execution records**: user feedback, agent replies, session branches, TaskRuns, verification results, and state changes.

Main code structure:

```text
frontend/              Web frontend: workbench, Chat, tasks, files, data sources, settings
backend/               1agents Go backend service
modules/ttyd/          Web terminal service
modules/cc-connect/    Messaging platform and agent bridge
modules/cc-switch-cli/ Agent provider / model configuration switching sidecar
modules/HarnessKit/    Controlled fork for extension inventory, audit, marketplace, adapters, and Kits
modules/1acp/          Agent Client Protocol adapters, examples, and conformance tests
modules/happy-cli/     Happy agent CLI and local launcher packaging source
modules/gstack/        Built-in engineering skills, QA, release, and browser automation workflows
modules/grok-build/    Grok-related agent, CLI, and build components
build/                 Local build outputs
docs/                  Product, design, and architecture docs
```

The repository is organized as a main product plus replaceable execution components and distributable modules: the frontend and backend provide the 1agents workbench; `ttyd` provides terminal capability; `cc-connect`, `cc-switch`, `happy`, `HarnessKit`, and `1acp` provide agent integration, extension management, protocol adapters, and CLI sidecars; npm splits core / web / cc-connect / cc-switch into platform or feature packages.

Related docs:

- [docs/README.md](docs/README.md) — docs index
- [docs/product/1Agents_项目介绍.md](docs/product/1Agents_项目介绍.md) — core product introduction for Agent Infra, the work Graph, and the Task Blueprint
- [docs/product/frontend-product-features.md](docs/product/frontend-product-features.md)
- [docs/design_rules/app-agentic-workbench-design-standard.md](docs/design_rules/app-agentic-workbench-design-standard.md)
- [docs/architecture/agentsOS-架构设计.md](docs/architecture/agentsOS-架构设计.md) — work Graph, task kernel, project shell, and the current App Registry boundary

---

## Prerequisites

Terminal session recovery depends on `tmux`:

```bash
brew install tmux                              # macOS (Homebrew)
sudo apt update && sudo apt install -y tmux    # Ubuntu / Debian
sudo dnf install -y tmux                       # Fedora / CentOS / RHEL
```

---

## Installation

### NPM (recommended · default)

```bash
# Scope: @1agents (same org as @1agents/wire)
npm install -g @1agents/1agents
# China mirror example
npm install -g @1agents/1agents --registry=https://registry.npmmirror.com

1agents [args]
```

Requirements:

- Node.js >= 22
- macOS arm64 / Linux x64 / Linux arm64 (Windows: WSL2 or build from source)

**Distribution (important):**

- **Multi-package** under `@1agents/*`. `@1agents/1agents` is the entry; **`@1agents/core-<plat>` ships `1agents`, `ttyd`, and `hk` directly on the npm registry** (no GitHub download during install).
- Design: [`docs/features/npm-package-split/prd.md`](docs/features/npm-package-split/prd.md).
- Legacy `@scottzx/1agents` and the “thin installer + GitHub tarball” flow are **deprecated**.

### Prebuilt archives (optional, non-npm)

If you do not use npm, download a full tarball from [GitHub Releases](https://github.com/scottzx/1agents/releases). **npm users do not need this path.**

### Docker

```bash
docker run -d -p 8080:8080 \
  -v /path/to/your/workspaces:/workspace \
  --name 1agents scottzx/1Agents:latest
```

### Build From Source

```bash
git clone --recursive https://github.com/scottzx/1Agents.git
cd 1agents_app
make all
```

Common build commands:

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

## Development

Frontend:

```bash
cd frontend
yarn install
yarn start
yarn build
yarn check
```

Backend:

```bash
cd backend
go build ./cmd/backend
```

cc-connect:

```bash
cd modules/cc-connect
make build
go test ./...
```

Submodules:

```bash
git submodule update --init --recursive
make submodules
```

---

## Related Projects

- [cc-connect](https://github.com/scottzx/cc-connect): messaging bridge and agent integration.
- [1Hive](https://00claw.com/): hardware for long-running AI agent workloads.

---

## License

Released under the [MIT License](LICENSE).

---

## Acknowledgements

1agents uses several open-source projects and libraries. Major dependencies include:

**Terminal and Frontend**

- [ttyd](https://github.com/tsl0922/ttyd) (MIT)
- [xterm.js](https://github.com/xtermjs/xterm.js) (MIT)
- [Preact](https://github.com/preactjs/preact) (MIT)
- [Marked](https://github.com/markedjs/marked) (MIT)
- [trzsz](https://github.com/trzsz/trzsz.js) (MIT)
- [webpack](https://github.com/webpack/webpack) (MIT)

**AI and Messaging Bridges**

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) (MIT)
- [discordgo](https://github.com/bwmarrin/discordgo) (BSD-3)
- [go-telegram/bot](https://github.com/go-telegram/bot) (MIT)
- [slack-go](https://github.com/slack-go/slack) (BSD-2)
- [line-bot-sdk-go](https://github.com/line/line-bot-sdk-go) (Apache-2.0)
- [larksuite/oapi-sdk-go](https://github.com/larksuite/oapi-sdk-go) (MIT)
- [dingtalk-stream-sdk-go](https://github.com/open-dingtalk/dingtalk-stream-sdk-go) (Apache-2.0)
- [gorilla/websocket](https://github.com/gorilla/websocket) (BSD-2)

**Agent Tooling and Data Infrastructure**

- [cc-switch](https://github.com/farion1231/cc-switch) (MIT)
- [cc-switch-cli](https://github.com/SaladDay/cc-switch-cli) (MIT)
- [HarnessKit](https://github.com/RealZST/HarnessKit) (Apache-2.0; 1agents uses a controlled fork)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3)
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT)
- [creack/pty](https://github.com/creack/pty) (MIT)
- [robfig/cron](https://github.com/robfig/cron) (MIT)

See [THIRD_PARTY.md](THIRD_PARTY.md) for full third-party dependency notes.
