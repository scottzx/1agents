# @scottzx/remote-agent

随时随地，通过浏览器远程访问你的 AI 智能体和开发工作台。

Remote Agent 是一个基于 Web 的远程工作台平台，让你打破必须在电脑前才能与 AI 交互的限制。它集成了终端访问、文件管理、Git 操作等功能，只需一个浏览器就能从任何地方连接到你的工作环境，继续对话、编辑代码、管理文件、查看仓库状态 —— 就像你正坐在它面前一样。

终端与通信能力基于 [ttyd](https://github.com/tsl0922/ttyd) 和 [cc-connect](https://github.com/scottzx/cc-connect) 构建。

## 安装

你可以通过 npm 全局安装 `remote-agents`：

```bash
npm install -g @scottzx/remote-agents
```

安装脚本会自动检测您的操作系统和架构，并从 GitHub Releases 自动下载对应的预编译二进制包（包含 `remote-agent`、`ttyd` 静态程序和前端 Web 静态资源）。

## 使用

```bash
# 启动远程工作台服务（默认端口 :8080）
remote-agent

# 指定监听端口和工作目录
remote-agent -listen :9000 -workdir /path/to/your/workspace
```

启动后，在浏览器中打开 `http://localhost:8080` (或对应的监听端口) 即可访问完整的工作台。

## 常用参数

- `-listen string`：服务对外监听地址（默认 `":8080"`）
- `-workdir string`：工作台暴露的文件系统根目录（默认 `"."`）
- `-tmux-session string`：用于终端持久化的 tmux 会话名称（默认 `"remote-agents"`）
- `-no-ttyd`：跳过启动内嵌的 ttyd 进程

更多详情与文档请参考 GitHub 仓库：https://github.com/scottzx/remote-agents
