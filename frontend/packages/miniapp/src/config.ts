// Backend the mini-program talks to. No same-origin host here, so we point at
// an explicit address. In the WeChat DevTools simulator, enable
// 详情 → 本地设置 → 「不校验合法域名…」 to allow http/ws to localhost.
export const BACKEND_BASE = 'http://localhost:38080';

/** ws:// or wss:// origin derived from BACKEND_BASE (http→ws, https→wss). */
export const BACKEND_WS_ORIGIN = BACKEND_BASE.replace(/^http/, 'ws');
