// CC 供应商 tab — lists detected agent providers from the backend catalog.
import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { View, Text, Button } from '@tarojs/components';
import { agentService, type AgentStatus } from '@1agents/core/services/agentService';
import { Screen } from '../../components/Screen';
import { useT } from '../../hooks/useUI';
import { Card } from '../../components/ui/Card';
import { Section } from '../../components/ui/Section';
import { Tag } from '../../components/ui/Tag';
import { EmptyState } from '../../components/ui/EmptyState';
import { Loading } from '../../components/ui/Loading';
import './index.scss';

export default function Providers() {
  const t = useT();
  const [catalog, setCatalog] = useState<AgentStatus[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = async (refresh = false) => {
    setLoading(true);
    setError('');
    try {
      const c = await agentService.getCatalog(refresh);
      setCatalog(c);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load(false);
  }, []);

  let body: ReactNode;
  if (loading) {
    body = <Loading text={t('providers.loading')} />;
  } else if (error) {
    body = (
      <EmptyState
        icon="⚠️"
        title={t('providers.error')}
        desc={error}
        action={{ label: t('common.retry'), onClick: () => load(true) }}
      />
    );
  } else if (catalog.length === 0) {
    body = <EmptyState icon="🔌" title={t('providers.empty')} />;
  } else {
    body = (
      <Section>
        {catalog.map((a) => (
          <Card key={a.type} className="pv-card">
            <View className="pv-card__head">
              <Text className="pv-card__name">{a.label}</Text>
              <Text className="pv-card__type">{a.type}</Text>
            </View>
            <View className="pv-card__tags">
              <Tag
                text={a.installed ? t('providers.installed') : t('providers.notInstalled')}
                tone={a.installed ? 'success' : 'muted'}
              />
              {a.integrated ? <Tag text={t('providers.integrated')} tone="accent" /> : null}
              {a.chatReady ? <Tag text={t('providers.chatReady')} tone="success" /> : null}
            </View>
          </Card>
        ))}
      </Section>
    );
  }

  return (
    <Screen titleKey="providers.title">
      <View className="pv-head">
        <Text className="pv-head__title">{t('providers.title')}</Text>
        <Text className="pv-head__sub">{t('providers.desc')}</Text>
      </View>
      <View className="pv-actions">
        <Button size="mini" className="pv-refresh" onClick={() => load(true)}>
          {t('providers.refresh')}
        </Button>
      </View>
      <View className="pv-body">{body}</View>
    </Screen>
  );
}
