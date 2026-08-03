# 语音输入迁移至 transcribe.cpp 实施计划

> 状态：待实施计划
>
> 本文只定义实施方案，不执行代码修改、构建、测试、模型下载或发布操作。

## 1. 目标与范围

将 1Agents 当前依赖浏览器原生 Web Speech API 的语言输入功能，迁移为本地 `transcribe.cpp` 子模块驱动的语音识别链路，使用：

- `SenseVoiceSmall-Q8_0.gguf` 作为 ASR 模型；
- WebRTC VAD 检测语音起止；
- VAD 分段后调用 SenseVoice ASR，形成伪流式识别体验；
- 普通网页端与 Tauri 桌面端共用前端语音输入交互；
- Chat 输入框与 Terminal 输入框都支持语音结果增量追加。

本次迁移的核心目标是让音频在本地 daemon/worker 中完成 VAD 和 ASR，不再依赖浏览器厂商的识别服务或 Web Speech API 实现。

## 2. 当前实现与迁移原则

当前前端通过 `speechRecognition.ts`、`useSpeechRecognition.ts` 及共用的语音控制逻辑调用浏览器原生语音识别能力，Chat 和 Terminal 的麦克风入口复用该控制器。

迁移时保留以下已有交互语义：

- 麦克风按钮仍由 Chat 与 Terminal 复用；
- 识别文本仍追加到当前输入框，而不是自动发送；
- 用户可以停止录音、取消本次录音；
- 取消时恢复录音开始前的输入文本；
- 普通 HTTP/HTTPS 页面和 Tauri WebView 使用同一套前端控制器。

迁移时不保留浏览器 Web Speech API 作为默认或隐式 fallback。对于无法建立本地语音链路的环境，需要显示明确的错误或“不支持语音输入”状态。

## 3. 总体架构

```text
┌──────────────────────────────┐
│ Browser / Tauri WebView       │
│                              │
│ getUserMedia                  │
│   → AudioWorklet              │
│   → PCM16 mono                │
│   → authenticated WebSocket  │
└───────────────┬──────────────┘
                │
                ▼
┌──────────────────────────────┐
│ Go daemon                     │
│                              │
│ auth + Origin 校验            │
│ 单会话 owner                  │
│ worker 生命周期管理           │
│ WebSocket ↔ worker protocol   │
└───────────────┬──────────────┘
                │ stdin/stdout framed protocol
                ▼
┌──────────────────────────────┐
│ C++ transcribe worker        │
│                              │
│ 输入重采样 → WebRTC VAD       │
│ → pre-roll / hangover         │
│ → segment queue               │
│ → SenseVoiceSmall ASR         │
└───────────────┬──────────────┘
                │ segment result
                ▼
┌──────────────────────────────┐
│ Go daemon → WebSocket         │
│ segmentId/startMs/endMs/text  │
└───────────────┬──────────────┘
                ▼
┌──────────────────────────────┐
│ Chat / Terminal 输入框        │
│ 增量追加纯文本                │
└──────────────────────────────┘
```

### 3.1 组件职责

| 组件 | 职责 | 不负责的内容 |
| --- | --- | --- |
| 前端 `SpeechController` | 申请麦克风、采集 PCM、建立 WS、展示状态、追加文本 | VAD、模型推理、模型文件校验 |
| Go speech WS | 认证、Origin 校验、全局单会话、转发音频与结果、管理 worker | 具体 VAD 算法、ASR 推理 |
| C++ worker | 重采样、VAD、分段、队列、SenseVoice 推理、结果时间轴 | 浏览器认证、UI 状态 |
| 模型下载器 | 按 manifest 下载、校验、原子安装、进度与重试 | 音频采集、推理 |
| Chat/Terminal | 消费干净文本并追加到输入框 | 识别生命周期管理 |

## 4. 前端音频采集方案

### 4.1 主路径：AudioWorklet + PCM16

使用 `getUserMedia` 获取麦克风流，通过 `AudioWorklet` 将音频转换为单声道 PCM16LE 帧，并通过 WebSocket 发送给 Go daemon。

关键约束：

- 发送设备实际采样率，不假定浏览器输出一定是 16 kHz；
- 在 worker 统一重采样到 16 kHz；
- 音频 payload 使用 PCM16LE，避免后端再引入 WebM/Opus 解码器；
- WebSocket 建立后先发送采样率、声道数、格式等 session metadata；
- 音频采集和 UI 状态更新解耦，避免主线程频繁处理大块音频；
- 停止、取消、页面卸载时必须停止 track、断开 worklet，并发送对应控制帧。

