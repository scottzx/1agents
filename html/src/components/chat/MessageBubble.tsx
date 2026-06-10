import { h } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { marked } from 'marked';
import type { ChatItem, ToolCallInfo } from './hooks';

// Configure marked once: GFM + soft line breaks so the assistant's
// streamed text wraps naturally inside the chat bubble.
marked.setOptions({
    gfm: true,
    breaks: true,
});

interface MessageBubbleProps {
    item: ChatItem;
    isLast: boolean;
    onRespondPermission?: (requestId: string, allow: boolean) => void;
}

export function MessageBubble({ item, isLast, onRespondPermission }: MessageBubbleProps) {
    switch (item.kind) {
        case 'user':
            return <UserBubble content={item.content} />;
        case 'assistant_text':
            return <AssistantBubble content={item.content} streaming={item.streaming} />;
        case 'thinking':
            return <ThinkingBubble content={item.content} isLast={isLast} />;
        case 'tool_use':
            return <ToolUseBubble calls={item.calls} />;
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

function ThinkingBubble({ content, isLast }: { content: string; isLast: boolean }) {
    const [isExpanded, setIsExpanded] = useState(isLast);

    // Auto-collapse when a newer item pushes this one out of the "last" position.
    useEffect(() => {
        setIsExpanded(isLast);
    }, [isLast]);

    const previewText = content.trim().replace(/\s+/g, ' ');
    const preview = previewText.length > 80 ? `${previewText.slice(0, 80)}…` : previewText;

    return (
        <div class={`chat-bubble chat-bubble-thinking ${isExpanded ? 'is-expanded' : 'is-collapsed'}`}>
            <div
                class="chat-bubble-header-clickable"
                role="button"
                tabIndex={0}
                onClick={() => setIsExpanded(prev => !prev)}
                onKeyDown={e => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        setIsExpanded(prev => !prev);
                    }
                }}
            >
                <span class="chat-bubble-caret" aria-hidden="true">
                    {isExpanded ? '▾' : '▸'}
                </span>
                <span class="chat-bubble-label">思考</span>
                {!isExpanded && preview && <span class="chat-thinking-preview">{preview}</span>}
            </div>
            {isExpanded && <div class="chat-bubble-body chat-thinking-body">{content}</div>}
        </div>
    );
}

function ToolUseBubble({ calls }: { calls: ToolCallInfo[] }) {
    const [isExpanded, setIsExpanded] = useState(true);

    return (
        <div class={`chat-bubble chat-bubble-tool ${isExpanded ? 'is-expanded' : 'is-collapsed'}`}>
            <div
                class="chat-bubble-header chat-bubble-header-clickable"
                role="button"
                tabIndex={0}
                onClick={() => setIsExpanded(prev => !prev)}
                onKeyDown={e => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        setIsExpanded(prev => !prev);
                    }
                }}
            >
                <span class="chat-bubble-caret" aria-hidden="true">
                    {isExpanded ? '▾' : '▸'}
                </span>
                <span class="chat-tool-icon" aria-hidden="true">
                    <svg
                        viewBox="0 0 24 24"
                        width="14"
                        height="14"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <path d="M14.7 6.3a4.5 4.5 0 1 0-6.4 6.4l-6 6a2.1 2.1 0 1 0 3 3l6-6a4.5 4.5 0 0 0 6.4-6.4l-2.2 2.2-2.4-2.4 2.6-2.8z" />
                    </svg>
                </span>
                <span class="chat-bubble-title">工具调用</span>
                <span class="chat-bubble-count">{calls.length}</span>
            </div>
            {isExpanded && (
                <div class="chat-tool-calls-list">
                    {calls.map((call, idx) => (
                        <ToolCallItem key={`${call.toolCallId ?? ''}-${idx}`} call={call} />
                    ))}
                </div>
            )}
        </div>
    );
}

function ToolCallItem({ call }: { call: ToolCallInfo }) {
    let args: Record<string, unknown> = {};
    try {
        const parsed = JSON.parse(call.input);
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
            args = parsed as Record<string, unknown>;
        }
    } catch {
        // input is not valid JSON; surface it as raw below
    }

    const command = pickString(args, ['command', 'commandLine']);
    const description = pickString(args, ['description', 'reason']);
    const remaining = filterArgs(args, ['command', 'commandLine', 'description', 'reason']);
    const remainingJson = Object.keys(remaining).length > 0 ? JSON.stringify(remaining, null, 2) : '';
    const inputWasInvalidJson = Object.keys(args).length === 0 && call.input.trim().length > 0;

    return (
        <div class="chat-tool-call-item">
            <div class="chat-tool-call-meta">
                <span class="chat-tool-name">{call.toolName}</span>
                {description && <span class="chat-tool-desc">{description}</span>}
            </div>
            {command !== undefined && (
                <div class="chat-tool-cmd-box">
                    <span class="chat-tool-cmd-prompt">$</span>
                    <span class="chat-tool-cmd-text">{command}</span>
                </div>
            )}
            {remainingJson && <pre class="chat-tool-args">{remainingJson}</pre>}
            {inputWasInvalidJson && !command && <pre class="chat-tool-args">{call.input}</pre>}
        </div>
    );
}

function pickString(args: Record<string, unknown>, keys: string[]): string | undefined {
    for (const key of keys) {
        const value = args[key];
        if (typeof value === 'string' && value.length > 0) {
            return value;
        }
    }
    return undefined;
}

function filterArgs(args: Record<string, unknown>, exclude: string[]): Record<string, unknown> {
    const remaining: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(args)) {
        if (!exclude.includes(k)) {
            remaining[k] = v;
        }
    }
    return remaining;
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
