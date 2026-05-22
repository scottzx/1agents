# Remote Agent

随时随地，通过浏览器远程访问你的 AI 智能体和开发工作台。

Remote Agent 是一个基于 Web 的远程工作台平台，让你打破必须在电脑前才能与 AI 交互的限制。它集成了终端访问、文件管理、Git 操作等功能，只需一个浏览器就能从任何地方连接到你的工作环境，继续对话、编辑代码、管理文件、查看仓库状态 —— 就像你正坐在它面前一样。

终端访问能力基于 [ttyd](https://github.com/tsl0922/ttyd) 构建，继承了其高性能的 Web 终端体验。

## 核心功能

- **Web 终端** —— 基于 xterm.js 和 WebSocket，支持 CJK、IME、文件传输（ZMODEM / trzsz）、Sixel 图像输出
- **文件管理器** —— 树形目录浏览 + 平铺列表视图，支持搜索、筛选、收藏
- **在线编辑器** —— 浏览器内直接查看和编辑文件，支持语法高亮、保存、重命名、下载
- **工作区管理** —— 多工作区切换，CRUD 管理，支持本地文件夹导入
- **Git 面板** —— 快速查看仓库变更状态和分支信息
- **多渠道通信** —— 通过 [cc-connect](https://github.com/scottzx/cc-connect) 集成实现多种通信渠道支持
- **任务列表** —— 实时追踪后台运行的任务
- **主题切换** —— 亮色 / 暗色主题，终端同步适配

## 跨平台

macOS、Linux、Windows 全面支持。

## 快速开始

### macOS

```bash
brew install ttyd
```

### Linux (Debian/Ubuntu)

```bash
sudo apt install ttyd
```

你也可以从 [Releases](https://github.com/scottzx/remote-agents/releases) 页面下载预编译的静态二进制文件。

## 使用方式

```bash
# 启动终端服务
ttyd -p 8080 bash

# 带密码保护
ttyd -p 8080 -c user:pass bash

# 启用 SSL
ttyd -p 8080 -S -C cert.pem -K key.pem bash
```

浏览器打开 `http://localhost:8080` 即可访问完整工作台。

## 从源码构建

```bash
git clone https://github.com/scottzx/remote-agents.git
cd remote-agents
mkdir build && cd build
cmake ..
make
```

## Docker

```bash
docker run -p 8080:8080 scottzx/remote-agents
```

## 许可

本项目基于 [MIT License](LICENSE) 开源。

---

**致谢**：
- 终端能力基于 [ttyd](https://github.com/tsl0922/ttyd) 构建，感谢原作者的卓越工作
- 多渠道通信功能基于 [cc-connect](https://github.com/scottzx/cc-connect) 项目实现，感谢作者对本项目的支持
