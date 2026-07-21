# PRD：npm 多包拆分与分发（`@1agents/*`）

| 字段 | 内容 |
|------|------|
| 状态 | **已决策 / 待实现** |
| 版本 | **v1.3** |
| 日期 | 2026-07-18 |
| 负责人 | human（`assignee=user`） |
| 组织 | **npm scope `@1agents`**（已有 `@1agents/wire`） |
| 定位一句话 | **core / 平台二进制直接 `npm publish`；cli 只做编排；默认安装不走 GitHub** |

---

## 0. 分发定位（防歧义 · 必读）

| 说法 | 是否正确 |
|------|----------|
| `@1agents/core-linux-x64` 等包内 **直接带二进制**，用户 `npm i` 即从 **npm registry** 拿到 | ✅ **正确 · 主路径** |
| `@1agents/1agents` 是 **JS 入口**（resolve 已安装的 core/web/skills…），**不**再下载大包 | ✅ **正确** |
| 安装时 `postinstall` 从 **GitHub Release 拉 tar** 再解压（薄安装器） | ❌ **错误 · 已废弃方向** |
| 单体 `@scottzx/1agents` 内嵌三端 / 或 GitHub 下大包 | ❌ **历史方案，不再作为默认** |
| GitHub Release 整包 tar 给桌面 / 离线 / 不用 npm 的人 | ✅ **可选旁路**，不是 npm 用户必经 |

**用户默认路径：**

```bash
npm install -g @1agents/1agents
# → registry 安装 web / skills / cc-connect / cc-switch / 当前 arch 的 core-*
# → 本地 node_modules 内已有二进制，无需访问 GitHub
```

---

## 1. 背景与问题

1. 单体 npm 包体积过大，易 `E413`。
2. 曾尝试「薄安装器 + GitHub 下 tar」——国内网络不稳，且与 npm 镜像生态脱节。
3. JS 侧车 / Python 技能 / 原生二进制生命周期不同，不应硬捆。

**目标：** 以 **npm registry（`@1agents`）为唯一默认分发通道**；平台二进制 **直接上传 registry**；cli 只 resolve 本地包。
---

## 2. 已锁定决策

### 2.1 组织与命名

| # | 决策 | 说明 |
|---|------|------|
| D0 | **统一 scope `@1agents`** | 不再使用 `@scottzx/*` 发新包；与已发布 `@1agents/wire` 一致 |
| D1 | **平台包命名 npm/cpu 惯例** | `@1agents/core-linux-x64`、`@1agents/core-linux-arm64`、`@1agents/core-darwin-arm64`（见 §4 命名表） |
| D2 | **主包名** | **`@1agents/1agents`**（用户装这个；bin: `1agents`）。历史名 `@1agents/cli` 已废弃 |

### 2.2 依赖关系（含 v1.2：cc-switch 独立 deps）

| # | 决策 | 说明 |
|---|------|------|
| D3 | **1skills → `dependencies` 独立包** | **不再**打进平台二进制包；装 `@1agents/1agents` 时 **一并安装** `@1agents/skills` |
| D4 | **1skills 运行时** | 本机 **优先 `uv`，回退 `pip` + venv**；不发 PyInstaller / `_internal` |
| D5 | **frontend → `@1agents/web`** | 只发生产 `dist`，**去掉 `.map`**；主包 `dependencies` |
| D6 | **happy-cli → optional** | `@1agents/happy`；`optionalDependencies`，失败不阻断核心 |
| D7 | **ACP bridge → `@1agents/acp-bridge`（deps）** | 1agents 专用 WebSocket `bridge-server`（`ws://127.0.0.1:38082`）。**不是**上游 `acpx` CLI 包本身——官方 `acpx` **不带** bridge-server |
| D8 | **运行时依赖 `@1agents/acpx`** | `@1agents/acp-bridge` **dependencies** 引入 **`@1agents/acpx`**（1agents fork of acpx，含 Grok `_x.ai/*` host extensions）；bridge 用 `import from "@1agents/acpx/runtime"`。**不要**依赖上游 npm `acpx`（无 ask_user / exit_plan）。Supervisor **禁止**默认 chdir 到不存在的 `modules/1acp` |
| D9 | **cc-connect → `dependencies`** | 装 `@1agents/1agents` 时 **一并安装**；独立包族 + 平台子包（见 §4.3） |
| D9b | **cc-switch-cli → `dependencies`** | **不进 core**；独立 `@1agents/cc-switch` + 平台子包；装 cli **必带**（原生二进制，**不是** uv/pip） |
| D10 | **技能 / 模块面板 embed 随 `@1agents/web`** | skill-manager / cc-connect 前端是独立 ESM（`<skills-panel>` / `<cc-connect-panel>`），**不是** webpack 进主包。发布物路径：`dist/embed/*.js`，运行时优先 `StaticDir/embed/`，禁止只认 monorepo `modules/*/dist-embed` |
| D10 | **主分发 = npm registry** | **core 等平台包直接 `npm publish`**，内含二进制 |
| D11 | **禁止默认 GitHub 下载二进制** | cli **不得** 在 postinstall/首次运行默认去 GitHub 拉 core/tar（已废弃薄安装器主路径） |
| D12 | **GitHub Release 仅为旁路** | 可选：桌面 / 离线整包 tar / 审计；**不**参与 `npm i -g @1agents/1agents` 成功路径 |
| D13 | **cloudflared 不进默认依赖图** | PATH 或首次 `-tunnel` 下载到 `~/.1agents/bin`（与 core 分发无关） |
| D14 | **同版本锁定** | 一次 release 内所有 **`@1agents/*` 自研包** version 一致（含 `@1agents/acpx` / `@1agents/acp-bridge`） |

