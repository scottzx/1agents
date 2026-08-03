# 本地语音输入功能设计

**状态：** 功能设计已确认，待同步实施计划并开发  
**日期：** 2026-08-03  
**关联需求：** #318「完善语音输入：本地 transcribe.cpp 伪流式识别」  
**实施计划：** [speech-input-transcribe-cpp-plan.md](./speech-input-transcribe-cpp-plan.md)  
**首个交付平台：** macOS arm64 Tauri 桌面端，单用户、本地模型  
**长期目标：** Tauri 与普通 Web 共用交互和协议，并补齐 Linux/Windows 分发

> 本文是语音输入的产品与功能边界单一真源。若本文与现有实施计划冲突，以本文“已确认决策”为准；实施前应同步修订实施计划。

## 1. 背景

1Agents 当前使用浏览器 Web Speech API 为 Chat 和 Terminal 输入框提供语音转文字。该能力依赖浏览器及浏览器厂商服务，在 Tauri WebView、不同浏览器和离线环境中的可用性与行为不一致。

仓库已经引入 `modules/transcribe.cpp`，并验证了 SenseVoiceSmall 的 GGUF 推理能力。新的语音输入应把音频采集、VAD 分段和 ASR 推理收敛为本地链路，不再依赖浏览器厂商的识别服务。

当前还存在一套从未启用的 Tauri `sherpa-rs`/ONNX 智能录音模块。它与语音输入的原始目标相同，但模型格式、运行时和调用方式不同。本功能落地时将删除该模块，统一使用 transcribe.cpp 与一个 GGUF 模型。

## 2. 产品目标

用户在 Chat 或 Terminal 输入区域点击麦克风后，可以连续讲话。系统在本机识别语音，并把每个完成的语音段依次追加到当前输入框。识别结果只进入编辑区，不自动发送或执行。

成功意味着：

- 用户无需安装或依赖浏览器语音识别服务；
- Tauri 桌面端能够使用本地 SenseVoiceSmall 模型；
- Chat 与 Terminal 复用同一套语音交互和状态语义；
- 连续讲话通过 VAD 分段形成伪流式反馈；
- 正常停止、取消、断线、过载和 worker 崩溃都有确定且可恢复的行为；
- 音频默认只在浏览器 WebView、1Agents daemon 和本地 worker 之间流动。

## 3. 用户故事

### 3.1 Chat 输入

作为 1Agents 用户，我希望在 Chat Composer 中点击麦克风并讲话，让完成识别的文本持续追加到草稿中，确认或修改后再手动发送。

### 3.2 Terminal 输入

作为 1Agents 用户，我希望在 Terminal 的输入面板中使用相同的麦克风入口，把识别文本追加到待输入内容中，而不是直接写入或执行终端命令。

### 3.3 停止与取消

作为正在录音的用户，我希望：

- 点击停止时，系统处理完已经接收的合法音频段，再保留识别结果；
- 点击取消时，系统终止本次 worker，并恢复录音开始前的输入文本；
- 页面关闭或连接断开时，系统立即终止本次 worker，不继续处理无人接收的结果。

### 3.4 错误恢复

作为用户，我希望模型缺失、麦克风权限拒绝、worker 启动失败、连接过载等问题以明确状态呈现，并能在问题修复后重新点击麦克风重试，不影响 Chat、Terminal 或 daemon 的其他能力。

## 4. 范围与阶段

### 4.1 第一阶段：macOS 桌面端本地闭环

第一阶段只承诺：

- macOS arm64；
- Tauri 桌面端内嵌的 WebView；
- 单用户、全局同时最多一个语音 session；
- Chat Composer 与 Terminal 输入面板；
- 用户本地已经准备好的 SenseVoiceSmall Q8_0 GGUF；
- AudioWorklet 采集 PCM16LE；
- Go loopback WebSocket；
- 每个语音 session 独立启动一个 C++ worker；
- WebRTC VAD 分段与 SenseVoiceSmall 一次一段推理；
- 本地构建、Tauri resources 打包、可执行权限和 macOS 签名验证。

第一阶段不实现模型下载器。开发机通过正式模型路径访问仓库中的已下载模型：

```text
~/.1agents/models/SenseVoiceSmall-Q8_0.gguf
  → <repo>/modules/transcribe.cpp/models/SenseVoiceSmall-Q8_0.gguf
```

当前开发机的软链接已经建立，并验证了文件大小与 SHA-256。

