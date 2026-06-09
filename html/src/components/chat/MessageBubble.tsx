import { h } from 'preact';
import type { ChatItem } from './hooks';

interface MessageBubbleProps {
    item: ChatItem;
}

export function MessageBubble({ item }: MessageBubbleProps) {
    switch (item.kind) {
        case 'user':
            return <UserBubble content={item.content} />;
        case 'assistant_text':
            return <AssistantBubble content={item.content} streaming={item.streaming} />;
        case 'thinking':
            return <ThinkingBubble content={item.content} />;
        case 'tool_use':
            return <ToolUseBubble name={item.toolName} input={item.input} />;
        case 'tool_result':
            return <ToolResultBubble content={item.content} isError={item.isError} />;
        case 'permission':
            return <PermissionBubble toolName={item.toolName} input={item.input} resolved={item.resolved} />;
        case 'error':
            return <ErrorBubble content={item.content} />;
    }
}

function UserBubble({ content }: { content: string }) {
    return (
        <div class="chat-bubble chat-bubble-user">
            <div class="chat-bubble-body">{content}</div>
        </div>
    );
}

function AssistantBubble({ content, streaming }: { content: string; streaming: boolean }) {
    return (
        <div class="chat-bubble chat-bubble-assistant">
            <div class="chat-bubble-body">
                {content}
                {streaming && <span class="chat-cursor">▍</span>}
            </div>
        </div>
    );
}

function ThinkingBubble({ content }: { content: string }) {
    return (
        <div class="chat-bubble chat-bubble-thinking">
            <div class="chat-bubble-label">思考</div>
            <div class="chat-bubble-body">{content}</div>
        </div>
    );
}

function ToolUseBubble({ name, input }: { name: string; input: string }) {
    return (
        <div class="chat-bubble chat-bubble-tool">
            <div class="chat-bubble-label">
                <span class="chat-tool-icon">⚙</span> {name}
            </div>
            <pre class="chat-bubble-code">{input}</pre>
        </div>
    );
}

function ToolResultBubble({ content, isError }: { content: string; isError: boolean }) {
    return (
        <div class={`chat-bubble chat-bubble-tool-result ${isError ? 'is-error' : ''}`}>
            <pre class="chat-bubble-code">{content}</pre>
        </div>
    );
}

function PermissionBubble({
    toolName,
    input,
    resolved,
}: {
    toolName: string;
    input: string;
    resolved?: 'allow' | 'deny';
}) {
    return (
        <div class="chat-bubble chat-bubble-permission">
            <div class="chat-bubble-label">需要授权 · {toolName}</div>
            <pre class="chat-bubble-code">{input}</pre>
            {resolved ? (
                <div class={`chat-permission-resolved chat-permission-${resolved}`}>
                    {resolved === 'allow' ? '已允许' : '已拒绝'}
                </div>
            ) : (
                <div class="chat-permission-hint">请在弹窗中确认（实现中）</div>
            )}
        </div>
    );
}

function ErrorBubble({ content }: { content: string }) {
    return (
        <div class="chat-bubble chat-bubble-error">
            <div class="chat-bubble-label">错误</div>
            <div class="chat-bubble-body">{content}</div>
        </div>
    );
}
