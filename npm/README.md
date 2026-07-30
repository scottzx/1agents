# `@1agents/1agents`（及关联包）

> 分发说明以 [`docs/features/npm-package-split/prd.md`](../docs/features/npm-package-split/prd.md) 为准。

## 安装（推荐）

```bash
npm install -g @1agents/1agents
# 国内镜像
npm install -g @1agents/1agents --registry=https://registry.npmmirror.com

# 模块运行时（HarnessKit / happy deps / 二进制校验）
1agents install all
1agents install --check
```

| 命令 | 含义 |
|------|------|
| `npm install -g @1agents/1agents` | 从 registry 拉包文件 |
| `1agents install …` | 本机运行时环境（幂等） |

## 分发定位（勿与旧方案混淆）

| 正确 | 错误（已废弃） |
|------|----------------|
| `@1agents/core-linux-x64` 等 **直接 `npm publish`，包内带二进制** | cli「薄安装器」再从 **GitHub Release 下 tar** |
| 安装只访问 **npm registry**（可用国内镜像） | 安装依赖 GitHub 网络 |
| `@1agents/1agents` 做 **resolve 已安装包 + 启动 + `install` 编排** | 单体 `@scottzx/1agents` 内嵌三端 |

装 `@1agents/1agents` 时 registry 会拉（同版本）：

- **deps：** `@1agents/web`、`@1agents/cc-connect`（+ 当前平台子包）、`@1agents/cc-switch`（+ 当前平台子包）、`@1agents/acp-bridge`（→ `@1agents/acpx` fork runtime）
- **optional：** `@1agents/core-<plat>`（`1agents` + `ttyd` + `hk`）、`@1agents/happy`

`cloudflared` 不进依赖图（`-tunnel` 时按需）。  
HarnessKit 的 `hk` 随平台 core 包分发，不再需要 Python/venv。

## 使用

```bash
1agents
1agents -listen :9000 -workdir /path/to/workspace
1agents -ssl
1agents -tunnel
1agents install all
1agents install harnesskit
1agents install happy
1agents install --check
```

## 可选：不用 npm 时

从 GitHub Releases 下整包 tar（桌面/离线）。**npm 用户无需此路径。**

## 历史

- `@scottzx/1agents` 与「postinstall 拉 GitHub tar」为历史试验，**不再作为默认文档路径**。
- 入口包曾用名 `@1agents/cli`，已更名为 **`@1agents/1agents`**。
