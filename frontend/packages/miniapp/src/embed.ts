// web-view embed URLs.
//
// The mini-program embeds the web's heavy module UIs (HarnessKit, cc-connect) in a
// <web-view> instead of re-implementing them in Taro — the cross-host equivalent
// of how desktop/web embed them in an iframe. The embedded page lives on the
// backend domain, so we build an absolute URL on BACKEND_BASE and carry the
// access token in the query (authMiddleware mechanism A): the gate authenticates
// the page load and sets the ra_access_token cookie, after which the page's own
// same-origin /api/* calls ride that cookie. cc-connect additionally needs its
// ManagementToken, but that's baked into the URL the backend mints
// (workspaceService.getCcConnectUrl) — the client only ever handles the access
// token.

import { BACKEND_BASE, getAccessToken } from './config';

/** Append ?access_token= so the backend gate admits the web-view and seeds the cookie. No-op when unset. */
export function withAccessToken(url: string): string {
  const token = getAccessToken();
  if (!token) return url;
  return `${url}${url.includes('?') ? '&' : '?'}access_token=${encodeURIComponent(token)}`;
}

/** Absolutize a backend-relative URL (e.g. core's getCcConnectUrl result) onto BACKEND_BASE, with the token. */
export function absoluteBackendUrl(relative: string): string {
  if (/^https?:\/\//.test(relative)) return withAccessToken(relative);
  return withAccessToken(`${BACKEND_BASE}${relative.startsWith('/') ? '' : '/'}${relative}`);
}

/** The HarnessKit Extensions embed URL — theme/lang boot + access token. */
export function skillsEmbedUrl(theme: string, lang: string): string {
  const base = `${BACKEND_BASE}/extensions/?theme=${encodeURIComponent(theme)}&lang=${encodeURIComponent(lang)}`;
  return withAccessToken(base);
}
