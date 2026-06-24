// 更多 tab — entry to sub-pages (discovery + settings) via shared Section/Cell.
import { View, Text } from '@tarojs/components';
import Taro from '@tarojs/taro';

import { Screen } from '../../components/Screen';
import { Section } from '../../components/ui/Section';
import { Cell } from '../../components/ui/Cell';
import { useT } from '../../hooks/useUI';
import './index.scss';

export default function More() {
  const t = useT();
  return (
    <Screen titleKey="more.title">
      <View className="more-head">
        <Text className="more-head__title">{t('more.title')}</Text>
        <Text className="more-head__sub">{t('more.subtitle')}</Text>
      </View>
      <View className="more-body">
        <Section>
          <Cell
            icon="🧭"
            title={t('more.discovery')}
            desc={t('more.discovery.desc')}
            arrow
            onClick={() => Taro.navigateTo({ url: '/pages/discovery/index' })}
          />
          <Cell
            icon="⚙️"
            title={t('more.settings')}
            desc={t('more.settings.desc')}
            arrow
            onClick={() => Taro.navigateTo({ url: '/pages/settings/index' })}
          />
        </Section>
      </View>
    </Screen>
  );
}
