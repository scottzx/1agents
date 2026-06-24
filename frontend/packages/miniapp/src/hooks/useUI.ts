// React bindings for the uiStore. Components subscribe via these hooks so they
// re-render when language or theme changes (mirrors useChat's bump pattern).

import { useCallback, useEffect, useState } from 'react';
import { t, type Lang } from '../i18n';
import { uiStore, type Theme } from '../store/uiStore';

function useStoreTick() {
  const [, setRev] = useState(0);
  useEffect(() => uiStore.subscribe(() => setRev(r => r + 1)), []);
}

export function useLang(): Lang {
  useStoreTick();
  return uiStore.getLang();
}

export function useTheme(): Theme {
  useStoreTick();
  return uiStore.getTheme();
}

/** Returns a translate fn bound to the current language; re-renders on change. */
export function useT(): (key: string, params?: Record<string, string | number>) => string {
  const lang = useLang();
  return useCallback((key, params) => t(key, lang, params), [lang]);
}

export interface UIControls {
  lang: Lang;
  theme: Theme;
  setLang: (lang: Lang) => void;
  setTheme: (theme: Theme) => void;
}

/** Full controls for settings UIs. */
export function useUI(): UIControls {
  useStoreTick();
  return {
    lang: uiStore.getLang(),
    theme: uiStore.getTheme(),
    setLang: uiStore.setLang,
    setTheme: uiStore.setTheme,
  };
}
