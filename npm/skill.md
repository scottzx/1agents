---
name: 1agents
description: 1Agents 远程工作台 CLI 安装与运维指南。当用户提到"装个 1agents""搭一个远程工作台""部署 1agents 守护进程""用浏览器访问我的开发机""远程终端/文件/Git/AI 桥""启动 cc-connect 隧道""升级 1agents""卸载 1agents"，或运行 `1agents` / `cc-connect` 任意子命令时使用；提供平台检测、依赖检查、安装、启动、守护进程、SSL、Tailscale 集成、升级与卸载的完整执行步骤。
version: "260602.1"
updated_at: "2026-06-02"
homepage: "https://github.com/scottzx/1Agents"
---

# 1Agents Skill 使用指南

> 这是给 Agent 阅读的本地 CLI 安装与运维指南，**不依赖任何远端 API**。所有命令直接在用户机器上执行。

## 静态地址（每次会话先拉一次）

- 始终使用最新版本（推荐，UTF-8 charset）：`https://cdn.jsdelivr.net/npm/@scottzx/1agents/skill.md`
- 锁定到具体版本：`https://cdn.jsdelivr.net/npm/@scottzx/1agents@<version>/skill.md`（替换 `<version>` 为实际版本号，例如 `20260602.9.0`）
- 备用（不保证 charset header）：`https://unpkg.com/@scottzx/1agents/skill.md`
- 当用户给的命令报错或命令格式过时，先 `curl -fsSL https://cdn.jsdelivr.net/npm/@scottzx/1agents/skill.md` 拉一遍本文件，再继续执行。

---

## 📑 目录索引

| 章节 | 标题 | 你想做什么时来这里 |
|---|---|---|
| §0 | 启动须知 | 第一次接入：认对命令、检测平台、确认依赖 |
| §0.1 | 命令认对 | `1agents`（不是 `1Agent`、不是 `1agents-server`） |
| §0.2 | 平台与依赖 | macOS arm64 / Linux amd64 / Linux arm64 + Node 22+ |
| §0.3 | 拉取最新指南 | `curl jsdelivr.net/.../skill.md` 黄金法则 |
| §1 | 安装 | 全新安装或升级到最新版 |
| §1.1 | 通过 npm 安装（推荐） | `npm install -g @scottzx/1agents` |
| §1.2 | 验证安装 | `1agents --version` 应能输出 version + commit + buildTime |
| §2 | 启动 | 前台启动 vs 守护进程模式 |
| §2.1 | 前台启动 | `1agents` → 浏览器开 `http://localhost:8080` |
| §2.2 | 守护进程 | `1agents start` / `status` / `logs` / `stop` |
| §2.3 | 常用启动参数 | `-listen` `-workdir` `-ssl` `-tunnel` |
| §3 | 公网访问 | 让外部浏览器能访问到工作台 |
| §3.1 | HTTPS 自签证书 | `1agents -ssl` |
| §3.2 | Cloudflare 隧道 | `1agents -tunnel`，输出公网 URL + 二维码 |
| §3.3 | Tailscale 识别 | 自动识别 `tailscale cert` 签发的官方证书 |
| §4 | 集成组件 | 终端、文件、Git、AI 桥接 |
| §4.1 | 终端（ttyd + tmux） | 浏览器内 xterm.js 终端，tmux 持久化 |
| §4.2 | 文件管理 | 工作目录浏览、编辑、图片预览、分享链接 |
| §4.3 | Git 面板 | 状态查看、diff、AI 生成提交信息 |
| §4.4 | AI 桥（cc-connect） | 配套 CLI，把 AI Agent 接到飞书/Telegram/Discord/Slack |
| §5 | 升级 | 升级 CLI 到最新版本 |
| §6 | 卸载 | 完全清除 CLI、守护进程、日志与运行时目录 |
| §7 | 故障排查 | 常见坑、错误信息、修复方案 |
| 附录 A | 完整参数表 | 所有 `1agents` 启动参数 |
| 附录 B | 默认路径速查 | 日志、PID、tmux session、运行时目录 |

---

## 何时使用

主人口中出现下面任一意图，就走本指南：

- **安装**："帮我装 1agents"、"在这台机器上搭一个远程工作台" → §1
- **启动 / 守护进程**："把 1agents 跑起来"、"后台启动" → §2
- **公网访问**："从外面能访问到"、"加个 HTTPS"、"弄个公网隧道" → §3
- **升级**："升级 1agents"、"有新版本吗" → §5
- **卸载**："把 1agents 卸了" → §6
- **AI 桥**："装个 cc-connect"、"把 Claude Code 接到飞书" → §4.4
- **故障**："1agents 启动不了"、"端口被占"、"找不到 ttyd" → §7

