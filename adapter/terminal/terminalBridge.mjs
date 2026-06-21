/**
 * 终端桥(占位 —— M1 骨架,尚未实现)。
 *
 * 目标(issue #17 终端那一路):把终端流变成结构化 relay 流,和聊天流同质化,共用中转,
 * 不再依赖裸 WS 直连 ttyd。
 *
 *   H5(relayTerminalSocket.ts)──rpc terminal-open──▶ 本机:node-pty spawn `tmux attach -t <session>`
 *   pty.onData(chunk) ───────────────────────────────▶ 分帧 → relay(session new-message / 专用 RPC 流)
 *   H5 ──rpc terminal-input──▶ pty.write(data)
 *   H5 ──rpc terminal-resize─▶ pty.resize(cols, rows)
 *   H5 ──rpc terminal-close──▶ pty.kill()
 *
 * 为什么 attach tmux 而非直接 spawn shell:复用现有 tmux session(12 个 agents 已在跑),
 * 和 ttyd 当前拓扑一致;也为 tmux 控制模式(-CC,结构化事件)留口子。
 *
 * 依赖边界:`node-pty`(唯一原生依赖)、wire/、注入的 ctx;**不**直接耦合 ttyd 二进制。
 *
 * ⚠️ 最高风险(assessment §200):中转面向消息/RPC,非透明高吞吐字节隧道。终端裸字节过 relay
 *    可能延迟/吞吐不达标。**决策(2026-06-21):终端定走 relay,不设旁路隧道后路。** 遇瓶颈的
 *    出路是把流本身做高效/结构化(分帧、批量、背压,乃至 tmux 控制模式结构化事件),不是切传输。
 *    实现前先跑 Spike A 标定优化目标(见 docs/happy-integration-skeleton.md 验证段)。
 *    (Cloudflare 仅作为用户手动开启的内网穿透工具,见 1agents-tunnel skill,与本传输方案解耦。)
 *
 * TODO(M1 spike → 实现):
 *   1. node-pty spawn `tmux attach-session -t <session> -r`(只读 attach 先验证读流)。
 *   2. pty.onData 分帧:决定帧格式(raw base64 chunk vs asciinema v2 [t,type,data] vs tmux -CC 事件)。
 *   3. 背压:relay RPC 单条体积上限 + 帧序号 + 批量合并,防止刷屏丢序/打爆中转。
 *   4. registerTerminalBridge(ctx, log) 注册 terminal-open/input/resize/close。
 *
 * @typedef {import('../rpc/ctxContract.js').AdapterCtx} AdapterCtx
 */

/**
 * @param {AdapterCtx} _ctx
 * @param {(msg: string, ...args: unknown[]) => void} log
 */
export function registerTerminalBridge(_ctx, log) {
  // 占位:M1 骨架不注册任何 handler。实现见上方 TODO。
  log('terminal bridge: skeleton placeholder (not registered)');
}
