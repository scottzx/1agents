/**
 * 终端桥:`terminal-open/input/close` —— 终端流过中转(issue #17 终端那一路)。
 *
 *   H5 ──rpc terminal-open──▶ 建 Happy session + dial 本机 ttyd ws://127.0.0.1:7681/ws(子协议 'tty'),
 *                              ttyd 每个二进制帧 → 批量缓冲 → 加密 POST /v3/sessions/:id/messages
 *   H5 ──rpc terminal-input─▶ 把原始 ttyd 帧字节(base64)写回 ttyd WS(吞掉前端自动发的握手 JSON 帧)
 *   relay 扇出(new-message)▶ H5 RelayTerminalSocket 订阅、解密、逐帧 dispatch 给 xterm
 *
 * adapter 只做「哑搬运 + 加密」:原样双向搬运 ttyd 帧,**不理解终端语义**(守住判废线)。
 * ttyd 已处理 pty/tmux/UTF-8/flow-control,故**不引入 node-pty**(见 docs/agent-convergence-roadmap.md)。
 *
 * 结构完全仿照 ../chat/chatBridge.mjs(relay session 建立 / ctx.encrypt / post / registerHandler)。
 * 依赖边界:Node 22 全局 WebSocket/fetch + node:crypto + 注入的 ctx;不 import happy-cli 内部、不耦合 ttyd 二进制。
 * (wire/ 映射是 Phase 2 的活,这里不碰。)
 *
 * @typedef {import('../rpc/ctxContract.js').AdapterCtx} AdapterCtx
 */
import { randomUUID } from 'node:crypto';

const TTYD_BASE = () =>
  (process.env.ONEAGENTS_TTYD_URL || 'http://127.0.0.1:7681').replace(/\/+$/, '');

// 下行批量参数(§200 缓解;Spike A 标定,先给保守默认)。
const FLUSH_INTERVAL_MS = Number(process.env.ONEAGENTS_TERM_FLUSH_MS || 16);
const FLUSH_MAX_BYTES = Number(process.env.ONEAGENTS_TERM_FLUSH_BYTES || 32 * 1024);

async function resolveWebSocket() {
  if (typeof globalThis.WebSocket === 'function') return globalThis.WebSocket;
  const mod = await import('ws'); // Node <22 回退
  return mod.default ?? mod.WebSocket;
}

/** ev.data → Uint8Array(arraybuffer / view / Buffer / string 都归一)。 */
function toU8(d) {
  if (d instanceof ArrayBuffer) return new Uint8Array(d);
  if (ArrayBuffer.isView(d)) return new Uint8Array(d.buffer, d.byteOffset, d.byteLength);
  if (typeof d === 'string') return new TextEncoder().encode(d);
  return new Uint8Array(d);
}

/**
 * @param {AdapterCtx} ctx
 * @param {(msg: string, ...args: unknown[]) => void} log
 */
