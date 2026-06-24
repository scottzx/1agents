// Singleton UI store for language + theme — the mini-program counterpart of the
// web's localStorage-backed i18n/theme prefs. Sticks to the codebase's existing
// store+subscribe pattern (see services/chat.ts + hooks/useChat.ts): a module
// singleton with a listener set, consumed by React via hooks/useUI.
//
// State is persisted with Taro storage (works on weapp and RN) and seeded from
// the system on first run (system dark mode + system language).

import Taro from '@tarojs/taro';
import { DEFAULT_LANG, SUPPORTED_LANGS, t, type Lang } from '../i18n';

export type Theme = 'light' | 'dark';

const LANG_KEY = '1agents-language';
const THEME_KEY = '1agents-theme';

/** Native-chrome colors per theme — kept in sync with style/tokens.scss --bg-page. */
export const THEME_CHROME: Record<Theme, { bg: string; frontColor: '#ffffff' | '#000000' }> = {
  light: { bg: '#f4f6f9', frontColor: '#000000' },
  dark: { bg: '#080b11', frontColor: '#ffffff' },
};

function readSystem(): { lang: Lang; theme: Theme } {
  let lang: Lang = DEFAULT_LANG;
  let theme: Theme = 'dark';
  try {
    const info = Taro.getSystemInfoSync();
    if (info.language && info.language.toLowerCase().startsWith('en')) lang = 'en-US';
    if (info.theme === 'light' || info.theme === 'dark') theme = info.theme;
  } catch {
    // getSystemInfoSync unavailable (e.g. RN before bridge ready) — keep defaults.
  }
  return { lang, theme };
}

function read(key: string): string | null {
  try {
    const v = Taro.getStorageSync(key);
    return v === '' || v === undefined || v === null ? null : (v as string);
  } catch {
    return null;
  }
}

function write(key: string, value: string): void {
  try {
    Taro.setStorageSync(key, value);
  } catch {
    // best-effort; ignore quota/unavailable errors
  }
}

const sys = readSystem();
const storedLang = read(LANG_KEY);
const storedTheme = read(THEME_KEY);

let lang: Lang = SUPPORTED_LANGS.includes(storedLang as Lang) ? (storedLang as Lang) : sys.lang;
let theme: Theme = storedTheme === 'light' || storedTheme === 'dark' ? storedTheme : sys.theme;

const listeners = new Set<() => void>();
function emit() {
  listeners.forEach(fn => fn());
}

/** The tab-bar pages, in tab order. Order matches app.config.ts `tabBar.list`. */
const TAB_PAGES = ['pages/workspaces/index', 'pages/providers/index', 'pages/skills/index', 'pages/more/index'];
const TAB_ITEM_KEYS = ['tab.workspaces', 'tab.providers', 'tab.skills', 'tab.more'];

/** True only when the currently-shown page owns the tab bar. The tab-bar APIs
 *  report failure via an async `fail` callback (not a throw), so try/catch can't
 *  suppress their console noise — we must avoid calling them off a tab page. */
function onTabPage(): boolean {
  try {
    const path = Taro.getCurrentInstance()?.router?.path;
    if (!path) return false;
    return TAB_PAGES.includes(path.replace(/^\//, '').split('?')[0]);
  } catch {
    return false;
  }
}

/** Push translated labels + themed colors onto the native tab bar. No-op unless
 *  the current page is a tab page (avoids "not TabBar page" errors). */
function syncTabBar() {
  if (!onTabPage()) return;
  const noop = () => {};
  TAB_ITEM_KEYS.forEach((key, index) => {
    Taro.setTabBarItem({ index, text: t(key, lang), success: noop, fail: noop });
  });
  const c = THEME_CHROME[theme];
  Taro.setTabBarStyle({
    color: '#8d96a0',
    selectedColor: theme === 'dark' ? '#388bfd' : '#0969da',
    backgroundColor: c.bg,
    borderStyle: theme === 'dark' ? 'white' : 'black',
    success: noop,
    fail: noop,
  });
}

export const uiStore = {
  getLang: () => lang,
  getTheme: () => theme,
  /** Re-apply tab bar labels + colors for the current lang/theme. Safe to call
   *  from any page (no-ops off a tab page); used by <Screen> on page show so a
   *  theme/lang change made on a sub-page lands when a tab page reappears. */
  syncTabBar,
  setLang(next: Lang) {
    if (next === lang) return;
    lang = next;
    write(LANG_KEY, next);
    syncTabBar();
    emit();
  },
  setTheme(next: Theme) {
    if (next === theme) return;
    theme = next;
    write(THEME_KEY, next);
    syncTabBar();
    emit();
  },
  subscribe(fn: () => void): () => void {
    listeners.add(fn);
    return () => listeners.delete(fn);
  },
};
