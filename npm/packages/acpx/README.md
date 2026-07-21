# `@1agents/acpx`

1agents fork of the **acpx** ACP runtime/CLI (from monorepo `modules/1acp`).

This package is **not** the upstream npm `acpx` from OpenClaw. It includes 1agents-specific host extensions used by Chat:

- `_x.ai/ask_user_question` → `onAskUserQuestion`
- `_x.ai/exit_plan_mode` → `onExitPlanMode`

## Install

Usually pulled in as a dependency of `@1agents/acp-bridge` (via `@1agents/1agents`):

```bash
npm install -g @1agents/1agents
```

## Runtime import

```js
import {
  createAcpRuntime,
  createRuntimeStore,
  createAgentRegistry,
} from "@1agents/acpx/runtime";
```

## CLI (optional)

```bash
1agents-acpx --help
```

## Source

Built from `modules/1acp` (`pnpm build` → `dist/`). See `scripts/npm-fill-packages.sh`.
