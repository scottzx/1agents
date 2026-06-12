import { h, Component } from 'preact';
import { useMemo } from 'preact/hooks';
import type { ChatSession } from '../types';
import { useBridge } from './hooks';
import { MessageList } from './MessageList';
import { Composer } from './Composer';
import { SessionStatusBar } from './SessionStatusBar';
import { PermissionPrompt } from './PermissionPrompt';

interface ChatPanelProps {
    session: ChatSession;
    pendingInitialMessage?: string | null;
    onClearPendingInitialMessage?: () => void;
}

export class ChatPanel extends Component<ChatPanelProps> {
    render() {
        return (
            <ChatPanelInner
                session={this.props.session}
                pendingInitialMessage={this.props.pendingInitialMessage}
                onClearPendingInitialMessage={this.props.onClearPendingInitialMessage}
            />
        );
    }
}

/**
 * Inner component so we can use the useBridge hook (a functional
 * component rule) while keeping the public class-based API for
 * symmetry with the rest of the codebase.
 */
function ChatPanelInner({ session, pendingInitialMessage, onClearPendingInitialMessage }: ChatPanelProps) {
    const { items, connection, typing, send, respondPermission } = useBridge(
        session,
        [],
        pendingInitialMessage,
        onClearPendingInitialMessage
    );

    // Find the most recent unresolved permission request to surface as a modal.
    const pendingPermission = useMemo(() => {
        for (let i = items.length - 1; i >= 0; i--) {
            const it = items[i];
            if (it.kind === 'permission' && !it.resolved) return it;
        }
        return null;
    }, [items]);

    const composerDisabled = connection !== 'connected' && connection !== 'reconnecting';

    return (
        <div class="chat-panel">
            <SessionStatusBar session={session} connection={connection} typing={typing} />
            <MessageList items={items} emptyHint={connection === 'connecting' ? '正在连接…' : '发送消息开始对话'} />
            <Composer onSend={send} disabled={composerDisabled} />
            {pendingPermission && <PermissionPrompt item={pendingPermission} onRespond={respondPermission} />}
        </div>
    );
}
