# @scottzx/1agents

随时随地，通过浏览器远程访问你的 AI 智能体和开发工作台。

**1Agents** 是一个基于 Web 的远程工作台平台，让你打破必须在电脑前才能与 AI 交互的限制。它集成了 Web 终端（`ttyd` + `tmux`）、文件管理、Git 操作、原生语音输入以及 AI 智能体消息桥接（`cc-connect`），只需一个浏览器就能从任何地方连接到你的工作环境，继续对话、编辑代码、管理文件、查看仓库状态 —— 就像你正坐在它面前一样。

终端与通信能力基于 [ttyd](https://github.com/tsl0922/ttyd) 和 [cc-connect](https://github.com/scottzx/cc-connect) 构建。

## 安装

通过 npm 全局安装 `@scottzx/1agents` 即可获得 `1agents` 与 `cc-connect` 两个命令行工具：

```bash
npm install -g @scottzx/1agents
```

安装脚本会自动检测你的操作系统与架构，并从 GitHub Releases 下载对应的预编译二进制包（包含 `1agents` 守护进程、`ttyd` 静态程序以及前端 Web 静态资源）。目前支持 macOS (arm64) 与 Linux (amd64 / arm64)。

## 使用

```bash
# 启动远程工作台服务（默认端口 :8080，工作目录为用户根目录 ~）
1agents

# 指定监听端口与工作目录
1agents -listen :9000 -workdir /path/to/your/workspace

# 开启 HTTPS（无证书时自动生成 10 年期自签名证书）
1agents -ssl

# 启动时自动拉起按需公网安全隧道（Cloudflare Web Tunnel）
1agents -tunnel
```

启动后，在浏览器中打开 `http://localhost:8080` (或对应的监听端口) 即可访问完整的工作台。

## 守护进程命令

`1agents` 支持后台守护进程模式，便于长时间运行：

```bash
1agents start              # 后台拉起服务，并返回 PID、监听地址与日志路径
1agents status             # 查看守护进程运行状态
1agents logs -f            # 实时跟踪日志输出（类似 tail -f）
1agents stop               # 优雅停止后台服务
```

日志与运行时元信息默认写入 `~/.1agents/` 目录。

## 常用参数

| 参数 | 类型 | 默认值 | 说明 |
| :--- | :---: | :---: | :--- |
| `-listen` | `string` | `":8080"` | 服务对外监听地址（如 `0.0.0.0:9000`） |
| `-workdir` | `string` | `"~"` | 工作台暴露的文件系统根目录 |
| `-tmux-session` | `string` | `"1agents"` | 终端持久化使用的 tmux 会话名称 |
| `-no-ttyd` | `bool` | `false` | 跳过启动内嵌的 ttyd 进程（开发调试用） |
| `-ttyd-addr` | `string` | `"127.0.0.1:7681"` | 内置 ttyd 与 Go 守护进程的本地回环通信地址 |
| `-ttyd-bin` | `string` | `"./ttyd"` | 外部指定的 ttyd 可执行文件路径 |
| `-ssl` | `bool` | `false` | 启用 HTTPS（无证书时自动生成 10 年期自签名证书；自动识别 Tailscale 官方证书） |
| `-ssl-cert` | `string` | `""` | 自定义 SSL 证书路径 (PEM) |
| `-ssl-key` | `string` | `""` | 自定义 SSL 私钥路径 (PEM) |
| `-tunnel` | `bool` | `false` | 启动时自动拉起 Cloudflare 按需公网安全隧道，并输出公网链接与二维码 |
| `-tunnel-idle-timeout` | `int` | `15` | 公网隧道空闲超时（分钟），0 表示禁用自动停止 |
| `-restart-delay` | `duration` | `"3s"` | ttyd 异常退出后守护进程的自动重启等待间隔 |
| `-max-restarts` | `int` | `5` | ttyd 连续异常重启的上限次数，防止循环崩溃 |

## 平台说明

`@scottzx/1agents` 的 npm 包在以下平台开箱即用：

- macOS Apple Silicon (arm64)
- Linux x86_64 (amd64)
- Linux arm64

Windows 与其他架构请参考 [GitHub 仓库](https://github.com/scottzx/1Agents) 中的源码构建说明。

## 链接

- 仓库主页：https://github.com/scottzx/1Agents
- 完整文档：https://github.com/scottzx/1Agents#readme
- 问题反馈：https://github.com/scottzx/1Agents/issues
- 许可协议：MIT
