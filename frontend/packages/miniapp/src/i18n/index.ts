// Mini-program i18n engine. Mirrors the web's t() API (frontend/src/i18n) so the
// usage is familiar, but the language state lives in the shared uiStore (storage
// persisted) rather than localStorage. In components, prefer the reactive useT()
// hook (hooks/useUI) so JSX re-renders on language change; the bare t() is for
// non-reactive contexts (toasts, imperative Taro calls).

import { zhCN, enUS } from './dict';

export type Lang = 'zh-CN' | 'en-US';

export const DEFAULT_LANG: Lang = 'zh-CN';
export const SUPPORTED_LANGS: Lang[] = ['zh-CN', 'en-US'];

/**
 * Look up a translation key. Falls back to zh-CN, then to the key itself.
 * @param params Optional {name} interpolation map.
 */
export function t(key: string, lang: Lang, params?: Record<string, string | number>): string {
  const dict = lang === 'en-US' ? enUS : zhCN;
  const value = dict[key] ?? zhCN[key] ?? key;
  if (!params) return value;
  return value.replace(/\{(\w+)\}/g, (_, name) => {
    const v = params[name];
    return v === undefined ? `{${name}}` : String(v);
  });
}
