// Per-page theme boundary. weapp can't put a dynamic class on `page`, so every
// page wraps its content in <Screen>: it carries the `.theme--{theme}` class
// (whence the CSS variables in style/tokens.scss cascade) and the base `.screen`
// surface, and syncs the native chrome (nav bar, page background, optional
// nav title) to the active theme/language.

import { type ReactNode, useEffect } from 'react';
import { View } from '@tarojs/components';
import Taro from '@tarojs/taro';

import { useTheme, useLang } from '../hooks/useUI';
import { t } from '../i18n';
import { THEME_CHROME } from '../store/uiStore';

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

  useEffect(() => {
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
  }, [theme]);

  useEffect(() => {
    if (!titleKey) return;
    try {
      Taro.setNavigationBarTitle({ title: t(titleKey, lang) });
    } catch {
      /* nav bar not present */
    }
  }, [titleKey, lang]);

  return <View className={`screen theme--${theme}${className ? ` ${className}` : ''}`}>{children}</View>;
}
