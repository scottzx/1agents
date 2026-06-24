import { View, Text } from '@tarojs/components';

import { Screen } from '../../components/Screen';
import { useT, useUI } from '../../hooks/useUI';
import type { Lang } from '../../i18n';
import type { Theme } from '../../store/uiStore';
import { BACKEND_BASE } from '../../config';
import './index.scss';

// 系统设置 sub-page (reached from 更多). The appearance section is the live
// control panel for the i18n + theme token system; backend address is shown
// (editable/persisted is a later step toward real-device / WeChat release).
export default function Settings() {
  const t = useT();
  const { lang, theme, setLang, setTheme } = useUI();

  const langOptions: Array<{ value: Lang; label: string }> = [
    { value: 'zh-CN', label: '中文' },
    { value: 'en-US', label: 'English' },
  ];
  const themeOptions: Array<{ value: Theme; label: string }> = [
    { value: 'light', label: t('settings.theme.light') },
    { value: 'dark', label: t('settings.theme.dark') },
  ];

  return (
    <Screen titleKey="more.settings">
      <View className="settings">
        <View className="settings__group">
          <Text className="settings__group-title">{t('settings.appearance')}</Text>

          <View className="settings__row">
            <Text className="settings__label">{t('settings.language')}</Text>
            <View className="seg">
              {langOptions.map(o => (
                <View
                  key={o.value}
                  className={`seg__item${lang === o.value ? ' seg__item--active' : ''}`}
                  onClick={() => setLang(o.value)}
                >
                  {o.label}
                </View>
              ))}
            </View>
          </View>

          <View className="settings__row">
            <Text className="settings__label">{t('settings.theme')}</Text>
            <View className="seg">
              {themeOptions.map(o => (
                <View
                  key={o.value}
                  className={`seg__item${theme === o.value ? ' seg__item--active' : ''}`}
                  onClick={() => setTheme(o.value)}
                >
                  {o.label}
                </View>
              ))}
            </View>
          </View>
        </View>

        <View className="settings__group">
          <Text className="settings__group-title">{t('settings.connection')}</Text>
          <View className="settings__row">
            <Text className="settings__label">{t('settings.backend')}</Text>
            <Text className="settings__value">{BACKEND_BASE}</Text>
          </View>
          <Text className="settings__note">{t('settings.backend.note')}</Text>
        </View>

        <View className="settings__group">
          <Text className="settings__group-title">{t('settings.account')}</Text>
          <Text className="settings__note">{t('settings.account.note')}</Text>
        </View>
      </View>
    </Screen>
  );
}
