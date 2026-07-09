import { h } from 'preact';
// Referenced by the compiled output of JSX fragments (<>…</>) via the
// jsxFragmentFactory compiler option, not by name in this file.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { Fragment } from 'preact';
import { useEffect, useRef, useMemo } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { marked } from 'marked';
import { t, getLang } from '../../i18n';
import type { PermissionDecision } from '../types';
import { renderMarkdown } from '../../utils/markdown';
import { renderMermaidBlocks } from '../../utils/mermaid';
import { activeProjectName } from '../../stores/taskNavStore';
import { theme } from '../../stores/uiStore';
import { ToolDiffView, deriveDiffsFromInput, deriveLocationsFromInput } from './ToolDiffView';
import { ToolKindIcon, deriveToolKind } from './ToolKindIcon';
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
    locations?: Array<{ path: string; line?: number }>;
    diffs?: Array<{ path: string; oldText?: string; newText: string }>;
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
    | { kind: 'assistant_text'; id: string; content: string }
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
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
    onCancelQueued?: (queueRequestId: string) => void;
}

export function MessageBubble({ item, isLast, active, onRespondPermission, onCancelQueued }: MessageBubbleProps) {
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
            <div class={`chat-bubble-body${isQueued ? ' chat-bubble-body-queued' : ''}`}>{content}</div>
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

function AssistantBubble({ content, streaming }: { content: string; streaming: boolean }) {
    // Render through the shared renderer so GitHub-style task references
    // (#N, `项目名#N`) autolink. Numbers are optimistic in chat (the task list
    // isn't loaded here); a dead reference falls back to a not-found toast.
    const html = renderMarkdown(content, { projectName: activeProjectName() });

    // Draw any ```mermaid blocks once the text is in the DOM. Reading the theme
    // signal here subscribes this bubble, so a light/dark toggle re-renders the
    // diagram. We hold off while streaming — a partially-arrived diagram is a
    // parse error, so the raw-source fallback shows until the turn completes.
    const bodyRef = useRef<HTMLDivElement>(null);
    const currentTheme = theme.value;
    useEffect(() => {
        if (streaming) return;
        renderMermaidBlocks(bodyRef.current, currentTheme);
    }, [html, streaming, currentTheme]);

    return (
        <div class="chat-message-row chat-message-row-assistant">
            <div class="chat-bubble chat-bubble-assistant">
                <div class="chat-bubble-body">
                    <div ref={bodyRef} class="markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
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

function callStatus(call: GroupedToolCall, active: boolean): CallStatus {
    if (call.permission && !call.permission.resolved) return 'waiting';
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
    onRespondPermission,
}: {
    calls: GroupedToolCall[];
    thinkingBlocks?: string[];
    elements?: ToolGroupElement[];
    pending?: boolean;
    active?: boolean;
    onRespondPermission?: (requestId: string, decision: PermissionDecision) => void;
}) {
    const key = groupKey(calls);
    // Expansion lives in a signal, not useState — see ThinkingBubble: the
    // app re-renders through @preact/signals, and a plain useState setter
    // in these chat bubbles fired its updater but didn't re-render, so the
    // header click did nothing. Reading `.value` subscribes this component.
    const isExpanded = useSignal(key && groupCollapseChoice.has(key) ? groupCollapseChoice.get(key)! : !!active);
    const lang = getLang();

    const statuses = calls.map(c => callStatus(c, !!active));
    const runningCount = statuses.filter(s => s === 'running').length;
    const errorCount = statuses.filter(s => s === 'error').length;
    const hasWaiting = statuses.includes('waiting');

    // A pending permission must never be hidden behind a collapsed
    // group — the turn is blocked on the user's decision. Force-expand
    // only on the transition INTO the waiting state. This is a transient
    // override, so it doesn't touch groupCollapseChoice: once the
    // permission resolves the group falls back to the user's choice.
    const prevWaiting = useRef(hasWaiting);
    useEffect(() => {
        if (prevWaiting.current === hasWaiting) return;
        prevWaiting.current = hasWaiting;
        if (hasWaiting) isExpanded.value = true;
    }, [hasWaiting]);

    const prevActive = useRef(!!active);
    useEffect(() => {
        if (prevActive.current === !!active) return;
        prevActive.current = !!active;
        if (active) {
            isExpanded.value = true;
            return;
        }
        if (!hasWaiting) {
            isExpanded.value = false;
            if (key) groupCollapseChoice.set(key, false);
        }
    }, [active, hasWaiting, key]);

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
                        } else if (el.kind === 'assistant_text') {
                            return <GroupedAssistantTextItem key={el.id || idx} content={el.content} />;
                        } else {
                            const callIdx = calls.indexOf(el.call);
                            return (
                                <GroupedToolCallItem
                                    key={el.call.id || idx}
                                    call={el.call}
                                    status={statuses[callIdx]}
                                    onRespondPermission={onRespondPermission}
                                />
                            );
                        }
                    })}
                </div>
            )}
        </div>
    );
}