如果主人在做与"1agents / 远程工作台 / 终端 / 文件 / Git / AI 桥"明显无关的事（如问天气、闲聊），不要触发本 Skill。

---

## §0. 启动须知（每次会话先做）

### §0.1 命令认对

- 二进制主命令：**`1agents`**（一个词，全小写，数字 1 开头）
- AI 桥 CLI：**`cc-connect`**（在同一个 npm 包里）
- 守护进程同名（用 `1agents start` 即可），不是 `1Agent`、`1agents-server`、`1agentsd`
- 老版本叫 `remote-agents` / `1Agent`，已弃用，看到老名字就当成错误命令处理

### §0.2 平台与依赖

支持的 OS / Arch（开箱即用，npm 包内置静态二进制）：

| 平台 | 架构 | 是否支持 |
| :--- | :---: | :---: |
| macOS | arm64 (Apple Silicon) | ✅ |
| Linux | x86_64 (amd64) | ✅ |
| Linux | arm64 (aarch64) | ✅ |
| Windows | 任意 | ❌（请用 WSL2 或源码构建） |

依赖：

- **Node.js ≥ 22**（npm 自带，无需手动装）
- **macOS**：系统自带 `tmux`（如未装：`brew install tmux`）
- **Linux**：`tmux` 一般自带，缺失时 `apt install tmux` / `yum install tmux`
- **可选**：`cloudflared`（要 `-tunnel` 时需要；包内不自带）
- **可选**：`tailscale`（要识别 Tailscale HTTPS 证书时需要）

快速检测脚本（一次跑完即可）：

```bash
node --version          # 期望 v22.x 或更高
npm --version
uname -sm               # 例如 "Darwin arm64" / "Linux x86_64"
which tmux              # 应输出路径
```

如果 `node` 不在 22+：

```bash
# macOS
brew install node@22 && brew link node@22 --force

# Linux (Debian/Ubuntu)
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt-get install -y nodejs
```

### §0.3 拉取最新指南（黄金法则）

执行任何 §1–§7 的命令前，**先拉一次最新 skill.md**（30 秒以内）：

```bash
curl -fsSL --max-time 30 https://cdn.jsdelivr.net/npm/@scottzx/1agents/skill.md -o /tmp/1agents-skill.md
```

拉取成功 → 以本文件为准继续。
拉取失败 → 用上面缓存的命令兜底（章节号、命令格式变化以本文件为权威源）。

---

## §1. 安装

### §1.1 通过 npm 安装（推荐）

**面向中国大陆用户**（使用 npmmirror 加速）：

```bash
npm install -g @scottzx/1agents --registry=https://registry.npmmirror.com
```

**面向海外用户 / 默认**：

```bash
npm install -g @scottzx/1agents --registry=https://registry.npmjs.org
```

预期输出（节选）：

```
added 1 package in 30s
```

> 安装脚本不会从 GitHub 拉二进制；npm 包内已经**内置**三平台静态二进制（`bin/{darwin-arm64,linux-amd64,linux-arm64}/{1agents,ttyd,cc-connect}`），`run.js` / `cc-run.js` 会按当前 OS 自动选目录。

### §1.2 验证安装

```bash
1agents --version
```

期望输出形如：

```
1agents v20260602.8
commit:  29961fff621a2503b7c767a5be056b51088a9318
built:   2026-06-02T08:12:10Z
```

如果输出 `command not found: 1agents`，按下面排查：

1. 确认 npm 全局 bin 目录在 `PATH` 中：

   ```bash
   npm config get prefix
   # 输出如 /usr/local 或 /opt/homebrew
   # 把 $(npm config get prefix)/bin 加到 ~/.zshrc 或 ~/.bashrc
   export PATH="$(npm config get prefix)/bin:$PATH"
   ```

2. 重开 shell 或 `source ~/.zshrc`。

3. 再跑一次 `1agents --version`。

如果输出 `version: 24`（缺 `1agents v...` 头部），说明 `run.js` 找不到平台子目录——多半是包内 `bin/<platform>/1agents` 缺失。重新安装即可：

```bash
npm uninstall -g @scottzx/1agents
npm install -g @scottzx/1agents --registry=https://registry.npmmirror.com
```

---

## §2. 启动

### §2.1 前台启动