> 国内加速：靠 **npm 镜像**（如 npmmirror），不是 ghproxy 下 release。
### 2.3 变更摘要

| 项 | v1.0 | v1.1 | v1.2（当前） |
|----|------|------|----------------|
| Scope | `@scottzx` | **`@1agents`** | 同左 |
| 1skills | 塞进平台包 | **独立 `@1agents/skills`，deps** | 同左；运行时 **uv/pip** |
| cc-connect | 塞进平台包 | **独立包族，deps** | 同左 |
| **cc-switch** | 塞进平台/core | 仍在 core | **独立 `@1agents/cc-switch` 包族，deps；不进 core** |
| acpx | 未定 | **独立 dist；optional** | 同左 |
| happy | optional | optional | 同左 |
| web | `@…/1agents-web` | **`@1agents/web`** | 同左 |

---

## 3. 目标与非目标

### 3.1 目标

1. `npm i -g @1agents/1agents`（可用国内 npm 镜像）即可跑通核心工作台。
2. 只装 **当前 arch** 的原生二进制（core / cc-connect / **cc-switch** 平台子包）。
3. 安装时 **自动带上** web + skills + cc-connect + **cc-switch** + **acp-bridge**（→ `@1agents/acpx`）；**尝试** optional happy。

4. JS 侧车一律 **dist-only**；Python skills 用 uv/pip。
5. CI 同版本发布所有 `@1agents/*` 自研包。

### 3.2 非目标 / 反模式

- ❌ GitHub / ghproxy 作为 npm 安装 **core 或其它平台二进制** 的主路径  
- ❌ 「薄安装器」：cli 几乎为空，运行时再下 `1agents-*.tar.gz`  
- ❌ 1skills / cc-connect / cc-switch 再塞回 core  
- ❌ cloudflared 默认进依赖图  
- ❌ Windows 平台包（除非另开需求）  
- ❌ happy / acpx 预打完整 `node_modules` 进 registry  

---

## 4. 包拓扑

### 4.0 总览

```text
@1agents/1agents                              # 主包：run.js / 路径解析 / install / daemon
├── dependencies (安装 1agents 时一并安装)
│   ├── @1agents/web@=VER                 # frontend dist（无 map）
│   ├── @1agents/skills@=VER              # 1skills Python 源码（仅此包用 uv/pip）
│   ├── @1agents/cc-connect@=VER          # meta → 当前平台 cc-connect 二进制
│   │     └── optionalDependencies
│   │           ├── @1agents/cc-connect-linux-x64@=VER
│   │           ├── @1agents/cc-connect-linux-arm64@=VER
│   │           └── @1agents/cc-connect-darwin-arm64@=VER
│   ├── @1agents/cc-switch@=VER           # meta → 当前平台 cc-switch 二进制（不进 core）
│   │     └── optionalDependencies
│   │           ├── @1agents/cc-switch-linux-x64@=VER
│   │           ├── @1agents/cc-switch-linux-arm64@=VER
│   │           └── @1agents/cc-switch-darwin-arm64@=VER
│   └── @1agents/acp-bridge@=VER          # Chat WS bridge
│         └── dependencies
│               └── @1agents/acpx@=VER    # forked ACP runtime (Grok host extensions)
├── optionalDependencies
│   ├── @1agents/core-linux-x64@=VER      # 仅 1agents + ttyd
│   ├── @1agents/core-linux-arm64@=VER
│   ├── @1agents/core-darwin-arm64@=VER
│   └── @1agents/happy@=VER               # happy-cli dist + lock + adapter
└── （已有）@1agents/wire                  # 独立版本线
```

