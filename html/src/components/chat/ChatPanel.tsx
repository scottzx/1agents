import { h, Component } from 'preact';
import type { ChatSession } from '../types';
import { useBridge } from './hooks';
import { MessageList } from './MessageList';
import { Composer } from './Composer';
import { SessionStatusBar } from './SessionStatusBar';

interface ChatPanelProps {
    session: ChatSession;
}

export class ChatPanel extends Component<ChatPanelProps> {
    render() {
        return <ChatPanelInner session={this.props.session} />;
    }
}

/**
 * Inner component so we can use the useBridge hook (a functional
 * component rule) while keeping the public class-based API for
 * symmetry with the rest of the codebase.
 */
function ChatPanelInner({ session }: ChatPanelProps) {
    const { items, connection, typing, ready, permissionMode, send, cancel, respondPermission, setPermissionMode } =
        useBridge(session);

    // The composer is only usable once BOTH the WS handshake finished
    // AND the bridge has confirmed the session is initialized. The
    // latter is the new gate — without it, the first user prompt on a
    // brand-new session would race `session_ready` and bounce with
    // SESSION_NOT_FOUND.
    const composerDisabled = (connection !== 'connected' && connection !== 'reconnecting') || !ready;

    // Show a spinner placeholder while the WebSocket is open but the
    // bridge hasn't confirmed the session yet. For reconnecting/error
    // states the existing status bar / empty hint is more accurate.
    const showInitLoading = connection === 'connected' && !ready;

    return (
        <div class="chat-panel">
            <SessionStatusBar session={session} connection={connection} typing={typing} />
            <MessageList
                items={items}
                agentType={session.agentType}
                emptyHint={connection === 'connecting' ? '正在连接…' : '发送消息开始对话'}
                loading={showInitLoading}
                onRespondPermission={respondPermission}
            />
            <Composer
                onSend={send}
                onCancel={cancel}
                isRunning={typing}
                disabled={composerDisabled}
                permissionMode={permissionMode}
                onPermissionModeChange={setPermissionMode}
            />
        </div>
    );
}
