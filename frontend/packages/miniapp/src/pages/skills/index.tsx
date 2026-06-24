import { Screen } from '../../components/Screen';
import { Placeholder } from '../../components/Placeholder';
import { useT } from '../../hooks/useUI';

// 技能中心 tab — mirrors the web mobile "skills" view. Skeleton for now.
export default function Skills() {
  const t = useT();
  return (
    <Screen titleKey="skills.title">
      <Placeholder title={t('skills.title')} desc={t('skills.desc')} />
    </Screen>
  );
}
