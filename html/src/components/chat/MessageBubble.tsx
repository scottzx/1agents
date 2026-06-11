import { h } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { marked } from 'marked';
import { AgentAvatar } from './AgentAvatar';
import { t, getLang } from '../../i18n';
import type { AgentType, PermissionDecision } from '../types';

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
    permission?: {
        requestId: string;
        toolName: string;
        input: string;
        options: Array<{ text: string; data: string }>;
        resolved?: 'allow' | 'deny';
    };
}

export type GroupedChatItem =
    | { id: string; kind: 'user'; content: string; createdAt: number; queueStatus?: 'queued'; queueRequestId?: string }
    | { id: string; kind: 'assistant_text'; content: string; createdAt: number; streaming: boolean }
    | { id: string; kind: 'thinking'; content: string; createdAt: number }
    | {
          id: string;
          kind: 'tool_group';
          calls: GroupedToolCall[];
          createdAt: number;
          // True for groups assembled from the realtime pending pool
          // (tool_result / permission_request without a matching
          // tool_use yet). The renderer labels these "待分配" so the
          // user can tell they're waiting for the runtime to pair
          // them with the actual call.
          pending?: boolean;
      }
    | { id: string; kind: 'error'; content: string; createdAt: number };

interface MessageBubbleProps {
    item: GroupedChatItem;
    agentType?: AgentType;
    isLast: boolean;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
    onCancelQueued?: (queueRequestId: string) => void;
}

export function MessageBubble({ item, agentType, isLast, onRespondPermission, onCancelQueued }: MessageBubbleProps) {
    switch (item.kind) {
        case 'user':
            return (
                <UserBubble
                    content={item.content}
                    queueStatus={item.queueStatus}
                    queueRequestId={item.queueRequestId}
                    onCancel={onCancelQueued}
                />
            );
        case 'assistant_text':
            return <AssistantBubble content={item.content} streaming={item.streaming} agentType={agentType} />;
        case 'thinking':
            return <ThinkingBubble content={item.content} isLast={isLast} />;
        case 'tool_group':
            return (
                <ToolGroupBubble calls={item.calls} pending={item.pending} onRespondPermission={onRespondPermission} />
            );
        case 'error':
            return <ErrorBubble content={item.content} />;
    }
}

