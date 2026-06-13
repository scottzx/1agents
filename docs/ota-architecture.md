# 1agents OTA 更新架构

> 本文档锚定 1agents 项目的 OTA（Over-The-Air）更新设计决策。代码改动请同步更新本文档。

## 1. 背景与目标

1agents 是一套**多形态分发**的远程工作台：
- **Web 端**：浏览器访问 `1agents` 提供的 HTML/JS
- **桌面端**：Tauri 2.x 打包的原生应用（macOS / Windows / Linux）
- **裸部署**：用户在自有服务器上跑 `1agents` 二进制（自托管 SaaS 场景）

旧版 OTA 走 **NPM 渠道**（`npm install -g @scottzx/1agents`），存在以下根本问题：

| 问题 | 影响 |
|---|---|
| 依赖全局 npm + Node.js | 自托管用户必须先装 Node；对桌面端（裸二进制）无效 |
| 桌面端无法走 npm | 桌面端无 OTA，必须手动下载 .dmg/.msi 重装 |
| 前端与后端版本强耦合 | webpack 产物版本跟随 npm 包版本 |
| 无原子升级 | `npm install` 期间旧服务挂着 |
| 无签名 | 二进制本身没有完整性校验 |

目标：**3 周内**达成"git tag → GitHub Release → 三端静默更新"完整链路。

## 2. 设计决策

| # | 决策点 | 选择 | 备注 |
|---|---|---|---|
| 1 | CDN | **GitHub Releases** | 复用现有 `.github/workflows/auto-release.yml` |
| 2 | 版本号 | `vYYYYMMDD-N` | 沿用 `auto-release.yml` `prepare` job 现有算法 |
| 3 | Channel | V1 仅 `stable` | manifest 预留 `rollout_percent` / `min_supported` 字段接口 |
| 4 | 桌面端签名 | **V1 跳过** | `pubkey: ""` + 大字 TODO；公网分发前必须补 |
| 5 | Go 自更新 | **做**，覆盖裸进程 + systemd | Docker 场景 README 注明 `docker pull` + 容器重启 |
| 6 | 自动回滚 | V1 不做 | manifest 留 `previous[]` 字段供手动切 |

## 3. 架构总览

```
                              GitHub Releases (auto-release.yml)
                                          │
        ┌─────────────────────────────────┼─────────────────────────────┐
        │                                 │                             │
   ┌────▼────┐                       ┌─────▼─────┐                 ┌─────▼─────┐
   │ Web 浏览器│                       │ Tauri 桌面 │                 │ 裸部署 Go  │
   │          │                       │           │                 │           │
   │ /api/ota/manifest               │ tauri-    │                 │ /api/system/version
   │  → 预下载 chunk + 提示刷新       │  plugin-  │                 │  → 平台 binary
   │                                 │  updater  │                 │  → 替换 + 重启
   │                                 │ (V1 无签名)│                │  (systemd/exec)
   └──────────┘                       └─────┬─────┘                 └──────┬────┘
                                           │ sidecar                     │
                                           │ spawn                       │
                                           └─────────┬───────────────────┘
                                                     │
                                              ┌──────▼──────┐
                                              │  1agents    │
                                              │  Go sidecar │
                                              │  (or 裸进程) │
                                              └─────────────┘
```

## 4. 更新协议

### 4.1 根 manifest（`manifest.json`）

Web 前端和 Go 自更新都拉这一个：

```json
{
  "channel": "stable",
  "released_at": "2026-06-15T10:00:00Z",
  "min_supported": "0.3.0",
  "components": {
    "frontend": {
      "version": "0.4.0",
      "entry": "https://github.com/<owner>/<repo>/releases/download/v20260615-1/frontend-v20260615-1.tar.gz",
      "integrity": "sha256-..."
    },
    "backend": {
      "version": "0.4.0",
      "platforms": {
        "darwin-arm64":  { "url": "...", "size": 12345678, "sha256": "..." },
        "darwin-amd64":  { "url": "...", "size": 12345678, "sha256": "..." },
        "linux-amd64":   { "url": "...", "size": 12345678, "sha256": "..." },
        "linux-arm64":   { "url": "...", "size": 12345678, "sha256": "..." },
        "windows-amd64": { "url": "...", "size": 12345678, "sha256": "..." }
      }
    }
  },
  "previous": [
    { "version": "v20260608-2", "url": "https://github.com/<owner>/<repo>/releases/download/v20260608-2/manifest.json" }
  ]
}
```