### 4.2 兼容路径

对于不支持 `AudioWorklet` 的旧 WebView，可以提供 `ScriptProcessorNode` fallback，但该路径只作为兼容实现，协议和后端输入格式保持不变。

`MediaRecorder` 不作为主路径，因为其输出通常是 WebM/Opus 等容器/编码格式，会把浏览器音频解码问题引入 daemon/worker。

### 4.3 平台行为

- 普通网页端：通过现有认证上下文连接 `/api/speech/ws`。
- Tauri：不再因为 Web Speech API 不可用而隐藏麦克风入口；使用相同的本地 WS 链路。
- relay/C2 模式：首版明确提示不支持语音输入，不发送未定义的明文音频转发。

## 5. VAD 与伪流式识别

SenseVoiceSmall 首版按独立音频段执行推理，不承诺 token 级真实 streaming。用户持续收音时，VAD 将音频切成多个 segment，每个 segment 完成识别后立即返回。

```text
持续收音
  → 接收 PCM 帧
  → 重采样到 16 kHz
  → WebRTC VAD 判断 voiced/unvoiced
  → 形成完整 segment
  → transcribe_run(segment)
  → 返回 segment 结果
  → 继续接收下一段
```

### 5.1 首版 VAD 参数

首版采用 standalone WebRTC VAD，使用固定的平衡参数：

| 参数 | 计划值 | 说明 |
| --- | ---: | --- |
| VAD mode | 2 | 在误检与漏检之间取平衡 |
| VAD frame | 20 ms | 每帧进行一次检测 |
| 起始触发 | 连续 2 个 voiced frame | 约 40 ms 后开始语音段 |
| pre-roll | 200 ms | 保留触发前的音频，减少吞字 |
| 结束触发 | 连续 600 ms unvoiced | 语音结束后再提交 segment |
| 最小语音段 | 160 ms | 更短的段落视为噪声 |
| 最大语音段 | 24 s | 超过后在合法 frame 边界强制切段 |

VAD、pre-roll、hangover、segment 时间轴和 ASR queue 统一由 C++ worker 负责，Go 不重复实现 VAD，也不通过 cgo 调 VAD。

`startMs` 和 `endMs` 表示 VAD 音频区间，不表示词级时间戳。内部保留这些 metadata，Chat 和 Terminal 只消费干净文本，不把时间戳插入最终 prompt。

### 5.2 stop、cancel 与断开

- 正常 stop：停止接收新音频，flush 当前合法 segment，排空已经提交的 ASR 任务，再返回 complete。
- cancel：停止输入并丢弃尚未返回的 segment 结果；前端恢复录音前文本。
- 页面卸载/网络断开：尝试在有限时间内 drain；超时后释放会话 owner。
- worker 崩溃：Go 终止当前 session，向前端返回明确错误；不伪造未完成 segment 的文本。

## 6. C++ worker 设计

新增独立目录：

```text
modules/transcribe-worker/
```

不修改 `modules/transcribe.cpp` 子模块内部代码，只通过其公开 C ABI 使用能力。

### 6.1 生命周期

- 使用 CMake 构建独立 worker；
- 首次点击麦克风时懒启动，避免影响 daemon 启动时间；
- worker 常驻加载已安装的 SenseVoice 模型；
- 后续语音会话复用 worker 和已加载模型；
- Go 负责启动、监控、重启策略和优雅关闭；
- worker stdout 只能输出协议帧，日志全部写入 stderr。

### 6.2 内部线程与队列

建议至少拆分为：

1. 协议输入线程：读取 Go 发来的控制帧与 PCM 音频；
2. VAD/segment 线程：重采样、执行 VAD、维护 segment 状态；
3. ASR 线程：从有界队列取 segment，调用 SenseVoice 推理并输出结果。

ASR queue 必须有界。队列过载时显式返回 backpressure/error，并停止当前会话，不能静默丢失 segment。

## 7. Go 与 C++ worker 协议

Go 与 worker 使用 stdin/stdout 通信，不新增 TCP 或 Unix socket 端口。

### 7.1 帧格式

协议正式版本化，帧头计划为：

```text
u32 magic
u16 protocolVersion
u16 frameType
u32 payloadLength
u64 sequence
payload
```

协议需要明确规定：

- 固定端序；
- `magic` 和 `protocolVersion` 的具体值；
- 最大允许 frame size；
- `sequence` 的单调性与错误处理；
- PCM16LE 音频 payload；
- UTF-8 JSON 控制 payload；
- worker 结果中的错误码和可重试性。

