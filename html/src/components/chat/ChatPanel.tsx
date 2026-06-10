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
    const { items, connection, typing, send, cancel, respondPermission } = useBridge(session);

    const composerDisabled = connection !== 'connected' && connection !== 'reconnecting';

    return (
        <div class="chat-panel">
            <SessionStatusBar session={session} connection={connection} typing={typing} />
            <MessageList
                items={items}
                agentType={session.agentType}
                emptyHint={connection === 'connecting' ? '正在连接…' : '发送消息开始对话'}
                onRespondPermission={respondPermission}
            />
            <Composer onSend={send} onCancel={cancel} isRunning={typing} disabled={composerDisabled} />
        </div>
    );
}
