import { Screen } from '../../components/Screen';
import { Placeholder } from '../../components/Placeholder';
import { useT } from '../../hooks/useUI';

// CC 供应商 tab — mirrors the web mobile "providers" view. Skeleton for now.
export default function Providers() {
  const t = useT();
  return (
    <Screen titleKey="providers.title">
      <Placeholder title={t('providers.title')} desc={t('providers.desc')} />
    </Screen>
  );
}