> **说明：** 「运行时 uv/pip」**只适用于 `@1agents/skills`**。`cc-switch` / `cc-connect` 是 **预编译原生二进制**，安装即用。

### 4.1 命名表

| 包名 | 角色 |
|------|------|
| `@1agents/1agents` | 用户安装入口（`1agents` / `cc-connect` / 包装） |
| `@1agents/core-<plat>` | 核心原生：**仅** `1agents`、`ttyd` |
| `@1agents/cc-connect` | cc-connect meta（无大文件） |
| `@1agents/cc-connect-<plat>` | `cc-connect` 单平台二进制 |
| `@1agents/cc-switch` | cc-switch-cli meta（无大文件） |
| `@1agents/cc-switch-<plat>` | `cc-switch` 单平台二进制 |
| `@1agents/web` | 前端静态资源 dist |
| `@1agents/skills` | 1skills Python 源码 + requirements（**仅此包**用 uv/pip） |
| `@1agents/happy` | happy-cli + adapter + package-lock |
| `@1agents/acpx` | ACP CLI/runtime dist（1agents fork of modules/1acp；见 §4.4） |
| `@1agents/acp-bridge` | Chat WebSocket bridge-server（deps `@1agents/acpx`） |
| `@1agents/wire` | 已发布；本 PRD 不改其版本策略 |

**平台后缀：** `linux-x64` | `linux-arm64` | `darwin-arm64`  
（历史 GitHub tar 可仍用 `linux-amd64` 文件名；**npm 包名必须 x64**。）

### 4.2 各包内容与体积目标

| 包 | 内容 | 体积目标 |
|----|------|----------|
| `@1agents/1agents` | JS 入口 only | &lt; 1 MB |
| `@1agents/core-*` | **仅** `1agents` + `ttyd` | 单包尽量 &lt; 50 MB tarball |
| `@1agents/cc-connect-*` | 仅 `cc-connect` 二进制 | 单包尽量 &lt; 40 MB |
| `@1agents/cc-switch-*` | 仅 `cc-switch` 二进制 | 单包尽量 &lt; 30 MB |
| `@1agents/web` | frontend dist **无 map** | 尽量 &lt; 20 MB |
| `@1agents/skills` | skill_manager + requirements + 可选 frontend/dist | ~2–5 MB |
| `@1agents/happy` | dist + bin + package-lock + adapter | 小；`npm ci` 装依赖 |
| `@1agents/acpx` | modules/1acp dist-only（~2 MB 级，含 Grok host extensions） | 小 |
| `@1agents/acp-bridge` | bridge-server.mjs only | 小 |

### 4.3 cc-connect / cc-switch 为何拆出 core

- 用户已定：二者均为 **deps、独立包、装 cli 必带**，可独立演进与升级。
- 体积：core 只留工作台最小集（`1agents` + `ttyd`），单包更易过 npm 限制。
- meta 包用 **optionalDependencies + os/cpu** 拉当前平台二进制（esbuild 模式）。
- **`cc-switch` 是原生二进制**，与 skills 的 uv/pip **无关**。

### 4.4 acpx 包源策略（已选定 **B**）

| 选项 | 状态 |
|------|------|
| **A. 依赖上游 `acpx`** | **弃用** — 上游无 Grok `_x.ai/ask_user_question` / `_x.ai/exit_plan_mode` |
| **B. 发 `@1agents/acpx`** | **当前** — 与 release VER 对齐；fill 自 `modules/1acp` dist |

**集成要求：**

- 运行时 **resolve `@1agents/acpx` 包内 dist**，禁止默认 `modules/1acp` + `npx tsx bridge-server.js`。
- bridge：`import from "@1agents/acpx/runtime"`；publish 顺序：`acpx` → `acp-bridge` → `cli`。
- 生产路径：`node <pkg>/dist/...` 或 `1agents-acpx` bin；开发可用 monorepo `modules/1acp`。