### 4.2 后续阶段：完整产品化

第一阶段验证完成后，依次补齐：

1. 普通 Web 页面通过同一 `speech-ws-v1` 协议使用本机 daemon；
2. 首次使用时按 manifest 下载、校验并原子安装模型；
3. Linux amd64、Linux arm64 和 Windows x64 构建与打包；
4. 正式 macOS 签名、公证及其他平台运行时依赖；
5. Cloudflare HTTPS/WSS tunnel 下的认证语音连接。

### 4.3 不在范围内

- SenseVoice token 级真实 streaming；
- relay/C2 的加密语音转发；
- 多客户端并行语音 session；
- daemon 重启后的语音 session 恢复；
- 词级时间戳；
- 语音段时间轴编辑 UI；
- 自动发送 Chat 消息或自动执行 Terminal 命令；
- 浏览器 Web Speech API 隐式 fallback；
- 保留旧 Tauri sherpa/ONNX 智能录音实现。

## 5. 用户体验

### 5.1 状态

前端统一使用以下状态：

```ts
type SpeechStatus =
    | 'idle'
    | 'preparing'
    | 'connecting'
    | 'recording'
    | 'draining'
    | 'cancelling'
    | 'busy'
    | 'error';
```

| 状态 | 用户看到的行为 | 可执行动作 |
| --- | --- | --- |
| `idle` | 麦克风可点击 | 开始录音 |
| `preparing` | 正在启动本地模型 | 取消 |
| `connecting` | 正在建立语音通道 | 取消 |
| `recording` | 麦克风高亮，完成语音段持续追加 | 停止、取消 |
| `draining` | 正在处理最后一段 | 等待、取消 |
| `cancelling` | 正在终止本次 worker | 等待 |
| `busy` | 另一个输入入口正在使用语音 | 关闭提示后重试 |
| `error` | 显示具体错误与恢复建议 | 重试 |

### 5.2 文本语义

- 只追加已完成的 segment，不展示临时 token；
- Chat 和 Terminal 只消费纯文本，不插入时间戳、JSON 或协议字段；
- `sessionId`、`segmentId`、`startMs`、`endMs` 只用于排序、去重和诊断；
- 取消时恢复录音开始前的文本快照；
- 已完成识别的文本不会自动发送；
- 重复、乱序、旧 session 和取消后的迟到结果必须被忽略。

## 6. 总体架构

```text
Tauri WebView / Browser
  │
  │ getUserMedia
  │ AudioWorklet → mono Float32 → PCM16LE
  │ speech-ws-v1：JSON control/status + binary PCM
  ▼
Go speech handler
  │
  ├── auth + strict Origin
  ├── global single-session owner
  ├── bounded WebSocket/worker queues
  ├── start/kill/wait one worker per session
  └── public WS ↔ internal worker protocol adapter
  │
  │ versioned stdin/stdout framed protocol
  ▼
C++ transcribe worker
  │
  ├── input resample → 16 kHz mono float32
  ├── WebRTC VAD + pre-roll + hangover
  ├── bounded segment/ASR queue
  └── transcribe.cpp SenseVoiceSmall inference
  │
  ▼
segment result
  │
  └── Chat / Terminal append clean text
```

### 6.1 边界

| 组件 | 负责 | 不负责 |
| --- | --- | --- |
| `SpeechController` | 麦克风、AudioWorklet、WS、状态、文本追加与回滚 | VAD、重采样、模型加载 |
| Go speech handler | 认证、Origin、单 owner、协议校验、有界转发、worker 进程生命周期 | VAD 算法和 ASR 推理 |
| C++ worker | 重采样、VAD、分段、SenseVoice 推理、结果时间轴 | 浏览器认证和 UI |
| Chat/Terminal | 提供输入值并消费纯文本 | 管理模型和 worker |

## 7. 公开协议：speech-ws-v1

Browser/Tauri 与 Go 使用独立的公开协议。Go 与 worker 的内部二进制协议不暴露给前端。

### 7.1 握手

```text
Client → Server text
{
  "type": "start",
  "protocolVersion": 1,
  "sampleRate": 48000,
  "channels": 1,
  "format": "pcm_s16le"
}

Server → Client text
{
  "type": "ready",
  "sessionId": "<server-generated>"
}
```

`sessionId` 必须由服务端生成。客户端只有收到 `ready` 后才能发送 binary PCM。

### 7.2 客户端消息

