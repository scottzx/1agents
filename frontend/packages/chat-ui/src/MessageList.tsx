import { h, Fragment, type ComponentChildren } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import type { ChatItem, GroupedChatItem, GroupedToolCall, ToolGroupElement } from './types';
import { MessageBubble } from './MessageBubble';
import { groupHistoricalTurns } from './turns';

export interface MessageListProps {
  items: ChatItem[];
  typing?: boolean;
  emptyHint?: string;
  loading?: boolean;
  loadingHint?: string;
  cwd?: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  onOpenFile?: (path: string) => void;
  onToast?: (msg: string) => void;
  onCancelQueued?: (queueRequestId: string) => void;
  timelineFooter?: ComponentChildren;
}

function isCallRenderable(call: GroupedToolCall): boolean {
  if (call.toolCallId) return true;
  if (call.output !== undefined) return true;
  if (call.permission) return true;
  if (call.askUser) return true;
  if (call.exitPlan) return true;
  if (!call.input || !call.input.trim()) return false;
  try {
    const parsed = JSON.parse(call.input);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return Object.keys(parsed as Record<string, unknown>).length > 0;
    }
    return true;
  } catch {
    return true;
  }
}

export function groupChatItems(items: ChatItem[]): GroupedChatItem[] {
  const grouped: GroupedChatItem[] = [];
  const pendingCalls: GroupedToolCall[] = [];
  let currentProcessGroup: Extract<GroupedChatItem, { kind: 'tool_group' }> | null = null;

  const ensureProcessGroup = (
    id: string,
    createdAt: number,
    turnId?: string,
    turnStatus?: ChatItem['turnStatus']
  ): Extract<GroupedChatItem, { kind: 'tool_group' }> => {
    if (currentProcessGroup && turnId && currentProcessGroup.turnId && currentProcessGroup.turnId !== turnId) {
      currentProcessGroup = null;
    }
    if (currentProcessGroup) return currentProcessGroup;
    currentProcessGroup = {
      id: `group-${id}`,
      kind: 'tool_group',
      calls: [],
      thinkingBlocks: [],
      elements: [],
      createdAt,
      turnId,
      turnStatus,
    };
    grouped.push(currentProcessGroup);
    return currentProcessGroup;
  };

  for (const item of items) {
    if (item.kind === 'tool_use') {
      const lastGroup = ensureProcessGroup(item.id, item.createdAt, item.turnId, item.turnStatus);

      if (!lastGroup.thinkingBlocks) lastGroup.thinkingBlocks = [];
      if (!lastGroup.elements) lastGroup.elements = [];

      for (const call of item.calls) {
        const callId = call.toolCallId;
        const existingCall = callId ? lastGroup.calls.find(c => c.toolCallId === callId) : null;
        if (existingCall) {
          existingCall.toolName = call.toolName;
          existingCall.input = call.input;
          existingCall.output = call.output;
          existingCall.isError = call.isError;
          if (call.permission) existingCall.permission = call.permission;
          if (call.askUser) existingCall.askUser = call.askUser;
          if (call.exitPlan) existingCall.exitPlan = call.exitPlan;
          if (call.kind) existingCall.kind = call.kind;
          if (call.status) existingCall.status = call.status;
          if (call.locations) existingCall.locations = call.locations;
          if (call.diffs) existingCall.diffs = call.diffs;
        } else {
          const newCall: GroupedToolCall = {
            id: `call-${callId || lastGroup.calls.length}`,
            toolCallId: callId,
            toolName: call.toolName,
            input: call.input,
            output: call.output,
            isError: call.isError,
            ...(call.kind ? { kind: call.kind } : {}),
            ...(call.status ? { status: call.status } : {}),
            ...(call.locations ? { locations: call.locations } : {}),
            ...(call.diffs ? { diffs: call.diffs } : {}),
            ...(call.permission ? { permission: call.permission } : {}),
            ...(call.askUser ? { askUser: call.askUser } : {}),
            ...(call.exitPlan ? { exitPlan: call.exitPlan } : {}),
          };
          lastGroup.calls.push(newCall);
          lastGroup.elements.push({ kind: 'call', call: newCall });
        }
      }
    } else if (item.kind === 'thinking') {
      const lastGroup = ensureProcessGroup(item.id, item.createdAt, item.turnId, item.turnStatus);

      if (!lastGroup.thinkingBlocks) lastGroup.thinkingBlocks = [];
      if (!lastGroup.elements) lastGroup.elements = [];

      const existingElement = lastGroup.elements.find(el => el.kind === 'thinking' && el.id === item.id);
      if (existingElement && existingElement.kind === 'thinking') {
        existingElement.content = item.content;
      } else {
        lastGroup.elements.push({
          kind: 'thinking',
          id: item.id,
          content: item.content,
        });
      }

      lastGroup.thinkingBlocks = lastGroup.elements
        .filter(el => el.kind === 'thinking')
        .map(el => (el as Extract<ToolGroupElement, { kind: 'thinking' }>).content);
    } else if (item.kind === 'tool_result') {
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

      if (!matchedCall) {
        for (let i = grouped.length - 1; i >= 0; i--) {
          const g = grouped[i];
          if (g.kind === 'tool_group' && !g.pending) {
            if (g.calls.length > 0) {
              matchedCall = g.calls.find(c => c.output === undefined) || g.calls[g.calls.length - 1];
              break;
            }
          }
        }
      }

      if (matchedCall) {
        matchedCall.output = item.content;
        matchedCall.isError = item.isError;
        matchedCall.status = item.isError ? 'failed' : 'completed';
      } else {
        pendingCalls.push({
          id: `pending-result-${item.id}`,
          toolCallId: callId,
          toolName: item.toolName || 'tool',
          input: '',
          output: item.content,
          isError: item.isError,
          status: item.isError ? 'failed' : 'completed',
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
          permission: {
            requestId: item.requestId,
            toolName: item.toolName,
            input: item.input,
            options: item.options,
            ...(item.resolved ? { resolved: item.resolved } : {}),
          },
        });
      }
    } else if (item.kind === 'ask_user_question') {
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
      const askUser = {
        requestId: item.requestId,
        toolCallId: callId,
        mode: item.mode,
        questions: item.questions,
        ...(item.resolved ? { resolved: item.resolved } : {}),
        ...(item.answers ? { answers: item.answers } : {}),
      };
      if (matchedCall) {
        matchedCall.askUser = askUser;
      } else {
        pendingCalls.push({
          id: `pending-ask-user-${item.id}`,
          toolCallId: callId,
          toolName: 'ask_user_question',
          input: JSON.stringify({ questions: item.questions }, null, 2),
          askUser,
        });
      }
    } else if (item.kind === 'exit_plan_mode') {
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
      const exitPlan = {
        requestId: item.requestId,
        toolCallId: callId,
        planContent: item.planContent,
        ...(item.resolved ? { resolved: item.resolved } : {}),
        ...(item.comments ? { comments: item.comments } : {}),
      };
      if (matchedCall) {
        matchedCall.exitPlan = exitPlan;
      } else {
        pendingCalls.push({
          id: `pending-exit-plan-${item.id}`,
          toolCallId: callId,
          toolName: 'exit_plan_mode',
          input: item.planContent?.slice(0, 500) || '',
          exitPlan,
        });
      }
    } else if (
      item.kind === 'user' ||
      item.kind === 'error' ||
      item.kind === 'assistant_text' ||
      item.kind === 'turn_receipt'
    ) {
      currentProcessGroup = null;
      grouped.push(item);
    }
  }

  if (pendingCalls.length > 0) {
    grouped.push({
      id: 'group-pending',
      kind: 'tool_group',
      calls: pendingCalls,
      createdAt: Date.now(),
      pending: true,
    });
  }

  const filtered = grouped.filter(item => {
    if (item.kind !== 'tool_group') return true;
    const hasRenderableCall = item.calls.some(isCallRenderable);
    const hasThinking = (item.thinkingBlocks && item.thinkingBlocks.length > 0) || false;
    return hasRenderableCall || hasThinking;
  });

  return filtered;
}

