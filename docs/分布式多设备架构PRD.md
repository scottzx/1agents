# Remote Agents 分布式多设备架构 PRD

## 1. 背景与目标

### 1.1 背景

当前 `remote-agents` 项目已实现单机版的 AI 远程工作台，包含终端访问、文件管理、Git 操作、工作区管理等功能。

随着 AI Agent 工作负载增长，单机资源（CPU/内存/会话数）有限，需要将工作负载分布到多台设备上。

### 1.2 目标

- 将 `remote-agents` 扩展为**分布式多设备协同平台**
- 设备通过 **Tailscale 虚拟局域网** 互联（纯 Tailscale 直连，无需穿透 NAT）
- 用户可在**母机**上管理多台**子设备**，将任务派发给子设备执行
- 通过**飞书 Bot** 实现任务派发（艾特机器人），构建可观测的 Agent 排班与任务调度系统

### 1.3 网络拓扑

```
用户浏览器 ←→ 母机 (remote-agent + cc-connect)
                    ↓ Tailscale 直连
        ┌───────────┼───────────┐
        ↓           ↓           ↓
   子设备 A      子设备 B      子设备 C
 (Linux)      (macOS)      (Windows)
 remote-agent  remote-agent  remote-agent
 + 5 Bots      + 5 Bots     + 5 Bots
```

---

## 2. 系统架构

### 2.1 组件关系

| 组件 | 位置 | 职责 |
|------|------|------|
| remote-agent | 母机 + 每台子设备 | 终端、工作区、文件管理、HTTP API |
| cc-connect | 母机 + 每台子设备 | 飞书 Bot 与消息通道集成 |
| 飞书 | 消息层 | 任务派发（艾特 Bot）、状态通知 |
| 排班系统 | 母机 | 管理所有 Bot 的注册与状态 |
| 任务系统 | 母机 | 管理任务的生命周期与工作流 |

### 2.2 通信机制

- **子设备 → 母机**：子设备启动后主动向母机注册自己的 Bot 列表（通过 HTTP）
- **母机 → 子设备**：通过 Tailscale IP + 固定端口直接调用子设备的 REST API
- **飞书 → 子设备**：cc-connect 监听飞书消息，转发给子设备的 remote-agent
- **子设备 → 飞书**：cc-connect 将执行结果通过 Bot 发送回飞书群

### 2.3 固定端口约定

所有 `remote-agent` 实例使用固定端口 `7681`，通过 Tailscale IP + 端口访问。

---

## 3. 功能模块

### 3.1 设备发现与注册

#### 3.1.1 子设备启动流程

1. 子设备启动 `remote-agent`，监听 `7681` 端口
2. 子设备读取本地配置（deviceId、token、母机地址）
3. 子设备向母机发送注册请求：`POST /api/device/register`
4. 母机验证 token，返回设备配置（包括该子设备上创建的飞书 Bot 列表）
5. 子设备启动 cc-connect，加载对应的飞书 Bot

#### 3.1.2 母机设备管理 API

```
GET  /api/devices              # 列出所有已注册设备
POST /api/devices              # 添加设备（手动输入 Tailscale IP + token）
DELETE /api/devices/{id}       # 删除设备
GET  /api/devices/{id}/status   # 查询设备状态（心跳、超时）
POST /api/devices/{id}/heartbeat  # 子设备上报心跳
```

#### 3.1.3 子设备信息结构

```json
{
  "deviceId": "linux-server-001",
  "name": "VPS 服务器",
  "hostname": "vps-host",
  "platform": "linux",
  "tailscaleIp": "100.123.45.68",
  "port": 7681,
  "token": "device-xxx-token",
  "status": "online",
  "maxSessions": 5,
  "activeSessions": 2,
  "registeredAt": "2026-05-22T10:00:00Z"
}
```

### 3.2 排班系统（Agent Registry）

#### 3.2.1 设计目标

管理每个飞书 Bot / Agent 的注册、状态、归属设备。

#### 3.2.2 Agent 数据结构