### 4.5 主包 resolve 伪代码

```js
const plat = mapPlatform(); // linux-x64 | linux-arm64 | darwin-arm64
const core = require.resolve(`@1agents/core-${plat}/package.json`);
const ccMeta = require.resolve('@1agents/cc-connect/package.json');
// cc 二进制：由 @1agents/cc-connect 内部再 resolve @1agents/cc-connect-${plat}
const web = require.resolve('@1agents/web/package.json');
const skills = require.resolve('@1agents/skills/package.json');
// optional:
try { require.resolve('@1agents/happy/package.json'); } catch { /* degrade */ }
try { require.resolve('@1agents/acpx/package.json'); } catch { /* degrade */ }
```

---

## 5. 用户旅程

### 5.1 默认

```bash
npm install -g @1agents/1agents
# 国内镜像示例
npm i -g @1agents/1agents --registry=https://registry.npmmirror.com

# 模块运行时环境（venv / happy deps / resolve 校验）
1agents install all
1agents install --check
```

一并得到：`web` + `skills` + `cc-connect`（当前平台）+ `acp-bridge`（→ `@1agents/acpx`）+ `core-*`（optional 成功时）+ 尝试 `happy`。

**两层安装：**

| 命令 | 含义 |
|------|------|
| `npm i -g @1agents/1agents` | registry 包文件 |
| `1agents install all\|skills\|happy\|core\|…` | 本机运行时脚本（幂等） |
| `1agents install --check` | 只诊断 |

入口包名固定为 **`@1agents/1agents`**（bin: `1agents`）。历史名 `@1agents/cli` 已废弃。

### 5.2 1skills

- 源码在 `@1agents/skills`。
- **推荐：** `1agents install skills`（或 `install all`）主动建 venv：`uv` 优先，否则 `pip` + venv（目录建议 `~/.1agents/1skills/.venv`）。
- 兜底：首次技能服务启动时 supervisor 仍可懒建 venv。
- 无 Python ≥ 3.11：日志明确，**核心不退出**。

### 5.3 happy 缺失

- 核心终端、文件、web、cc-connect 包装命令仍可用。
- `1agents install happy` 失败时 Happy 相关入口降级提示。

---

## 6. 运行时行为

| 资源 | 来源 |
|------|------|
| `1agents` / `ttyd` | `@1agents/core-<plat>/bin` |
| `cc-connect` | `@1agents/cc-connect-<plat>/bin` |
| `cc-switch` | `@1agents/cc-switch-<plat>/bin` |
| 前端 static | `@1agents/web/dist` |
| 1skills 源码 | `@1agents/skills` 包根（**uv/pip 仅此处**） |
| happy | `@1agents/happy` + `npm ci` |
| ACP bridge | `@1agents/acp-bridge` + `@1agents/acpx` dist |
| cloudflared | PATH 或 on-demand 下载 |

---

## 7. 发布与 CI

### 7.1 版本

- 自研包：`YYYYMMDD.N.0` 全员一致。
- Git tag：`vYYYYMMDD-N`（Release 可选）。
- 依赖写死精确版本（`=` / 无 `^`），`@1agents/wire` 可例外注明。

### 7.2 发布顺序

```text
1. 构建各平台 core / cc-connect / cc-switch 二进制、web、skills、happy
2. publish @1agents/core-* 、@1agents/cc-connect-* 、@1agents/cc-switch-*
3. publish meta：cc-connect、cc-switch、web、skills、happy、acpx、acp-bridge
4. publish @1agents/1agents（dependencies 指向已存在版本）
5. （可选）GitHub Release 整包 tar
```

### 7.3 门禁

- `@1agents/1agents` pack &lt; 2 MB。
- 单平台 core / cc-connect pack 超阈值 fail。
- 冒烟：`npm i` 后 resolve 到 core + web + skills + cc-connect 当前平台。

---

## 8. 仓库布局（建议）