export function MessageList({
  items,
  typing,
  emptyHint,
  loading,
  loadingHint,
  cwd,
  projectName,
  lang = 'zh-CN',
  onOpenFile,
  onToast,
  onCancelQueued,
  timelineFooter,
}: MessageListProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  if (loading) {
    return (
      <div class="chat-empty-state">
        <div class="chat-loading-spinner" />
        <div class="chat-empty-text">{loadingHint || (lang === 'zh-CN' ? '正在加载会话...' : 'Loading session...')}</div>
      </div>
    );
  }

  if (!items || items.length === 0) {
    return (
      <div class="chat-empty-state">
        <div class="chat-empty-text">{emptyHint || (lang === 'zh-CN' ? '暂无对话记录' : 'No conversation records')}</div>
      </div>
    );
  }

  const grouped = groupHistoricalTurns(groupChatItems(items));

  let latestAssistantId: string | undefined;
  for (let i = grouped.length - 1; i >= 0; i--) {
    const item = grouped[i];
    if (item.kind === 'assistant_text') {
      latestAssistantId = item.id;
      break;
    }
  }

  return (
    <div class="chat-messages-body" ref={containerRef}>
      <div class="chat-message-track">
        {grouped.map((item, index) => (
          <MessageBubble
            key={item.id}
            item={item}
            isLast={index === grouped.length - 1}
            isLatestAssistant={item.id === latestAssistantId}
            cwd={cwd}
            projectName={projectName}
            lang={lang}
            onOpenFile={onOpenFile}
            onToast={onToast}
            onCancelQueued={onCancelQueued}
          />
        ))}

        {typing && (
          <div class="chat-message-row chat-message-row-assistant">
            <div class="chat-bubble chat-bubble-assistant">
              <div class="chat-typing-indicator">
                <span class="chat-typing-dot" />
                <span class="chat-typing-dot" />
                <span class="chat-typing-dot" />
              </div>
            </div>
          </div>
        )}

        {timelineFooter}
      </div>
    </div>
  );
}