> `previous[]` V1 不消费，仅为未来手动回滚留接口。

### 4.2 Tauri per-platform manifest

Tauri 2.x 的 `tauri-plugin-updater` 端点要求**每个 target/arch 独立一份**：

```json
{
  "version": "0.4.0",
  "notes": "修复 OAuth 登录；新增多窗口",
  "pub_date": "2026-06-15T10:00:00Z",
  "platforms": {
    "darwin-aarch64": { "url": ".../1Agents_aarch64.dmg" },
    "darwin-x86_64":  { "url": ".../1Agents_x64.dmg" },
    "linux-x86_64":   { "url": ".../1Agents_amd64.AppImage" },
    "windows-x86_64": { "url": ".../1Agents_x64-setup.exe" }
  }
}
```

> V1 `signature` 字段为空（Tauri 2.x 允许跳过签名验证，仅 console warning）。

## 5. 端点契约

### 5.1 后端保留端点

| 路径 | 方法 | 用途 | 前端是否使用 |
|---|---|---|---|
| `/api/system/version` | GET | 当前版本 + 远端最新版本 + `has_update` | 暂未使用（保留供 Settings 接入） |
| `/api/system/update` | POST | 触发 OTA 更新（异步，202 Accepted） | 暂未使用 |
| `/api/system/update/status` | GET | 更新进度实时日志 | 暂未使用 |
| `/api/ota/manifest` | GET | 返回根 manifest（前端 OTA 用） | Week 1 新增 |

> `/api/system/*` 三个端点的**路径与请求/响应 schema 保持不变**，仅替换底层实现（NPM → GitHub Releases）。

### 5.2 三层更新矩阵

| 层 | 部署场景 | 更新通道 | 用户感知 |
|---|---|---|---|
| **Frontend** | Web 用户 | `/api/ota/manifest` → 预下载 + banner 提示刷新 | 横幅 + 手动刷新 |
| **Tauri Desktop** | 桌面端用户 | `tauri-plugin-updater` 端点 | 自动弹窗 → 静默下载 → 自动重启 |
| **Go Backend** | 自托管 SaaS | `system.go` 拉 `manifest.json` → `selfupdate.Apply` | 后台执行，前端可看进度日志 |

## 6. 重启模式

`backend/internal/system/system.go` 已实现三种重启检测，**V1 直接复用**：

| 模式 | 触发条件 | 实现 |
|---|---|---|
| `systemd` | Linux + `systemctl is-active 1agents` 成功 | `systemctl restart 1agents`（detached） |
| `exec` | Unix（macOS / 无 systemd 的 Linux） | `syscall.Exec` 替换当前进程为新二进制 |
| `manual` | Windows / 其他 | 下载完成后返回，前端提示用户手动重启 |

## 7. 安全

- **HTTPS only**：所有 manifest / asset URL 走 GitHub `releases/download/`
- **Tauri 签名**：V1 跳过；TODO 注释提醒发布前必须补
- **Go 二进制**：SHA256 校验（写在 manifest），可选 cosign
- **前端**：暂不强制 SRI（HTTP chunk 加载依赖 webpack runtime hash）

## 8. Docker 用户

Docker 部署**不参与自动 OTA**。需要在 README 注明：

```bash
docker pull <owner>/1agents:latest
docker compose restart 1agents
```

后续若需要容器内自更新，可加 `watchtower` 或在容器内挂 sidecar 进程，但**V1 不做**。

## 9. 发版流程

1. 开发者本地：`git tag v20260615-1 && git push --tags`
2. 触发 `.github/workflows/auto-release.yml`（或手动 `gh workflow run auto-release.yml`）
3. CI 跑完所有 `build-*` job，收集 artifact
4. `release` job 跑 `scripts/build-manifest.py` + `scripts/build-tauri-manifest.py` 生成 manifest
5. `softprops/action-gh-release@v2` 上传所有 manifest + binary 到同一个 release
6. 三端下次检查时拉新版本

## 10. 紧急回滚（V1 手工）

V1 无自动回滚。紧急回滚步骤：

1. 找到上一个稳定版本的 release tag（`manifest.json` 的 `previous[]` 字段记录）
2. 编辑 GitHub Release 描述，更新 "latest" 指引到上一个 release
3. 后端 selfupdate 客户端会按新指引下载旧版本
4. Tauri 用户需手动下载旧 .dmg/.msi 重装

后续 V2 计划加自动回滚（连续 crash 检测 + 切回上一版本）。