### 7.2 控制帧与结果帧

至少支持以下类型：

| 方向 | 帧类型 | 作用 |
| --- | --- | --- |
| Go → worker | `START_SESSION` | 传递 session、采样率、声道、模型配置 |
| Go → worker | `AUDIO` | 传递 PCM16LE 音频帧 |
| Go → worker | `STOP_INPUT` | 停止接收新音频并 flush 当前段 |
| Go → worker | `CANCEL` | 取消当前 session，丢弃未完成工作 |
| Go → worker | `DRAIN` | 请求排空已提交的 segment |
| Go → worker | `SHUTDOWN` | 关闭 worker |
| worker → Go | `MODEL_STATUS` | 返回模型加载/就绪状态 |
| worker → Go | `SEGMENT` | 返回识别结果 |
| worker → Go | `ERROR` | 返回结构化错误 |

`STOP_INPUT` 与 `DRAIN` 必须分离：前者表示不再提交新音频，后者表示等待已提交任务完成。正常结束时不能只依赖关闭 stdin 来推断语义。

### 7.3 协议健壮性

Go 和 C++ 两端都必须处理：

- partial read/write；
- EOF；
- 坏 magic；
- 不支持的协议版本；
- 未知帧类型；
- 超大或溢出的 payload length；
- sequence 错乱；
- worker 提前退出。

Go 读取固定长度数据使用 `io.ReadFull`。C++ 端必须实现完整的 read/write helper，不能假设一次系统调用就能完成整个帧。

`SEGMENT` payload 至少包含：

```json
{
  "sessionId": "...",
  "segmentId": 1,
  "startMs": 1200,
  "endMs": 3180,
  "text": "识别结果",
  "isFinal": true
}
```

具体字段名、整数单位、文本规范化和 `isFinal` 语义需在实现前固化为协议文档或共享常量。

## 8. Go speech WebSocket API

新增语音路由：

```text
GET /api/speech/ws
```

### 8.1 安全要求

- 继续经过现有全局 tunnel/access-token middleware；
- 不新增 speech 专用 token；
- 使用同源 cookie 或现有 localhost 认证逻辑；
- 严格校验 `Origin`；
- 缺失或不匹配的 Origin 直接拒绝；
- HTTPS 页面必须使用 WSS；
- 允许经过认证的 Cloudflare HTTPS/WSS tunnel；
- relay/C2 模式首版不支持。

语音 WS 不得复用现有过于宽松的 `CheckOrigin` 实现；需要为语音路由提供明确的允许来源策略。

### 8.2 全局单会话状态机

本次采用全局单会话，不支持多个 Chat/Terminal 同时占用麦克风。

```text
IDLE
  → PREPARING_MODEL
  → STARTING
  → RECORDING
  → DRAINING
  → IDLE

任意阶段 → FAILED / CANCELLED → IDLE
```

状态规则：

- 第二个客户端立即收到 `busy` 并关闭；
- 不支持跨连接接管；
- 不支持 daemon 重启后的语音恢复；
- 已经发送到前端的 segment 保留；
- 未完成 segment 不伪造结果；
- disconnect 后只能在限定时间内 drain，超时必须释放 owner。

## 9. 前端 SpeechController 状态与行为

现有 `start(): void` 和 `recording: boolean` 不足以表达模型下载、连接、排空和失败状态。计划扩展为异步控制器，同时尽量保留现有 `toggle()` 调用方式。

### 9.1 状态类型

```ts
type SpeechStatus =
  | 'idle'
  | 'preparing'
  | 'downloading'
  | 'connecting'
  | 'recording'
  | 'draining'
  | 'busy'
  | 'error';
```

控制器至少提供：

- 异步 `start()`；
- `status`；
- `errorCode`；
- `downloadProgress`；
- cancel/cleanup；
- late-result suppression，避免取消或旧 session 的迟到结果污染当前输入框。

### 9.2 文本追加规则

- 识别结果只在 `SEGMENT` 完成后追加；
- Chat 和 Terminal 只显示纯文本；
- segment metadata 在控制器内部保留；
- 不向最终 prompt 插入时间戳或 JSON；
- 取消本次录音时恢复录音开始前的文本快照；
- sessionId/segmentId 用于丢弃重复、乱序和迟到结果。

## 10. 模型 manifest 与首次下载

模型不随安装包发布，首次使用麦克风时下载到用户缓存目录。

### 10.1 固定模型信息