export async function registerTerminalBridge(ctx, log) {
  const WebSocketCtor = await resolveWebSocket();

  /**
   * @type {Map<string,{ttydWs:any, happySessionId:string, buf:Uint8Array[], bytes:number,
   *   timer:any, postChain:Promise<unknown>}>} keyed by termId
   */
  const bridges = new Map();

  async function ensureHappySession(termId) {
    // tag 实现 get-or-create:重开同一终端复用 session。
    const meta = { kind: 'terminal', termId, host: 'localhost' };
    const resp = await fetch(`${ctx.serverUrl}/v1/sessions`, {
      method: 'POST',
      headers: { 'content-type': 'application/json', Authorization: `Bearer ${ctx.token}` },
      body: JSON.stringify({
        tag: `1agents-terminal:${termId}`,
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

  async function postBody(happySessionId, body) {
    const resp = await fetch(`${ctx.serverUrl}/v3/sessions/${encodeURIComponent(happySessionId)}/messages`, {
      method: 'POST',
      headers: { 'content-type': 'application/json', Authorization: `Bearer ${ctx.token}` },
      body: JSON.stringify({ messages: [{ content: ctx.encrypt(body), localId: randomUUID() }] }),
      signal: AbortSignal.timeout(30000),
    });
    if (!resp.ok) log('terminal post failed', resp.status);
  }

  // 关键:每条 post 串行入队(postChain),保证扇出到 relay 的顺序 = ttyd 产出顺序,
  // 否则两次 flush 的 fetch 竞态会乱序,终端屏幕错乱。
  function chain(bridge, body) {
    bridge.postChain = bridge.postChain
      .then(() => postBody(bridge.happySessionId, body))
      .catch((error) => log('terminal post error', error));
  }

  function flush(bridge) {
    if (bridge.timer) {
      clearTimeout(bridge.timer);
      bridge.timer = null;
    }
    if (bridge.buf.length === 0) return;
    const frames = bridge.buf.map((u8) =>
      Buffer.from(u8.buffer, u8.byteOffset, u8.byteLength).toString('base64')
    );
    bridge.buf = [];
    bridge.bytes = 0;
    chain(bridge, { frames });
  }

  function enqueue(bridge, frameU8) {
    bridge.buf.push(frameU8);
    bridge.bytes += frameU8.length;
    if (bridge.bytes >= FLUSH_MAX_BYTES) {
      flush(bridge);
      return;
    }
    if (!bridge.timer) {
      bridge.timer = setTimeout(() => flush(bridge), FLUSH_INTERVAL_MS);
    }
  }

  async function fetchTtydToken() {
    try {
      const resp = await fetch(`${TTYD_BASE()}/token`, { signal: AbortSignal.timeout(10000) });
      if (!resp.ok) return '';
      const data = await resp.json();
      return data?.token ?? '';
    } catch {
      return ''; // ttyd 未配 credential 时无 token,空串即可
    }
  }

  ctx.registerHandler('terminal-open', async (data) => {
    const termId = data.termId;
    if (!termId) return { success: false, error: 'missing termId' };
    const existing = bridges.get(termId);
    if (existing && existing.ttydWs.readyState === 1) {
      return { success: true, happySessionId: existing.happySessionId };
    }
    try {
      const happySessionId = await ensureHappySession(termId);
      const token = await fetchTtydToken();
      const wsUrl = `${TTYD_BASE().replace(/^http/, 'ws')}/ws`;
      log('terminal open', wsUrl, '→ happy session', happySessionId);

      const ttydWs = new WebSocketCtor(wsUrl, 'tty');
      ttydWs.binaryType = 'arraybuffer';
      const bridge = { ttydWs, happySessionId, buf: [], bytes: 0, timer: null, postChain: Promise.resolve() };
      bridges.set(termId, bridge);

      ttydWs.addEventListener('message', (ev) => enqueue(bridge, toU8(ev.data)));
      ttydWs.addEventListener('close', () => {
        flush(bridge); // 先把缓冲帧排出
        bridges.delete(termId);
        // 哨兵:ttyd WS 关了 → 让 H5 侧 onclose 触发重连(排在所有帧之后)。
        chain(bridge, { event: '__relay_closed' });
      });
      ttydWs.addEventListener('error', (ev) => log('ttydWs error', ev?.message ?? ev));

      await new Promise((resolve, reject) => {
        const t = setTimeout(() => reject(new Error('ttyd ws connect timeout')), 15000);
        ttydWs.addEventListener(
          'open',
          () => {
            clearTimeout(t);
            // ttyd 握手:首条 JSON 帧(无前缀),adapter 自己发(它是连 ttyd 的一方)。
            const init = JSON.stringify({ AuthToken: token, columns: data.cols || 80, rows: data.rows || 24 });
            ttydWs.send(new TextEncoder().encode(init));
            resolve();
          },
          { once: true }
        );
        ttydWs.addEventListener(
          'error',
          (ev) => {
            clearTimeout(t);
            reject(ev?.message ? new Error(ev.message) : new Error('ttyd ws error'));
          },
          { once: true }
        );
      });

      return { success: true, happySessionId };
    } catch (error) {
      bridges.delete(termId);
      log('terminal open failed', error);
      return { success: false, error: error instanceof Error ? error.message : 'open failed' };
    }
  });

  ctx.registerHandler('terminal-input', async (data) => {
    const bridge = bridges.get(data.termId);
    if (!bridge || bridge.ttydWs.readyState !== 1) {
      return { success: false, error: 'no open terminal bridge' };
    }
    try {
      const bytes = Buffer.from(data.raw, 'base64');
      // 吞掉前端自动发的握手帧(首字节 '{' = 0x7b):adapter 已自行握手,避免 ttyd 二次解析。
      if (bytes.length > 0 && bytes[0] === 0x7b) return { success: true };
      bridge.ttydWs.send(bytes);
      return { success: true };
    } catch (error) {
      return { success: false, error: error instanceof Error ? error.message : 'input failed' };
    }
  });

  ctx.registerHandler('terminal-close', async (data) => {
    const bridge = bridges.get(data.termId);
    if (bridge) {
      if (bridge.timer) clearTimeout(bridge.timer);
      try {
        bridge.ttydWs.close();
      } catch {
        /* ignore */
      }
      bridges.delete(data.termId);
    }
    return { success: true };
  });
}
