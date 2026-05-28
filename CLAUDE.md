# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Remote Agent is a Web-based remote workbench integrating terminal access (xterm.js + ttyd), file management, and AI agent communication through cc-connect.

### Key Dependency: Yarn 3.6.3 with node-modules Linker

**The frontend (html/) uses Yarn 3 with PnP disabled** (`nodeLinker: node-modules` in `.yarnrc.yml`). This is important:
- Use `yarn install` (not npm/pnpm) to install dependencies
- Yarn 3 is specified via `packageManager: yarn@3.6.3` in package.json
- Enable Corepack: `corepack enable` if `yarn --version` doesn't show 3.6.3

## Build & Package Workflow

### 1. Unified Root Build System (Recommended)
The project features a root `Makefile` that orchestrates compilation and local deployment packaging for all components:
```bash
make help               # Display build target details and active host info
make all                # Build all components (frontend, ttyd, cc-connect, agent)
make frontend           # Build frontend assets (html/) & generate src/html.h
make ttyd               # Compile terminal server natively on the current host
make cc-connect         # Compile cc-connect bridge daemon (incl. web assets)
make cc-connect-noweb   # Compile cc-connect (WITHOUT rebuilding web assets)
make agent              # Compile remote-agents Go server with metadata ldflags
make package            # Bundle binaries and assets into a target-named archive in dist/
make clean              # Clean all intermediate and built assets across directories
```

### 2. Component Development Builds

#### Frontend (html/)
```bash
cd html
yarn install        # Install dependencies (Yarn 3.6.3)
yarn start          # Dev server with hot reload
yarn build          # Production build with webpack + gulp (generates html.h)
yarn check          # gts type checking
yarn fix            # gts auto-fix
```

#### Backend (agent/)
```bash
cd agent
go build ./cmd/agent # Build Go agent server
./remote-agents      # Run the agent server
```

#### cc-connect (daemon/)
```bash
cd cc-connect
make build           # Full build with selective compilation (incl. web)
make build-noweb     # Build cc-connect daemon without web assets rebuild
go test ./...        # Run unit tests
```

#### Terminal Server (src/)
```bash
cmake -DCMAKE_BUILD_TYPE=Release -B build-ttyd -S . # Configure build natively
make -C build-ttyd                                  # Compile native ttyd C binary
```

## Binary Versioning & Hostname Philosophy

To ensure that binaries compiled on different environments (such as Mac vs Linux) are easily distinguishable even when using the same commit hash:
1. **Metadata Injection**: Go components (`remote-agents`, `cc-connect`) and the C terminal server (`ttyd`) inject host details (`OS`, `Arch`, `Hostname`) during the compile phase.
2. **Version Commands**:
   - `remote-agents -version` prints standard version, commit with OS/Arch/Hostname, and build time.
   - `cc-connect --version` prints matching version, commit, and build time.
   - `ttyd --version` prints C server version with OS/Arch/Hostname details.
3. **Packaging Names**: `make package` copies all compiled components into a structured directory named `dist/remote-agents-$(VERSION)-$(OS)-$(ARCH)-$(HOSTNAME)/` and compresses it to a target-named `.tar.gz` archive to guarantee distinct, self-describing build outputs.

## Architecture

```
remote-agents/
├── build/           # Centralized output directory for all compiled binaries
│   ├── remote-agents# Go agent server binary (compiled)
│   ├── ttyd         # Native C terminal server binary (compiled)
│   └── cc-connect   # Go bridge daemon binary (compiled)
├── html/            # TypeScript/React frontend (xterm.js, preact)
│   ├── src/         # Components: app.tsx, drawer/, sidebar/, terminal/
│   ├── dist/        # Built assets (not committed)
│   └── gulpfile.js  # Generates src/html.h via gzip compression
├── agent/           # Go agent server (main entry point source code)
│   ├── cmd/         # CLI entry points
│   └── internal/    # Core logic: server, terminal, gateway, ccconnect
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