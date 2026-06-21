/**
 * 聊天桥:`1agents-chat-open/send/close` —— Agent 聊天流过中转(issue #17)。
 *
 *   H5 ──rpc 1agents-chat-open──▶ 建 Happy session + dial 本地 Go /api/agent/chat/ws,
 *                                 每个 Go event 加密镜像 → POST /v3/sessions/:id/messages
 *   H5 ──rpc 1agents-chat-send──▶ 把原始 action JSON 写回本地 Go WS
 *   relay 扇出(new-message) ────▶ H5 订阅并用同一 machine key 解密
 *
 * 消息内容是 Go WsMessage JSON 逐字加密(中转盲转)。Happy session 仅用于让中转路由/扇出。
 * 终端(ttyd)**不**走这里 —— 见 ../terminal/terminalBridge.mjs。
 *
 * 从 modules/happy-adapter/index.mjs 原样重定位,行为不变。
 * 依赖边界:Node WS/fetch + 注入的 ctx;不 import happy-cli 内部。
 * (wire/ 映射在 M2/M3 接入,见 docs/agent-convergence-roadmap.md。)
 *
 * @typedef {import('../rpc/ctxContract.js').AdapterCtx} AdapterCtx
 */
import { randomUUID } from 'node:crypto';

const BACKEND_BASE = () =>
  (process.env.ONEAGENTS_BACKEND_URL || 'http://127.0.0.1:38080').replace(/\/+$/, '');

async function resolveWebSocket() {
  if (typeof globalThis.WebSocket === 'function') return globalThis.WebSocket;
  const mod = await import('ws'); // Node <22 回退
  return mod.default ?? mod.WebSocket;
}

/**
 * @param {AdapterCtx} ctx
 * @param {(msg: string, ...args: unknown[]) => void} log
 */
export async function registerChatBridge(ctx, log) {
  const WebSocketCtor = await resolveWebSocket();

  /** @type {Map<string,{goWs:any, happySessionId:string}>} keyed by 1Agents chat session id */
  const bridges = new Map();

  async function ensureHappySession(oneAgentsSessionId, workspaceId) {
    // tag 实现 get-or-create:重开同一聊天复用 session。
    const meta = { path: workspaceId, host: 'localhost', oneAgentsChatSessionId: oneAgentsSessionId };
    const resp = await fetch(`${ctx.serverUrl}/v1/sessions`, {
      method: 'POST',
      headers: { 'content-type': 'application/json', Authorization: `Bearer ${ctx.token}` },
      body: JSON.stringify({
        tag: `1agents-chat:${oneAgentsSessionId}`,
        metadata: ctx.encrypt(meta),
        agentState: null,
        dataEncryptionKey: null,
      }),
      signal: AbortSignal.timeout(30000),
    });
    if (!resp.ok) throw new Error(`create session failed: ${resp.status}`);
    const data = await resp.json();
    const id = data?.session?.id;
    if (!id) throw new Error('create session: no id in response');
    return id;
  }

  async function postMessage(happySessionId, body) {
    try {
      const resp = await fetch(`${ctx.serverUrl}/v3/sessions/${encodeURIComponent(happySessionId)}/messages`, {
        method: 'POST',
        headers: { 'content-type': 'application/json', Authorization: `Bearer ${ctx.token}` },
        body: JSON.stringify({ messages: [{ content: ctx.encrypt(body), localId: randomUUID() }] }),
        signal: AbortSignal.timeout(30000),
      });
      if (!resp.ok) log('postMessage failed', resp.status);
    } catch (error) {
      log('postMessage error', error);
    }
  }

  ctx.registerHandler('1agents-chat-open', async (data) => {
    const existing = bridges.get(data.sessionId);
    if (existing && existing.goWs.readyState === 1) {
      return { success: true, happySessionId: existing.happySessionId };
    }
    try {
      const happySessionId = await ensureHappySession(data.sessionId, data.workspaceId);
      const wsBase = BACKEND_BASE().replace(/^http/, 'ws');
      const qs = new URLSearchParams({
        workspace_id: data.workspaceId,
        task_id: data.taskId || '',
        session_id: data.sessionId,
        agent_type: data.agentType,
        reply_id: data.replyId || '',
      });
      const wsUrl = `${wsBase}/api/agent/chat/ws?${qs.toString()}`;
      log('chat open', wsUrl, '→ happy session', happySessionId);

      const goWs = new WebSocketCtor(wsUrl);
      bridges.set(data.sessionId, { goWs, happySessionId });

      goWs.addEventListener('message', (ev) => {
        let obj;
        try {
          obj = JSON.parse(typeof ev.data === 'string' ? ev.data : ev.data.toString());
        } catch {
          return;
        }
        void postMessage(happySessionId, obj);
      });
      goWs.addEventListener('close', () => {
        bridges.delete(data.sessionId);
        // sentinel:本地 Go WS 关了 → 让 H5 侧 fire onclose 触发重连。
        void postMessage(happySessionId, { event: '__relay_closed' });
      });
      goWs.addEventListener('error', (ev) => log('goWs error', ev?.message ?? ev));

      await new Promise((resolve, reject) => {
        const t = setTimeout(() => reject(new Error('local ws connect timeout')), 15000);
        goWs.addEventListener('open', () => {
          clearTimeout(t);
          resolve();
        }, { once: true });
        goWs.addEventListener('error', (ev) => {
          clearTimeout(t);
          reject(ev?.message ? new Error(ev.message) : new Error('ws error'));
        }, { once: true });
      });

      return { success: true, happySessionId };
    } catch (error) {
      bridges.delete(data.sessionId);
      log('chat open failed', error);
      return { success: false, error: error instanceof Error ? error.message : 'open failed' };
    }
  });

  ctx.registerHandler('1agents-chat-send', async (data) => {
    const bridge = bridges.get(data.sessionId);
    if (!bridge || bridge.goWs.readyState !== 1) {
      return { success: false, error: 'no open chat bridge' };
    }
    try {
      bridge.goWs.send(data.raw);
      return { success: true };
    } catch (error) {
      return { success: false, error: error instanceof Error ? error.message : 'send failed' };
    }
  });

  ctx.registerHandler('1agents-chat-close', async (data) => {
    const bridge = bridges.get(data.sessionId);
    if (bridge) {
      try {
        bridge.goWs.close();
      } catch {
        /* ignore */
      }
      bridges.delete(data.sessionId);
    }
    return { success: true };
  });
}