| 类型 | WebSocket frame | 说明 |
| --- | --- | --- |
| `start` | text JSON | 声明协议和音频格式 |
| AUDIO | binary | 裸 PCM16LE，长度必须是 2 的倍数 |
| `stop` | text JSON | 停止接收新音频并排空已提交段 |
| `cancel` | text JSON | 终止 worker 并回滚本次输入 |

### 7.3 服务端消息

| 类型 | 说明 |
| --- | --- |
| `preparing` | worker 正在启动和加载模型 |
| `ready` | 可以开始接收 PCM |
| `segment` | 一个完成识别段 |
| `draining` | 正在处理最后的合法段 |
| `complete` | 正常结束，worker 即将退出 |
| `cancelled` | 取消完成，worker 已退出 |
| `busy` | 全局 owner 已被占用 |
| `error` | 结构化错误 |

错误统一包含：

```json
{
  "type": "error",
  "code": "worker_start_failed",
  "message": "本地语音识别启动失败",
  "retryable": true
}
```

### 7.4 协议约束

- text control frame 设置小型上限；
- binary audio frame 设置明确上限；
- `start` 前、`ready` 前和 `stop` 后的 binary frame 均为协议错误；
- WebSocket 自身保证消息有序，不为每个 PCM frame 重复设计 sequence；
- segment 使用单调递增的 `segmentId`；
- Go 端只有一个 WebSocket writer goroutine；
- 业务错误使用稳定 `code`，WebSocket close code 只表达连接终态；
- 后续模型下载进度通过新增 `model_progress` JSON 事件扩展，不改变 binary PCM 格式。

## 8. worker 生命周期

不使用常驻 worker。每个语音 session 拥有一个独立进程：

```text
IDLE
  → acquire owner
  → spawn worker
  → PREPARING
  → READY
  → RECORDING

正常停止：
  RECORDING → STOPPING_INPUT → DRAINING → COMPLETE
  → worker exit(0) → Wait() → release owner → IDLE

取消或断线：
  任意活动态 → kill worker → Wait()
  → discard pending results → release owner → IDLE

worker 错误：
  任意活动态 → Wait()
  → structured error → release owner → IDLE
```

只有 worker 已经退出并完成 `Wait()` 后才能释放 owner。正常路径只有在收到 `COMPLETE` 且进程退出码为 0 时才算成功。

该设计接受每次录音重新加载模型的延迟，用进程退出换取确定的 session 隔离。第一阶段必须测量点击麦克风到 `ready` 的 P50/P95，而不是继续为未验证的热启动需求维护常驻状态。

## 9. 音频、VAD 与伪流式

### 9.1 音频采集

- 使用 `getUserMedia` 获取麦克风；
- AudioWorklet 在音频线程读取实际 block 长度，不假定永远为 128 samples；
- 多声道输入下混为单声道；
- Float32 转换为 PCM16LE；
- 前端聚合为约 80–100 ms 的 binary 消息；
- 发送实际 AudioContext sample rate；
- worker 统一重采样为 16 kHz mono float32；
- 第一阶段不增加 `ScriptProcessorNode` fallback，macOS Tauri 不支持 AudioWorklet 时直接显示不支持。

### 9.2 VAD 参数基线

| 参数 | 基线值 |
| --- | ---: |
| WebRTC VAD mode | 2 |
| VAD frame | 20 ms |
| 起始触发 | 连续 2 个 voiced frame |
| pre-roll | 200 ms |
| 结束触发 | 连续 600 ms unvoiced |
| 最小语音段 | 160 ms |
| 最大语音段 | 24 s |

SenseVoiceSmall 不是 token 级 streaming 模型。worker 在 VAD 形成完整 segment 后调用一次 `transcribe_run`，每段完成后立即返回文本，从用户视角形成伪流式体验。

## 10. 端到端有界管线

所有音频缓冲区必须有明确上限：

```text
AudioWorklet
  → 前端聚合器
  → WebSocket bufferedAmount 上限
  → Go WebSocket ReadLimit / deadline
  → Go 有界 worker 写队列
  → worker 有界输入/segment/ASR queue
```

规则：

- 任一层超过上限，停止当前 session 并返回 `backpressure` 错误；
- 不允许静默丢弃最旧或最新音频；
- 不允许以无限延迟换取表面连续录音；
- 每层限制优先用“可缓冲音频毫秒数”表达，并同时落实为字节上限；
- 压力测试必须证明长时间录音和慢 worker 下的内存上界。

