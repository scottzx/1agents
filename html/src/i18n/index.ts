import { zhCN, enUS } from './dict';

export type Lang = 'zh-CN' | 'en-US';

export const DEFAULT_LANG: Lang = 'zh-CN';
export const LANG_STORAGE_KEY = '1agents-language';
export const SUPPORTED_LANGS: Lang[] = ['zh-CN', 'en-US'];

/** Read current language from localStorage; fall back to default. */
export function getLang(): Lang {
    if (typeof localStorage === 'undefined') return DEFAULT_LANG;
    const saved = localStorage.getItem(LANG_STORAGE_KEY);
    if (saved === 'zh-CN' || saved === 'en-US') return saved;
    return DEFAULT_LANG;
}

/** Persist language preference. */
export function setLang(lang: Lang): void {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(LANG_STORAGE_KEY, lang);
}

const warned = new Set<string>();

/**
 * Look up a translation key for the given language.
 * Falls back to the zh-CN value if missing in the requested language,
 * then to the key itself (with a console.warn in dev).
 *
 * @param key   Translation key (e.g. 'common.save')
 * @param lang  Target language. Defaults to getLang() — only use the default
 *              in non-reactive contexts (toasts, prompts), never inside JSX
 *              that must re-render on language change.
 * @param params Optional {varName} interpolation map.
 */
export function t(key: string, lang: Lang = getLang(), params?: Record<string, string | number>): string {
    const fromTarget = (lang === 'zh-CN' ? zhCN : enUS)[key];
    const fromZh = zhCN[key];
    const value = fromTarget ?? fromZh ?? key;
    if (value === key && !warned.has(key) && typeof console !== 'undefined') {
        warned.add(key);
        console.warn(`[i18n] missing key: ${key}`);
    }
    if (!params) return value;
    return value.replace(/\{(\w+)\}/g, (_, name) => {
        const v = params[name];
        return v === undefined ? `{${name}}` : String(v);
    });
}
