// Backend the mini-program talks to. WeChat enforces 合法域名 (HTTPS/WSS only,
// no IP/localhost) for released mini-programs, so the defaults point at the
// configured legal domains: agent-dev for development, agents for production.
//   request/socket/uploadFile/downloadFile 合法域名:
//     https://agent-dev.dreammate.work · wss://agent-dev.dreammate.work
//     https://agents.dreammate.work     · wss://agents.dreammate.work
// The settings page can override this at runtime (persisted under
// BACKEND_OVERRIDE_KEY); the override wins on next launch. For local backend
// work, enable 详情 → 本地设置 → 「不校验合法域名…」 in WeChat DevTools.
import Taro from '@tarojs/taro';

const DEV_BACKEND = 'http://localhost:3000';
const PROD_BACKEND = 'https://agents.dreammate.work';

/** Storage key for a user-set backend address (settings page). */
export const BACKEND_OVERRIDE_KEY = '1agents-backend';

/** The compiled-in default for this build target (no override applied). */
export function defaultBackend(): string {
  return process.env.NODE_ENV === 'development' ? DEV_BACKEND : PROD_BACKEND;
}

/** Read the stored override, falling back to the build default. */
export function getBackendBase(): string {
  try {
    const v = Taro.getStorageSync(BACKEND_OVERRIDE_KEY);
    if (typeof v === 'string' && v) return v;
  } catch {
    // storage unavailable — use default
  }
  return defaultBackend();
}

/** Effective backend origin, resolved once at module load (override or default). */
export const BACKEND_BASE = getBackendBase();

/** ws:// or wss:// origin derived from BACKEND_BASE (http→ws, https→wss). */
export const BACKEND_WS_ORIGIN = BACKEND_BASE.replace(/^http/, 'ws');

/**
 * Access token (the "wire-Token account" the 1agents backend's authMiddleware
 * requires for any non-localhost access). The web H5 is same-origin and leans on
 * the ra_access_token cookie the gate sets; weapp can't rely on that, so we store
 * the same token here and attach it explicitly (Authorization header on requests,
 * ?access_token= on the WebSocket and web-view URLs). Set on the settings page.
 */
export const ACCESS_TOKEN_KEY = '1agents-access-token';

/** Current access token, '' if none. Read live so a settings change applies on the next request. */
export function getAccessToken(): string {
  try {
    const v = Taro.getStorageSync(ACCESS_TOKEN_KEY);
    return typeof v === 'string' ? v : '';
  } catch {
    return '';
  }
}
