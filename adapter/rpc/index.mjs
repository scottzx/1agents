/**
 * adapter 根入口 —— `HAPPY_RPC_ADAPTER_ENTRY` 指向本文件。
 *
 * happy-cli daemon(machine scope)启动时动态 import() 本文件并调用 register(ctx),
 * 由这里汇总注册所有 1Agents 专属 machine-scoped RPC handler。
 *
 * 组合层(根):import 各子模块的注册函数。子模块之间不互相 import(wire/ 是叶子)。
 *
 * @typedef {import('./ctxContract.js').AdapterCtx} AdapterCtx
 */
import { registerProxy } from './proxy.mjs';
import { registerChatBridge } from '../chat/chatBridge.mjs';
import { registerTerminalBridge } from '../terminal/terminalBridge.mjs';

/** @param {AdapterCtx} ctx */
export async function register(ctx) {
  const log = (msg, ...args) =>
    ctx.log ? ctx.log(msg, ...args) : console.error('[1agents-adapter]', msg, ...args);

  // 控制面:1agents-proxy → 本机 Go 后端 HTTP API
  registerProxy(ctx, log);

  // Agent 聊天流:本地 Go WS ⇄ Happy session 扇出(issue #17 chat)
  await registerChatBridge(ctx, log);

  // 终端流:本机 ttyd WS ⇄ Happy session 扇出(issue #17 终端那一路)
  await registerTerminalBridge(ctx, log);

  log('registered: 1agents-proxy, 1agents-chat-open/send/close, terminal-open/input/close');
}

export default { register };