## 11. 安全边界

### 11.1 认证

- 精确 loopback，即 `127.0.0.1` 和 `::1`，可以沿用桌面本机免认证体验；
- 非 loopback 请求不得继承现有“私有 LAN 也视为 localhost”的放宽；
- Cloudflare tunnel 请求必须验证现有 `ra_session_token`；
- 配置 access token 时，其他非 loopback 请求必须验证现有 `ra_access_token`；
- 不新增 speech 专用 token；
- 认证失败必须发生在 WebSocket upgrade 之前。

### 11.2 Origin

- 缺失、`null` 或不匹配的 Origin 直接拒绝；
- 比较 scheme、host 和 port；
- HTTPS 页面必须使用 WSS；
- Tauri 当前导航到 daemon 的真实 loopback HTTP 地址，因此按普通同源 WebSocket 处理；
- 不复用已有 `CheckOrigin: true` 的宽松 upgrader。

## 12. 模型契约

正式路径：

```text
~/.1agents/models/SenseVoiceSmall-Q8_0.gguf
```

固定身份：

```text
fileName: SenseVoiceSmall-Q8_0.gguf
sizeBytes: 252684608
sha256: 6c759ee4c9748c9b3f7a5a60ca74f0f7e685fb9d45d1378fce7cfd62f59adf29
```

第一阶段规则：

- 不扫描当前目录或多个候选路径；
- 允许正式路径本身是开发期软链接，但必须校验其最终目标文件；
- 精确校验文件名、大小和 SHA-256；
- 模型缺失或校验失败只影响语音 session，不影响 daemon 启动和其他功能；
- UI 显示具体目标路径和恢复建议。

后续下载器仍写入同一正式路径：manifest 固定 URL、host allowlist、大小、SHA-256、license 和 worker 兼容版本；下载到临时文件，校验后原子 rename。

## 13. 旧 Tauri 智能录音清理

本功能实施时删除从未启用的 `src-tauri/src/recording.rs`，移除所有相关 Tauri command 注册，并清理仅被该模块使用的依赖：

- `sherpa-rs`；
- `eyre`；
- `rusqlite`；
- `uuid`；
- `chrono`；
- `dirs`；
- `base64`。

历史上追加到该文件的 `save_studio_assets` 已无当前前端调用方；对应 Vlog Studio 前端已经删除，因此不需要迁移该命令。

删除后必须运行 Rust 编译与依赖检查，确认不存在动态调用或遗留 feature flag。

## 14. 功能需求

| ID | 功能需求 |
| --- | --- |
| FR-1 | Chat 和 Terminal 使用同一 `SpeechController` 行为契约 |
| FR-2 | Tauri macOS 可通过 AudioWorklet 采集麦克风并发送 PCM16LE |
| FR-3 | Go 提供经过认证和严格 Origin 校验的 `/api/speech/ws` |
| FR-4 | `speech-ws-v1` 在实现前版本化并形成契约测试 |
| FR-5 | 同时最多一个全局语音 session，第二个连接得到 `busy` |
| FR-6 | 每个 session 启动独立 worker，结束后退出并 `Wait()` |
| FR-7 | worker 把设备采样率重采样到 16 kHz mono float32 |
| FR-8 | WebRTC VAD 按基线参数形成最大 24 秒 segment |
| FR-9 | 每个 segment 通过 SenseVoiceSmall 产生纯文本结果 |
| FR-10 | stop 排空，cancel/断线直接终止 worker |
| FR-11 | 所有音频队列有界，过载明确失败且不静默丢帧 |
| FR-12 | 模型从固定路径加载并校验大小与 SHA-256 |
| FR-13 | 语音失败不阻塞 daemon 启动或其他功能 |
| FR-14 | worker 纳入 root Makefile、普通 package 和 Tauri resources |
| FR-15 | 删除旧 sherpa/ONNX 智能录音模块与专用依赖 |

## 15. 第一阶段验收标准

### 15.1 用户行为

- macOS arm64 Tauri 中，Chat 麦克风可以开始、停止和取消；
- Terminal 输入面板使用相同语音能力；
- 中文和英文短句能够形成非空、合理的 segment 文本；
- 连续讲话能够返回多个按序 segment；
- 文本只追加到输入框，不自动发送或执行；
- cancel 恢复录音开始前文本；
- 页面刷新或 WebSocket 断开后 worker 被终止，下一次可重新录音；
- 同时点击两个语音入口时，第二个入口收到明确 `busy`。

