import { h } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { marked } from 'marked';
import { AgentAvatar } from './AgentAvatar';
import type { AgentType } from '../types';

// Configure marked once: GFM + soft line breaks so the assistant's
// streamed text wraps naturally inside the chat bubble.
marked.setOptions({
    gfm: true,
    breaks: true,
});

export interface GroupedToolCall {
    id: string;
    toolCallId?: string;
    toolName: string;
    input: string;
    output?: string;
    isError?: boolean;
}

export type GroupedChatItem =
    | { id: string; kind: 'user'; content: string; createdAt: number }
    | { id: string; kind: 'assistant_text'; content: string; createdAt: number; streaming: boolean }
    | { id: string; kind: 'thinking'; content: string; createdAt: number }
    | {
          id: string;
          kind: 'tool_group';
          calls: GroupedToolCall[];
          createdAt: number;
      }
    | {
          id: string;
          kind: 'permission';
          requestId: string;
          toolName: string;
          input: string;
          createdAt: number;
          resolved?: 'allow' | 'deny';
      }
    | { id: string; kind: 'error'; content: string; createdAt: number };

interface MessageBubbleProps {
    item: GroupedChatItem;
    agentType?: AgentType;
    isLast: boolean;
    onRespondPermission?: (requestId: string, allow: boolean) => void;
}

export function MessageBubble({ item, agentType, isLast, onRespondPermission }: MessageBubbleProps) {
    switch (item.kind) {
        case 'user':
            return <UserBubble content={item.content} />;
        case 'assistant_text':
            return <AssistantBubble content={item.content} streaming={item.streaming} agentType={agentType} />;
        case 'thinking':
            return <ThinkingBubble content={item.content} isLast={isLast} />;
        case 'tool_group':
            return <ToolGroupBubble calls={item.calls} />;
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

function AssistantBubble({
    content,
    streaming,
    agentType,
}: {
    content: string;
    streaming: boolean;
    agentType?: AgentType;
}) {
    const html = marked.parse(content, { async: false }) as string;
    return (
        <div class="chat-message-row chat-message-row-assistant">
            {agentType && <AgentAvatar agentType={agentType} class="chat-message-avatar" />}
            <div class="chat-bubble chat-bubble-assistant">
                <div class="chat-bubble-body">
                    <div class="markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
                    {streaming && <span class="chat-cursor">▍</span>}
                </div>
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
    const html = marked.parse(content, { async: false }) as string;

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
            {isExpanded && (
                <div
                    class="chat-bubble-body chat-thinking-body markdown-body"
                    dangerouslySetInnerHTML={{ __html: html }}
                />
            )}
        </div>
    );
}

function ToolGroupBubble({ calls }: { calls: GroupedToolCall[] }) {
    const [isExpanded, setIsExpanded] = useState(true);

    return (
        <div class={`chat-bubble chat-bubble-tool-group ${isExpanded ? 'is-expanded' : 'is-collapsed'}`}>
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
                        <GroupedToolCallItem key={call.id || idx} call={call} />
                    ))}
                </div>
            )}
        </div>
    );
}

function FormatParamValue({ value }: { value: unknown }) {
    const [wordWrap, setWordWrap] = useState(true);

    if (value === null) return <span class="chat-tool-arg-value-null">null</span>;
    if (value === undefined) return <span class="chat-tool-arg-value-undefined">undefined</span>;

    if (typeof value === 'object') {
        return (
            <div class="chat-tool-arg-value-object-wrapper">
                <button
                    type="button"
                    onClick={() => setWordWrap(!wordWrap)}
                    class={`chat-tool-word-wrap-toggle ${wordWrap ? 'is-active' : ''}`}
                    title="切换自动换行"
                >
                    <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M4 6h16M4 12h10a4 4 0 0 1 0 8h-2" />
                        <polyline points="14 16 10 20 14 24" />
                    </svg>
                </button>
                <pre class={`chat-tool-arg-value-pre ${wordWrap ? 'whitespace-pre-wrap' : 'whitespace-pre'}`}>
                    {JSON.stringify(value, null, 2)}
                </pre>
            </div>
        );
    }

    if (typeof value === 'boolean') {
        return (
            <span class={`chat-tool-arg-value-bool ${value ? 'is-true' : 'is-false'}`}>{value ? 'true' : 'false'}</span>
        );
    }

    return <span class="chat-tool-arg-value-text">{String(value)}</span>;
}

