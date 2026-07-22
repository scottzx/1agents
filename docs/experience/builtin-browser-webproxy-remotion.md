# 内置浏览器反代与 Remotion Studio（成功经验）

**Status:** Reference · 经验沉淀（实现已落地，本文记「为何这样」与踩坑）  
**Date:** 2026-07-22  
**Scope:** 内置浏览器（`BuiltinBrowser`）+ Go `handleProxy` / `handleWebProxy`；典型目标：Remotion Studio `localhost:3000`  
**代码入口:**

| 层 | 路径 |
|----|------|
| 前端 iframe 包装 | `frontend/src/components/browser/BuiltinBrowser.tsx` |
| 反代与 HTML 注入 | `backend/internal/server/server.go`（`/api/webproxy/`、`/api/proxy`） |
| 测试 | `backend/internal/server/proxy_test.go` |

---

## 1. 产品约束（为何必须反代）

| 场景 | 谁打开 1agents | `localhost:3000` 应指谁 |
|------|----------------|-------------------------|
| 本机 PC | `http://localhost:38080` | 本机 |
| 局域网 / Happy Relay | 手机访问 `http://192.168.x.x:38080` 等 | **运行 1agents 的主机**，不是手机 |

iframe **直连** `http://localhost:3000`：

- 本机：Remotion 正常（pathname 真实）
- 远程：打到手机 localhost → **空屏**

结论：**一律经主机 Go 反代**，保证「localhost = 1agents 主机」语义一致。不能靠「本机直连」救 Remotion。

---

## 2. Remotion 如何读 Composition ID

Remotion Studio（`@remotion/studio`）大致：

```js
// getRoute() → window.location.pathname
// deriveCanvasContentFromRoute: 按 / 分段，取 lastPart 为 compositionId
// 错误文案: pathname.replace('/', '')  // 只去掉第一个 /
```

| 实际 pathname | compositionId（last 段 / 文案） | 结果 |
|---------------|----------------------------------|------|
| `/TalkingHeadComposition` | `TalkingHeadComposition` | ✅ |
| `/` | 空 → 选第一个 composition | ✅ |
| `/api/proxy` | **`api/proxy`** | ❌ `Composition with ID api/proxy not found` |

正确目标 URL 示例：

```text
http://localhost:3000/TalkingHeadComposition
http://localhost:3000/   # 自动第一个 composition
```

---

## 3. 失败方案回顾（为何不行）

### 3.1 查询串反代 `/api/proxy?url=…`

```text
iframe: http://localhost:38080/api/proxy?url=http%3A%2F%2Flocalhost%3A3000%2FTalkingHeadComposition
pathname 永远 = /api/proxy
```

Remotion 必炸。Chromium 里 **`location.pathname` 不可可靠地 redefine**，原型伪装无效。

### 3.2 `history.replaceState` 成 bare path `/TalkingHeadComposition`

意图：让真实 pathname 变成 composition 路径。

问题：

1. **`<base href="http://localhost:3000/">`**：相对 path 的 `replaceState` 会按 **base** 解析成 `http://localhost:3000/...` → 相对 38080 **跨域** → `SecurityError`，改写失败。  
2. 即使改成同源绝对 URL `http://localhost:38080/TalkingHeadComposition`：  
   - 刷新 / `location.reload()` / Remotion `location.href = pathname`  
   - 整页 GET 打到 **1agents 静态路由** → **404**（没有该 SPA 路由）。

### 3.3 其它曾踩坑

| 现象 | 原因 |
|------|------|
| JSON 解析出现 `�` 二进制 | 转发了浏览器 `Accept-Encoding`；Go 不解压却剥了 `Content-Encoding` |
| CORS 打到 `:3000/events` | 未改写 EventSource / fetch 参数未用 `.call` 生效 |
| SSE 挂死 | 反代整包 `ReadAll` 缓冲 |
| Worker SecurityError | `<base>` 让 Worker 脚本落在 `:3000`，页面 origin 是 `:38080` |