```text
npm/
  packages/
    cli/
    web/
    skills/
    happy/
    cc-connect/                 # meta
    cc-connect-linux-x64/
    cc-connect-linux-arm64/
    cc-connect-darwin-arm64/
    cc-switch/                  # meta（cc-switch-cli）
    cc-switch-linux-x64/
    cc-switch-linux-arm64/
    cc-switch-darwin-arm64/
    core-linux-x64/             # 仅 1agents + ttyd
    core-linux-arm64/
    core-darwin-arm64/
    acpx/                       # 仅当选择 @1agents/acpx
  scripts/
    publish-all.mjs
    map-platform.mjs
docs/features/npm-package-split/
  prd.md
```

---

## 9. 与其它模块的关系

| 模块 | 策略 |
|------|------|
| **1acp / `@1agents/acpx`** | 独立 dist 包（fork）；supervisor **resolve `@1agents/acp-bridge`** → `@1agents/acpx`，淘汰 `modules/1acp`+tsx 默认路径 |
| **happy-cli** | `@1agents/happy` optional |
| **frontend** | `@1agents/web` deps |
| **1skills** | `@1agents/skills` deps + uv/pip |
| **cc-connect** | `@1agents/cc-connect` + 平台子包 **deps** |
| **cc-switch-cli** | `@1agents/cc-switch` + 平台子包 **deps**（不进 core；**非** uv/pip） |
| **@1agents/wire** | 已存在；不强制与 cli 同版本号 |

---

## 10. 风险

| 风险 | 缓解 |
|------|------|
| optional core 在部分镜像未装上 | 启动强校验 + 打印 `npm i -g @1agents/core-<plat>@VER` |
| cc-connect / core 双平台包漏装 | meta 包 postinstall 检测；CI 装跑冒烟 |
| skills uv/pip 环境差 | 文档 + 分层日志；不 fatal 核心 |
| `@1agents/acpx` 漏填 dist | fill 必须产出 `dist/runtime.js`；publish 前硬校验 |
| scope 权限 | 确认 `@1agents` org publish 权限覆盖所有新包名；**Granular Token 若只勾了 `@1agents/wire`，发布 `core-linux-x64` 会 E404**（npm 用 404 代替 403） |

### npm 首次发布 E404（运维）

`PUT .../@1agents%2fcore-linux-x64` → **404** 时：

1. 包构建/填充一般是成功的；失败在 **registry 权限**。
2. 到 https://www.npmjs.com/settings/~/tokens 新建 **Granular Access Token**：
   - Packages: **Read and write**
   - Organizations: **1agents**（可 publish）
   - Packages 选择：**All packages**，或允许在 `@1agents` 下 **创建新包**
3. 将 token 写入 GitHub secret **`NPM_TOKEN`**
4. 确认账号是 `@1agents` org 成员且具备 publish
5. 仅有 `@1agents/wire` 权限的 token **不能** 发布 `core-*` / `cli` 等新名

---

## 11. 成功标准

1. 仅 npm 镜像、断 GitHub：`npm i -g @1agents/1agents` 可启动并加载 web。
2. 安装图含：`web`、`skills`、`cc-connect`、**`cc-switch`**（+ 当前平台子包）、`core-*`（当前平台）。
3. **无**其它 arch 的 core/cc-connect/cc-switch 平台包；**core 内无 cc-switch**。
4. skills 在独立包内；首次技能启动走 uv 或 pip（**仅 skills**）。
5. 无 happy 时核心仍可用；Chat ACP 需要 `@1agents/acp-bridge` + `@1agents/acpx`（cli deps）。
6. 同次 release 自研 `@1agents/*` version 一致。

---

## 12. 实现阶段（看板）

| 阶段 | 内容 |
|------|------|
| P0 | `@1agents` 包骨架、core 平台包、web、cli resolve、CI 同版本 publish |
| P1 | `@1agents/skills` deps + uv/pip；`@1agents/cc-connect` + **`@1agents/cc-switch`** 平台分包 deps |
| P2 | `@1agents/happy` optional；`@1agents/acpx` + `@1agents/acp-bridge` deps；文档 |
| P3 |（后续）GitHub proxy 兜底、体积精修、退役 submodule 源码启动路径 |

---

## 13. 参考

- `@1agents/wire`（已发布）
- `modules/1acp`（发布为 `@1agents/acpx`，dist-only；bridge 为 `@1agents/acp-bridge`）
- `scripts/build-happy-bundle.sh`、`scripts/package-1skills-python.sh`
- `backend/internal/supervisor/skills.go`、`acpx.go`
- esbuild optional platform packages 模式
