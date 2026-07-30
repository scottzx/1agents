# 小程序 web-view 嵌入 skills —— 后端改造方案

> **已废弃（2026-07-29）：** 本文记录的是 1skills / Skills-manager 时代的历史方案，
> 不再描述当前运行时。现行实现使用 `/extensions/` 页面、
> `/api/embed/harnesskit-embed.js` 和 `/api/harnesskit/*` 白名单代理；迁移与安全边界见
> [`../features/harnesskit-migration/README.md`](../features/harnesskit-migration/README.md)。
> 请勿按下文的 `/1skills/` 路由或 Python sidecar 设计实现新功能。

> 目标:让小程序用 `<web-view>` 复用 web 端的 1skills(技能/MCP/slash-commands)
> 重界面,而不是在 Taro 里重画一套。等价于桌面/web 今天的"嵌入模块"做法,但跨到
> 小程序这个**非同源、受微信约束**的宿主。
>
> 本文只定义 web-view 路成立的**后端 + 模块 + 小程序鉴权**前置。

## 0. 现状:已有的鉴权边界(关键)

1agents 后端整张 mux 都被 `authMiddleware` 包着(`server.go:493`)。非 localhost 访问
**必须**带 access token(即"wire Token 账号"),三种机制任一:

- **A**:`?access_token=<token>` query
- **B**:`Authorization: Bearer <token>`
- **C**:`ra_access_token` cookie

命中后中间件**回种 `ra_access_token` cookie**(`Path=/`、`HttpOnly`、TLS 下 `Secure`、
`SameSite=Lax`、1 年)。`/api/*` 无 token → 401;整页请求放行(SPA 自己弹门)。localhost 跳过。

**这层 gate 覆盖了 `/1skills/`(反代到 :38085 的独立整页前端)和 `/api/skills*` 透传**
(`server.go:300,328-337`)—— 它们本来就在 gate 后面。

→ **因此**:① skills API 不"裸奔",已被 access token 门盖住;② skills URL 里就算带
`ManagementToken` 也不构成泄露 —— 没有 access token 根本碰不到后端。**之前方案里
"补 token 门 / 签发 skills scope token"的担忧作废。**

## 1. 嵌入契约(因为有 gate,极简)

web-view 直接指向:

```
https://<域>/1skills/?access_token=<ACCESS_TOKEN>&bare=1&theme=<t>&lang=<l>
```

链路:
1. web-view 加载该整页 → `authMiddleware` 机制 A 认 `?access_token=` → 放行 **并种 `ra_access_token` cookie**。
2. 1skills 前端(`VITE_API_BASE=/api`)发的 `/api/skills`、`/api/mcp` … 是**同源 XHR** →
   自动带上刚种的 cookie → 过 gate(同源请求不受 `SameSite=Lax` 限制)。
3. → **1skills 不需要改任何鉴权**;skills 也不需要 `ManagementToken`(那是 cc-connect 的)。

不需要 `/api/skills/url` 这种铸 token 端点 —— URL 客户端直接拼即可。

## 2. 真正的前置:小程序得**持有并带上** access token

`grep` 确认:**core / 小程序当前对 `/api` 调用不带任何 access token**
(apiClient 没有 Authorization / access_token 逻辑)。今天能跑只因为测的是
`localhost`(中间件跳过)。一旦连**已部署、带 token 的**后端,连 `workspaceService.list()`
都会 401 —— 这是**所有**小程序↔远端后端的前置,不止 skills。

要做的(core + 小程序,**这才是本方案的真活**):

1. **存 token**:小程序 settings 加一个 access token 输入(类比 web 的访问门),
   存 storage(同 `BACKEND_OVERRIDE_KEY` 旁边,如 `1agents-access-token`)。
2. **带 token**:`TaroPlatformBridge.httpFetch` 给每个请求加 `Authorization: Bearer <token>`
   (或 apiClient 统一注入)。WS(`Taro.connectSocket`)同理在 query 带 `?access_token=`。
3. **传给 web-view**:core 加 `getSkillsEmbedUrl(theme, lang)` = `BACKEND_BASE + '/1skills/?access_token=' + token + '&bare=1&theme=&lang='`;小程序 skills 包壳页用它做 `<web-view src>`。

> 注:access token 怎么发到用户手里属既有机制(`accessService.generateToken` /
> 访问门),小程序只是**复用**这枚 token,不新发。

## 3. 后端唯一要改的:1skills 的 `bare` 模式

`/1skills/` 透传 `bare=1` 给 1skills 前端;bare 下隐藏其自身侧栏/顶栏,只渲染内容
(小程序导航交给原生 + web-view)。`theme/lang` 同样 URL boot。
**若 1skills 暂不支持 bare**:可先全量渲染,体验打折但能跑,bare 作为体验优化后补。

→ 后端 Go 侧基本**零改动**(gate、反代、透传都已具备);改动集中在
**小程序鉴权(§2)** 和 **1skills bare(§3,可选)**。

## 4. 落地顺序与验证

1. **小程序带 access token**(§2.1+2.2,core/miniapp)→ 验:小程序连一台**开了 token 的远端**
   后端,`workspaceService.list()` 不再 401(chat/tasks 页也随之在远端可用)。**这步本身就解锁了
   小程序连真实部署后端,价值独立于 skills。**
2. **`getSkillsEmbedUrl` + skills 包壳页**(§2.3,小程序)→ `pages/skills` 改 `<web-view>`,从「更多」进入。
   本机可先用"localhost + 不校验合法域名"在微信 DevTools 验渲染。
3. **1skills bare 模式**(§3,模块)→ `/1skills/?bare=1&theme=dark&lang=en` 浏览器直开验。
4. **部署**到白名单 https 域(`agents.dreammate.work`,已配)→ 真机 web-view。

## 5. 仍需你确认 / 不在内

- ⚠️ **微信 web-view 要已验证非个人主体账号**。若当前 appid(`wx2106fb17c3803118`)是个人号,
  web-view 整体不可用,要先换主体 —— 这是唯一的硬外部阻塞。
- **providers**:小程序现用 `agentService.getCatalog` 原生渲染(轻数据),**不必** web-view。
- **主题实时切换**:web-view 内 skills 随 URL boot;小程序切主题需 reload web-view(可接受)。
- access token 存进小程序 storage 的安全级别(设备本地,够用;若要更强需另设计),按需。
