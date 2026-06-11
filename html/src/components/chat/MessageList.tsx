import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { MessageBubble, GroupedChatItem, GroupedToolCall } from './MessageBubble';
import type { ChatItem } from './hooks';
import type { AgentType, PermissionDecision } from '../types';

interface MessageListProps {
    items: ChatItem[];
    agentType?: AgentType;
    emptyHint?: string;
    /**
     * When true, render a centered spinner placeholder instead of the
     * normal empty hint. Used during the bridge's `ensure_session` window
     * for new chats so users see "preparing session" rather than a hint
     * that implies they can type immediately.
     */
    loading?: boolean;
    loadingHint?: string;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
    /**
     * Per-queue-prompt cancel. Wired to the X button on queued user
     * bubbles — distinct from the global "stop the current turn" cancel
     * which only stops `activeTurn` and leaves the queue running.
     */
    onCancelQueued?: (queueRequestId: string) => void;
}

function isCallRenderable(call: GroupedToolCall): boolean {
    // A call is renderable as soon as we know *something* concrete about it:
    //   - it has a toolCallId (the runtime committed to this call — render
    //     a streaming placeholder even before the arguments JSON arrives)
    //   - it has an inline permission request waiting on the user
    //   - it already produced an output
    //   - it has parseable input
    // This fixes the "invisible tool card until arguments stream in" bug
    // and keeps permission requests inside the matching tool group.
    if (call.toolCallId) return true;
    if (call.output !== undefined) return true;
    if (call.permission) return true;
    if (!call.input || !call.input.trim()) return false;
    try {
        const parsed = JSON.parse(call.input);
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
            return Object.keys(parsed as Record<string, unknown>).length > 0;
        }
        return true;
    } catch {
        // Non-JSON but has content — render as raw.
        return true;
    }
}

function groupChatItems(items: ChatItem[]): GroupedChatItem[] {
    const grouped: GroupedChatItem[] = [];
    // Calls assembled from tool_result / permission_request items
    // that didn't find a matching tool_use yet. They get folded into
    // a single "待分配" tool_group at the end of the stream so the
    // user still sees the data instead of having it silently dropped.
    const pendingCalls: GroupedToolCall[] = [];

    for (const item of items) {
        if (item.kind === 'tool_use') {
            let lastGroup = grouped[grouped.length - 1];
            if (!lastGroup || lastGroup.kind !== 'tool_group' || lastGroup.pending) {
                // Don't fold new tool_use items into the pending group
                // — it only collects orphan results / permission
                // requests waiting to be matched. Start a fresh group.
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
                        id: `call-${callId || Math.random()}`,
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

            // Search backward for the most recent non-pending group
            // that contains this callId. The pending group is skipped
            // — it's just a holding pen for unmatched entries, not a
            // legitimate target for new attachments.
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

            // Fallback: if not found by callId, attach to the most
            // recent non-pending group's last call (mirrors realtime
            // tool_result's reverse-scan fallback).
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
                // No tool_use matched — park the result in the
                // pending pool. A later tool_use with the right
                // toolCallId (or a history reload) will attach it.
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
                    input: '',
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

export function MessageList({
    items,
    agentType,
    emptyHint,
    loading,
    loadingHint,
    onRespondPermission,
    onCancelQueued,
}: MessageListProps) {
    const scrollRef = useRef<HTMLDivElement | null>(null);

    // Auto-scroll to bottom on new content unless user has scrolled up.
    useEffect(() => {
        const el = scrollRef.current;
        if (!el) return;
        const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
        if (distanceFromBottom < 120) {
            el.scrollTop = el.scrollHeight;
        }
    }, [items]);

    if (loading) {
        // Spinner takes priority over the empty hint: while the bridge is
        // spinning up the agent we don't want to advertise an "empty
        // conversation, send a message" prompt that the composer can't
        // honor yet (it would be disabled and the user would wonder why).
        return (
            <div class="chat-empty chat-loading">
                <div class="chat-loading-spinner" aria-hidden="true" />
                <p>{loadingHint ?? '会话正在初始化…'}</p>
            </div>
        );
    }

    if (items.length === 0) {
        return (
            <div class="chat-empty">
                <p>{emptyHint ?? '发送消息开始对话'}</p>
            </div>
        );
    }

    const groupedItems: GroupedChatItem[] = [];
    for (const item of groupChatItems(items)) {
        if (item.kind !== 'tool_group') {
            groupedItems.push(item);
            continue;
        }
        // Drop empty calls so we don't briefly render "工具调用 1" with no
        // body while waiting for the streaming input to land. If everything
        // is empty, hide the whole group — it'll reappear once content
        // arrives.
        const renderable = item.calls.filter(isCallRenderable);
        if (renderable.length === 0) continue;
        if (renderable.length === item.calls.length) {
            groupedItems.push(item);
        } else {
            groupedItems.push({ ...item, calls: renderable });
        }
    }

    return (
        <div class="chat-messages" ref={scrollRef}>
            {groupedItems.map((item, index) => (
                <MessageBubble
                    key={item.id}
                    item={item}
                    agentType={agentType}
                    isLast={index === groupedItems.length - 1}
                    onRespondPermission={onRespondPermission}
                    onCancelQueued={onCancelQueued}
                />
            ))}
        </div>
    );
}
