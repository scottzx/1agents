# 语音识别与麦克风权限兼容性指南 (Voice Recognition Tips)

在 `remote-agents` 的 Localhost 模式或多设备局域网模式下，网页端提供了实用的语音输入（Speech-to-Text）功能。为了让不同设备和浏览器能够顺畅使用语音识别，我们总结了以下测试表现、底层原理以及解决方案。

---

## 1. 核心注意事项与兼容性测试矩阵

经过实际测试，在不同客户端和网络环境下，浏览器原生语音识别的可用性表现如下：

| 客户端设备 | 浏览器 | 访问协议/地址 | 语音识别可用性 | 核心说明 / 报错行为 |
| :--- | :--- | :--- | :---: | :--- |
| **Mac 电脑端** | **Safari** | `http://localhost:8086`<br>`https://localhost:8086` | **直接可用 (推荐) ✅** | 苹果 Safari 完美调用系统原生离线语音引擎，无网络限制。 |
| **Mac 电脑端** | **Chrome** | `http://localhost:8086`<br>`https://localhost:8086` | **常报 `network` 错误 ❌** | Chrome 强依赖 Google 云端识别服务，国内直连被墙或因代理拦截导致报错。 |
| **Mac 电脑端** | **Edge** | `http://localhost:8086`<br>`https://localhost:8086` | **视网络连通性而定 ⚠️** | 依赖微软云端识别服务，若网络连接较慢则易超时。 |
| **移动端 (手机)** | **Safari** | `https://[域名或IP]` | **可用 ✅** | 强制要求 **HTTPS**（安全上下文），使用 HTTP 时麦克风权限会被浏览器禁用。 |
| **移动端 (手机)** | **Edge** | `https://[域名或IP]` | **可用 ✅** | 同样强制要求 **HTTPS**。若用 HTTP 访问，界面无法调用麦克风。 |

---

## 2. 深入理解：为什么 Chrome 报 `network` 错误？

当在 Chrome 中使用 `localhost` 访问并尝试语音输入时，可能会在控制台看到以下错误：
```
index.tsx:357 Speech recognition error: network
网络连接失败，请检查是否处于内网或代理拦截。
```

### 根本原因：
1. **云端解析机制**：Chrome 的 `webkitSpeechRecognition` API **并不是本地离线运行的**。它会将麦克风录制的音频数据实时发送至 Google 的云端识别服务器（如 `speech.googleapis.com`）进行解析，再将文本传回网页。
2. **国内网络封锁**：由于 Google 的服务在国内被屏蔽，如果您的设备未开启全局系统代理，或者代理工具未能成功代理 Chrome 浏览器底层的 gRPC/WebSocket 音频流，连接就会超时报错。
3. **安全上下文（Secure Context）**：`localhost` 在桌面端被浏览器特殊对待，即便使用不加密的 `http://`，浏览器也认为其是安全上下文并允许请求麦克风。但在跨设备或移动端访问时，非 HTTPS 连接会被直接拒绝调用任何多媒体设备。

---

## 3. 完美解决方案

### 方案 A：在电脑端（macOS）首选 Safari 浏览器
* **优势**：macOS 上的 Safari 的 Web Speech API 可以直接调用苹果系统层级的离线听写引擎（与 Siri 听写同源），**完全不需要连接 Google 外部网络**。
* **表现**：无论是通过 `http` 还是 `https` 访问 `localhost`，Safari 均能秒速识别，且支持完美的中文普通话输入。

### 方案 B：在移动端/局域网使用 Tailscale 申请权威 HTTPS 证书
为了能在手机端（Safari / Edge）以及其他设备上通过安全绿锁正常启用语音识别和麦克风权限，建议通过 **Tailscale + Let's Encrypt 官方证书** 方案来提供 HTTPS 支持。

详细的 Tailscale HTTPS 证书配置步骤，请参阅同目录下的：
👉 [Tailscale 访问与 HTTPS 证书配置指南](ssl-certificate-guide.md)
