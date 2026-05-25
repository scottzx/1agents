# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Remote Agent is a Web-based remote workbench integrating terminal access (xterm.js + ttyd), file management, and AI agent communication through cc-connect.

### Key Dependency: Yarn 3.6.3 with node-modules Linker

**The frontend (html/) uses Yarn 3 with PnP disabled** (`nodeLinker: node-modules` in `.yarnrc.yml`). This is important:
- Use `yarn install` (not npm/pnpm) to install dependencies
- Yarn 3 is specified via `packageManager: yarn@3.6.3` in package.json
- Enable Corepack: `corepack enable` if `yarn --version` doesn't show 3.6.3

## Build Commands

### Frontend (html/)
```bash
cd html
yarn install        # Install dependencies (Yarn 3.6.3)
yarn start          # Dev server with hot reload
yarn build          # Production build with webpack + gulp (generates html.h)
yarn check          # gts type checking
yarn fix            # gts auto-fix
```

### Backend (agent/)
```bash
cd agent
go build ./...       # Build main server
./remote-agents      # Run the agent server
```

### cc-connect (used by agent/)
```bash
cd cc-connect
go build ./...       # Build all packages
go test ./...        # Run tests
make build           # Full build with selective compilation
```

### Terminal Server (src/)
```bash
mkdir build && cd build
cmake ..
make                 # Builds ttyd native binary
```

## Architecture

```
remote-agents/
├── html/            # TypeScript/React frontend (xterm.js, preact)
│   ├── src/         # Components: app.tsx, drawer/, sidebar/, terminal/
│   ├── dist/        # Built assets (not committed)
│   └── gulpfile.js  # Generates src/html.h via gzip compression
├── agent/           # Go agent server (main entry point)
│   ├── cmd/         # CLI entry points
│   ├── internal/    # Core logic: server, terminal, gateway, ccconnect
│   └── remote-agents # Compiled binary
├── cc-connect/      # Go bridge between AI agents and messaging platforms
│   ├── agent/       # Claude Code, Codex, Cursor, Gemini, etc.
│   ├── platform/    # Feishu, Telegram, Discord, Slack, etc.
│   ├── core/        # Engine, interfaces, i18n, cards
│   └── daemon/      # systemd/launchd integration
└── src/             # C terminal server (ttyd-based)
    └── server.c     # Main server logic
```

### cc-connect Design Principles
- `core/` is the nucleus - never imports from `agent/` or `platform/`
- Plugin architecture via registries (RegisterAgent/RegisterPlatform in init())
- Dependency flow: cmd/ → config/, core/ → agent/*, platform/*