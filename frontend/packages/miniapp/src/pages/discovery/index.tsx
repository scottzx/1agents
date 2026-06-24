// 发现 sub-page (reached from 更多). Mirrors the web DiscoveryPanel. Skeleton.
import { View, Text } from '@tarojs/components';
import { Screen } from '../../components/Screen';
import { Section } from '../../components/ui/Section';
import { EmptyState } from '../../components/ui/EmptyState';
import { useT } from '../../hooks/useUI';
import './index.scss';

export default function Discovery() {
  const t = useT();
  return (
    <Screen titleKey="discovery.title">
      <View className="dc-head">
        <Text className="dc-head__title">{t('discovery.title')}</Text>
        <Text className="dc-head__sub">{t('discovery.desc')}</Text>
      </View>
      <View className="dc-body">
        <Section title={t('discovery.featured')}>
          <EmptyState icon="✨" title={t('discovery.comingSoon')} />
        </Section>
        <Section title={t('discovery.opensource')}>
          <EmptyState icon="🌱" title={t('discovery.comingSoon')} />
        </Section>
      </View>
    </Screen>
  );
}