### 15.2 技术行为

- 44.1 kHz 和 48 kHz 输入可统一重采样到 16 kHz；
- VAD pre-roll、hangover、最短段和最长段符合基线；
- 模型通过正式路径软链接被正确解析和校验；
- 错误模型、缺失模型和错误 hash 不会启动推理；
- WebSocket 非法 Origin、非认证 LAN 请求和超大 frame 被拒绝；
- 慢 worker/管道阻塞不会产生无界内存增长；
- 正常完成后 worker 退出码为 0；取消、断线和错误后无残留 worker；
- Tauri resources 中包含可执行 worker，并通过 macOS 权限与签名检查；
- 旧 `sherpa-rs` 模块和专用依赖已移除。

### 15.3 性能基线

第一阶段不预设未经测量的硬指标，但必须记录：

- 点击麦克风到 `ready` 的 P50/P95；
- segment 结束到文本返回的 P50/P95；
- worker 峰值 RSS；
- 连续录音期间 Go、worker 和 WebView 的内存上界；
- 取消/断线到 worker 退出并释放 owner 的耗时。

数据用于决定是否需要重新引入 warm worker；在测量证明需要之前，不增加常驻生命周期。

## 16. 测试策略

- 前端：fake AudioWorklet、MediaStream 和 WebSocket，覆盖状态、文本追加、迟到结果、过载和 cleanup；
- Go：表驱动认证/Origin/协议测试、全局 owner 竞争、worker 生命周期、超时和有界队列；
- C++：协议编解码、partial read/write、重采样、VAD 状态机、队列上限、模型错误和真实短音频 fixture；
- 跨层：fake worker 驱动 Browser/Go 状态闭环；
- 真实模型：macOS opt-in smoke test，不把 241 MiB 模型放进普通 CI；
- 打包：验证 Tauri resources 路径、可执行权限、签名和启动退出。

## 17. 交付顺序

1. 删除旧 Tauri 智能录音模块及专用依赖，建立干净基线；
2. 固化 `speech-ws-v1` 与内部 worker protocol；
3. 实现最小 C++ worker：模型加载、单段 16 kHz PCM 推理、退出语义；
4. 增加重采样、WebRTC VAD、分段与有界队列；
5. 实现 Go speech handler、严格认证/Origin、全局 owner 和每 session worker；
6. 实现 AudioWorklet 与共享 `SpeechController`；
7. 接入 Chat 和 Terminal；
8. 接入 root Makefile 与 Tauri resources，完成 macOS 真机验收；
9. 根据性能测量决定是否需要进一步优化；
10. 后续补模型下载、普通 Web 和其他平台。

## 18. 已确认决策

| 决策 | 结论 |
| --- | --- |
| 总体范围 | 保留完整目标，macOS 桌面端先形成纵向闭环 |
| 模型路径 | 长期固定 `~/.1agents/models/SenseVoiceSmall-Q8_0.gguf` |
| 当前模型 | 正式路径软链接到仓库已验证 GGUF |
| 推理引擎 | 只保留 transcribe.cpp + GGUF |
| 旧录音模块 | 删除 Tauri sherpa/ONNX 实现与专用依赖 |
| worker 生命周期 | 一次 session 一个进程，所有终态后退出 |
| 断线 | 直接 kill + Wait，不 drain |
| 背压 | 每一跳有界，超限失败，不静默丢音频 |
| WebSocket 协议 | 独立 `speech-ws-v1`，JSON 控制 + binary PCM |
| 本机认证 | 仅精确 loopback 免认证 |
| 非本机认证 | 必须现有 access/tunnel cookie + 严格 Origin |
| 首阶段下载器 | 不实现，使用本地已有模型 |

## 19. 文档后续

现有 [speech-input-transcribe-cpp-plan.md](./speech-input-transcribe-cpp-plan.md) 仍包含常驻 worker、断线 drain、旧范围和不完整的公开 WebSocket 协议。实施前需要按本文同步修订，至少删除以下过时设计：

- worker 常驻与跨 session 复用；
- 断线后 drain；
- 仅 C++ 队列有界；
- 普通私网请求继承全局免认证；
- 第一阶段模型下载器；
- 第一阶段 `ScriptProcessorNode` fallback；
- 保留旧 Tauri 智能录音模块。

工程审查的架构章节已经完成；代码质量、完整测试覆盖图与性能审查仍需在进入实现前补完。
