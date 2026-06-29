// Home tab — real session list for the first workspace (over @1agents/core).
import { View, Text, Button } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useEffect, useState } from 'react';

import { workspaceService } from '@1agents/core/services/workspaceService';
import { agentService } from '@1agents/core/services/agentService';
import type { ChatSession } from '@1agents/core/types';
import { AGENT_TYPE_LABELS } from '@1agents/core/protocol/session';

import { Screen } from '../../components/Screen';
import { Card } from '../../components/ui/Card';
import { Section } from '../../components/ui/Section';
import { Tag } from '../../components/ui/Tag';
import { EmptyState } from '../../components/ui/EmptyState';
import { Loading } from '../../components/ui/Loading';
import { useT } from '../../hooks/useUI';
import './index.scss';

export default function Workspaces() {
  const t = useT();
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const reload = async () => {
    setLoading(true);
    setError('');
    try {
      const wss = await workspaceService.list();
      const ws = wss[0];
      if (!ws) {
        setError(t('workspaces.noWorkspace'));
        return;
      }
      const list = await agentService.list(ws.id);
      setSessions(list);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const openChat = (s?: ChatSession) => {
    if (s) {
      Taro.navigateTo({ url: `/pages/chat/index?session_id=${s.id}` });
    } else {
      Taro.navigateTo({ url: '/pages/chat/index' });
    }
  };

  return (
    <Screen titleKey="workspaces.title">
      <View className="ws-content">
        <View className="ws-head">
          <Text className="ws-head__title">{t('workspaces.title')}</Text>
          <Text className="ws-head__sub">{t('workspaces.subtitle')}</Text>
        </View>

        {loading ? (
          <Loading text={t('workspaces.loading')} />
        ) : error ? (
          <EmptyState
            icon="⚠️"
            title={t('workspaces.error')}
            desc={error}
          />
        ) : sessions.length === 0 ? (
          <EmptyState
            icon="💬"
            title={t('workspaces.empty')}
            action={{ label: t('workspaces.newChat'), onClick: () => openChat() }}
          />
        ) : (
          <Section title={t('workspaces.sessions')}>
            {sessions.map((s) => (
              <Card key={s.id} onClick={() => openChat(s)} className="ws-card">
                <Text className="ws-card__name">{s.name}</Text>
                <View className="ws-card__tags">
                  <Tag text={AGENT_TYPE_LABELS[s.agentType]} tone="accent" />
                  <Tag
                    text={s.archived ? t('workspaces.archived') : t('workspaces.active')}
                    tone={s.archived ? 'muted' : 'success'}
                  />
                </View>
              </Card>
            ))}
          </Section>
        )}
      </View>

      <View className="ws-foot">
        <Button className="ws-foot__btn" onClick={() => openChat()}>
          {t('workspaces.newChat')}
        </Button>
      </View>
    </Screen>
  );
}
