# 1agents

**1agents** is an open-source, self-hosted AI-native work operating system. It puts tasks, context, executors, verification, and write-back into one loop, so one person can organize an AI team that keeps work moving.

Traditional AI assistants improve one prompt or one chat turn. 1agents focuses on the full path from work entering the system to work being verified as done. It brings Inbox, IM, data sources, projects, tasks, agent sessions, terminals, files, agenda, and app extensions together not as a pile of features, but as a way to capture work, structure it, execute it, verify it, and preserve the context that made it happen.

[简体中文](README.md) | **English**

[![NPM](https://img.shields.io/badge/npm-@1agents%2Fcli-blue?logo=npm)](https://www.npmjs.com/org/1agents)
[![Platform Support](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-brightgreen)](https://github.com/scottzx/1Agents)
[![License](https://img.shields.io/github/license/scottzx/1Agents)](LICENSE)

---

## Product Direction

1agents is not just a larger Chat panel. It is a work loop:

1. **Work enters the system**: ideas, requirements, bugs, reminders, and business objects can come from Inbox, IM, data sources, manual entry, agenda events, or app modules and become project issues or tasks.
2. **The system organizes work**: Project / Task / Issue carries context, dependencies, schedule, priority, acceptance criteria, subtasks, and templates, so the system knows what should happen next.
3. **Executors complete the work**: AI agents, deterministic programs, and humans are routed by task type. Judgment and open-ended execution go to agents, fixed repeatable steps go to functions, and final responsibility stays with humans.
4. **Results close the loop**: discussions, sessions, tool calls, code diffs, verification, rework, and status changes all return to the task timeline, becoming context for the next run and data for future metrics.

The goal is to move AI beyond "help me with this step" and into the collaboration chain: once work enters the system, it can keep moving, expose blockers explicitly, hand results back for human confirmation, and turn repeated experience into reusable capability.

---

## Main Features

### Agent Workbench

- Access a local or remote 1agents backend from the Web frontend.
- Support direct local connection and Relay-based remote connection.
- Support Access Token gate and Relay pairing gate.
- Desktop layout with workspace tree, context header, main workbench canvas, and side content panel.
- Mobile layout with single-column navigation and mobile Chat, terminal, and settings views.
- Support tabs, built-in browser, file preview, global search, and resizable panes.

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
- Support initial command, tmux mouse mode, light/dark terminal themes, and mobile terminal adaptation.

### PMO, Tasks, and Roadmap

- Manage discussions, requirements, tasks, bugs, AI suggestions, and milestones inside each project.
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
- Support governance table detail, dependency relations, and execution records.

### Contacts, Messages, Inbox, and Agenda

- Aggregate contact identities from external channels.
- Contact table supports search, grouping, detail view, and editing.
- Message view shows external-channel messages and group-chat content.
- Inbox captures text or URLs, marks items read/unread, archives items, and dispatches items to projects.
- Agenda aggregates reminders, project tasks, and milestone dates in a calendar view.

### Assistants, Skills, and Agent Capabilities

- Assistant pages manage Assistant workspaces.
- Assistant detail includes sessions, team, tasks, files, channels, and settings.
- Team and skill pages manage skills attached to a project or assistant.
- Integrated 1skills panel manages Skills, Agents, Slash Commands, MCP, and Marketplace entries.
- Agent Catalog shows available agent types and install commands.

### App Extensions

- Support App Manifest and mount points.
- Apps can contribute top-level pages, project tabs, and lens overlays.
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

1agents organizes work as a four-layer loop:

- **Intake layer**: Inbox, IM channels, data sources, manual tasks, agenda, and app business objects turn external information into actionable work.
- **Organization layer**: projects / workspaces, Task / Issue, dependencies, schedule, priority, milestones, and project configuration preserve context and decide the next step.
- **Execution layer**: AI agents, deterministic functions, and humans run through the same task kernel and are selected by task type.
- **Closure layer**: the task timeline records discussions, sessions, outputs, verification, rework, completion state, and metrics so results are traceable, recoverable, and improvable.

Core objects:

- **Project / workspace**: a directory, sessions, tasks, and project configuration.
- **Task**: executable work with dependencies, schedule, status, and acceptance criteria.
- **Executor**: AI agent, deterministic program, or human user.
- **Timeline**: user feedback, agent replies, session branches, verification results, and status changes.

Main code structure:

```text
frontend/        Web frontend: workbench, Chat, tasks, files, data sources, settings
backend/         1agents backend service
modules/ttyd/    Web terminal service
modules/cc-connect/  Messaging platform and agent bridge
build/           Local build outputs
docs/            Product, design, and architecture docs
```

Related docs:

- [docs/frontend-product-features.md](docs/frontend-product-features.md)
- [docs/design_rules/app-agentic-workbench-design-standard.md](docs/design_rules/app-agentic-workbench-design-standard.md)
- [docs/agentsOS-架构设计.md](docs/agentsOS-架构设计.md)

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

- **Multi-package** under `@1agents/*`. `@1agents/1agents` is the entry; **`@1agents/core-<plat>` ships binaries directly on the npm registry** (no GitHub download during install).
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
make frontend
make ttyd
make cc-connect
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
- [skill-manager](https://github.com/mode-io/skill-manager) (MIT)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3)
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT)
- [creack/pty](https://github.com/creack/pty) (MIT)
- [robfig/cron](https://github.com/robfig/cron) (MIT)

See [THIRD_PARTY.md](THIRD_PARTY.md) for full third-party dependency notes.