```text
fileName: SenseVoiceSmall-Q8_0.gguf
sizeBytes: 252684608
sha256: 6c759ee4c9748c9b3f7a5a60ca74f0f7e685fb9d45d1378fce7cfd62f59adf29
url: https://huggingface.co/handy-computer/SenseVoiceSmall-gguf/resolve/main/SenseVoiceSmall-Q8_0.gguf
```

必须严格匹配大小写和完整文件名。`modules/transcribe.cpp/models/sensevoice-small-q8_0.gguf` 是错误的小文件，不能通过大小写不敏感或模糊匹配选取。

### 10.2 manifest

新增版本化 manifest，至少包含：

- `schemaVersion`；
- 精确文件名；
- 下载 URL；
- 允许的 host allowlist；
- `sizeBytes`；
- SHA-256；
- license 信息；
- worker/模型兼容版本。

### 10.3 缓存和安装

缓存目录：

```text
~/.1agents/models/
```

下载器要求：

- 仅首次点击麦克风时触发；
- 下载到 `.tmp` 文件；
- 下载完成后校验实际大小和 SHA-256；
- 校验通过后原子 rename；
- 设置最大文件大小；
- 仅允许 HTTPS 和 manifest 中的 host；
- 并发下载去重；
- 支持取消、超时、磁盘不足、hash mismatch；
- daemon 重启时清理或忽略残留临时文件；
- 前端展示准备中、下载中和进度；
- 失败可重试；
- 模型下载或加载失败不能阻塞 daemon 启动和其他功能。

## 11. Build、资源和打包

计划增加 worker 构建入口，例如：

```text
make speech-worker
```

并纳入以下工作流：

- `make all`；
- `make package`；
- `make tauri-resources`。

### 11.1 构建产物

- CMake 构建 `modules/transcribe-worker`；
- worker 进入统一 `build/`；
- 普通 package 将 worker 放入 `bin/`；
- Tauri 将 worker 放入 `resources/bin/`；
- Go 在开发模式、普通 package 和 Tauri `-resources-dir` 下都能正确解析 worker 路径；
- 模型仍不放入安装资源，继续由用户缓存目录下载。

### 11.2 平台覆盖

至少验证：

- Linux amd64；
- Linux arm64；
- macOS arm64；
- Windows x64。

需要同步处理：

- macOS ad-hoc signing；
- Windows 运行库和 DLL 分发；
- transcribe.cpp、WebRTC VAD 及相关第三方依赖的 license notices；
- worker 可执行文件权限、资源目录解析和进程启动错误提示。

## 12. 测试矩阵

### 12.1 前端单元和集成测试

使用 fake `AudioWorklet`、`WebSocket`、`MediaStream` 覆盖：

- 申请麦克风成功和失败；
- 采样率 metadata 正确发送；
- PCM16 转换和单声道处理；
- connecting、recording、draining、busy、error 状态；
- 下载进度展示；
- segment 文本按顺序追加；
- 重复、乱序、迟到结果被忽略；
- stop 后 flush 并完成；
- cancel 后恢复原文本；
- 页面卸载 cleanup；
- relay 模式显示不支持提示；
- Tauri WebView 不依赖 Web Speech API。

### 12.2 Go 测试

覆盖：

- access-token/cookie 认证；
- Origin 缺失、错误和合法场景；
- HTTP 页面与 HTTPS/WSS 约束；
- 单会话抢占和 `busy`；
- START、AUDIO、STOP、DRAIN、CANCEL 的状态转换；
- worker 启动、退出、崩溃、超时；
- worker stdout 协议帧转发；
- 无效 frame、超大 payload、未知版本和未知类型；
- disconnect 后 drain 和 owner 释放；
- 模型 manifest、缓存、下载并发去重；
- hash mismatch、大小不匹配、取消、超时、磁盘错误；
- 开发模式、普通包、Tauri resources 目录下的 worker 路径解析。

### 12.3 C++ worker 测试

覆盖：

- protocol header 编解码；
- partial read/write；
- EOF、坏 magic、坏版本、未知 frame type；
- payload length 边界和整数溢出防护；
- 44.1 kHz 与 48 kHz 重采样；
- 10/20/30 ms 输入帧；
- WebRTC VAD mode 2；
- pre-roll、起始触发、hangover、最小段和最大段；
- stop 时 flush；
- cancel 时清空当前 session；
- queue 满载时显式 backpressure；
- worker crash/OOM 的错误传播；
- 无效或缺失模型；
- 小型真实音频 fixture 的端到端识别。