---

## 4. 成功方案

### 4.1 路径型反代 URL

```text
用户输入:  http://localhost:3000/TalkingHeadComposition

iframe src:
  /api/webproxy/{base64url(origin)}/TalkingHeadComposition

例:
  /api/webproxy/aHR0cDovL2xvY2FsaG9zdDozMDAw/TalkingHeadComposition
```

- **最后一段 = composition id** → Remotion 能选中  
- 刷新仍打 `/api/webproxy/...` → 仍走反代 → **不 404**  
- 旧接口 `/api/proxy?url=` 保留兼容，新 UI 走 webproxy  

`origin` 用 Go `base64.RawURLEncoding`（无 padding），与前端 `btoa` + urlsafe 一致。

### 4.2 History 永远停在 webproxy 上

- **禁止** 把 history 改成 bare `/TalkingHeadComposition`  
- Remotion `pushState('/OtherComp')` → 映射为  
  `/api/webproxy/{b64}/OtherComp`  
- 刷新按钮：重设 `iframe.src` 为 `getIframeUrl(tab.url)`（可 cache-bust），**不要** `contentWindow.location.reload()`（可能已不在 proxy path）

### 4.3 HTML 注入（bootstrap）

响应为 `text/html` 时文档**最前**注入：

1. `<base href="{targetOrigin}/">` —— 相对资源解析到目标站（注意：是 **origin 根**，不是带 composition 的完整 URL）  
2. 脚本：改写 `fetch` / XHR / EventSource / **Worker** / history / 链接点击，使网络经 webproxy  

注入时注意：

- 不转发浏览器 `Accept-Encoding`，让 Go 透明解压 gzip  
- 非 HTML（含 SSE）流式转发 + Flush  
- 改写 `Origin`/`Referer` 为目标站，减少上游 4xx/5xx  

### 4.4 验证清单

```bash
# Studio 在跑
curl -sS -o /dev/null -w "%{http_code}\n" http://localhost:3000/TalkingHeadComposition

# 反代 HTML 含 bootstrap
curl -sS "http://127.0.0.1:38080/api/webproxy/aHR0cDovL2xvY2FsaG9zdDozMDAw/TalkingHeadComposition" | head
```

浏览器（内置浏览器 iframe）：

1. 地址栏：`http://localhost:3000/TalkingHeadComposition`  
2. iframe `src` 形如：`/api/webproxy/.../TalkingHeadComposition`（**不是** `?url=`）  
3. 刷新后仍为 webproxy，**不是** `GET /TalkingHeadComposition 404`  
4. 标题类似：`TalkingHeadComposition / … - Remotion Studio`  
5. **无** `Composition with ID api/proxy not found`  

---

## 5. 原则沉淀

1. **语义优先于「看起来像源站」**：远程语义要求反代；path-SPA 要求 path 最后一段可读 → 用 **path 编码目标**，不要 query-only。  
2. **History 必须可整页刷新**：任何 `replaceState` 后的 URL，在 1agents 上必须有 handler；bare composition path 没有。  
3. **`<base>` 与 History API 交互危险**：相对 URL 的 history 会按 base 跨域解析。  
4. **Location 原型伪装在 Chromium 上不可靠**，不要当主路径。  
5. **压缩 / 流式 / Worker / SSE** 是反代默认坑位，要显式测。  

---

## 6. 已知限制

- Worker 经 webproxy 同源后，worker 内部 `importScripts` 相对路径仍可能出边角问题。  
- Remotion 根路径 `/` 时 webproxy 末段可能是 b64 token；应用侧更推荐打开明确 composition URL。  
- WebSocket（HMR 等）若目标站强依赖，可能还需单独 WS 隧道（当前重点在 composition 路由 + 主文档/API）。  

---

## 7. 相关用户可见文案

内置浏览器欢迎页提示（i18n `app.browser.tipProxyDesc`）：说明一律主机反代、localhost 指 1agents 主机。  
`</think>`