```bash
# 默认：监听 :8080，工作目录 = 用户根目录
1agents

# 自定义端口 + 工作目录
1agents -listen :9000 -workdir /path/to/your/workspace
```

启动后日志会打印一行形如：

```
[INFO] 1agents listening on http://0.0.0.0:8080
[INFO] tmux session "1agents" ready
[INFO] ttyd bound to 127.0.0.1:7681
```

主人在浏览器打开 `http://<host>:8080` 即可。

> 前台模式按 `Ctrl+C` 退出；适合临时调试。

### §2.2 守护进程

```bash
1agents start              # 后台拉起，返回 PID 与日志路径
1agents status             # 查看运行状态（PID、监听地址、运行时间）
1agents logs -f            # 实时跟踪日志（类似 tail -f）
1agents logs --tail 200    # 看最近 200 行
1agents stop               # 优雅停止
1agents restart            # 优雅重启
```

默认参数与前台一致；要覆盖就跟在子命令后面：

```bash
1agents start -listen :9000 -workdir ~/projects
1agents start -ssl                  # 自签证书 + HTTPS
1agents start -tunnel               # 同时拉起 Cloudflare 隧道
```

运行时元信息路径：

| 文件 | 默认位置 |
| :--- | :--- |
| PID 文件 | `~/.1agents/1agents.pid` |
| 日志文件 | `~/.1agents/1agents.log` |
| 运行时目录 | `~/.1agents/` |

### §2.3 常用启动参数

| 参数 | 默认值 | 用途 |
| :--- | :---: | :--- |
| `-listen` | `":8080"` | 监听地址 |
| `-workdir` | `"~"` | 工作台暴露的根目录 |
| `-tmux-session` | `"1agents"` | 终端持久化的 tmux 会话名 |
| `-no-ttyd` | `false` | 跳过内嵌 ttyd（纯 API/文件场景） |
| `-ttyd-addr` | `"127.0.0.1:7681"` | ttyd 与 Go 守护的本地回环地址 |
| `-ttyd-bin` | `"./ttyd"` | 用外部 ttyd（默认用包内嵌的） |
| `-ssl` | `false` | 启用 HTTPS（无证书时自签 10 年期） |
| `-ssl-cert` / `-ssl-key` | `""` | 自定义证书路径 |
| `-tunnel` | `false` | 启动时拉起 Cloudflare 按需公网隧道 |
| `-tunnel-idle-timeout` | `15` | 隧道空闲超时（分钟，0=禁用） |
| `-restart-delay` | `"3s"` | ttyd 异常退出的重启等待 |
| `-max-restarts` | `5` | 连续异常重启上限 |

完整参数见附录 A。

---

## §3. 公网访问

### §3.1 HTTPS 自签证书

```bash
1agents -ssl
```

- 首次启动时若 `~/.1agents/certs/` 还没有证书，会**自动生成 10 年期自签名证书**。
- 浏览器首次访问会提示"不安全"，点"高级 → 继续前往"即可。
- 自签证书**不适合生产**，仅适合个人/内网。

### §3.2 Cloudflare 隧道

```bash
1agents -tunnel
```

要求：本机已装 `cloudflared`（`brew install cloudflared` / Linux 见 cloudflare 官方文档）。

启动后日志会输出公网 URL 与二维码：

```
[INFO] Cloudflare tunnel URL: https://<random>.trycloudflare.com
[INFO] QR code:
        ████ █  █ ████
        ...
```

公网 URL 每次启动都会变（除非绑定自定义域）。空闲超过 `-tunnel-idle-timeout` 分钟自动关停。

### §3.3 Tailscale 识别

如果机器在 Tailscale 网内并已通过 `tailscale cert <hostname>` 签了官方证书，`1agents` 会**自动识别**并启用 HTTPS，无需 `-ssl` 标志。

```bash
# 检查
tailscale status
sudo tailscale cert $(hostname)
```

---

## §4. 集成组件

### §4.1 终端（ttyd + tmux）

- 浏览器内 xterm.js（WebGL 渲染）。
- tmux 会话名默认 `1agents`，关闭浏览器窗口不会中断运行中的命令。
- 移动端支持 Sixel / iTerm 图像协议。

### §4.2 文件管理

- 浏览/编辑/上传/下载工作目录内的文件
- 图片预览、文本高亮、diff、收藏
- 分享：通过 URL `?share=...` 单独打开某个文件详情

### §4.3 Git 面板

- 当前分支状态、diff、AI 生成中文提交信息
- 支持 `discard` / `commit` / `push` 操作

