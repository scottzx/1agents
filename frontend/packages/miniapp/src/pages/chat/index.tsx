import { useEffect, useMemo, useState } from 'react';
import { View, Text, ScrollView, Input, Button, RichText } from '@tarojs/components';
import Taro from '@tarojs/taro';
import type { ChatSession } from '@1agents/core/types';
import type { ChatItem } from '@1agents/core/protocol/types';
import { workspaceService } from '@1agents/core/services/workspaceService';
import { agentService } from '@1agents/core/services/agentService';

import { Screen } from '../../components/Screen';
import { useT } from '../../hooks/useUI';
import { useChat } from '../../hooks/useChat';
import { renderMarkdown } from '../../utils/markdown';

import './index.scss';

const AGENT_LABELS: Record<string, string> = {
  claudecode: 'Claude Code',
  codex: 'Codex',
  cursor: 'Cursor',
};

export interface GroupedToolCall {
  id: string;
  toolCallId?: string;
  toolName: string;
  input: string;
  output?: string;
  isError?: boolean;
  permission?: {
    requestId: string;
    toolName: string;
    input: string;
    options: Array<{ text: string; data: string }>;
    resolved?: 'allow' | 'deny';
  };
}

export type GroupedChatItem =
  | { id: string; kind: 'user'; content: string; createdAt: number; queueStatus?: 'queued'; queueRequestId?: string }
  | { id: string; kind: 'assistant_text'; content: string; createdAt: number; streaming: boolean }
  | { id: string; kind: 'thinking'; content: string; createdAt: number }
  | {
      id: string;
      kind: 'tool_group';
      calls: GroupedToolCall[];
      createdAt: number;
      pending?: boolean;
    }
  | { id: string; kind: 'error'; content: string; createdAt: number };

function groupChatItems(items: ChatItem[]): GroupedChatItem[] {
  const grouped: GroupedChatItem[] = [];
  const pendingCalls: GroupedToolCall[] = [];

  for (const item of items) {
    if (item.kind === 'tool_use') {
      let lastGroup = grouped[grouped.length - 1];
      if (!lastGroup || lastGroup.kind !== 'tool_group' || lastGroup.pending) {
        lastGroup = {
          id: `group-${item.id}`,
          kind: 'tool_group',
          calls: [],
          createdAt: item.createdAt,
        };
        grouped.push(lastGroup);
      }

      for (const call of item.calls) {
        const callId = call.toolCallId;
        const existingCall = callId ? lastGroup.calls.find(c => c.toolCallId === callId) : null;
        if (existingCall) {
          existingCall.toolName = call.toolName;
          existingCall.input = call.input;
          existingCall.output = call.output;
          existingCall.isError = call.isError;
          if (call.permission) existingCall.permission = call.permission;
        } else {
          lastGroup.calls.push({
            id: `call-${callId || lastGroup.calls.length}`,
            toolCallId: callId,
            toolName: call.toolName,
            input: call.input,
            output: call.output,
            isError: call.isError,
            ...(call.permission ? { permission: call.permission } : {}),
          });
        }
      }
    } else if (item.kind === 'tool_result') {
      const callId = item.toolCallId;
      let matchedCall: GroupedToolCall | null = null;
      let matchedGroup: Extract<GroupedChatItem, { kind: 'tool_group' }> | null = null;

      if (callId) {
        for (let i = grouped.length - 1; i >= 0; i--) {
          const g = grouped[i];
          if (g.kind === 'tool_group' && !g.pending) {
            const c = g.calls.find(call => call.toolCallId === callId);
            if (c) {
              matchedCall = c;
              matchedGroup = g;
              break;
            }
          }
        }
      }

      if (!matchedCall) {
        for (let i = grouped.length - 1; i >= 0; i--) {
          const g = grouped[i];
          if (g.kind === 'tool_group' && !g.pending) {
            matchedGroup = g;
            break;
          }
        }
        if (matchedGroup && matchedGroup.calls.length > 0) {
          matchedCall = matchedGroup.calls.find(c => c.output === undefined) || null;
          if (!matchedCall) {
            matchedCall = matchedGroup.calls[matchedGroup.calls.length - 1];
          }
        }
      }

      if (matchedCall) {
        matchedCall.output = item.content;
        matchedCall.isError = item.isError;
      } else {
        pendingCalls.push({
          id: `pending-result-${item.id}`,
          toolCallId: callId,
          toolName: item.toolName || 'tool',
          input: '',
          output: item.content,
          isError: item.isError,
        });
      }
    } else if (item.kind === 'permission_request') {
      const callId = item.toolCallId;
      let matchedCall: GroupedToolCall | null = null;

      if (callId) {
        for (let i = grouped.length - 1; i >= 0; i--) {
          const g = grouped[i];
          if (g.kind === 'tool_group' && !g.pending) {
            const c = g.calls.find(call => call.toolCallId === callId);
            if (c) {
              matchedCall = c;
              break;
            }
          }
        }
      }

      if (matchedCall) {
        matchedCall.permission = {
          requestId: item.requestId,
          toolName: item.toolName,
          input: item.input,
          options: item.options,
          ...(item.resolved ? { resolved: item.resolved } : {}),
        };
      } else {
        pendingCalls.push({
          id: `pending-permission-${item.id}`,
          toolCallId: callId,
          toolName: item.toolName,
          input: item.input,
          output: undefined,
          isError: undefined,
          permission: {
            requestId: item.requestId,
            toolName: item.toolName,
            input: item.input,
            options: item.options,
            ...(item.resolved ? { resolved: item.resolved } : {}),
          },
        });
      }
    } else {
      grouped.push(item as GroupedChatItem);
    }
  }

  if (pendingCalls.length > 0) {
    grouped.push({
      id: 'pending-group',
      kind: 'tool_group',
      calls: pendingCalls,
      createdAt: Date.now(),
      pending: true,
    });
  }

  return grouped;
}