function GroupedToolCallItem({ call }: { call: GroupedToolCall }) {
    const [isExpanded, setIsExpanded] = useState(true);

    let args: Record<string, unknown> = {};
    let parsedInput = false;
    try {
        if (call.input) {
            const parsed = JSON.parse(call.input);
            if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
                args = parsed as Record<string, unknown>;
                parsedInput = true;
            }
        }
    } catch {
        // input is not valid JSON
    }

    // Skip rendering when the input parses to an empty object — likely a
    // streaming/incomplete payload that will fill in shortly. Also skip when
    // there's no input at all (the bridge may omit `arguments` while waiting
    // for rawInput to arrive).
    if (parsedInput && Object.keys(args).length === 0) {
        return null;
    }
    if (!call.input) {
        return null;
    }

    const description = pickString(args, ['description', 'reason', 'Reason']);
    const inputWasInvalidJson = Object.keys(args).length === 0 && call.input && call.input.trim().length > 0;

    const hasOutput = call.output !== undefined;
    const isError = call.isError;

    return (
        <div
            class={`chat-tool-call-subcard ${isExpanded ? 'is-expanded' : 'is-collapsed'} ${isError ? 'has-error' : ''}`}
        >
            <div
                class="chat-tool-call-subcard-header"
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
                <span class="chat-tool-subcard-caret" aria-hidden="true">
                    {isExpanded ? '▾' : '▸'}
                </span>
                <span class="chat-tool-name-badge">{call.toolName}</span>
                {description && <span class="chat-tool-subcard-desc">{description}</span>}
                <span class="chat-tool-subcard-status">
                    {!hasOutput ? (
                        <span class="status-badge status-running">执行中...</span>
                    ) : isError ? (
                        <span class="status-badge status-error">失败</span>
                    ) : (
                        <span class="status-badge status-success">成功</span>
                    )}
                </span>
            </div>
            {isExpanded && (
                <div class="chat-tool-call-subcard-body">
                    {/* Input parameters section */}
                    <div class="chat-tool-subcard-section">
                        <div class="chat-tool-section-title">调用入参列表 (Arguments)</div>
                        <div class="chat-tool-section-content">
                            {Object.keys(args).length > 0 ? (
                                <div class="chat-tool-args-grid">
                                    {Object.entries(args).map(([paramName, paramVal]) => (
                                        <div key={paramName} class="chat-tool-arg-item">
                                            <div class="chat-tool-arg-indicator"></div>
                                            <div class="chat-tool-arg-header">
                                                <svg
                                                    viewBox="0 0 24 24"
                                                    width="10"
                                                    height="10"
                                                    fill="none"
                                                    stroke="currentColor"
                                                    stroke-width="2.5"
                                                    stroke-linecap="round"
                                                    stroke-linejoin="round"
                                                    class="chat-tool-arg-arrow"
                                                >
                                                    <polyline points="15 10 20 15 15 20" />
                                                    <path d="M4 4v7a4 4 0 0 0 4 4h12" />
                                                </svg>
                                                <code class="chat-tool-arg-name">{paramName}</code>
                                                <span class="chat-tool-arg-type">{typeof paramVal}</span>
                                            </div>
                                            <div class="chat-tool-arg-body">
                                                <FormatParamValue value={paramVal} />
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            ) : inputWasInvalidJson ? (
                                <pre class="chat-tool-args">{call.input}</pre>
                            ) : (
                                <div class="chat-tool-no-args">无附加调用参数 (No Arguments)</div>
                            )}
                        </div>
                    </div>

                    {/* Output result section */}
                    <div class="chat-tool-subcard-section">
                        <div class="chat-tool-section-title">工具返回结果 (Tool Output)</div>
                        <div class="chat-tool-section-content">
                            {!hasOutput ? (
                                <div class="chat-tool-output-pending">正在执行，请稍候...</div>
                            ) : (
                                <div class="chat-tool-terminal-box">
                                    <div class="chat-tool-terminal-header">
                                        <div class="chat-tool-terminal-dots">
                                            <span class="dot dot-close"></span>
                                            <span class="dot dot-minimize"></span>
                                            <span class="dot dot-expand"></span>
                                        </div>
                                        <span class="chat-tool-terminal-title">
                                            {call.toolName.toLowerCase()} - output.log
                                        </span>
                                        <div class="chat-tool-terminal-spacer"></div>
                                    </div>
                                    <pre class={`chat-tool-terminal-body ${isError ? 'has-error' : ''}`}>
                                        {call.output || '（执行完成，无返回内容）'}
                                    </pre>
                                </div>
                            )}
                        </div>
                    </div>
                </div>
            )}
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
                        <svg
                            viewBox="0 0 24 24"
                            width="14"
                            height="14"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            aria-hidden="true"
                        >
                            <path d="M18 6 6 18M6 6l12 12" />
                        </svg>
                        拒绝
                    </button>
                    <button class="chat-permission-btn allow" onClick={() => onRespond && onRespond(requestId, true)}>
                        <svg
                            viewBox="0 0 24 24"
                            width="14"
                            height="14"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            aria-hidden="true"
                        >
                            <path d="M5 12l5 5L20 7" />
                        </svg>
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
