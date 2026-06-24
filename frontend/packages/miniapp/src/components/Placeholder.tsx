// Shared "under construction" placeholder for skeleton pages. Each feature page
// that hasn't been ported from the web view yet renders this so the navigation
// (tabBar + sub-page routing) is complete while the content is filled in later.
// Rendered inside a <Screen>, so it inherits the themed surface (no own bg).

import { View, Text } from '@tarojs/components';
import { useT } from '../hooks/useUI';
import './Placeholder.scss';

export interface PlaceholderProps {
  /** Already-translated title. */
  title: string;
  /** Already-translated description. */
  desc?: string;
}

export function Placeholder({ title, desc }: PlaceholderProps) {
  const t = useT();
  return (
    <View className="ph">
      <Text className="ph__icon">🚧</Text>
      <Text className="ph__title">{title}</Text>
      {desc ? <Text className="ph__desc">{desc}</Text> : null}
      <Text className="ph__tag">{t('common.skeleton')}</Text>
    </View>
  );
}
