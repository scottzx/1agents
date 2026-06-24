// Per-page theme boundary. weapp can't put a dynamic class on `page`, so every
// page wraps its content in <Screen>: it carries the `.theme--{theme}` class
// (whence the CSS variables in style/tokens.scss cascade) and the base `.screen`
// surface, and syncs the NATIVE chrome (nav bar, page background, tab bar,
// optional nav title) to the active theme/language.
//
// Native chrome is per-page and applied imperatively, so a theme/language change
// made on another page (e.g. the settings sub-page) doesn't reach a backgrounded
// tab page. We therefore re-apply it both when theme/lang change AND on every
// page show (useDidShow) — the latter is what fixes stale nav bar / tab bar
// colors after navigating back from settings.

import { type ReactNode, useCallback, useEffect } from 'react';
import { View } from '@tarojs/components';
import Taro, { useDidShow } from '@tarojs/taro';

import { useTheme, useLang } from '../hooks/useUI';
import { t } from '../i18n';
import { THEME_CHROME, uiStore } from '../store/uiStore';

export interface ScreenProps {
  children: ReactNode;
  /** Extra class on the screen root. */
  className?: string;
  /** i18n key for the native navigation bar title; updates on language change. */
  titleKey?: string;
}

export function Screen({ children, className, titleKey }: ScreenProps) {
  const theme = useTheme();
  const lang = useLang();

  const applyChrome = useCallback(() => {
    const c = THEME_CHROME[theme];
    try {
      Taro.setNavigationBarColor({ frontColor: c.frontColor, backgroundColor: c.bg });
    } catch {
      /* nav bar not present */
    }
    try {
      Taro.setBackgroundColor({ backgroundColor: c.bg, backgroundColorTop: c.bg, backgroundColorBottom: c.bg });
    } catch {
      /* unsupported on this host */
    }
    // Re-skin the tab bar (labels + colors). No-ops off a tab page; runs in the
    // tab page's own context via useDidShow, which is where it actually lands.
    uiStore.syncTabBar();
    if (titleKey) {
      try {
        Taro.setNavigationBarTitle({ title: t(titleKey, lang) });
      } catch {
        /* nav bar not present */
      }
    }
  }, [theme, lang, titleKey]);

  // Apply when theme/lang change while this page is active.
  useEffect(() => {
    applyChrome();
  }, [applyChrome]);

  // Re-apply every time the page shows (tab switch / navigate back) so chrome
  // changed elsewhere catches up.
  useDidShow(() => {
    applyChrome();
  });

  return <View className={`screen theme--${theme}${className ? ` ${className}` : ''}`}>{children}</View>;
}
