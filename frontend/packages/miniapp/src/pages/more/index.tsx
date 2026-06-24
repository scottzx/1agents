import { View, Text } from '@tarojs/components';
import Taro from '@tarojs/taro';

import { Screen } from '../../components/Screen';
import { useT } from '../../hooks/useUI';
import './index.scss';

// 更多 tab — entry to the sub-pages that don't warrant their own tab slot.
// Mirrors the web mobile "more" menu (discovery + settings).
const ENTRIES = [
  { icon: '🧭', titleKey: 'more.discovery', descKey: 'more.discovery.desc', url: '/pages/discovery/index' },
  { icon: '⚙️', titleKey: 'more.settings', descKey: 'more.settings.desc', url: '/pages/settings/index' },
];

export default function More() {
  const t = useT();
  return (
    <Screen titleKey="more.title">
      <View className="more">
        <View className="more__header">
          <Text className="more__title">{t('more.title')}</Text>
          <Text className="more__subtitle">{t('more.subtitle')}</Text>
        </View>

        <View className="more__list">
          {ENTRIES.map(e => (
            <View key={e.url} className="more__row" onClick={() => Taro.navigateTo({ url: e.url })}>
              <Text className="more__row-icon">{e.icon}</Text>
              <View className="more__row-text">
                <Text className="more__row-title">{t(e.titleKey)}</Text>
                <Text className="more__row-desc">{t(e.descKey)}</Text>
              </View>
              <Text className="more__row-chevron">›</Text>
            </View>
          ))}
        </View>
      </View>
    </Screen>
  );
}
