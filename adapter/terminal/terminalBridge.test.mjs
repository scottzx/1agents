/**
 * terminalBridge.mjs 行为测试(node:test,零依赖,零构建)。
 *
 *   node --test adapter/terminal/      # 或 npm test(在 adapter/)
 *
 * 用假的 globalThis.WebSocket / fetch 驱动 registerTerminalBridge,断言三条不变量:
 *   1) terminal-input 吞掉前端握手帧('{'),其余原样转发给 ttyd。
 *   2) 下行 ttyd 帧批量成单条 { frames:[...] } 扇出,且保持顺序。
 *   3) ttyd 关闭后扇出 __relay_closed 哨兵,且排在所有帧之后。
 *
 * 约定:测试里把 ctx.encrypt 做成 JSON.stringify(可读),便于从扇出 body 里读回明文。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { registerTerminalBridge } from './terminalBridge.mjs';

const delay = (ms) => new Promise((r) => setTimeout(r, ms));
const b64 = (str) => Buffer.from(str, 'utf8').toString('base64');
const enc = (str) => new TextEncoder().encode(str);

class FakeWS {
  constructor(url, protocol) {
    this.url = url;
    this.protocol = protocol;
    this.readyState = 0;
    this.binaryType = 'blob';
    this.sent = [];
    this._l = {};
    FakeWS.last = this;
  }
  addEventListener(type, fn, opts) {
    (this._l[type] ||= []).push({ fn, once: opts && opts.once });
  }
  emit(type, ev) {
    for (const h of (this._l[type] || []).slice()) {
      if (h.once) this._l[type] = this._l[type].filter((x) => x !== h);
      h.fn(ev);
    }
  }
  send(data) {
    this.sent.push(data);
  }
  close() {
    this.readyState = 3;
    this.emit('close', {});
  }
  fakeOpen() {
    this.readyState = 1;
    this.emit('open', {});
  }
}

/** 装好假 WebSocket / fetch,注册 bridge,完成 ttyd open 握手,返回 { handlers, ws, posts }。 */
async function setup() {
  const posts = []; // 扇出到 relay 的明文 body 列表
  const realWS = globalThis.WebSocket;
  const realFetch = globalThis.fetch;
  globalThis.WebSocket = FakeWS;
  globalThis.fetch = async (url, init) => {
    const u = String(url);
    if (u.endsWith('/v1/sessions')) {
      return { ok: true, json: async () => ({ session: { id: 'happy-1' } }) };
    }
    if (u.endsWith('/token')) {
      return { ok: true, json: async () => ({ token: 'T' }) };
    }
    if (u.includes('/v3/sessions/')) {
      const body = JSON.parse(init.body);
      // content = ctx.encrypt(body) = JSON.stringify(body)(见 fakeCtx)
      posts.push(JSON.parse(body.messages[0].content));
      return { ok: true, json: async () => ({}) };
    }
    return { ok: true, json: async () => ({}) };
  };

  const handlers = {};
  const ctx = {
    registerHandler: (m, h) => {
      handlers[m] = h;
    },
    serverUrl: 'http://relay.test',
    token: 'tok',
    encrypt: (b) => JSON.stringify(b),
    decrypt: () => null,
  };

  await registerTerminalBridge(ctx, () => {});

  // 触发 terminal-open;它先 await 建 session/取 token,之后才 new FakeWS,故轮询等 ws 出现再 open。
  const openP = handlers['terminal-open']({ termId: 't1', cols: 80, rows: 24 });
  for (let i = 0; i < 50 && !FakeWS.last; i++) await delay(2);
  const ws = FakeWS.last;
  ws.fakeOpen();
  const res = await openP;
  assert.equal(res.success, true);

  const restore = () => {
    globalThis.WebSocket = realWS;
    globalThis.fetch = realFetch;
    FakeWS.last = undefined;
  };
  return { handlers, ws, posts, restore };
}

test('terminal-open: 完成 ttyd 握手(adapter 自发首帧 JSON)', async () => {
  const { ws, restore } = await setup();
  try {
    // 握手帧是首条 send,且是 '{' 开头的 JSON。
    const first = Buffer.from(ws.sent[0]).toString('utf8');
    assert.equal(first[0], '{');
    assert.match(first, /"AuthToken":"T"/);
    assert.match(first, /"columns":80/);
  } finally {
    restore();
  }
});

test('terminal-input: 吞掉前端握手帧,其余原样转发', async () => {
  const { handlers, ws, restore } = await setup();
  try {
    const before = ws.sent.length;
    // 前端在 onSocketOpen 自动发的握手帧(首字节 '{')→ 被吞。
    await handlers['terminal-input']({ termId: 't1', raw: b64('{"AuthToken":"x","columns":80,"rows":24}') });
    assert.equal(ws.sent.length, before, '握手帧不应转发给 ttyd');
    // 普通输入帧('0'+data)→ 透传。
    await handlers['terminal-input']({ termId: 't1', raw: b64('0ls\r') });
    assert.equal(ws.sent.length, before + 1);
    assert.equal(Buffer.from(ws.sent[before]).toString('utf8'), '0ls\r');
  } finally {
    restore();
  }
});

test('下行:多个 ttyd 帧批量成单条 { frames }、保序', async () => {
  const { ws, posts, restore } = await setup();
  try {
    ws.emit('message', { data: enc('0hello ').buffer });
    ws.emit('message', { data: enc('0world').buffer });
    await delay(40); // 等过 flush 窗口(默认 16ms)
    assert.equal(posts.length, 1, '两帧应合并成一条扇出');
    const frames = posts[0].frames.map((f) => Buffer.from(f, 'base64').toString('utf8'));
    assert.deepEqual(frames, ['0hello ', '0world']);
  } finally {
    restore();
  }
});

test('关闭:flush 残帧后扇出 __relay_closed 哨兵(排在帧之后)', async () => {
  const { ws, posts, restore } = await setup();
  try {
    ws.emit('message', { data: enc('0tail').buffer });
    ws.close();
    await delay(40);
    // 最后一条是哨兵;它之前存在带 frames 的扇出。
    const last = posts[posts.length - 1];
    assert.deepEqual(last, { event: '__relay_closed' });
    const framed = posts.filter((p) => Array.isArray(p.frames));
    assert.ok(framed.length >= 1, '残帧应在哨兵之前扇出');
    assert.deepEqual(
      framed[framed.length - 1].frames.map((f) => Buffer.from(f, 'base64').toString('utf8')),
      ['0tail']
    );
  } finally {
    restore();
  }
});
