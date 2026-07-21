# @1agents/1agents

User-facing CLI entry (`bin`: `1agents`).

```bash
npm install -g @1agents/1agents
# domestic mirror
npm install -g @1agents/1agents --registry=https://registry.npmmirror.com

# bootstrap module runtimes (1skills venv, happy deps, binary resolve, …)
1agents install all
1agents install skills
1agents install happy
1agents install --check
```

| Step | Meaning |
|------|---------|
| `npm install -g @1agents/1agents` | Pull package files from the npm registry |
| `1agents install …` | Host runtime setup (venv / deps / resolve), idempotent |

Platform binaries ship in `@1agents/core-<plat>` **directly on the npm registry**.
This package only resolves local `node_modules` paths — it does **not** download GitHub Release tarballs.

> Historical package name `@1agents/cli` is **renamed** to `@1agents/1agents`.
