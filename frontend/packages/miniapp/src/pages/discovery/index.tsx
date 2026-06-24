import { Screen } from '../../components/Screen';
import { Placeholder } from '../../components/Placeholder';
import { useT } from '../../hooks/useUI';

// 发现 sub-page (reached from 更多). Mirrors the web DiscoveryPanel. Skeleton.
export default function Discovery() {
  const t = useT();
  return (
    <Screen titleKey="discovery.title">
      <Placeholder title={t('discovery.title')} desc={t('discovery.desc')} />
    </Screen>
  );
}
