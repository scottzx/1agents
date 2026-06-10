import { h } from 'preact';
import { AGENT_TYPE_LABELS, type ChatSession } from '../types';
import type { ConnectionState } from './hooks';
import { AgentAvatar } from './AgentAvatar';

interface SessionStatusBarProps {
    session: ChatSession;
    connection: ConnectionState;
    typing: boolean;
}

export function SessionStatusBar({ session, connection, typing }: SessionStatusBarProps) {
    const connLabel = connectionLabel(connection);
    return (
        <div class="chat-status-bar">
            <AgentAvatar agentType={session.agentType} class="chat-status-avatar" />
            <span class="chat-status-agent">{AGENT_TYPE_LABELS[session.agentType] ?? session.agentType}</span>
            <span class={`chat-status-conn chat-status-${connection}`}>{connLabel}</span>
            {typing && <span class="chat-status-typing">正在生成…</span>}
        </div>
    );
}

function connectionLabel(state: ConnectionState): string {
    switch (state) {
        case 'idle':
            return '未连接';
        case 'connecting':
            return '连接中…';
        case 'connected':
            return '已连接';
        case 'reconnecting':
            return '连接已断开，正在重连…';
        case 'closed':
            return '已关闭';
        case 'error':
            return '错误';
    }
}
