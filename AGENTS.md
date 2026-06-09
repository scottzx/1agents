# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Behavioral Guidelines

These are universal behavioral principles to reduce common LLM coding mistakes. They apply to all work in this project unless explicitly overridden by a user instruction.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make them pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

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
make all                # Build all components (frontend, ttyd, cc-connect, backend)
make frontend           # Build frontend assets (html/) & generate src/html.h
make ttyd               # Compile terminal server natively on the current host
make cc-connect         # Compile cc-connect bridge daemon (incl. web assets)
make cc-connect-noweb   # Compile cc-connect (WITHOUT rebuilding web assets)
make backend            # Compile 1agents Go server (backend) with version ldflags
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

#### Backend (backend/)
```bash
cd backend
go build ./cmd/backend # Build Go backend server
./1agents      # Run the backend server
```

#### cc-connect (daemon/)
```bash
cd cc-connect
make build           # Full build with selective compilation (incl. web)
make build-noweb     # Build cc-connect daemon without web assets rebuild
go test ./...        # Run unit tests
```

#### Terminal Server (modules/ttyd/)
```bash
cmake -DCMAKE_BUILD_TYPE=Release -B build-ttyd -S modules/ttyd # Configure build natively
make -C build-ttyd                                  # Compile native ttyd C binary
```

## Binary Versioning & Hostname Philosophy

To ensure that binaries compiled on different environments (such as Mac vs Linux) are easily distinguishable even when using the same commit hash:
1. **Metadata Injection**: Go components (`1agents`, `cc-connect`) and the C terminal server (`ttyd`) inject host details (`OS`, `Arch`, `Hostname`) during the compile phase.
2. **Version Commands**:
   - `1agents -version` prints standard version, commit with OS/Arch/Hostname, and build time.
   - `cc-connect --version` prints matching version, commit, and build time.
   - `ttyd --version` prints C server version with OS/Arch/Hostname details.
3. **Packaging Names**: `make package` copies all compiled components into a structured directory named `dist/1agents-$(VERSION)-$(OS)-$(ARCH)-$(HOSTNAME)/` and compresses it to a target-named `.tar.gz` archive to guarantee distinct, self-describing build outputs.

## Architecture

```
1agents/
├── build/           # Centralized output directory for all compiled binaries
│   ├── 1agents# Go agent server binary (compiled)
│   ├── ttyd         # Native C terminal server binary (compiled)
│   └── cc-connect   # Go bridge daemon binary (compiled)
├── html/            # TypeScript/React frontend (xterm.js, preact)
│   ├── src/         # Components: app.tsx, drawer/, sidebar/, terminal/
│   ├── dist/        # Built assets (not committed)
│   └── gulpfile.js  # Generates modules/ttyd/src/html.h via gzip compression
├── backend/         # Go server (backend main entry point source code)
│   ├── cmd/         # CLI entry points
│   └── internal/    # Core logic: server, terminal, gateway, ccconnect
└── modules/         # Reusable service modules
    ├── ttyd/        # C terminal server (ttyd-based)
    │   └── src/     # Main server logic
    └── cc-connect/  # Go bridge between AI agents and messaging platforms
        ├── agent/   # Codex, Codex, Cursor, Gemini, etc.
        ├── platform/# Feishu, Telegram, Discord, Slack, etc.
        ├── core/    # Engine, interfaces, i18n, cards
        └── daemon/  # systemd/launchd integration
```

### cc-connect Design Principles
- `core/` is the nucleus - never imports from `agent/` or `platform/`
- Plugin architecture via registries (RegisterAgent/RegisterPlatform in init())
- Dependency flow: cmd/ → config/, core/ → agent/*, platform/*