/**
 * RelayTerminalSocket — 占位(M1 骨架,尚未实现)。
 *
 * 终端流过中转(issue #17 终端那一路)的前端半。设计上**完全仿照** relayChatSocket.ts 的
 * `ChatTransport` 模式:做一个 WebSocket 形状的薄传输,让 xterm 几乎不用改 —— 只换构造哪个 socket
 * (直连 ttyd 裸 WS vs 本类),选择方式同聊天今日做法。
 *
 * 对接的 node 侧:adapter/terminal/terminalBridge.mjs(node-pty attach tmux)。
 *
 *   Down(node → H5):node 桥把 pty.onData 分帧 → relay → 本类订阅 relay update、按
 *                    终端 session 过滤、用 machine key 解密 → onmessage 回吐给 xterm。
 *   Up(H5 → node):send() 把 stdin 经 RPC 转发;另需 resize 通道(cols/rows)。
 *
 * ⚠️ 见 adapter/terminal/terminalBridge.mjs 的 §200 吞吐风险 + Spike A。**终端定走 relay,不设
 *    旁路隧道后路**;遇瓶颈靠分帧/批量/背压/结构化优化解决,不切传输。直连 ttyd 裸 WS 仅在
 *    同源(无中转)场景保留。(Cloudflare 仅用户手动内网穿透,见 1agents-tunnel,与此解耦。)
 *
 * TODO(M1 spike → 实现):
 *   1. 复用 ChatTransport 接口(见 relayChatSocket.ts),实现 RelayTerminalSocket。
 *   2. 帧格式与 terminalBridge.mjs 对齐(raw chunk / asciinema v2 / tmux -CC 事件)。
 *   3. resize:新增 relayClient 的 terminal-resize RPC helper。
 */

export const PLACEHOLDER = true; // M1 占位,无实现。
