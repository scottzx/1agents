import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { MessageBubble } from './MessageBubble';
import type { ChatItem } from './hooks';

interface MessageListProps {
    items: ChatItem[];
    emptyHint?: string;
    onRespondPermission?: (requestId: string, allow: boolean) => void;
}

export function MessageList({ items, emptyHint, onRespondPermission }: MessageListProps) {
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

    return (
        <div class="chat-messages" ref={scrollRef}>
            {items.map(item => (
                <MessageBubble key={item.id} item={item} onRespondPermission={onRespondPermission} />
            ))}
        </div>
    );
}
