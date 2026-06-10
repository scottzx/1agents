import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { MessageBubble, GroupedChatItem } from './MessageBubble';
import type { ChatItem } from './hooks';
import type { AgentType } from '../types';

interface MessageListProps {
    items: ChatItem[];
    agentType?: AgentType;
    emptyHint?: string;
    onRespondPermission?: (requestId: string, allow: boolean) => void;
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

            const callId = item.toolCallId;
            let matchedCall = callId ? lastGroup.calls.find(c => c.toolCallId === callId) : null;
            if (!matchedCall && lastGroup.calls.length > 0) {
                matchedCall = lastGroup.calls.find(c => c.output === undefined) || null;
            }
            if (!matchedCall && lastGroup.calls.length > 0) {
                matchedCall = lastGroup.calls[lastGroup.calls.length - 1];
            }

            if (matchedCall) {
                matchedCall.output = item.content;
                matchedCall.isError = item.isError;
            } else {
                lastGroup.calls.push({
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

    const groupedItems = groupChatItems(items);

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
