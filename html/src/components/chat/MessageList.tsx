import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { MessageBubble, GroupedChatItem, GroupedToolCall } from './MessageBubble';
import type { ChatItem } from './hooks';
import type { AgentType, PermissionDecision } from '../types';

interface MessageListProps {
    items: ChatItem[];
    agentType?: AgentType;
    emptyHint?: string;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
}

function isCallRenderable(call: GroupedToolCall): boolean {
    // Mirrors the skip condition in GroupedToolCallItem so MessageList can
    // suppress tool_groups that would render as a title-only placeholder.
    if (call.output !== undefined) return true;
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

    for (const item of items) {
        if (item.kind === 'tool_use') {
            let lastGroup = grouped[grouped.length - 1];
            if (!lastGroup || lastGroup.kind !== 'tool_group') {
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
                } else {
                    lastGroup.calls.push({
                        id: `call-${callId || Math.random()}`,
                        toolCallId: callId,
                        toolName: call.toolName,
                        input: call.input,
                    });
                }
            }
        } else if (item.kind === 'tool_result') {
            const callId = item.toolCallId;
            let matchedCall: GroupedToolCall | null = null;
            let matchedGroup: GroupedChatItem | null = null;

            // Search backward for the most recent group that contains this callId
            if (callId) {
                for (let i = grouped.length - 1; i >= 0; i--) {
                    const g = grouped[i];
                    if (g.kind === 'tool_group') {
                        const c = g.calls.find(call => call.toolCallId === callId);
                        if (c) {
                            matchedCall = c;
                            matchedGroup = g;
                            break;
                        }
                    }
                }
            }

            // Fallback: if not found by callId, check the latest tool_group
            if (!matchedCall) {
                for (let i = grouped.length - 1; i >= 0; i--) {
                    if (grouped[i].kind === 'tool_group') {
                        matchedGroup = grouped[i];
                        break;
                    }
                }
                if (matchedGroup && matchedGroup.kind === 'tool_group' && matchedGroup.calls.length > 0) {
                    matchedCall = matchedGroup.calls.find(c => c.output === undefined) || null;
                    if (!matchedCall) {
                        matchedCall = matchedGroup.calls[matchedGroup.calls.length - 1];
                    }
                }
            }

            if (matchedCall && matchedGroup && matchedGroup.kind === 'tool_group') {
                matchedCall.output = item.content;
                matchedCall.isError = item.isError;
            } else {
                let targetGroup: Extract<GroupedChatItem, { kind: 'tool_group' }> | null = null;
                if (matchedGroup && matchedGroup.kind === 'tool_group') {
                    targetGroup = matchedGroup;
                } else {
                    targetGroup = {
                        id: `group-${item.id}`,
                        kind: 'tool_group',
                        calls: [],
                        createdAt: item.createdAt,
                    };
                    grouped.push(targetGroup);
                }
                targetGroup.calls.push({
                    id: `call-${callId || Math.random()}`,
                    toolCallId: callId,
                    toolName: 'tool',
                    input: '',
                    output: item.content,
                    isError: item.isError,
                });
            }
        } else {
            grouped.push(item as GroupedChatItem);
        }
    }

    return grouped;
}

export function MessageList({ items, agentType, emptyHint, onRespondPermission }: MessageListProps) {
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
                />
            ))}
        </div>
    );
}
