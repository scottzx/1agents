// Backend the mini-program talks to. WeChat enforces 合法域名 (HTTPS/WSS only,
// no IP/localhost) for released mini-programs, so we point at the configured
// legal domains: agent-dev for development builds, agents for production.
//   request/socket/uploadFile/downloadFile 合法域名:
//     https://agent-dev.dreammate.work · wss://agent-dev.dreammate.work
//     https://agents.dreammate.work     · wss://agents.dreammate.work
// For local backend work, enable 详情 → 本地设置 → 「不校验合法域名…」 in
// WeChat DevTools and override DEFAULT_BACKEND below.
const DEV_BACKEND = 'https://agent-dev.dreammate.work';
const PROD_BACKEND = 'https://agents.dreammate.work';

export const BACKEND_BASE =
  process.env.NODE_ENV === 'development' ? DEV_BACKEND : PROD_BACKEND;

/** ws:// or wss:// origin derived from BACKEND_BASE (http→ws, https→wss). */
export const BACKEND_WS_ORIGIN = BACKEND_BASE.replace(/^http/, 'ws');
