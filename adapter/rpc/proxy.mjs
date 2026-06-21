/**
 * 控制面代理:`1agents-proxy` —— 把中转来的请求转发到本机 Go 后端 HTTP API。
 *
 * 从 modules/happy-adapter/index.mjs 原样重定位,行为不变。
 * 依赖边界:只用 Node stdlib(fetch)+ 注入的 ctx;不 import happy-cli 内部、不碰 wire/。
 *
 * @typedef {import('./ctxContract.js').AdapterCtx} AdapterCtx
 */

const BACKEND_BASE = () =>
  (process.env.ONEAGENTS_BACKEND_URL || 'http://127.0.0.1:38080').replace(/\/+$/, '');

/**
 * @param {AdapterCtx} ctx
 * @param {(msg: string, ...args: unknown[]) => void} log
 */
export function registerProxy(ctx, log) {
  ctx.registerHandler('1agents-proxy', async (data) => {
    const method = (data.method || 'GET').toUpperCase();
    const url = `${BACKEND_BASE()}${data.path.startsWith('/') ? '' : '/'}${data.path}`;
    try {
      const resp = await fetch(url, {
        method,
        headers: { 'content-type': 'application/json', ...(data.headers || {}) },
        body: method === 'GET' || method === 'HEAD' ? undefined : (data.body ?? undefined),
        signal: AbortSignal.timeout(30000),
      });
      const body = await resp.text();
      return { success: resp.ok, status: resp.status, body };
    } catch (error) {
      log('proxy failed', error);
      return { success: false, error: error instanceof Error ? error.message : 'proxy request failed' };
    }
  });
}
