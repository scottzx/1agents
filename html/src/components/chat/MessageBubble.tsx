import { h } from 'preact';
// Referenced by the compiled output of JSX fragments (<>…</>) via the
// jsxFragmentFactory compiler option, not by name in this file.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { Fragment } from 'preact';
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
    /**
     * True while a turn is actively running. Distinguishes "no output
     * yet because the tool is still executing" (spinner) from "no
     * output recorded in history" (e.g. a cancelled turn replayed via
     * resume — shown as a neutral incomplete state, not an eternal
     * spinner).
     */
    active?: boolean;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
    onCancelQueued?: (queueRequestId: string) => void;
}

export function MessageBubble({
    item,
    agentType,
    isLast,
    active,
    onRespondPermission,
    onCancelQueued,
}: MessageBubbleProps) {
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
            return <ThinkingBubble content={item.content} streaming={!!active && isLast} />;
        case 'tool_group':
            return (
                <ToolGroupBubble
                    calls={item.calls}
                    pending={item.pending}
                    active={active}
                    onRespondPermission={onRespondPermission}
                />
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

/**
 * Reasoning block. While the model is actively thinking (this is the
 * last item of a running turn) the block stays expanded with a shimmer
 * label; once the turn moves on it auto-collapses to a one-line
 * preview the user can re-open. The same rule covers resume: replayed
 * thinking blocks arrive with `streaming = false` and start collapsed.
 */
function ThinkingBubble({ content, streaming }: { content: string; streaming: boolean }) {
    const [isExpanded, setIsExpanded] = useState(streaming);

    // Follow the streaming state: expand while the model thinks,
    // collapse when the turn moves on. A manual toggle afterwards
    // still works — this only fires on streaming transitions.
    useEffect(() => {
        setIsExpanded(streaming);
    }, [streaming]);

    const lang = getLang();
    const previewText = content.trim().replace(/\s+/g, ' ');
    const preview = previewText.length > 80 ? `${previewText.slice(0, 80)}…` : previewText;
    const html = marked.parse(content, { async: false }) as string;

    return (
        <div
            class={`chat-bubble chat-bubble-thinking ${isExpanded ? 'is-expanded' : 'is-collapsed'} ${streaming ? 'is-streaming' : ''}`}
        >
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
                <span class="chat-bubble-label">
                    {streaming ? t('chat.thinking.streaming', lang) : t('chat.thinking.label', lang)}
                </span>
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

/**
 * User expand/collapse choices per tool call, keyed by toolCallId.
 * Module-level so the choice survives the component remount caused by
 * the post-`done` history reload (history items get fresh ids, but
 * toolCallId is stable across streaming and replay).
 */
const userExpandChoice = new Map<string, boolean>();

type CallStatus = 'running' | 'waiting' | 'success' | 'error' | 'incomplete';

function callStatus(call: GroupedToolCall, active: boolean): CallStatus {
    if (call.permission && !call.permission.resolved) return 'waiting';
    if (call.output !== undefined) return call.isError ? 'error' : 'success';
    return active ? 'running' : 'incomplete';
}

function ToolGroupBubble({
    calls,
    pending,
    active,
    onRespondPermission,
}: {
    calls: GroupedToolCall[];
    pending?: boolean;
    active?: boolean;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
}) {
    const [isExpanded, setIsExpanded] = useState(true);
    const lang = getLang();

    const statuses = calls.map(c => callStatus(c, !!active));
    const runningCount = statuses.filter(s => s === 'running').length;
    const errorCount = statuses.filter(s => s === 'error').length;
    const hasWaiting = statuses.includes('waiting');

    // A pending permission must never be hidden behind a collapsed
    // group — the turn is blocked on the user's decision.
    useEffect(() => {
        if (hasWaiting) setIsExpanded(true);
    }, [hasWaiting]);

    let summary: { cls: string; text: string } | null = null;
    if (hasWaiting) {
        summary = { cls: 'status-waiting', text: t('chat.tool.summary.waiting', lang) };
    } else if (runningCount > 0) {
        summary = { cls: 'status-running', text: t('chat.tool.summary.running', lang, { n: String(runningCount) }) };
    } else if (errorCount > 0) {
        summary = { cls: 'status-error', text: t('chat.tool.summary.error', lang, { n: String(errorCount) }) };
    }

    return (
        <div
            class={`chat-bubble chat-bubble-tool-group ${isExpanded ? 'is-expanded' : 'is-collapsed'} ${pending ? 'is-pending' : ''}`}
        >
            <div
                class="chat-tool-group-header"
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
                <span class="chat-tool-group-title">
                    {pending ? t('chat.tool.groupPending', lang) : t('chat.tool.groupTitle', lang)}
                </span>
                <span class="chat-tool-group-count">{calls.length}</span>
                {summary && (
                    <span class={`chat-tool-group-summary ${summary.cls}`}>
                        {(hasWaiting || runningCount > 0) && <span class="chat-tool-spinner" aria-hidden="true" />}
                        {summary.text}
                    </span>
                )}
            </div>
            {isExpanded && (
                <div class="chat-tool-calls-list">
                    {calls.map((call, idx) => (
                        <GroupedToolCallItem
                            key={call.id || idx}
                            call={call}
                            status={statuses[idx]}
                            onRespondPermission={onRespondPermission}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

/**
 * Arg keys most likely to identify what a call is doing, in priority
 * order. Used to surface a one-line summary in the collapsed row so
 * the user can tell `Bash: git status` from `Read: foo.ts` without
 * expanding anything.
 */
const SUMMARY_KEYS = [
    'command',
    'file_path',
    'path',
    'pattern',
    'query',
    'url',
    'prompt',
    'description',
    'reason',
    'Reason',
];

function summarizeArgs(args: Record<string, unknown>): string | undefined {
    for (const key of SUMMARY_KEYS) {
        const value = args[key];
        if (typeof value === 'string' && value.trim()) {
            return value.replace(/\s+/g, ' ').trim();
        }
    }
    for (const value of Object.values(args)) {
        if (typeof value === 'string' && value.trim()) {
            return value.replace(/\s+/g, ' ').trim();
        }
    }
    return undefined;
}

function StatusIcon({ status }: { status: CallStatus }) {
    switch (status) {
        case 'running':
            return <span class="chat-tool-status-icon chat-tool-spinner" aria-hidden="true" />;
        case 'waiting':
            return (
                <span class="chat-tool-status-icon is-waiting" aria-hidden="true">
                    !
                </span>
            );
        case 'success':
            return (
                <span class="chat-tool-status-icon is-success" aria-hidden="true">
                    ✓
                </span>
            );
        case 'error':
            return (
                <span class="chat-tool-status-icon is-error" aria-hidden="true">
                    ✕
                </span>
            );
        case 'incomplete':
            return (
                <span class="chat-tool-status-icon is-incomplete" aria-hidden="true">
                    ◦
                </span>
            );
    }
}

function GroupedToolCallItem({
    call,
    status,
    onRespondPermission,
}: {
    call: GroupedToolCall;
    status: CallStatus;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
}) {
    const lang = getLang();
    const hasPendingPermission = status === 'waiting';

    // Rows start collapsed — the header already carries tool name,
    // key-arg summary and status. A pending permission force-expands
    // (the user must see the action buttons); an explicit user choice
    // (persisted by toolCallId across history reloads) wins otherwise.
    const [isExpanded, setIsExpanded] = useState(() => {
        if (hasPendingPermission) return true;
        if (call.toolCallId && userExpandChoice.has(call.toolCallId)) {
            return userExpandChoice.get(call.toolCallId)!;
        }
        return false;
    });

    useEffect(() => {
        if (hasPendingPermission) {
            setIsExpanded(true);
            return;
        }
        // Forced-open reason went away (permission resolved): fall back
        // to the user's remembered choice, or collapsed.
        if (call.toolCallId && userExpandChoice.has(call.toolCallId)) {
            setIsExpanded(userExpandChoice.get(call.toolCallId)!);
            return;
        }
        setIsExpanded(false);
    }, [hasPendingPermission]);

    const toggle = () => {
        setIsExpanded(prev => {
            const next = !prev;
            if (call.toolCallId) userExpandChoice.set(call.toolCallId, next);
            return next;
        });
    };

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

    const summary = Object.keys(args).length > 0 ? summarizeArgs(args) : undefined;

    return (
        <div class={`chat-tool-row ${isExpanded ? 'is-expanded' : 'is-collapsed'} status-${status}`}>
            <div
                class="chat-tool-row-header"
                role="button"
                tabIndex={0}
                onClick={toggle}
                onKeyDown={e => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        toggle();
                    }
                }}
            >
                <StatusIcon status={status} />
                <span class="chat-tool-name-badge">{call.toolName}</span>
                {summary && <span class="chat-tool-row-summary">{summary}</span>}
                {status === 'waiting' && (
                    <span class="chat-tool-row-status is-waiting">{t('chat.tool.status.waiting', lang)}</span>
                )}
                {status === 'running' && (
                    <span class="chat-tool-row-status is-running">{t('chat.tool.status.running', lang)}</span>
                )}
                <span class="chat-tool-row-caret" aria-hidden="true">
                    {isExpanded ? '▾' : '▸'}
                </span>
            </div>
            {isExpanded && (
                <div class="chat-tool-row-body">
                    {/* Arguments */}
                    <div class="chat-tool-section">
                        <div class="chat-tool-section-title">{t('chat.tool.args', lang)}</div>
                        {Object.keys(args).length > 0 ? (
                            <div class="chat-tool-args-list">
                                {Object.entries(args).map(([paramName, paramVal]) => (
                                    <div key={paramName} class="chat-tool-arg">
                                        <code class="chat-tool-arg-name">{paramName}</code>
                                        <ArgValue value={paramVal} />
                                    </div>
                                ))}
                            </div>
                        ) : inputWasInvalidJson ? (
                            <pre class="chat-tool-pre">{call.input}</pre>
                        ) : (
                            <div class="chat-tool-muted">{t('chat.tool.noArgs', lang)}</div>
                        )}
                    </div>

                    {/* Inline permission: pending shows the action buttons,
                        resolved collapses to a one-line receipt. */}
                    {hasPermission && (
                        <div class="chat-tool-section">
                            <div class="chat-tool-section-title">{t('chat.tool.permission', lang)}</div>
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
                                            lang
                                        )}{' '}
                                        · {call.permission!.toolName}
                                    </span>
                                </div>
                            ) : (
                                <PermissionActionRow permission={call.permission!} onRespond={onRespondPermission} />
                            )}
                        </div>
                    )}

                    {/* Output */}
                    <div class="chat-tool-section">
                        <div class="chat-tool-section-title">{t('chat.tool.output', lang)}</div>
                        {!hasOutput ? (
                            <div class="chat-tool-muted">
                                {status === 'running'
                                    ? t('chat.tool.outputPending', lang)
                                    : t('chat.tool.outputMissing', lang)}
                            </div>
                        ) : call.output ? (
                            <pre class={`chat-tool-pre chat-tool-output ${call.isError ? 'has-error' : ''}`}>
                                {call.output}
                            </pre>
                        ) : (
                            <div class="chat-tool-muted">{t('chat.tool.outputEmpty', lang)}</div>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}

function ArgValue({ value }: { value: unknown }) {
    if (value === null || value === undefined) {
        return <span class="chat-tool-arg-empty">{value === null ? 'null' : 'undefined'}</span>;
    }
    if (typeof value === 'object') {
        return <pre class="chat-tool-pre">{JSON.stringify(value, null, 2)}</pre>;
    }
    const text = String(value);
    if (text.includes('\n') || text.length > 120) {
        return <pre class="chat-tool-pre">{text}</pre>;
    }
    return <span class="chat-tool-arg-value">{text}</span>;
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
    return (
        <div class="chat-permission-inline">
            <div class="chat-permission-inline-label">
                {t('chat.permission.title', lang, { tool: permission.toolName })}
            </div>
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
