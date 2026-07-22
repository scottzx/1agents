import { h } from 'preact';
// Referenced by the compiled output of JSX fragments (<>…</>) via the
// jsxFragmentFactory compiler option, not by name in this file.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { Fragment } from 'preact';
import { useEffect, useRef, useMemo, useState } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { marked } from 'marked';
import { t, getLang, type Lang } from '../../i18n';
import type { PermissionDecision } from '../types';
import type {
    AskUserAnswerValue,
    AskUserOutcome,
    AskUserQuestionState,
    ExitPlanModeState,
    ExitPlanOutcome,
} from '@1agents/core/protocol/types';
import { renderMarkdown } from '../../utils/markdown';
import { renderMermaidBlocks } from '../../utils/mermaid';
import { activeProjectName } from '../../stores/taskNavStore';
import { showToast, theme } from '../../stores/uiStore';
import { ToolDiffView, deriveDiffsFromInput, deriveLocationsFromInput } from './ToolDiffView';
import { deriveToolKind } from './ToolKindIcon';
import { terminalCommandLine } from './terminalCommand';

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
    kind?: string;
    /** ACP ToolCallStatus when the agent reported it. */
    status?: 'pending' | 'in_progress' | 'completed' | 'failed';
    locations?: Array<{ path: string; line?: number }>;
    diffs?: Array<{ path: string; oldText?: string; newText: string }>;
    askUser?: AskUserQuestionState;
    exitPlan?: ExitPlanModeState;
    permission?: {
        requestId: string;
        toolName: string;
        input: string;
        options: Array<{ text: string; data: string }>;
        resolved?: 'allow' | 'deny';
    };
}

export type ToolGroupElement =
    | { kind: 'thinking'; id: string; content: string }
    | { kind: 'call'; call: GroupedToolCall };

export type GroupedChatItem =
    | { id: string; kind: 'user'; content: string; createdAt: number; queueStatus?: 'queued'; queueRequestId?: string }
    | { id: string; kind: 'assistant_text'; content: string; createdAt: number; streaming: boolean }
    | { id: string; kind: 'thinking'; content: string; createdAt: number }
    | {
          id: string;
          kind: 'tool_group';
          calls: GroupedToolCall[];
          thinkingBlocks?: string[];
          elements?: ToolGroupElement[];
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
    isLast: boolean;
    /**
     * True while a turn is actively running. Distinguishes "no output
     * yet because the tool is still executing" (spinner) from "no
     * output recorded in history" (e.g. a cancelled turn replayed via
     * resume — shown as a neutral incomplete state, not an eternal
     * spinner).
     */
    active?: boolean;
    onCancelQueued?: (queueRequestId: string) => void;
}