```json
{
  "agentId": "linux-001-bot-1",
  "deviceId": "linux-server-001",
  "deviceName": "VPS 服务器",
  "botToken": "feishu-bot-token-xxx",
  "status": "idle",
  "currentTaskId": null,
  "capabilities": ["code", "terminal", "git"],
  "maxConcurrentTasks": 1,
  "registeredAt": "2026-05-22T10:00:00Z",
  "lastHeartbeat": "2026-05-22T12:30:00Z"
}
```

#### 3.2.3 Agent 状态

| 状态 | 含义 |
|------|------|
| `idle` | 空闲，可接收任务 |
| `busy` | 正在执行任务 |
| `offline` | 未上报心跳（超过阈值） |

#### 3.2.4 API

```
GET    /api/agents              # 列出所有 Agent
GET    /api/agents/{id}         # 查询单个 Agent
GET    /api/agents/available    # 查询空闲 Agent（用于任务派发）
POST   /api/agents/{id}/heartbeat  # Agent 上报心跳（含当前任务状态）
```

### 3.3 任务调度系统（Task Scheduler）

#### 3.3.1 设计目标

管理任务的完整生命周期，包括工作流阶段、状态转换、卡点检测、人工介入标记。

#### 3.3.2 Task 数据结构

```json
{
  "taskId": "task-001",
  "name": "实现用户登录功能",
  "description": "在前端添加登录表单，调用后端 API",
  "workflow": {
    "stages": [
      { "id": "analyze", "name": "需求分析", "status": "completed" },
      { "id": "implement", "name": "编码实现", "status": "running" },
      { "id": "test", "name": "测试验证", "status": "pending" },
      { "id": "deploy", "name": "部署上线", "status": "pending" }
    ],
    "currentStage": "implement"
  },
  "assignee": "linux-001-bot-2",
  "status": "running",
  "blockedReason": null,
  "priority": "normal",
  "dependsOn": [],
  "createdAt": "2026-05-22T10:00:00Z",
  "startedAt": "2026-05-22T10:05:00Z",
  "completedAt": null
}
```

#### 3.3.3 任务状态

| 状态 | 含义 |
|------|------|
| `pending` | 等待分配 |
| `running` | 执行中 |
| `blocked` | 卡住（超时/错误/需人工介入） |
| `completed` | 完成 |
| `cancelled` | 取消 |

#### 3.3.4 任务阻塞类型

| blockedReason | 含义 |
|---------------|------|
| `timeout` | 执行超时 |
| `error` | 执行出错 |
| `human_needed` | 需要人工介入 |
| `dependency_blocked` | 等待前置任务 |

#### 3.3.5 API

```
GET    /api/tasks              # 列出任务（支持筛选：status, assignee, deviceId）
POST   /api/tasks              # 创建任务
GET    /api/tasks/{id}         # 查询任务详情
PUT    /api/tasks/{id}         # 更新任务（状态、阶段、阻塞原因）
DELETE /api/tasks/{id}         # 删除任务
POST   /api/tasks/{id}/assign   # 派发任务给指定 Agent
POST   /api/tasks/{id}/complete # Agent 报告任务完成
POST   /api/tasks/{id}/block    # Agent 报告任务卡住
POST   /api/tasks/{id}/unblock  # 人工介入后解阻塞
```

### 3.4 飞书 Bot 集成

#### 3.4.1 Bot 创建规则

每台子设备创建固定数量的飞书 Bot（默认 5 个，对应 5 个并发会话槽位）。

命名规则：`{deviceName}-agent-{N}`，例如：
- `vps-agent-1`
- `vps-agent-2`
- `macbook-agent-1`
- ...

#### 3.4.2 任务派发流程（飞书）

```
1. 用户在飞书群艾特母机的调度 Bot
2. 母机的 cc-connect 接收消息，解析任务内容
3. 母机调度 Agent 选择空闲的子设备 Bot
4. 母机的 cc-connect 向子设备的 Bot 发送任务指令
5. 子设备的 cc-connect 将任务转给 remote-agent 执行
6. 子设备的 Bot 在接收到任务时，调用 Hook 报告到排班系统（status: busy）
7. 子设备的 Bot 在任务完成时，调用 Hook 报告到任务系统（status: completed）
8. 结果通过子设备 Bot 回复到飞书群
```