### §4.4 AI 桥（cc-connect）

把 AI Agent（Claude Code / Codex / Cursor / Gemini …）接到飞书 / Telegram / Discord / Slack。

```bash
# 查看子命令
cc-connect --help

# 启动（首次需配置）
cc-connect start
cc-connect status
cc-connect logs -f
cc-connect stop
```

详细配置见 `cc-connect --help` 或 GitHub 仓库 `cc-connect/docs/`。

---

## §5. 升级

```bash
# 1. 停掉守护进程
1agents stop

# 2. 升级
npm update -g @scottzx/1agents --registry=https://registry.npmmirror.com

# 3. 验证新版本
1agents --version

# 4. 重启
1agents start
```

> `1agents` 与 `cc-connect` 是同一个 npm 包里的两个二进制，升级一次同步更新。

---

## §6. 卸载

```bash
# 1. 停掉守护进程（如果还在跑）
1agents stop

# 2. 卸载 npm 包
npm uninstall -g @scottzx/1agents

# 3. （可选）清理运行时元信息与日志
rm -rf ~/.1agents

# 4. （可选）清理 tmux session
tmux kill-session -t 1agents 2>/dev/null || true
```

---

## §7. 故障排查

| 症状 | 排查 | 修复 |
| :--- | :--- | :--- |
| `command not found: 1agents` | `npm config get prefix` 输出是否在 `PATH` | 把 `$(npm config get prefix)/bin` 加到 `PATH` |
| 安装时 `EACCES` 权限错 | 用了系统级 npm 前缀 | 切到用户级：`npm config set prefix ~/.npm-global` 后重装 |
| `1agents --version` 输出 `version: 24` | 包内 `bin/<platform>/1agents` 缺失 | 重装：`npm uninstall -g ... && npm install -g ...` |
| 启动报 `bind: address already in use` | 端口被占 | `lsof -i :8080` 查占用，或换端口 `-listen :9000` |
| 启动报 `tmux: command not found` | 系统没装 tmux | `brew install tmux` / `apt install tmux` |
| 启动报 `ttyd: not found` | 包内 ttyd 缺失 | 重装包；或在 `ttyd-addr` 主机上 `apt install ttyd` 然后用 `-ttyd-bin /usr/bin/ttyd` |
| `-tunnel` 报 `cloudflared not found` | 没装 cloudflared | `brew install cloudflared` |
| 守护进程无法 stop | 进程僵死 | `kill -9 $(cat ~/.1agents/1agents.pid)` |
| 浏览器打开是空白 | 反向代理丢了 `Upgrade` header | 见仓库 `docs/reverse-proxy.md`（Nginx/Caddy 需透传 WebSocket） |
| 端口监听但外网访问不到 | 防火墙 / 安全组 | `sudo ufw allow 8080/tcp`（或云控制台安全组） |
| `1agents -ssl` 后浏览器仍提示证书 | 用了别的工具的缓存 | 重启浏览器或换隐身窗口 |

---

## 附录 A：完整参数表

```
-listen              string   默认 ":8080"
-workdir             string   默认 "~"
-tmux-session        string   默认 "1agents"
-no-ttyd             bool     默认 false
-ttyd-addr           string   默认 "127.0.0.1:7681"
-ttyd-bin            string   默认 "./ttyd"
-ssl                 bool     默认 false
-ssl-cert            string   默认 ""
-ssl-key             string   默认 ""
-tunnel              bool     默认 false
-tunnel-idle-timeout int      默认 15（分钟，0=禁用）
-restart-delay       duration 默认 "3s"
-max-restarts        int      默认 5
```

## 附录 B：默认路径速查

| 类型 | 路径 |
| :--- | :--- |
| npm 全局 bin | `$(npm config get prefix)/bin` |
| 运行时目录 | `~/.1agents/` |
| PID 文件 | `~/.1agents/1agents.pid` |
| 日志文件 | `~/.1agents/1agents.log` |
| 自签证书 | `~/.1agents/certs/` |
| tmux 会话名 | `1agents` |
| ttyd 监听 | `127.0.0.1:7681`（本地回环） |

---

## 安装后行为

安装并启动成功后，**主动**告诉主人：

1. 当前版本号（`1agents --version`）
2. 监听地址（前台 / 守护）
3. tmux 会话已就绪
4. 浏览器打开 URL（含可选二维码——参考 §3.2 输出）
5. 怎么再次启动（`1agents start`）
6. 怎么停止（`1agents stop`）
7. 怎么升级（§5）