function GroupedAssistantTextItem({ content }: { content: string }) {
    const html = renderMarkdown(content, { projectName: activeProjectName() });

    return (
        <div class="chat-tool-row">
            <div class="chat-tool-row-body">
                <div class="markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
            </div>
        </div>
    );
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
            preview: previewText.length > 60 ? `${previewText.slice(0, 60)}…` : previewText,
            html: marked.parse(content, { async: false }) as string,
        };
    }, [content]);

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
                {streaming ? (
                    <span class="chat-tool-status-icon chat-tool-spinner" aria-hidden="true" />
                ) : (
                    <span class="chat-tool-status-icon is-thinking" aria-hidden="true">
                        ·
                    </span>
                )}
                <span class="chat-tool-name-badge is-thinking">
                    {streaming ? t('chat.thinking.streaming', lang) : t('chat.thinking.label', lang)}
                </span>
                {!expanded && preview && <span class="chat-tool-row-summary is-thinking-preview">{preview}</span>}
                <span class="chat-tool-row-caret" aria-hidden="true">
                    {expanded ? '▾' : '▸'}
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
    // Held in a signal (not useState) so the header click re-renders
    // reliably under @preact/signals — see ThinkingBubble.
    const isExpanded = useSignal(
        hasPendingPermission
            ? true
            : call.toolCallId && userExpandChoice.has(call.toolCallId)
              ? userExpandChoice.get(call.toolCallId)!
              : false
    );

    // Sync only on the waiting transition: force-open when a permission
    // arrives, and on resolution fall back to the user's remembered choice
    // (or collapsed). Gating on the transition avoids a mount-time write.
    const prevPending = useRef(hasPendingPermission);
    useEffect(() => {
        if (prevPending.current === hasPendingPermission) return;
        prevPending.current = hasPendingPermission;
        if (hasPendingPermission) {
            isExpanded.value = true;
            return;
        }
        if (call.toolCallId && userExpandChoice.has(call.toolCallId)) {
            isExpanded.value = userExpandChoice.get(call.toolCallId)!;
            return;
        }
        isExpanded.value = false;
    }, [hasPendingPermission]);

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

    const summary = Object.keys(args).length > 0 ? summarizeArgs(args) : undefined;
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

    // Terminal/execute tools render as a durable terminal block — the command
    // (from input) as a `$` prompt line + output in a dark terminal box. Both
    // sources are in history, so it survives the post-turn reload.
    const isTerminal = kind === 'execute';
    const command = isTerminal ? terminalCommandLine(args) : undefined;

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
                <StatusIcon status={status} />
                <ToolKindIcon kind={kind} />
                <span class="chat-tool-name-badge">{call.toolName}</span>
                {summary && <span class="chat-tool-row-summary">{summary}</span>}
                {status === 'waiting' && (
                    <span class="chat-tool-row-status is-waiting">{t('chat.tool.status.waiting', lang)}</span>
                )}
                {status === 'running' && (
                    <span class="chat-tool-row-status is-running">{t('chat.tool.status.running', lang)}</span>
                )}
                <span class="chat-tool-row-caret" aria-hidden="true">
                    {expanded ? '▾' : '▸'}
                </span>
            </div>
            {expanded && (
                <div class="chat-tool-row-body">
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
                        edit-family input. */}
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
                            <pre
                                class={`${isTerminal ? 'chat-tool-output-box' : 'chat-tool-pre chat-tool-output'} ${call.isError ? 'has-error' : ''}`}
                            >
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
