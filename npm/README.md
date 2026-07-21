# `@1agents/cli`（及关联包）

> 分发说明以 [`docs/features/npm-package-split/prd.md`](../docs/features/npm-package-split/prd.md) 为准。

## 安装（推荐）

```bash
npm install -g @1agents/cli
# 国内镜像
npm install -g @1agents/cli --registry=https://registry.npmmirror.com
```

## 分发定位（勿与旧方案混淆）

| 正确 | 错误（已废弃） |
|------|----------------|
| `@1agents/core-linux-x64` 等 **直接 `npm publish`，包内带二进制** | cli「薄安装器」再从 **GitHub Release 下 tar** |
| 安装只访问 **npm registry**（可用国内镜像） | 安装依赖 GitHub 网络 |
| `@1agents/cli` 只做 **resolve 已安装包 + 启动** | 单体 `@scottzx/1agents` 内嵌三端 |

装 `@1agents/cli` 时 registry 会拉（同版本）：

- **deps：** `@1agents/web`、`@1agents/skills`、`@1agents/cc-connect`（+ 当前平台子包）、`@1agents/cc-switch`（+ 当前平台子包）、`@1agents/acp-bridge`（→ `@1agents/acpx` fork runtime）
- **optional：** `@1agents/core-<plat>`（`1agents` + `ttyd`）、`@1agents/happy`

`cloudflared` 不进依赖图（`-tunnel` 时按需）。  
1skills 仅 `@1agents/skills`：本机 **uv 优先 / pip 回退**。

## 使用

```bash
1agents
1agents -listen :9000 -workdir /path/to/workspace
1agents -ssl
1agents -tunnel
```

## 可选：不用 npm 时

从 GitHub Releases 下整包 tar（桌面/离线）。**npm 用户无需此路径。**

## 历史

- `@scottzx/1agents` 与「postinstall 拉 GitHub tar」为历史试验，**不再作为默认文档路径**。