export function MessageBubble({ item, isLast, active, onCancelQueued }: MessageBubbleProps) {
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
            return <AssistantBubble content={item.content} streaming={item.streaming} />;
        case 'thinking':
            return <ThinkingBubble content={item.content} streaming={!!active && isLast} />;
        case 'tool_group':
            return (
                <ToolGroupBubble
                    calls={item.calls}
                    thinkingBlocks={item.thinkingBlocks}
                    elements={item.elements}
                    pending={item.pending}
                    active={active}
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
    const html = renderMarkdown(content, { projectName: activeProjectName() });
    const bodyRef = useRef<HTMLDivElement>(null);
    const currentTheme = theme.value;

    useEffect(() => {
        if (isQueued) return;
        renderMermaidBlocks(bodyRef.current, currentTheme);
    }, [html, isQueued, currentTheme]);

    return (
        <div class={`chat-bubble chat-bubble-user${isQueued ? ' chat-bubble-user-queued' : ''}`}>
            {isQueued ? (
                <div class="chat-bubble-body chat-bubble-body-queued">{content}</div>
            ) : (
                <div class="chat-bubble-body">
                    <div ref={bodyRef} class="markdown-body md-conv" dangerouslySetInnerHTML={{ __html: html }} />
                </div>
            )}
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

function copyToClipboardFallback(text: string): boolean {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    try {
        return document.execCommand('copy');
    } catch {
        return false;
    } finally {
        textarea.remove();
    }
}

async function copyToClipboard(text: string): Promise<boolean> {
    if (navigator.clipboard?.writeText) {
        try {
            await navigator.clipboard.writeText(text);
            return true;
        } catch {
            return copyToClipboardFallback(text);
        }
    }

    return copyToClipboardFallback(text);
}

function CopyIcon() {
    return (
        <svg viewBox="0 0 16 16" aria-hidden="true">
            <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
            <path d="M10.5 5.5V4A1.5 1.5 0 0 0 9 2.5H4A1.5 1.5 0 0 0 2.5 4v5A1.5 1.5 0 0 0 4 10.5h1.5" />
        </svg>
    );
}

function FoldIcon({ expanded }: { expanded: boolean }) {
    return (
        <svg viewBox="0 0 16 16" aria-hidden="true">
            {expanded ? (
                <>
                    <path d="M5 4v8l3-4z" />
                    <path d="M11 4v8l-3-4z" />
                </>
            ) : (
                <>
                    <path d="M8 3 5 6h6z" />
                    <path d="M8 13l-3-3h6z" />
                </>
            )}
        </svg>
    );
}

function AssistantContent({
    content,
    streaming,
    showActions,
}: {
    content: string;
    streaming: boolean;
    showActions: boolean;
}) {
    // Expansion + "selected" state stay local to this AssistantContent instance.
    // Clicking the block activates it (shows copy/fold); clicking outside
    // deactivates. Mutual exclusivity across blocks falls out of each block's
    // own document pointerdown listener — no shared focusedMessageId needed.
    const isExpanded = useSignal(true);
    const isActive = useSignal(false);
    const isCollapsible = useSignal(false);
    const blockRef = useRef<HTMLDivElement>(null);
    const lang = getLang();
    const projectName = activeProjectName();
    const html = useMemo(() => renderMarkdown(content, { projectName }), [content, projectName]);
    const bodyRef = useRef<HTMLDivElement>(null);
    const currentTheme = theme.value;

    useEffect(() => {
        if (!showActions) return;

        const handlePointerDown = (event: PointerEvent) => {
            if (!blockRef.current?.contains(event.target as Node)) {
                isExpanded.value = false;
                isActive.value = false;
            }
        };

        document.addEventListener('pointerdown', handlePointerDown);
        return () => document.removeEventListener('pointerdown', handlePointerDown);
    }, [showActions]);

    useEffect(() => {
        if (streaming) return;
        renderMermaidBlocks(bodyRef.current, currentTheme);
    }, [html, streaming, currentTheme]);

    useEffect(() => {
        if (!showActions || !bodyRef.current) return;

        const body = bodyRef.current;
        const updateCollapsible = () => {
            const collapsedMaxHeight = parseFloat(getComputedStyle(body).fontSize) * 30;
            isCollapsible.value = body.scrollHeight > collapsedMaxHeight + 1;
        };

        updateCollapsible();
        const observer = new ResizeObserver(updateCollapsible);
        observer.observe(body);
        return () => observer.disconnect();
    }, [html, showActions]);

    const copy = async () => {
        const copied = await copyToClipboard(content);
        showToast(t(copied ? 'app.toast.copySuccess' : 'app.toast.copyFailed', lang));
    };

    const toggle = () => {
        isExpanded.value = !isExpanded.value;
    };

    const activate = () => {
        if (showActions) isActive.value = true;
    };

    const canCollapse = showActions && isCollapsible.value;
    const expanded = !canCollapse || isExpanded.value;
    const actionsVisible = showActions && isActive.value;

    return (
        <div
            ref={blockRef}
            class={`chat-assistant-block${isActive.value ? ' is-active' : ''}`}
            onPointerDown={activate}
        >
            <div class={`chat-assistant-content ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
                <div ref={bodyRef} class="markdown-body md-conv" dangerouslySetInnerHTML={{ __html: html }} />
                {streaming && <span class="chat-cursor">▍</span>}
            </div>
            {actionsVisible && (
                <div class="chat-assistant-actions">
                    <button
                        type="button"
                        class="chat-assistant-action"
                        onClick={() => void copy()}
                        aria-label={t('common.copy', lang)}
                        title={t('common.copy', lang)}
                    >
                        <CopyIcon />
                    </button>
                    {canCollapse && (
                        <button
                            type="button"
                            class="chat-assistant-action"
                            onClick={toggle}
                            aria-label={t(expanded ? 'chat.bubble.collapse' : 'chat.bubble.expand', lang)}
                            title={t(expanded ? 'chat.bubble.collapse' : 'chat.bubble.expand', lang)}
                        >
                            <FoldIcon expanded={expanded} />
                        </button>
                    )}
                </div>
            )}
        </div>
    );
}

function AssistantBubble({ content, streaming }: { content: string; streaming: boolean }) {
    return (
        <div class="chat-message-row chat-message-row-assistant">
            <div class="chat-bubble chat-bubble-assistant">
                <div class="chat-bubble-body">
                    <AssistantContent content={content} streaming={streaming} showActions />
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
    // Expansion is held in a signal, not useState. The app re-renders
    // through @preact/signals; a plain `useState` setter in this component
    // fired its updater but never re-rendered (the header click ran yet the
    // block stayed collapsed). Reading `isExpanded.value` during render
    // subscribes this component to the signal, so toggling it re-renders
    // reliably via the same mechanism the rest of the UI uses.
    const isExpanded = useSignal(streaming);

    // Auto-expand while the model is actively thinking and auto-collapse
    // once the turn moves on — only on an actual change of `streaming`, so
    // a manual toggle in between sticks until the next real transition.
    const prevStreaming = useRef(streaming);
    useEffect(() => {
        if (prevStreaming.current === streaming) return;
        prevStreaming.current = streaming;
        isExpanded.value = streaming;
    }, [streaming]);

    const toggle = () => {
        isExpanded.value = !isExpanded.value;
    };

    const expanded = isExpanded.value;
    const lang = getLang();
    // Parse markdown and build the preview once per content change, not on
    // every render. Toggling only flips `isExpanded`, so without this memo
    // each click re-ran marked.parse over the whole reasoning block —
    // exactly the kind of per-click hitch that makes expand/collapse feel
    // laggy. Memoised, a toggle is just a cheap show/hide.
    const { preview, html } = useMemo(() => {
        const previewText = content.trim().replace(/\s+/g, ' ');
        return {
            preview: previewText.length > 80 ? `${previewText.slice(0, 80)}…` : previewText,
            html: marked.parse(content, { async: false }) as string,
        };
    }, [content]);

    return (
        <div
            class={`chat-bubble chat-bubble-thinking ${expanded ? 'is-expanded' : 'is-collapsed'} ${streaming ? 'is-streaming' : ''}`}
        >
            <div
                class="chat-bubble-header-clickable"
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
                <span class="chat-bubble-caret" aria-hidden="true">
                    {expanded ? '▾' : '▸'}
                </span>
                <span class="chat-bubble-label">
                    {streaming ? t('chat.thinking.streaming', lang) : t('chat.thinking.label', lang)}
                </span>
                {!expanded && preview && <span class="chat-thinking-preview">{preview}</span>}
            </div>
            {expanded && (
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

/**
 * User collapse choices per tool *group*. Like userExpandChoice above,
 * this is module-level so the choice survives the remount caused by the
 * post-`done` history reload — the group's React key changes from
 * `group-<cryptoId>` (streaming) to `group-h-tool-<id>` (replay), which
 * would otherwise reset the local `useState(true)` and silently re-open
 * a group the user had collapsed. Keyed by the first call's toolCallId,
 * which is stable across streaming and replay.
 */
const groupCollapseChoice = new Map<string, boolean>();

function groupKey(calls: GroupedToolCall[]): string | undefined {
    return calls[0]?.toolCallId;
}

type CallStatus = 'running' | 'waiting' | 'success' | 'error' | 'incomplete';

/**
 * Prefer ACP tool-call status when the agent reported it. Fall back to the
 * previous heuristic (output / turn active) for history replay and agents that
 * never emit status. Permission waiting still wins over ACP pending — the user
 * must act before the tool can progress.
 */
function callStatus(call: GroupedToolCall, active: boolean): CallStatus {
    if (call.permission && !call.permission.resolved) return 'waiting';
    if (call.status === 'failed') return 'error';
    if (call.status === 'completed') return 'success';
    if (call.status === 'in_progress' || call.status === 'pending') return 'running';
    if (call.output !== undefined) return call.isError ? 'error' : 'success';
    return active ? 'running' : 'incomplete';
}

function agenticGroupTitle(
    calls: GroupedToolCall[],
    thinkingBlocks: string[],
    pending: boolean,
    lang: ReturnType<typeof getLang>
): string {
    if (pending) return t('chat.tool.groupPending', lang);

    const readNames = new Set(['Read', 'read', 'Glob', 'glob', 'Grep', 'grep', 'LS', 'ls', 'List', 'list']);
    const commandCount = calls.filter(call => (call.kind ?? deriveToolKind(call.toolName)) === 'execute').length;
    const readFileCount = calls.reduce((count, call) => {
        if (!readNames.has(call.toolName)) return count;
        if (call.locations && call.locations.length > 0) return count + call.locations.length;
        return count + 1;
    }, 0);

    if (thinkingBlocks.length > 0 && calls.length > 0) {
        return t('chat.tool.groupTitleAgenticWithThinking', lang, {
            thoughts: String(thinkingBlocks.length),
            files: String(readFileCount),
            commands: String(commandCount),
            tools: String(calls.length),
        });
    }
    if (calls.length > 0) {
        return t('chat.tool.groupTitleAgentic', lang, {
            files: String(readFileCount),
            commands: String(commandCount),
            tools: String(calls.length),
        });
    }
    return t('chat.tool.groupTitleThinkingOnly', lang, { thoughts: String(thinkingBlocks.length) });
}

function ToolGroupBubble({
    calls,
    thinkingBlocks = [],
    elements = [],
    pending,
    active,
}: {
    calls: GroupedToolCall[];
    thinkingBlocks?: string[];
    elements?: ToolGroupElement[];
    pending?: boolean;
    active?: boolean;
}) {
    const key = groupKey(calls);
    // Expansion lives in a signal, not useState — see ThinkingBubble: the
    // app re-renders through @preact/signals, and a plain useState setter
    // in these chat bubbles fired its updater but didn't re-render, so the
    // header click did nothing. Reading `.value` subscribes this component.
    // Tool groups default collapsed; pending permissions surface in the
    // composer-adjacent prompt, so waiting state no longer force-opens.
    const isExpanded = useSignal(key && groupCollapseChoice.has(key) ? groupCollapseChoice.get(key)! : false);
    const lang = getLang();

    const statuses = calls.map(c => callStatus(c, !!active));
    const runningCount = statuses.filter(s => s === 'running').length;
    const errorCount = statuses.filter(s => s === 'error').length;
    const hasWaiting = statuses.includes('waiting');

    const toggle = () => {
        const next = !isExpanded.value;
        isExpanded.value = next;
        if (key) groupCollapseChoice.set(key, next);
    };

    const expanded = isExpanded.value;

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
            class={`chat-bubble chat-bubble-tool-group ${expanded ? 'is-expanded' : 'is-collapsed'} ${pending ? 'is-pending' : ''}`}
        >
            <div
                class="chat-tool-group-header"
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
                <span class="chat-bubble-caret" aria-hidden="true">
                    {expanded ? '▾' : '▸'}
                </span>
                <span class="chat-tool-group-title">{agenticGroupTitle(calls, thinkingBlocks, !!pending, lang)}</span>
                {!pending && thinkingBlocks.length === 0 && <span class="chat-tool-group-count">{calls.length}</span>}
                {!pending && !active && <span class="chat-tool-group-processed">{t('chat.process.done', lang)}</span>}
                {summary && (
                    <span class={`chat-tool-group-summary ${summary.cls}`}>
                        {(hasWaiting || runningCount > 0) && <span class="chat-tool-spinner" aria-hidden="true" />}
                        {summary.text}
                    </span>
                )}
            </div>
            {expanded && (
                <div class="chat-tool-calls-list">
                    {elements.map((el, idx) => {
                        if (el.kind === 'thinking') {
                            const isLastThinking = el.content === thinkingBlocks[thinkingBlocks.length - 1];
                            return (
                                <GroupedThinkingItem
                                    key={el.id || idx}
                                    content={el.content}
                                    streaming={!!active && isLastThinking}
                                />
                            );
                        }
                        const callIdx = calls.indexOf(el.call);
                        return (
                            <GroupedToolCallItem key={el.call.id || idx} call={el.call} status={statuses[callIdx]} />
                        );
                    })}
                </div>
            )}
        </div>
    );
}

/** Four agentic categories shown as fixed-width 2-char labels. */
type KindCategory = 'think' | 'read' | 'execute' | 'tool';

function kindCategory(kind?: string): KindCategory {
    if (kind === 'think') return 'think';
    if (kind === 'read' || kind === 'search') return 'read';
    if (kind === 'execute') return 'execute';
    return 'tool';
}

function kindCategoryLabel(category: KindCategory, lang: ReturnType<typeof getLang>): string {
    switch (category) {
        case 'think':
            return t('chat.tool.kind.think', lang);
        case 'read':
            return t('chat.tool.kind.read', lang);
        case 'execute':
            return t('chat.tool.kind.execute', lang);
        case 'tool':
            return t('chat.tool.kind.tool', lang);
    }
}

function GroupedThinkingItem({ content, streaming }: { content: string; streaming: boolean }) {
    const isExpanded = useSignal(streaming);
    const lang = getLang();

    // Auto-expand/collapse on streaming changes
    const prevStreaming = useRef(streaming);
    useEffect(() => {
        if (prevStreaming.current === streaming) return;
        prevStreaming.current = streaming;
        isExpanded.value = streaming;
    }, [streaming]);

    const toggle = () => {
        isExpanded.value = !isExpanded.value;
    };

    const expanded = isExpanded.value;

    const { preview, html } = useMemo(() => {
        const previewText = content.trim().replace(/\s+/g, ' ');
        return {
            // Collapsed header shows the first ~50 chars of the thought (same
            // idea as the old thinking-row summary, slightly shorter).
            preview: previewText.length > 50 ? `${previewText.slice(0, 50)}…` : previewText,
            html: marked.parse(content, { async: false }) as string,
        };
    }, [content]);

    // Header: 「思考」+ first 50 chars. Empty stream falls back to "思考中…".
    const headerText = preview || (streaming ? t('chat.thinking.streaming', lang) : t('chat.thinking.label', lang));

    return (
        <div class={`chat-tool-row ${expanded ? 'is-expanded' : 'is-collapsed'} status-thinking`}>
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
                <span class="chat-tool-row-caret" aria-hidden="true">
                    {expanded ? '▾' : '▸'}
                </span>
                <span class="chat-tool-kind-label">{kindCategoryLabel('think', lang)}</span>
                <span class="chat-tool-name-badge is-thinking-preview" title={preview || undefined}>
                    {headerText}
                </span>
            </div>
            {expanded && (
                <div class="chat-tool-row-body is-thinking">
                    <div class="chat-thinking-body markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
                </div>
            )}
        </div>
    );
}

/**
 * Arg keys most likely to identify what a call is doing, in priority
 * order. Used for permission-prompt previews (not the collapsed tool row).
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

function GroupedToolCallItem({ call, status }: { call: GroupedToolCall; status: CallStatus }) {
    const lang = getLang();

    // Rows start collapsed — the header already carries tool name,
    // key-arg summary and status. Pending permissions are answered via
    // the composer-adjacent prompt, so rows no longer force-expand.
    // Held in a signal (not useState) so the header click re-renders
    // reliably under @preact/signals — see ThinkingBubble.
    const isExpanded = useSignal(
        call.toolCallId && userExpandChoice.has(call.toolCallId) ? userExpandChoice.get(call.toolCallId)! : false
    );

    const toggle = () => {
        const next = !isExpanded.value;
        isExpanded.value = next;
        if (call.toolCallId) userExpandChoice.set(call.toolCallId, next);
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

    const expanded = isExpanded.value;

    // Prefer the ACP-forwarded metadata; fall back to deriving from the tool
    // name/input so diff/kind/locations all survive the post-turn history
    // reload (history carries the tool name + input, not the ACP fields).
    const diffs = call.diffs && call.diffs.length > 0 ? call.diffs : deriveDiffsFromInput(call.toolName, args);
    const derivedLocations = deriveLocationsFromInput(args);
    const locations =
        call.locations && call.locations.length > 0
            ? call.locations
            : derivedLocations.length > 0
              ? derivedLocations
              : undefined;
    const kind = call.kind ?? deriveToolKind(call.toolName);
    const category = kindCategory(kind);
    const isError = status === 'error';
    const labelCls = isError ? 'is-error' : '';

    // Terminal/execute tools render as a durable terminal block — the command
    // (from input) as a `$` prompt line + output in a dark terminal box. Both
    // sources are in history, so it survives the post-turn reload.
    const isTerminal = kind === 'execute';
    const command = isTerminal ? terminalCommandLine(args) : undefined;

    // switch_mode: show from→to inside the expanded body (header is name-only).
    const modeSwitch =
        kind === 'switch_mode'
            ? (() => {
                  const from = typeof args.fromMode === 'string' ? args.fromMode : '';
                  const to = typeof args.toMode === 'string' ? args.toMode : '';
                  if (!from && !to) return null;
                  const fromTo = t('chat.toolKind.modeFromTo', lang, { from, to });
                  return `${t('chat.toolKind.switchMode', lang)}: ${fromTo}`;
              })()
            : null;

    return (
        <div class={`chat-tool-row ${expanded ? 'is-expanded' : 'is-collapsed'} status-${status}`}>
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
                <span class="chat-tool-row-caret" aria-hidden="true">
                    {expanded ? '▾' : '▸'}
                </span>
                <span class={`chat-tool-kind-label ${labelCls}`}>{kindCategoryLabel(category, lang)}</span>
                <span class={`chat-tool-name-badge ${labelCls}`}>{call.toolName}</span>
            </div>
            {expanded && (
                <div class="chat-tool-row-body">
                    {modeSwitch && <div class="chat-tool-muted">{modeSwitch}</div>}
                    {/* Terminal command line (execute tools) */}
                    {command && (
                        <div class="chat-tool-cmd-box">
                            <span class="chat-tool-cmd-prompt" aria-hidden="true">
                                $
                            </span>
                            <span class="chat-tool-cmd-text">{command}</span>
                        </div>
                    )}
                    {/* Arguments — hidden for terminal tools (command shown above). */}
                    {!isTerminal && (
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
                    )}

                    {/* File diffs (Phase 6): ACP diff blocks or derived from
                        edit-family input. When a diff exists it is the result
                        surface — raw output is omitted to avoid duplicate noise. */}
                    {diffs.length > 0 && (
                        <div class="chat-tool-section">
                            <div class="chat-tool-section-title">{t('chat.tool.diff', lang)}</div>
                            <ToolDiffView diffs={diffs} />
                        </div>
                    )}

                    {/* Files the tool touched (ACP locations). */}
                    {locations && (
                        <div class="chat-tool-section">
                            <div class="chat-tool-section-title">{t('chat.tool.locations', lang)}</div>
                            <div class="chat-tool-locations">
                                {locations.map((loc, i) => (
                                    <span key={i} class="chat-tool-location" title={loc.path}>
                                        {loc.path}
                                        {typeof loc.line === 'number' ? `:${loc.line}` : ''}
                                    </span>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* Resolved permission receipt only. Pending requests are
                        answered in the composer-adjacent PermissionPrompt. */}
                    {hasPermission && call.permission!.resolved && (
                        <div class="chat-tool-section">
                            <div class="chat-tool-section-title">{t('chat.tool.permission', lang)}</div>
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
                        </div>
                    )}

                    {/* Resolved ask_user_question receipt. Live prompts sit in
                        the composer-adjacent AskUserPrompt. */}
                    {call.askUser?.resolved && (
                        <div class="chat-tool-section">
                            <div class="chat-tool-section-title">{t('chat.askUser.title', lang)}</div>
                            <div class="chat-ask-user-resolved">
                                <span class="chat-ask-user-resolved-mark" aria-hidden="true">
                                    {call.askUser.resolved === 'accepted' ? '✓' : '·'}
                                </span>
                                <span class="chat-ask-user-resolved-text">
                                    {formatAskUserResolved(call.askUser, lang)}
                                </span>
                            </div>
                        </div>
                    )}

                    {/* Resolved exit_plan_mode receipt. Live approvals sit in
                        the composer-adjacent ExitPlanPrompt. */}
                    {call.exitPlan?.resolved && (
                        <div class="chat-tool-section">
                            <div class="chat-tool-section-title">{t('chat.exitPlan.title', lang)}</div>
                            <div class={`chat-exit-plan-resolved chat-exit-plan-${call.exitPlan.resolved}`}>
                                <span class="chat-exit-plan-resolved-mark" aria-hidden="true">
                                    {call.exitPlan.resolved === 'approved'
                                        ? '✓'
                                        : call.exitPlan.resolved === 'abandoned'
                                          ? '✕'
                                          : '↩'}
                                </span>
                                <span class="chat-exit-plan-resolved-text">
                                    {formatExitPlanResolved(call.exitPlan, lang)}
                                </span>
                            </div>
                        </div>
                    )}

                    {/* Output only when there is no diff result. */}
                    {diffs.length === 0 && (
                        <div class="chat-tool-section">
                            <div class="chat-tool-section-title">{t('chat.tool.output', lang)}</div>
                            {!hasOutput ? (
                                <div class="chat-tool-muted">
                                    {status === 'running'
                                        ? t('chat.tool.outputPending', lang)
                                        : t('chat.tool.outputMissing', lang)}
                                </div>
                            ) : call.output ? (
                                <pre
                                    class={`${isTerminal ? 'chat-tool-output-box' : 'chat-tool-pre chat-tool-output'} ${call.isError ? 'has-error' : ''}`}
                                >
                                    {formatToolOutput(call.output)}
                                </pre>
                            ) : (
                                <div class="chat-tool-muted">{t('chat.tool.outputEmpty', lang)}</div>
                            )}
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}

/**
 * Pretty-print tool output when it is JSON (object/array); otherwise return
 * the original text. Handles leading/trailing whitespace and double-encoded
 * JSON strings that some agents emit as output.
 */
function formatToolOutput(raw: string): string {
    const text = raw.trim();
    if (!text) return raw;

    const tryPretty = (s: string): string | null => {
        try {
            const parsed = JSON.parse(s);
            // Only reformat structured JSON — leave bare strings/numbers as-is
            // so a plain path or status message is not wrapped in quotes.
            if (parsed !== null && typeof parsed === 'object') {
                return JSON.stringify(parsed, null, 2);
            }
            return null;
        } catch {
            return null;
        }
    };

    const direct = tryPretty(text);
    if (direct !== null) return direct;

    // Some runtimes wrap the whole payload in an extra JSON string layer.
    if ((text.startsWith('"') && text.endsWith('"')) || (text.startsWith("'") && text.endsWith("'"))) {
        try {
            const unwrapped = JSON.parse(text);
            if (typeof unwrapped === 'string') {
                const nested = tryPretty(unwrapped.trim());
                if (nested !== null) return nested;
            }
        } catch {
            // keep original
        }
    }

    return raw;
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

export type PendingPermission = NonNullable<GroupedToolCall['permission']>;

/**
 * Action row for a single pending permission. Shared by the
 * composer-adjacent PermissionPrompt (live requests).
 */
export function PermissionActionRow({
    permission,
    onRespond,
    preview,
}: {
    permission: PendingPermission;
    onRespond?: (requestId: string, decision: PermissionDecision) => void;
    /** Optional tool-input summary shown between the title and action buttons. */
    preview?: string | null;
}) {
    const lang = getLang();
    const respond = (decision: PermissionDecision) => {
        if (onRespond) onRespond(permission.requestId, decision);
    };
    // Four buttons, ordered left → right by escalation:
    //   deny-always · deny · allow · allow-always
    // Layout: toolname → description → options
    return (
        <div class="chat-permission-inline">
            <div class="chat-permission-inline-label">
                {t('chat.permission.title', lang, { tool: permission.toolName })}
            </div>
            {preview && <div class="chat-permission-prompt-preview">{preview}</div>}
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

function permissionInputPreview(input: string): string | null {
    if (!input || !input.trim()) return null;
    try {
        const parsed = JSON.parse(input);
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
            const summary = summarizeArgs(parsed as Record<string, unknown>);
            if (summary) return summary.length > 160 ? `${summary.slice(0, 160)}…` : summary;
            const compact = JSON.stringify(parsed);
            return compact.length > 160 ? `${compact.slice(0, 160)}…` : compact;
        }
    } catch {
        // fall through to raw text
    }
    const flat = input.replace(/\s+/g, ' ').trim();
    return flat.length > 160 ? `${flat.slice(0, 160)}…` : flat;
}

/**
 * Half-panel above the composer: surfaces the current unresolved permission
 * request so the user can act without digging into a collapsed tool group.
 * Callers should pass only the head of the pending queue (one card at a time);
 * if multiple arrive, only the first is rendered. Once resolved, the receipt
 * folds into the matching tool row and the next pending prompt can appear.
 */
export function PermissionPrompt({
    permissions,
    onRespond,
}: {
    permissions: PendingPermission[];
    onRespond?: (requestId: string, decision: PermissionDecision) => void;
}) {
    const permission = permissions[0];
    if (!permission) return null;
    const preview = permissionInputPreview(permission.input);
    return (
        <div class="chat-permission-prompt" role="region" aria-label="permission">
            <div key={permission.requestId} class="chat-permission-prompt-card">
                <PermissionActionRow
                    permission={permission}
                    onRespond={onRespond}
                    preview={preview}
                />
            </div>
        </div>
    );
}

export type PendingAskUser = AskUserQuestionState;

function formatAnswerValue(value: AskUserAnswerValue): string {
    return Array.isArray(value) ? value.join(', ') : String(value);
}

/** Human-readable receipt for a resolved ask_user_question (tool card). */
function formatAskUserResolved(state: AskUserQuestionState, lang: Lang): string {
    switch (state.resolved) {
        case 'accepted': {
            const answers = state.answers ?? {};
            const parts = Object.entries(answers).map(([q, a]) => `${q}=${formatAnswerValue(a)}`);
            const summary = parts.length > 0 ? parts.join('; ') : '—';
            return t('chat.askUser.resolved.accepted', lang, { summary });
        }
        case 'skip_interview':
            return t('chat.askUser.resolved.skip', lang);
        case 'chat_about_this':
            return t('chat.askUser.resolved.chat', lang);
        case 'cancelled':
            return t('chat.askUser.resolved.cancelled', lang);
        default:
            return t('chat.askUser.resolved.cancelled', lang);
    }
}

/**
 * One questionnaire card: radio/checkbox options per question + free-text
 * Other, then Submit / Skip / Discuss / Cancel.
 *
 * Multiple questions in one request are stepped through one at a time
 * (answer → next); the wire `accepted` response still submits the full
 * answers map when the last question is confirmed.
 */
function AskUserQuestionCard({
    request,
    onRespond,
}: {
    request: PendingAskUser;
    onRespond?: (requestId: string, outcome: AskUserOutcome, answers?: Record<string, AskUserAnswerValue>) => void;
}) {
    const lang = getLang();
    const questions = request.questions;
    const [step, setStep] = useState(0);
    // Selections keyed by question text. multi → string[]; single → string.
    const [selections, setSelections] = useState<Record<string, string[]>>(() => {
        const init: Record<string, string[]> = {};
        for (const q of questions) init[q.question] = [];
        return init;
    });
    const [otherText, setOtherText] = useState<Record<string, string>>(() => {
        const init: Record<string, string> = {};
        for (const q of questions) init[q.question] = '';
        return init;
    });

    const safeStep = Math.min(step, Math.max(0, questions.length - 1));
    const q = questions[safeStep];
    const isLast = safeStep >= questions.length - 1;
    const multiQuestion = questions.length > 1;

    const toggleOption = (question: string, label: string, multi: boolean) => {
        setSelections(prev => {
            const cur = prev[question] ?? [];
            if (multi) {
                const next = cur.includes(label) ? cur.filter(x => x !== label) : [...cur, label];
                return { ...prev, [question]: next };
            }
            return { ...prev, [question]: [label] };
        });
        // Choosing a listed option clears free-text Other for that question.
        setOtherText(prev => ({ ...prev, [question]: '' }));
    };

    const setOther = (question: string, text: string) => {
        setOtherText(prev => ({ ...prev, [question]: text }));
        if (text.trim()) {
            // Free text replaces listed selections for that question.
            setSelections(prev => ({ ...prev, [question]: [] }));
        }
    };

    const answerForQuestion = (question: (typeof questions)[number]): AskUserAnswerValue | null => {
        const other = (otherText[question.question] ?? '').trim();
        const picked = selections[question.question] ?? [];
        if (other) return other;
        if (picked.length === 0) return null;
        return question.multiSelect === true ? picked : picked[0]!;
    };

    const buildAnswers = (): Record<string, AskUserAnswerValue> | null => {
        const answers: Record<string, AskUserAnswerValue> = {};
        for (const question of questions) {
            const value = answerForQuestion(question);
            if (value === null) return null;
            answers[question.question] = value;
        }
        return answers;
    };

    const submit = () => {
        const answers = buildAnswers();
        if (!answers) {
            showToast(t('chat.askUser.needAnswer', lang));
            return;
        }
        onRespond?.(request.requestId, 'accepted', answers);
    };

    const goNext = () => {
        if (!q) return;
        if (answerForQuestion(q) === null) {
            showToast(t('chat.askUser.needAnswer', lang));
            return;
        }
        if (isLast) {
            submit();
            return;
        }
        setStep(s => Math.min(s + 1, questions.length - 1));
    };

    if (!q) return null;

    const multi = q.multiSelect === true;
    const picked = selections[q.question] ?? [];
    const other = otherText[q.question] ?? '';

    return (
        <div class="chat-ask-user-card">
            <div class="chat-ask-user-card-header">
                {t('chat.askUser.title', lang)}
                {multiQuestion && (
                    <span class="chat-ask-user-step">
                        {t('chat.askUser.step', lang, {
                            current: String(safeStep + 1),
                            total: String(questions.length),
                        })}
                    </span>
                )}
            </div>
            <div class="chat-ask-user-questions">
                <div key={safeStep} class="chat-ask-user-question">
                    <div class="chat-ask-user-question-text">
                        {multiQuestion ? `${safeStep + 1}. ` : null}
                        {q.question}
                        {multi && <span class="chat-ask-user-multi-tag">{t('chat.askUser.multiSelect', lang)}</span>}
                    </div>
                    <div class="chat-ask-user-options" role={multi ? 'group' : 'radiogroup'}>
                        {q.options.map((opt, oi) => {
                            const selected = picked.includes(opt.label);
                            const recommended = /\(Recommended\)|（推荐）/i.test(opt.label);
                            return (
                                <button
                                    key={oi}
                                    type="button"
                                    class={`chat-ask-user-option${selected ? ' is-selected' : ''}${recommended ? ' is-recommended' : ''}`}
                                    onClick={() => toggleOption(q.question, opt.label, multi)}
                                    aria-pressed={selected}
                                >
                                    <span class="chat-ask-user-option-mark" aria-hidden="true">
                                        {multi ? (selected ? '☑' : '☐') : selected ? '●' : '○'}
                                    </span>
                                    <span class="chat-ask-user-option-body">
                                        <span class="chat-ask-user-option-label">{opt.label}</span>
                                        {opt.description && (
                                            <span class="chat-ask-user-option-desc">{opt.description}</span>
                                        )}
                                        {opt.preview && <pre class="chat-ask-user-option-preview">{opt.preview}</pre>}
                                    </span>
                                </button>
                            );
                        })}
                    </div>
                    <label class="chat-ask-user-other">
                        <span class="chat-ask-user-other-label">{t('chat.askUser.other', lang)}</span>
                        <input
                            type="text"
                            class="chat-ask-user-other-input"
                            value={other}
                            placeholder={t('chat.askUser.otherPlaceholder', lang)}
                            onInput={e => setOther(q.question, (e.target as HTMLInputElement).value)}
                        />
                    </label>
                </div>
            </div>
            <div class="chat-ask-user-actions">
                <button
                    type="button"
                    class="chat-ask-user-btn cancel"
                    onClick={() => onRespond?.(request.requestId, 'cancelled')}
                >
                    {t('chat.askUser.cancel', lang)}
                </button>
                <button
                    type="button"
                    class="chat-ask-user-btn skip"
                    onClick={() => onRespond?.(request.requestId, 'skip_interview')}
                    title={t('chat.askUser.skipHint', lang)}
                >
                    {t('chat.askUser.skip', lang)}
                </button>
                <button
                    type="button"
                    class="chat-ask-user-btn chat"
                    onClick={() => onRespond?.(request.requestId, 'chat_about_this')}
                    title={t('chat.askUser.chatHint', lang)}
                >
                    {t('chat.askUser.chat', lang)}
                </button>
                {multiQuestion && safeStep > 0 && (
                    <button
                        type="button"
                        class="chat-ask-user-btn back"
                        onClick={() => setStep(s => Math.max(0, s - 1))}
                    >
                        {t('chat.askUser.back', lang)}
                    </button>
                )}
                <button type="button" class="chat-ask-user-btn submit" onClick={isLast ? submit : goNext}>
                    {isLast ? t('chat.askUser.submit', lang) : t('chat.askUser.next', lang)}
                </button>
            </div>
        </div>
    );
}

/**
 * Half-panel above the composer for the current Grok ask_user_question prompt.
 * Only the first request is rendered; callers should pass a single-item queue.
 */
export function AskUserPrompt({
    requests,
    onRespond,
}: {
    requests: PendingAskUser[];
    onRespond?: (requestId: string, outcome: AskUserOutcome, answers?: Record<string, AskUserAnswerValue>) => void;
}) {
    const req = requests[0];
    if (!req) return null;
    return (
        <div class="chat-ask-user-prompt" role="region" aria-label="ask user question">
            <AskUserQuestionCard key={req.requestId} request={req} onRespond={onRespond} />
        </div>
    );
}

export type PendingExitPlan = ExitPlanModeState;

/** Human-readable receipt for a resolved exit_plan_mode (tool card). */
function formatExitPlanResolved(state: ExitPlanModeState, lang: Lang): string {
    const comments = state.comments?.trim();
    switch (state.resolved) {
        case 'approved':
            return comments
                ? t('chat.exitPlan.resolved.approvedWithComments', lang, { comments })
                : t('chat.exitPlan.resolved.approved', lang);
        case 'rejected':
            return comments
                ? t('chat.exitPlan.resolved.rejectedWithComments', lang, { comments })
                : t('chat.exitPlan.resolved.rejected', lang);
        case 'abandoned':
            return t('chat.exitPlan.resolved.abandoned', lang);
        default:
            return t('chat.exitPlan.resolved.abandoned', lang);
    }
}

/**
 * One plan-approval card: scrollable plan preview + feedback + 3 actions
 * (Approve / Request changes / Quit), mirroring tool permission escalation.
 */
function ExitPlanCard({
    request,
    onRespond,
}: {
    request: PendingExitPlan;
    onRespond?: (requestId: string, outcome: ExitPlanOutcome, comments?: string) => void;
}) {
    const lang = getLang();
    const [comments, setComments] = useState('');
    const planHtml = useMemo(() => {
        const md = request.planContent?.trim() || t('chat.exitPlan.empty', lang);
        try {
            return marked.parse(md, { async: false }) as string;
        } catch {
            return `<pre>${md.replace(/</g, '&lt;')}</pre>`;
        }
    }, [request.planContent, lang]);

    const respond = (outcome: ExitPlanOutcome) => {
        const trimmed = comments.trim();
        onRespond?.(request.requestId, outcome, trimmed || undefined);
    };

    return (
        <div class="chat-exit-plan-card">
            <div class="chat-exit-plan-card-header">{t('chat.exitPlan.title', lang)}</div>
            <div class="chat-exit-plan-preview chat-md" dangerouslySetInnerHTML={{ __html: planHtml }} />
            <label class="chat-exit-plan-feedback">
                <span class="chat-exit-plan-feedback-label">{t('chat.exitPlan.feedback', lang)}</span>
                <textarea
                    class="chat-exit-plan-feedback-input"
                    rows={2}
                    value={comments}
                    placeholder={t('chat.exitPlan.feedbackPlaceholder', lang)}
                    onInput={e => setComments((e.target as HTMLTextAreaElement).value)}
                />
            </label>
            <div class="chat-exit-plan-actions">
                <button
                    type="button"
                    class="chat-exit-plan-btn abandon"
                    onClick={() => respond('abandoned')}
                    title={t('chat.exitPlan.abandonHint', lang)}
                >
                    {t('chat.exitPlan.abandon', lang)}
                </button>
                <button
                    type="button"
                    class="chat-exit-plan-btn reject"
                    onClick={() => respond('rejected')}
                    title={t('chat.exitPlan.rejectHint', lang)}
                >
                    {t('chat.exitPlan.reject', lang)}
                </button>
                <button
                    type="button"
                    class="chat-exit-plan-btn approve"
                    onClick={() => respond('approved')}
                    title={t('chat.exitPlan.approveHint', lang)}
                >
                    {t('chat.exitPlan.approve', lang)}
                </button>
            </div>
        </div>
    );
}

/**
 * Half-panel above the composer for the current Grok exit_plan_mode approval.
 * Layout mirrors PermissionPrompt (composer-adjacent action bar). Only the
 * first request is rendered so stacked plan prompts don't pile up.
 */
export function ExitPlanPrompt({
    requests,
    onRespond,
}: {
    requests: PendingExitPlan[];
    onRespond?: (requestId: string, outcome: ExitPlanOutcome, comments?: string) => void;
}) {
    const req = requests[0];
    if (!req) return null;
    return (
        <div class="chat-exit-plan-prompt" role="region" aria-label="exit plan mode">
            <ExitPlanCard key={req.requestId} request={req} onRespond={onRespond} />
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