// Modernized chat interface. Streams agent messages over the
// shared ChatBridgeManager (native WebSocket via Taro.connectSocket)
// and renders markdown formatted AI responses with rich components.
export default function Index() {
  const t = useT();
  const [session, setSession] = useState<ChatSession | null>(null);
  const [bootError, setBootError] = useState('');
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(true);

  // Creation panel states
  const [agentType, setAgentType] = useState<string>('claudecode');
  const [sessionName, setSessionName] = useState<string>('');
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const router = Taro.getCurrentInstance().router;
        const sessionIdParam = router?.params.session_id;

        const workspaces = await workspaceService.list();
        const ws = workspaces[0];
        if (!ws) {
          setBootError(t('chat.noWorkspace'));
          return;
        }

        if (sessionIdParam) {
          // Load existing session by ID
          const sessions = await agentService.list(ws.id);
          const found = sessions.find(s => s.id === sessionIdParam);
          if (found) {
            if (!cancelled) {
              setSession(found);
              setLoading(false);
            }
          } else {
            if (!cancelled) {
              setBootError(`未找到会话 ${sessionIdParam}`);
              setLoading(false);
            }
          }
        } else {
          // No session_id -> Show creation form
          if (!cancelled) {
            setLoading(false);
          }
        }
      } catch (e) {
        if (!cancelled) {
          setBootError(String(e));
          setLoading(false);
        }
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

  const handleCreate = async () => {
    try {
      setCreating(true);
      const workspaces = await workspaceService.list();
      const ws = workspaces[0];
      if (!ws) {
        setBootError(t('chat.noWorkspace'));
        return;
      }
      const defaultName = sessionName.trim() || `${AGENT_LABELS[agentType] || agentType} 会话`;
      const s = await agentService.index({
        workspace_id: ws.id,
        name: defaultName,
        agent_type: agentType,
      });
      setSession(s);
    } catch (e) {
      Taro.showToast({ title: `创建失败: ${String(e)}`, icon: 'none' });
    } finally {
      setCreating(false);
    }
  };

  const groupedItems = useMemo(() => groupChatItems(items), [items]);

  if (loading) {
    return (
      <Screen titleKey="chat.title">
        <View className="chat chat--loading">
          <Text className="chat__hint">正在加载会话...</Text>
        </View>
      </Screen>
    );
  }

  if (bootError) {
    return (
      <Screen titleKey="chat.title">
        <View className="chat chat--error">
          <Text className="chat__error-text">{bootError}</Text>
        </View>
      </Screen>
    );
  }

  if (!session) {
    return (
      <Screen titleKey="chat.title">
        <View className="chat chat--create">
          <View className="chat-create-card">
            <Text className="chat-create-card__title">新建 AI 对话</Text>
            
            <View className="chat-create-field">
              <Text className="chat-create-field__label">选择 AI 助手</Text>
              <View className="chat-create-agents">
                <View 
                  className={`chat-create-agent ${agentType === 'claudecode' ? 'is-selected' : ''}`}
                  onClick={() => setAgentType('claudecode')}
                >
                  <View className="chat__avatar chat__avatar--claudecode">
                    <Text className="chat__avatar-text">CL</Text>
                  </View>
                  <Text className="chat-create-agent__name">Claude</Text>
                </View>
                
                <View 
                  className={`chat-create-agent ${agentType === 'codex' ? 'is-selected' : ''}`}
                  onClick={() => setAgentType('codex')}
                >
                  <View className="chat__avatar chat__avatar--codex">
                    <Text className="chat__avatar-text">CX</Text>
                  </View>
                  <Text className="chat-create-agent__name">Codex</Text>
                </View>

                <View 
                  className={`chat-create-agent ${agentType === 'cursor' ? 'is-selected' : ''}`}
                  onClick={() => setAgentType('cursor')}
                >
                  <View className="chat__avatar chat__avatar--cursor">
                    <Text className="chat__avatar-text">CS</Text>
                  </View>
                  <Text className="chat-create-agent__name">Cursor</Text>
                </View>
              </View>
            </View>

            <View className="chat-create-field">
              <Text className="chat-create-field__label">对话名称 (可选)</Text>
              <Input
                className="chat-create-input"
                placeholder="例如: 协助修复Bug"
                value={sessionName}
                onInput={e => setSessionName(e.detail.value)}
              />
            </View>

            <Button 
              className="chat-create-submit" 
              onClick={handleCreate} 
              disabled={creating}
            >
              {creating ? '创建中...' : '开始新对话'}
            </Button>
          </View>
        </View>
      </Screen>
    );
  }

  const sessionAgentType = session.agentType || 'claudecode';

  return (
    <Screen titleKey="chat.title">
      <View className="chat">
        <View className="chat__status">
          <Text className={`chat__dot chat__dot--${connection}`} />
          <Text className="chat__status-text">{statusText}</Text>
        </View>

        <ScrollView className="chat__stream" scrollY scrollIntoView={groupedItems.length ? `i${groupedItems.length - 1}` : ''}>
          <View className="chat__stream-inner">
            {groupedItems.map((it, i) => (
              <View id={`i${i}`} key={it.id} className="chat__item">
                {renderItem(it, i === groupedItems.length - 1, sessionAgentType, respondPermission, t)}
              </View>
            ))}
            {!groupedItems.length && session && <Text className="chat__hint">{t('chat.startHint')}</Text>}
          </View>
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

function AgentAvatar({ agentType }: { agentType: string }) {
  const label = agentType.slice(0, 2).toUpperCase();
  return (
    <View className={`chat__avatar chat__avatar--${agentType}`}>
      <Text className="chat__avatar-text">{label}</Text>
    </View>
  );
}

function ThinkingBubble({ content, active }: { content: string; active: boolean }) {
  const [expanded, setExpanded] = useState(active);

  useEffect(() => {
    if (active) {
      setExpanded(true);
    }
  }, [active]);

  const toggle = () => {
    setExpanded(prev => !prev);
  };

  const previewText = content.trim().replace(/\s+/g, ' ');
  const preview = previewText.length > 40 ? `${previewText.slice(0, 40)}...` : previewText;

  return (
    <View className={`chat__thinking-bubble ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      <View className="chat__thinking-header" onClick={toggle}>
        <Text className="chat__thinking-caret">{expanded ? '▾' : '▸'}</Text>
        <Text className="chat__thinking-label">{active ? 'AI 正在思考...' : '思考过程'}</Text>
        {!expanded && preview && <Text className="chat__thinking-preview">{preview}</Text>}
      </View>
      {expanded && (
        <View className="chat__thinking-body">
          <Text className="chat__thinking-text" userSelect>{content}</Text>
        </View>
      )}
    </View>
  );
}

function ToolGroupBubble({
  item,
  active,
  respondPermission,
  t,
}: {
  item: Extract<GroupedChatItem, { kind: 'tool_group' }>;
  active: boolean;
  respondPermission: (requestId: string, decision: 'allow_once' | 'reject_once') => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const [expanded, setExpanded] = useState(active);

  useEffect(() => {
    if (active) {
      setExpanded(true);
    }
  }, [active]);

  const toggle = () => {
    setExpanded(prev => !prev);
  };

  const calls = item.calls || [];
  const running = calls.some(c => c.output === undefined && (!c.permission || !c.permission.resolved));
  const hasError = calls.some(c => c.isError);

  let statusText = '执行成功';
  let statusClass = 'chat__tool-group-status--success';
  if (running) {
    statusText = '正在运行...';
    statusClass = 'chat__tool-group-status--running';
  } else if (hasError) {
    statusText = '执行出错';
    statusClass = 'chat__tool-group-status--error';
  }

  return (
    <View className={`chat__tool-group ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      <View className="chat__tool-group-header" onClick={toggle}>
        <Text className="chat__tool-group-caret">{expanded ? '▾' : '▸'}</Text>
        <Text className="chat__tool-group-title">
          {item.pending ? '待分配工具' : `工具执行 (${calls.length})`}
        </Text>
        <Text className={`chat__tool-group-status ${statusClass}`}>{statusText}</Text>
      </View>
      {expanded && (
        <View className="chat__tool-group-body">
          {calls.map((call, idx) => (
            <GroupedToolCallItem 
              key={call.id || idx} 
              call={call} 
              respondPermission={respondPermission}
              t={t}
            />
          ))}
        </View>
      )}
    </View>
  );
}

function GroupedToolCallItem({
  call,
  respondPermission,
  t,
}: {
  call: GroupedToolCall;
  respondPermission: (requestId: string, decision: 'allow_once' | 'reject_once') => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const [expanded, setExpanded] = useState(!!call.permission && !call.permission.resolved);

  const toggle = () => {
    setExpanded(prev => !prev);
  };

  let args: Record<string, unknown> = {};
  try {
    if (call.input) {
      const parsed = JSON.parse(call.input);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        args = parsed as Record<string, unknown>;
      }
    }
  } catch (e) {}

  const hasOutput = call.output !== undefined;
  const isRunning = !hasOutput && (!call.permission || !call.permission.resolved);
  const isWaitingPermission = !!call.permission && !call.permission.resolved;

  const summaryKeys = ['command', 'file_path', 'path', 'pattern', 'query', 'url'];
  let summary = '';
  for (const key of summaryKeys) {
    if (args[key] && typeof args[key] === 'string') {
      summary = args[key] as string;
      break;
    }
  }
  if (!summary && Object.keys(args).length > 0) {
    summary = String(Object.values(args)[0]);
  }
  const cleanSummary = summary.length > 30 ? `${summary.slice(0, 30)}...` : summary;

  return (
    <View className={`chat__tool-item ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      <View className="chat__tool-item-header" onClick={toggle}>
        <Text className="chat__tool-item-caret">{expanded ? '▾' : '▸'}</Text>
        <Text className="chat__tool-item-name">{call.toolName}</Text>
        {cleanSummary && <Text className="chat__tool-item-summary">{cleanSummary}</Text>}
        <Text className="chat__tool-item-status-icon">
          {isWaitingPermission ? '🔐' : isRunning ? '⏳' : call.isError ? '✕' : '✓'}
        </Text>
      </View>

      {expanded && (
        <View className="chat__tool-item-body">
          {/* Arguments */}
          <View className="chat-tool-section">
            <Text className="chat-tool-section-title">参数</Text>
            {Object.keys(args).length > 0 ? (
              <View className="chat-tool-args-list">
                {Object.entries(args).map(([k, v]) => (
                  <View key={k} className="chat-tool-arg">
                    <Text className="chat-tool-arg-name">{k}:</Text>
                    <Text className="chat-tool-arg-value">{typeof v === 'object' ? JSON.stringify(v) : String(v)}</Text>
                  </View>
                ))}
              </View>
            ) : call.input ? (
              <Text className="chat-tool-pre">{call.input}</Text>
            ) : (
              <Text className="chat-tool-muted">无参数</Text>
            )}
          </View>

          {/* Permission */}
          {call.permission && (
            <View className="chat-tool-section">
              <Text className="chat-tool-section-title">授权要求</Text>
              {call.permission.resolved ? (
                <View className={`chat-tool-permission is-${call.permission.resolved}`}>
                  <Text>{call.permission.resolved === 'allow' ? '✓ 已允许' : '✕ 已拒绝'}</Text>
                </View>
              ) : (
                <View className="chat-tool-permission-actions">
                  <Button className="chat-tool-perm-btn chat-tool-perm-btn--allow" size="mini" onClick={() => respondPermission(call.permission!.requestId, 'allow_once')}>
                    允许
                  </Button>
                  <Button className="chat-tool-perm-btn chat-tool-perm-btn--reject" size="mini" onClick={() => respondPermission(call.permission!.requestId, 'reject_once')}>
                    拒绝
                  </Button>
                </View>
              )}
            </View>
          )}

          {/* Output */}
          <View className="chat-tool-section">
            <Text className="chat-tool-section-title">执行输出</Text>
            {!hasOutput ? (
              <Text className="chat-tool-muted">{isRunning ? '正在等待输出...' : '无输出'}</Text>
            ) : (
              <ScrollView className="chat-tool-pre-scroll" scrollX scrollY>
                <Text className={`chat-tool-pre ${call.isError ? 'has-error' : ''}`} userSelect>{call.output}</Text>
              </ScrollView>
            )}
          </View>
        </View>
      )}
    </View>
  );
}

function renderItem(
  it: GroupedChatItem,
  isActive: boolean,
  agentType: string,
  respondPermission: (requestId: string, decision: 'allow_once' | 'reject_once') => void,
  t: (key: string, params?: Record<string, string | number>) => string
) {
  switch (it.kind) {
    case 'user':
      return (
        <View className="chat__user-row">
          <View className="chat__user-bubble">
            <Text className="chat__user-text" userSelect>{it.content}</Text>
          </View>
        </View>
      );
    case 'assistant_text': {
      const html = renderMarkdown(it.content);
      return (
        <View className="chat__assistant-row">
          <AgentAvatar agentType={agentType} />
          <View className="chat__assistant-bubble">
            <RichText className="markdown-body" nodes={html} userSelect />
          </View>
        </View>
      );
    }
    case 'thinking':
      return (
        <View className="chat__assistant-row">
          <AgentAvatar agentType={agentType} />
          <View className="chat__bubble-wrapper">
            <ThinkingBubble content={it.content} active={isActive} />
          </View>
        </View>
      );
    case 'tool_group':
      return (
        <View className="chat__assistant-row">
          <AgentAvatar agentType={agentType} />
          <View className="chat__bubble-wrapper">
            <ToolGroupBubble 
              item={it} 
              active={isActive} 
              respondPermission={respondPermission}
              t={t}
            />
          </View>
        </View>
      );
    case 'error':
      return (
        <View className="chat__assistant-row">
          <AgentAvatar agentType={agentType} />
          <View className="chat__bubble-wrapper">
            <View className="chat__error-card">
              <Text className="chat__error-title">⚠️ 出错了</Text>
              <Text className="chat__error-text">{it.content}</Text>
            </View>
          </View>
        </View>
      );
    default:
      return null;
  }
}
