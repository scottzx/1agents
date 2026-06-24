// 系统设置：外观(语言/主题) + 可编辑后端地址 + 账户说明
import { View, Text, Input, Button } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';

import { Screen } from '../../components/Screen';
import { Section } from '../../components/ui/Section';
import { Segmented } from '../../components/ui/Segmented';
import { useT, useUI } from '../../hooks/useUI';
import type { Lang } from '../../i18n';
import type { Theme } from '../../store/uiStore';
import { normalizeOrigin, isHttpOrigin } from '@1agents/core/services/apiClient';
import { BACKEND_BASE, BACKEND_OVERRIDE_KEY, defaultBackend, ACCESS_TOKEN_KEY, getAccessToken } from '../../config';
import './index.scss';

export default function Settings() {
  const t = useT();
  const { lang, theme, setLang, setTheme } = useUI();
  const [draft, setDraft] = useState(BACKEND_BASE);
  const [tokenDraft, setTokenDraft] = useState(getAccessToken());

  const langOptions = [
    { value: 'zh-CN' as Lang, label: '中文' },
    { value: 'en-US' as Lang, label: 'English' },
  ];
  const themeOptions = [
    { value: 'light' as Theme, label: t('settings.theme.light') },
    { value: 'dark' as Theme, label: t('settings.theme.dark') },
  ];

  const save = () => {
    if (!isHttpOrigin(draft)) {
      Taro.showToast({ title: t('settings.backend.invalid'), icon: 'none' });
      return;
    }
    Taro.setStorageSync(BACKEND_OVERRIDE_KEY, normalizeOrigin(draft));
    Taro.showToast({ title: t('settings.backend.saved'), icon: 'none' });
  };

  const reset = () => {
    Taro.removeStorageSync(BACKEND_OVERRIDE_KEY);
    setDraft(defaultBackend());
    Taro.showToast({ title: t('settings.backend.saved'), icon: 'none' });
  };

  const saveToken = () => {
    const v = tokenDraft.trim();
    if (v) Taro.setStorageSync(ACCESS_TOKEN_KEY, v);
    else Taro.removeStorageSync(ACCESS_TOKEN_KEY);
    Taro.showToast({ title: t('settings.backend.saved'), icon: 'none' });
  };

  return (
    <Screen titleKey="more.settings">
      <View className="st">
        <Section title={t('settings.appearance')}>
          <View className="st-row">
            <Text className="st-row__label">{t('settings.language')}</Text>
            <Segmented options={langOptions} value={lang} onChange={setLang} />
          </View>
          <View className="st-row">
            <Text className="st-row__label">{t('settings.theme')}</Text>
            <Segmented options={themeOptions} value={theme} onChange={setTheme} />
          </View>
        </Section>

        <Section title={t('settings.connection')}>
          <Input
            className="st-input"
            value={draft}
            placeholder={t('settings.backend.placeholder')}
            onInput={e => setDraft(e.detail.value)}
          />
          <View className="st-btns">
            <Button className="st-btn st-btn--primary" onClick={save}>
              {t('settings.backend.save')}
            </Button>
            <Button className="st-btn" onClick={reset}>
              {t('settings.backend.reset')}
            </Button>
          </View>
          <Text className="st-note">{t('settings.backend.note')}</Text>
        </Section>

        <Section title={t('settings.account')}>
          <Input
            className="st-input"
            value={tokenDraft}
            password
            placeholder={t('settings.token.placeholder')}
            onInput={e => setTokenDraft(e.detail.value)}
          />
          <View className="st-btns">
            <Button className="st-btn st-btn--primary" onClick={saveToken}>
              {t('settings.backend.save')}
            </Button>
          </View>
          <Text className="st-note">{t('settings.token.note')}</Text>
        </Section>
      </View>
    </Screen>
  );
}
