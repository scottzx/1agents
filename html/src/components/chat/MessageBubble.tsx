import { h } from 'preact';
import { marked } from 'marked';
import type { ChatItem } from './hooks';

interface MessageBubbleProps {
    item: ChatItem;
    onRespondPermission?: (requestId: string, allow: boolean) => void;
}

export function MessageBubble({ item, onRespondPermission }: MessageBubbleProps) {
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
            return (
                <PermissionBubble
                    requestId={item.requestId}
                    toolName={item.toolName}
                    input={item.input}
                    resolved={item.resolved}
                    onRespond={onRespondPermission}
                />
            );
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
    const html = marked.parse(content, { async: false }) as string;
    return (
        <div class="chat-bubble chat-bubble-assistant">
            <div class="chat-bubble-body">
                <div class="markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
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
    requestId,
    toolName,
    input,
    resolved,
    onRespond,
}: {
    requestId: string;
    toolName: string;
    input: string;
    resolved?: 'allow' | 'deny';
    onRespond?: (requestId: string, allow: boolean) => void;
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
                <div class="chat-permission-actions">
                    <button class="chat-permission-btn deny" onClick={() => onRespond && onRespond(requestId, false)}>
                        拒绝
                    </button>
                    <button class="chat-permission-btn allow" onClick={() => onRespond && onRespond(requestId, true)}>
                        允许
                    </button>
                </div>
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