### 12.4 跨层测试

增加 fake worker，验证前端、Go WS、worker protocol 的组合行为，不把所有测试都绑定到真实模型。

真实 `SenseVoiceSmall-Q8_0.gguf` smoke test 设为 opt-in，避免普通 CI 因模型体积、平台和运行时间受到影响。Tauri 打包测试还需验证 worker 路径、可执行权限和签名结果。

## 13. 失败模式与用户可见行为

| 场景 | 前端行为 | 后端行为 |
| --- | --- | --- |
| 麦克风权限拒绝 | 显示权限错误，回到 idle | 关闭当前 session |
| 模型首次使用 | 显示准备/下载进度 | 下载并校验后启动 worker |
| 模型 hash 不匹配 | 显示模型校验失败，可重试 | 删除临时/坏文件，不加载模型 |
| 另一个客户端占用 | 显示 busy | 不创建第二个 owner |
| Origin 不合法 | 显示连接失败 | 拒绝 WS upgrade |
| worker 启动失败 | 显示本地 ASR 不可用 | 返回结构化错误并释放 session |
| ASR queue 满 | 显示识别繁忙/失败 | 停止当前 session，不静默丢段 |
| worker 崩溃 | 保留已收到文本，提示本次失败 | 终止子进程并清理 owner |
| 用户 cancel | 恢复录音前文本 | 发送 CANCEL，丢弃未完成结果 |
| 正常 stop | 追加已完成结果后回 idle | STOP_INPUT → DRAIN → complete |
| relay/C2 模式 | 显示语音输入暂不可用 | 不转发明文音频 |

## 14. 实施顺序

1. 固化 worker protocol、错误码、时间单位和 session 状态转移。
2. 创建 `modules/transcribe-worker`，接入 transcribe.cpp 公共 C ABI、重采样、WebRTC VAD 和 SenseVoice ASR。
3. 实现 Go worker supervisor 与 `/api/speech/ws`，补齐认证、Origin、单会话和 drain/cancel 语义。
4. 实现模型 manifest、下载、校验、缓存和 worker 路径解析。
5. 将前端采集从 Web Speech API 切换到 AudioWorklet + PCM16 WS，扩展 `SpeechController` 状态。
6. 接入 Chat 与 Terminal 的文本追加、取消恢复和错误提示。
7. 更新 Makefile、普通 package、Tauri resources 与跨平台运行时文件。
8. 先运行 fake worker/fixture 测试，再进行 opt-in 真实模型 smoke test 和 Tauri 打包验收。

每一步完成后都应以对应测试或静态检查作为验收条件；本计划阶段不执行这些操作。

## 15. 验收标准

- 浏览器和 Tauri 的语音输入不再依赖 Web Speech API；
- 音频通过认证 WebSocket 进入 Go daemon，再进入本地 C++ worker；
- 44.1 kHz/48 kHz 输入可以被统一处理；
- WebRTC VAD 能按约定参数切段，并保留 pre-roll/hangover；
- 每个完成 segment 返回稳定的 `segmentId/startMs/endMs/text`；
- 连续讲话可以分段后连续追加识别文本；
- 正常 stop、cancel、断开、worker 崩溃都有确定行为；
- 同时只允许一个全局语音 session；
- 模型首次使用时才下载，并进行大小、SHA-256 和 host 校验；
- 模型失败不影响 daemon 启动和其他功能；
- 普通 package 与 Tauri package 都能找到正确的 worker；
- Linux amd64/arm64、macOS arm64、Windows x64 的打包约束有对应验证；
- CI 默认不依赖 241 MB 真实模型，真实识别 smoke test 可显式开启；
- Chat/Terminal 输入框只显示干净文本，不引入时间戳和协议字段。

## 16. 本次不在范围内

- 不实现 SenseVoice token 级真实 streaming；
- 不实现 relay/C2 模式的加密语音流；
- 不实现多个客户端并行占用麦克风；
- 不实现 daemon 重启后的语音 session 恢复；
- 不实现词级时间戳；
- 不增加 segment 时间轴编辑 UI；
- 不把模型文件打入安装包；
- 不保留浏览器 Web Speech API 作为默认识别路径。

## 17. 后续 TODO

1. relay 模式的加密语音流支持。
2. segment 时间轴编辑 UI。

## 18. 明确执行边界

本文创建后，本次任务即完成计划文档交付。除新增本文外，不修改其他仓库文件，不执行源码实现，不运行 build/test/Makefile/CI/发布命令，不下载或校验模型文件。