function UserBubble({
    content,
    queueStatus,
    queueRequestId,
    onCancel,
}: {
    content: string;
    queueStatus?: 'queued';
    queueRequestId?: string;
    onCancel?: (queueRequestId: string) => void;
}) {
    const isQueued = queueStatus === 'queued';
    return (
        <div class={`chat-bubble chat-bubble-user${isQueued ? ' chat-bubble-user-queued' : ''}`}>
            <div class="chat-bubble-body chat-bubble-body-queued">{content}</div>
            {isQueued && (
                <>
                    <span class="chat-bubble-queue-badge">{t('chat.queue.queued', getLang())}</span>
                    {queueRequestId && onCancel && (
                        <button
                            type="button"
                            class="chat-bubble-queue-cancel"
                            aria-label={t('chat.queue.cancelAria', getLang())}
                            title={t('chat.queue.cancelTitle', getLang())}
                            onClick={() => onCancel(queueRequestId)}
                        >
                            ×
                        </button>
                    )}
                </>
            )}
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

function ToolGroupBubble({
    calls,
    pending,
    onRespondPermission,
}: {
    calls: GroupedToolCall[];
    pending?: boolean;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
}) {
    const [isExpanded, setIsExpanded] = useState(true);

    return (
        <div
            class={`chat-bubble chat-bubble-tool-group ${isExpanded ? 'is-expanded' : 'is-collapsed'} ${pending ? 'is-pending' : ''}`}
        >
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
                <span class="chat-bubble-title">{pending ? '待分配' : '工具调用'}</span>
                <span class="chat-bubble-count">{calls.length}</span>
            </div>
            {isExpanded && (
                <div class="chat-tool-calls-list">
                    {calls.map((call, idx) => (
                        <GroupedToolCallItem
                            key={call.id || idx}
                            call={call}
                            onRespondPermission={onRespondPermission}
                        />
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

function GroupedToolCallItem({
    call,
    onRespondPermission,
}: {
    call: GroupedToolCall;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
}) {
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

    const hasOutput = call.output !== undefined;
    const inputWasInvalidJson = call.input && call.input.trim().length > 0 && Object.keys(args).length === 0;
    const emptyParsedInput = parsedInput && Object.keys(args).length === 0;
    const hasPermission = !!call.permission;

    // Skip rendering only when nothing concrete has arrived yet: no input,
    // no output, no inline permission, and no toolCallId to identify the
    // call. The MessageList isCallRenderable filter is the primary gate;
    // this is a defensive double-check for direct callers.
    if (!call.toolCallId && !call.input && !hasOutput && !hasPermission) {
        return null;
    }
    if (emptyParsedInput && !inputWasInvalidJson && !hasOutput && !hasPermission) {
        return null;
    }

    const description = pickString(args, ['description', 'reason', 'Reason']);

    const isError = call.isError;

    return (
        <div
            class={`chat-tool-call-subcard ${isExpanded ? 'is-expanded' : 'is-collapsed'} ${isError ? 'has-error' : ''} ${hasPermission && !call.permission?.resolved ? 'has-pending-permission' : ''}`}
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

                    {/* Inline permission section. Replaces the old
                        standalone PermissionBubble. Collapses to a single
                        line once resolved so the card stays compact as
                        more events stream in. */}
                    {hasPermission && (
                        <div class="chat-tool-subcard-section">
                            <div class="chat-tool-section-title">权限确认 (Permission)</div>
                            <div class="chat-tool-section-content">
                                {call.permission!.resolved ? (
                                    <div
                                        class={`chat-bubble chat-bubble-permission is-resolved chat-permission-${call.permission!.resolved}`}
                                    >
                                        <span class="chat-permission-resolved-mark" aria-hidden="true">
                                            {call.permission!.resolved === 'allow' ? '✓' : '✕'}
                                        </span>
                                        <span class="chat-permission-resolved-text">
                                            {t(
                                                call.permission!.resolved === 'allow'
                                                    ? 'chat.permission.resolved.allow'
                                                    : 'chat.permission.resolved.deny',
                                                getLang()
                                            )}{' '}
                                            · {call.permission!.toolName}
                                        </span>
                                    </div>
                                ) : (
                                    <PermissionActionRow
                                        permission={call.permission!}
                                        onRespond={onRespondPermission}
                                    />
                                )}
                            </div>
                        </div>
                    )}

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

function PermissionActionRow({
    permission,
    onRespond,
}: {
    permission: NonNullable<GroupedToolCall['permission']>;
    onRespond?: (requestId: string, decision: PermissionDecision) => void;
}) {
    const lang = getLang();
    const respond = (decision: PermissionDecision) => {
        if (onRespond) onRespond(permission.requestId, decision);
    };
    // Four buttons, ordered left → right by escalation:
    //   deny-always · deny · allow · allow-always
    // Reuses the same `chat-permission-btn` classes the old standalone
    // PermissionBubble used, so colours and focus styles are identical.
    return (
        <div class="chat-permission-inline">
            <div class="chat-permission-inline-label">
                {t('chat.permission.title', lang, { tool: permission.toolName })}
            </div>
            {permission.input && <pre class="chat-bubble-code">{permission.input}</pre>}
            <div class="chat-permission-actions">
                <button
                    type="button"
                    class="chat-permission-btn deny-always"
                    onClick={() => respond('reject_always')}
                    title={t('chat.permission.denyAlways', lang)}
                >
                    <span class="chat-permission-btn-label">{t('chat.permission.denyAlways', lang)}</span>
                </button>
                <button
                    type="button"
                    class="chat-permission-btn deny"
                    onClick={() => respond('reject_once')}
                    title={t('chat.permission.deny', lang)}
                >
                    <span class="chat-permission-btn-label">{t('chat.permission.deny', lang)}</span>
                </button>
                <button
                    type="button"
                    class="chat-permission-btn allow"
                    onClick={() => respond('allow_once')}
                    title={t('chat.permission.allow', lang)}
                >
                    <span class="chat-permission-btn-label">{t('chat.permission.allow', lang)}</span>
                </button>
                <button
                    type="button"
                    class="chat-permission-btn allow-always"
                    onClick={() => respond('allow_always')}
                    title={t('chat.permission.allowAlways', lang)}
                >
                    <span class="chat-permission-btn-label">{t('chat.permission.allowAlways', lang)}</span>
                </button>
            </div>
        </div>
    );
}

function ErrorBubble({ content }: { content: string }) {
    return (
        <div class="chat-bubble chat-bubble-error">
            <div class="chat-bubble-label">{t('chat.bubble.error', getLang())}</div>
            <div class="chat-bubble-body">{content}</div>
        </div>
    );
}