#### 3.4.3 Hook 机制

子设备在任务状态变化时自动调用母机 API：

```
POST /api/agents/{id}/heartbeat
Body: {
  "status": "busy" | "idle",
  "currentTaskId": "task-xxx",
  "taskStatus": "running" | "completed" | "blocked"
}

POST /api/tasks/{id}/report
Body: {
  "stage": "implement",
  "status": "running" | "completed" | "blocked",
  "blockedReason": null | "error" | "human_needed",
  "message": "可选的日志或结果"
}
```

### 3.5 负载控制

#### 3.5.1 会话槽位控制

每台设备根据自身资源配置设定 `maxSessions`，例如：
- 4GB 内存的 Linux VPS：maxSessions = 5
- 16GB 内存的 MacBook Pro：maxSessions = 10

#### 3.5.2 调度策略

- 任务派发时，调度 Agent 查询 `/api/agents/available` 获取空闲 Bot
- 优先选择负载最低的设备
- 如果所有 Bot 都忙，任务进入 pending 队列等待

---

## 4. 数据存储

### 4.1 存储方案

采用**母机中心化存储**，所有数据存在母机上，子设备只做路由和执行。

存储格式：JSON 文件（与现有 `workspaces_dir.json` 一致）

存储位置：`~/.remote-agents/`

### 4.2 数据文件

| 文件 | 内容 |
|------|------|
| `devices.json` | 设备注册表 |
| `agents.json` | Bot 排班表 |
| `tasks.json` | 任务调度表 |
| `config.json` | 全局配置（母机地址、子设备列表等） |

---

## 5. Web UI 扩展

### 5.1 现有界面集成

在现有的 Web 工作台界面中增加以下面板：

1. **设备管理面板**
   - 查看所有已注册设备
   - 添加/删除设备
   - 查看设备状态（在线/离线）

2. **Bot 状态面板**
   - 查看所有 Bot 的状态（idle/busy/offline）
   - 归属设备、当前任务

3. **任务管理面板**
   - 查看任务列表（支持筛选）
   - 任务详情（阶段、状态、阻塞原因）
   - 手动派发/取消/解阻塞

---

## 6. 实现优先级

### Phase 1：设备发现与注册（核心基础）

1. 子设备启动时向母机注册
2. 母机管理设备列表（CRUD）
3. 心跳机制检测设备存活

### Phase 2：排班系统

1. Agent 数据结构与存储
2. Agent 状态管理（idle/busy/offline）
3. 心跳上报接口

### Phase 3：任务调度系统

1. Task 数据结构与存储
2. 任务 CRUD API
3. 派发、状态更新、阻塞检测

### Phase 4：飞书集成

1. 子设备 Bot 创建脚本
2. cc-connect 任务转发
3. Hook 机制实现状态上报

### Phase 5：Web UI

1. 设备管理面板
2. Bot 状态面板
3. 任务管理面板

---

## 7. 关键 API 汇总

### 设备相关

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/devices | 列出所有设备 |
| POST | /api/devices | 添加设备 |
| DELETE | /api/devices/{id} | 删除设备 |
| GET | /api/devices/{id}/info | 获取设备详细信息 |

### Agent 相关

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/agents | 列出所有 Agent |
| GET | /api/agents/available | 获取空闲 Agent |
| POST | /api/agents/{id}/heartbeat | Agent 心跳 |

### 任务相关

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/tasks | 列出任务 |
| POST | /api/tasks | 创建任务 |
| GET | /api/tasks/{id} | 任务详情 |
| PUT | /api/tasks/{id} | 更新任务 |
| POST | /api/tasks/{id}/assign | 派发任务 |
| POST | /api/tasks/{id}/report | Agent 上报状态 |

---

## 8. 待讨论事项

- [ ] Token 安全存储与传输
- [ ] 任务失败重试策略
- [ ] 人工介入的触发方式（飞书消息/UI）
- [ ] 多母机支持（集群模式）