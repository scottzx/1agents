# Happy 集成架构骨架

> 状态:M1 骨架已落地(2026-06-21)。本文档是 `adapter/` 接缝的设计基准。
> 相关:[agent 收敛路线图](agent-convergence-roadmap.md) · [happy-cli fork 同步](happy-cli-fork-sync.md)

## 背景

`1agents_app`(Tauri 壳 + Preact 前端 + Go 后端)把成熟的 happy 项目当作"中转 + 边车"
基础设施融进来。已拍板方向(2026-06-17 assessment):**Plan B = Node 边车**,Go 后端仍是大脑,
happy-server 当 relay,happy-wire 发成 `@1agents/wire`。

阻塞点(issue #17):两条裸 WebSocket 绕过 relay —— 终端(ttyd `/ws`)和 Agent 聊天
(`/api/agent/chat/ws`),都写死 `window.location.host`,中转上没有对应端点。

接缝胶水原本散在 happy-cli(服务器仓)里(角色错位,upstream 同步会痛)。本骨架把它收拢进
1agents_app 的 `adapter/` 目录,happy-cli 退化成只读蓝图 + 库。

## CSC 拓扑

```
C1 浏览器(Preact H5 + Tauri 壳)
   │ HTTPS/WSS (user-scoped)
   ▼
S happy-server(relay,哑中转,只见密文)
   │ WSS (machine-scoped) + 点对点 RPC
   ▼
C2 电脑端:happy-cli daemon ── 经 HAPPY_RPC_ADAPTER_ENTRY 加载 ──▶ adapter/(本仓胶水)
                                                                   │ HTTP / WS
                                                                   ▼
                                                          本机 Go 后端 :38080
```

## adapter/ 模块骨架

```
1agents_app/
├── modules/happy-cli/        # submodule: scottzx/happy-cli (upstream=slopus/happy),只读蓝图+库
├── adapter/                  # 1Agents 自有 C2 胶水
│   ├── rpc/
│   │   ├── index.mjs         # register(ctx) 根入口;HAPPY_RPC_ADAPTER_ENTRY 目标
│   │   ├── proxy.mjs         # 1agents-proxy → 本机 Go HTTP
│   │   └── ctxContract.d.ts  # ctx 契约镜像(耦合锁定点)
│   ├── chat/chatBridge.mjs   # 聊天桥(issue #17 chat)— 已实现,从 happy-adapter 重定位
│   ├── terminal/terminalBridge.mjs  # 终端桥(issue #17 terminal)— 占位
│   ├── agent/{runAgent,registry}.mjs  # 未来 AgentBackend 家 — 占位
│   └── wire/envelope.mjs     # ACP 形 ACPMessageData↔WsMessage 映射(thinking 一等公民)— 已实现
└── html/src/core/services/relay/
    ├── relayClient.ts        # 前端 relay 客户端(已有)
    ├── relayChatSocket.ts    # 聊天 ChatTransport(已有)
    └── relayTerminalSocket.ts# 终端 ChatTransport — 占位
```

### 模块职责 & 依赖边界

硬约束:`wire/` 是叶子,`rpc/` 是根;**任何代码都不改 `modules/happy-cli/` 源码**。

| 模块 | 职责 | 可 import | 禁止 import |
|---|---|---|---|
| `rpc/` | `register(ctx)` 汇总注册所有 machine-scoped handler | `chat/`、`terminal/`、`agent/`、`wire/`、stdlib、`ctx` | happy-cli 内部(只经 ctx);Go/TSX |
| `chat/` | 聊天桥:拨本地 Go chat WS,event 加密镜像成 relay session 消息 | `wire/`、`ctx`、Node WS/fetch | happy-cli 内部;ttyd 细节 |
| `terminal/` | node-pty attach tmux,stdout 分帧过 relay,stdin/resize 回写 | `wire/`、`ctx`、`node-pty` | 直接耦合 ttyd 二进制 |
| `agent/` | 消费 happy `AgentBackend`/`AgentRegistry` + DI 重写 runAcp | happy-cli Tier-1、`wire/` | happy `runAcp.ts`、`@/api`、`@/daemon`、`@/persistence` |
| `wire/` | ACP 形 `ACPMessageData`↔`WsMessage` 字段映射(thinking 一等公民)| stdlib(零依赖)| 其它一切(叶子) |

## issue #17 如何归位

两条裸 WS 都经骨架变成 relay 流,复用已验证的 `ChatTransport` 模式:

- **聊天**(`html/src/components/chat/hooks.ts:234`)→ `relayChatSocket.ts` ⇄ `adapter/chat/chatBridge.mjs`。**已通**。
- **终端**(`html/src/components/terminal/xterm/index.ts:291`)→ `relayTerminalSocket.ts` ⇄ `adapter/terminal/terminalBridge.mjs`(node-pty attach tmux)。**占位,待 M1 spike**。

### 终端传输决策(2026-06-21):relay-only,无旁路后路

relay 面向消息/RPC,**非透明高吞吐字节隧道**(assessment §200),终端裸字节过 relay 可能延迟/
吞吐损耗。**决策:终端定走 relay,不设 Tailscale/Cloudflare 架构后路。** 遇瓶颈的出路是把终端流
本身做高效/结构化 —— 分帧、批量合并、背压,乃至 tmux 控制模式(`-CC`)的结构化事件,让它和聊天流
同质化共用中转。Spike A 的作用从"决定是否旁路"变为"标定优化目标"。

直连 ttyd 裸 WS 仅在同源(无中转)场景保留。**Cloudflare 不是终端后路** —— 它只作为用户**手动开启**
的内网穿透工具(见 `1agents-tunnel` skill),与终端传输方案完全解耦。

## 验证

- **Spike A — 终端吞吐标定(§200 风险,最高优先):** `adapter/terminal/` node-pty spawn
  `tmux attach -t <session>`,stdout 分块经 relay RPC 推到临时 H5(`relayTerminalSocket.ts`)。
  在代表性负载(`yes` 刷屏、`vim` 滚动)测往返延迟 + 吞吐。目标:交互手感 ≤ 直连 ttyd 在可用
  范围内。**因终端定走 relay(无旁路后路),不达标 → 迭代优化方案**(分帧粒度/批量窗口/背压阈值/
  tmux `-CC` 结构化事件),而非切传输。Spike A 标定的就是这些优化参数的基线。
- **Spike B — 聊天重定位 parity:** `HAPPY_RPC_ADAPTER_ENTRY` 指向 `adapter/rpc/index.mjs`,
  确认现有 `relayChatSocket.ts` 流逐字节一致(现有 relay 聊天不回归)。
- **Submodule 同步演练:** `git -C modules/happy-cli merge upstream/main`,确认 `adapter/` 零改动、
  `ctx` 契约 / Tier-1 导出面仍匹配 `ctxContract.d.ts`。
- **契约测试(M2 闸):** golden-file 测 `adapter/wire/envelope.mjs` 把样本 happy ACP 形
  `ACPMessageData`(thinking 一等公民)映成现有前端解析器期望的精确 `WsMessage` JSON。

## 风险 & 开放问题

1. **终端过 relay 吞吐(最高):** §200 警告可能不达标。**已定 relay-only、无旁路后路**,故风险
   收敛为单一工程问题:把流做到足够高效(分帧/批量/背压/tmux `-CC`)。Spike A 先标定基线。
2. **happy-cli Tier-1 引入机制:** `file:` 依赖 vs 引 happy-cli 编译 `dist` —— 影响多少 happy
   pnpm 依赖树漏进 daemon。M2 决策。
3. **`runAcp.ts` DI 面:** 要注入什么(`@/api`/`@/daemon`/`@/persistence`),Go HTTP API 能否
   满足还是要 Node 薄 shim。决定 M2 工作量。
4. **两套加密栈:** 前端 `relay/crypto`(TS)与 `ctx.encrypt`(happy machine key)须随 upstream
   同步保持一致;终端桥引入 machine key 第三个消费者,验证分帧不破 E2E。
5. **`catalog.go` 双角色:** 既管传输路由又管安装元数据。收敛须拆开(见 agent 路线图 M3)。
6. **node-pty 原生构建:** 给目前零构建的 adapter 加编译依赖;与 `make package` 跨主机哲学交互。
7. **缺失的引用文档:** 已补 `happy-cli-fork-sync.md`;`open-vs-closed-boundary.md` 在 1agents_server 仓。

## 关键文件锚点

- `modules/happy-adapter/index.mjs` —— 旧种子,现为薄 shim re-export `adapter/rpc/index.mjs`
- `html/src/core/services/relay/relayChatSocket.ts` —— 终端要仿照的 `ChatTransport` 模式
- `backend/internal/agent/acpx_client.go` —— `WsMessage`,`adapter/wire/` 的映射锚点
- `backend/internal/agent/catalog.go` —— 静态 AgentInfo 表,M3 由 AgentRegistry 替代
