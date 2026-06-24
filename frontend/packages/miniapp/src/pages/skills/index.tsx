// 技能中心 tab — tasteful skeleton, awaiting real data.
import { View, Text } from '@tarojs/components';
import { Screen } from '../../components/Screen';
import { EmptyState } from '../../components/ui/EmptyState';
import { useT } from '../../hooks/useUI';
import './index.scss';

export default function Skills() {
  const t = useT();
  return (
    <Screen titleKey="skills.title">
      <View className="sk-head">
        <Text className="sk-head__title">{t('skills.title')}</Text>
        <Text className="sk-head__sub">{t('skills.desc')}</Text>
      </View>
      <EmptyState icon="🧩" title={t('skills.comingSoon')} />
    </Screen>
  );
}
