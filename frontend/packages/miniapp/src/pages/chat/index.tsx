import { useEffect, useMemo, useState } from 'react';
import { View, Text, ScrollView, Input, Button } from '@tarojs/components';
import type { ChatSession } from '@1agents/core/types';
import type { ChatItem } from '@1agents/core/protocol/types';
import { workspaceService } from '@1agents/core/services/workspaceService';
import { agentService } from '@1agents/core/services/agentService';

import { Screen } from '../../components/Screen';
import { useT } from '../../hooks/useUI';
import { useChat } from '../../hooks/useChat';

import './index.scss';

// Terminal-style ScrollView chat (xterm.js isn't available on weapp). Bootstraps
// a session against the first workspace, then streams agent messages over the
// shared ChatBridgeManager (native WebSocket via Taro.connectSocket).
export default function Index() {
  const t = useT();
  const [session, setSession] = useState<ChatSession | null>(null);
  const [bootError, setBootError] = useState('');
  const [draft, setDraft] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const workspaces = await workspaceService.list();
        const ws = workspaces[0];
        if (!ws) {
          setBootError(t('chat.noWorkspace'));
          return;
        }
        const s = await agentService.index({
          workspace_id: ws.id,
          name: t('chat.sessionName'),
          agent_type: 'claudecode',
        });
        if (!cancelled) setSession(s);
      } catch (e) {
        if (!cancelled) setBootError(String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const { items, connection, ready, send, respondPermission } = useChat(session);

  const statusText = useMemo(() => {
    if (bootError) return t('chat.bootFailed', { error: bootError });
    if (!session) return t('chat.booting');
    return t('chat.connection', { state: connection }) + (ready ? ` · ${t('chat.ready')}` : '');
  }, [bootError, session, connection, ready, t]);

  const onSend = () => {
    const text = draft.trim();
    if (!text || !ready) return;
    send(text);
    setDraft('');
  };

  return (
    <Screen titleKey="chat.title">
      <View className="chat">
        <View className="chat__status">
          <Text className={`chat__dot chat__dot--${connection}`} />
          <Text className="chat__status-text">{statusText}</Text>
        </View>

        <ScrollView className="chat__stream" scrollY scrollIntoView={items.length ? `i${items.length - 1}` : ''}>
          {items.map((it, i) => (
            <View id={`i${i}`} key={it.id} className="chat__item">
              {renderItem(it, respondPermission, t)}
            </View>
          ))}
          {!items.length && session && <Text className="chat__hint">{t('chat.startHint')}</Text>}
        </ScrollView>

        <View className="chat__composer">
          <Input
            className="chat__input"
            value={draft}
            placeholder={ready ? t('chat.inputPlaceholder') : t('chat.inputDisabled')}
            confirmType="send"
            onInput={e => setDraft(e.detail.value)}
            onConfirm={onSend}
          />
          <Button className="chat__send" disabled={!ready || !draft.trim()} onClick={onSend}>
            {t('chat.send')}
          </Button>
        </View>
      </View>
    </Screen>
  );
}

function renderItem(
  it: ChatItem,
  respondPermission: (requestId: string, decision: 'allow_once' | 'reject_once') => void,
  t: (key: string, params?: Record<string, string | number>) => string
) {
  switch (it.kind) {
    case 'user':
      return <Text className="chat__user">{it.content}</Text>;
    case 'assistant_text':
      return <Text className="chat__assistant">{it.content}</Text>;
    case 'thinking':
      return <Text className="chat__thinking">{it.content}</Text>;
    case 'tool_use':
      return <Text className="chat__tool">🔧 {it.toolName}</Text>;
    case 'tool_result':
      return (
        <Text className={`chat__tool-result${it.isError ? ' chat__tool-result--error' : ''}`}>{it.content}</Text>
      );
    case 'permission_request':
      return (
        <View className="chat__perm">
          <Text className="chat__perm-title">🔐 {t('chat.perm.request', { tool: it.toolName })}</Text>
          <View className="chat__perm-actions">
            <Button size="mini" onClick={() => respondPermission(it.requestId, 'allow_once')}>
              {t('chat.perm.allow')}
            </Button>
            <Button size="mini" onClick={() => respondPermission(it.requestId, 'reject_once')}>
              {t('chat.perm.reject')}
            </Button>
          </View>
        </View>
      );
    case 'error':
      return <Text className="chat__error">{it.content}</Text>;
    default:
      return null;
  }
}
