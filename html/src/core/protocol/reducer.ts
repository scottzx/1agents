// Platform-agnostic chat-protocol pure reducers.
//
// Side-effect-free folds the bridge applies to conversation state — no `this`,
// no WebSocket, no preact, no DOM. Moved verbatim out of
// components/chat/hooks.ts (Phase 0 carve) so non-DOM hosts (Tauri Mobile, 小程序)
// reuse the exact same conversion + folding logic; the host layer keeps only the
// transport + notify orchestration.

import type { ChatItem, HistoryItem, ToolCallInfo } from './types';
import type { ChatStatus } from './session';

/**
 * The slice of session state the folds read and write. The host's full
 * SessionBridgeState (which also carries ws / listeners / reconnect bookkeeping)
 * structurally satisfies this, so it can be passed straight through.
 */
export interface PendingState {
    items: ChatItem[];
    /**
     * Real-time-only holding pen for tool_result / permission_request events that
     * arrived before (or without) their matching tool_call. Folded in by the next
     * tool_call via tryAssignPending; leftovers surface as a "待分配" group.
     */
    pendingResults: ChatItem[];
    pendingPermissions: ChatItem[];
}

export function cryptoId(): string {
    if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
        return (crypto as Crypto).randomUUID();
    }
    return `id-${Math.random().toString(36).slice(2)}-${Date.now()}`;
}

// Treat a tool_call event's `arguments` as "renderable" only when it
// carries real data the card can show. The backend's SSE safety fallback
// either omits `arguments` entirely (rawInput wasn't streamed yet) or
// sends `arguments: {}` (the runtime's no-input placeholder); neither
// should drive a card into the "no args" empty state, so drop them at
// the source. Primitive empties ("" / 0 / false) are also dropped —
// they would render as "无附加调用参数" just like a true empty object.
export function hasRenderableArguments(args: unknown): boolean {
    if (args === undefined || args === null) return false;
    if (typeof args === 'string') return args.length > 0;
    if (typeof args === 'number') return Number.isFinite(args);
    if (typeof args === 'boolean') return true;
    if (Array.isArray(args)) return args.length > 0;
    if (typeof args === 'object') {
        return Object.keys(args as Record<string, unknown>).length > 0;
    }
    return true;
}

export function parseCreatedAt(value: string | undefined): number {
    if (!value) return Date.now();
    const ts = new Date(value).getTime();
    return Number.isFinite(ts) ? ts : Date.now();
}

// History ids are derived from the item's position (and toolCallId for
// tool_use) instead of cryptoId(). Each `done` triggers a history
// reload that rebuilds the whole list; random ids changed every React
// key on every reload, remounting every bubble (visible flicker, all
// expand/collapse state lost). Positional ids are stable between
// consecutive reloads of the same on-disk record.
export function historyItemToChatItem(it: HistoryItem, index: number): ChatItem {
    const createdAt = parseCreatedAt(it.createdAt);
    switch (it.kind) {
        case 'user':
            return { id: `h-${index}`, kind: 'user', content: it.text, createdAt };
        case 'assistant_text':
            return {
                id: `h-${index}`,
                kind: 'assistant_text',
                content: it.text,
                createdAt,
                streaming: false,
            };
        case 'thinking':
            return { id: `h-${index}`, kind: 'thinking', content: it.text, createdAt };
        case 'tool_use': {
            const inputJson = typeof it.input === 'string' ? it.input : JSON.stringify(it.input ?? {}, null, 2);
            const call: ToolCallInfo = {
                toolName: it.toolName || 'tool',
                input: inputJson,
            };
            if (it.toolCallId) call.toolCallId = it.toolCallId;
            return {
                id: `h-tool-${it.toolCallId || index}`,
                kind: 'tool_use',
                toolName: call.toolName,
                input: call.input,
                calls: [call],
                createdAt,
                ...(call.toolCallId ? { toolCallId: call.toolCallId } : {}),
            };
        }
        case 'tool_result':
            return {
                id: `h-${index}`,
                kind: 'tool_result',
                content: it.content,
                isError: !!it.isError,
                createdAt,
                ...(it.toolCallId ? { toolCallId: it.toolCallId } : {}),
            };
    }
}

/**
 * Normalize a `history_response` payload into ChatItems. Prefers the structured
 * `items` array; falls back to the legacy `{role,text}[]` messages shape.
 */
export function normalizeHistory(
    items: HistoryItem[] | undefined,
    messages: Array<{ role: string; text: string }> | undefined
): ChatItem[] {
    const historyItems: HistoryItem[] =
        items && items.length > 0
            ? items
            : (messages || []).map(m => ({
                  kind: (m.role === 'user' ? 'user' : 'assistant_text') as 'user' | 'assistant_text',
                  text: m.text,
              }));
    return historyItems.map((it, idx) => historyItemToChatItem(it, idx));
}

// Flush the streaming cursor on the trailing assistant_text block. Called
// whenever a non-text_delta event arrives, so the blink stops at the boundary
// between text and whatever comes next (tool, permission, ...). Returns the same
// reference when there is nothing to flush.
export function flushStreamingCursor(items: ChatItem[]): ChatItem[] {
    if (items.length === 0) return items;
    const last = items[items.length - 1];
    if (last && last.kind === 'assistant_text' && last.streaming) {
        return [...items.slice(0, -1), { ...last, streaming: false }];
    }
    return items;
}

/**
 * Fold a `text_delta` into the items stream. `type === 'thought'` appends to /
 * extends a thinking block; otherwise it extends the trailing streaming
 * assistant_text, or starts a new one — clearing the queue badge off the oldest
 * still-queued user bubble (the bridge drains its promptQueue FIFO).
 */
export function applyTextDelta(items: ChatItem[], delta: string, type: string): ChatItem[] {
    const next = [...items];
    const last = next[next.length - 1];
    if (type === 'thought') {
        if (last && last.kind === 'thinking') {
            next[next.length - 1] = {
                ...last,
                content: last.content + delta,
            };
        } else {
            next.push({
                id: cryptoId(),
                kind: 'thinking',
                content: delta,
                createdAt: Date.now(),
            });
        }
    } else {
        if (last && last.kind === 'assistant_text' && last.streaming) {
            next[next.length - 1] = {
                ...last,
                content: last.content + delta,
                streaming: true,
            };
        } else {
            // First text_delta for a freshly dequeued turn. Clear the queue badge
            // from the oldest still-queued user bubble — the bridge drains its
            // promptQueue FIFO, so the first remaining queued bubble just started.
            for (let i = 0; i < next.length; i++) {
                const it = next[i];
                if (it.kind === 'user' && it.queueStatus === 'queued') {
                    next[i] = { ...it, queueStatus: undefined };
                    break;
                }
            }
            next.push({
                id: cryptoId(),
                kind: 'assistant_text',
                content: delta,
                createdAt: Date.now(),
                streaming: true,
            });
        }
    }
    return next;
}

/**
 * Mark the most-recent not-yet-queued user bubble as queued. Returns the same
 * reference when nothing changed (the latest user bubble is already queued, or
 * there is no user bubble), so the caller can skip a repaint.
 */
export function applyPromptQueued(items: ChatItem[], requestId: string | undefined): ChatItem[] {
    for (let i = items.length - 1; i >= 0; i--) {
        const it = items[i];
        if (it.kind !== 'user') continue;
        if (it.queueStatus === 'queued') break;
        return [
            ...items.slice(0, i),
            { ...it, queueStatus: 'queued', queueRequestId: requestId },
            ...items.slice(i + 1),
        ];
    }
    return items;
}

/**
 * Clear the queue badge from every still-queued user bubble. Returns `mutated`
 * so the caller repaints only when something actually changed.
 */
export function applyPromptCancelled(items: ChatItem[]): { items: ChatItem[]; mutated: boolean } {
    let mutated = false;
    const next = items.map(it => {
        if (it.kind === 'user' && it.queueStatus === 'queued') {
            mutated = true;
            return { ...it, queueStatus: undefined };
        }
        return it;
    });
    return mutated ? { items: next, mutated } : { items, mutated };
}

/**
 * Walk the pending result/permission pools and fold any entries that now match
 * an existing tool_use in `items` straight into the call. Leftover entries that
 * still don't have a matching call stay in the pool for the renderer to surface
 * as a "待分配" tool_group. Returns the same reference when both pools are empty.
 */
export function tryAssignPending(s: PendingState): PendingState {
    if (s.pendingResults.length === 0 && s.pendingPermissions.length === 0) return s;
    let items = s.items;
    const nextResults: ChatItem[] = [];
    for (const p of s.pendingResults) {
        if (p.kind !== 'tool_result') {
            nextResults.push(p);
            continue;
        }
        let matched = false;
        for (let i = items.length - 1; i >= 0; i--) {
            const it = items[i];
            if (it.kind !== 'tool_use') continue;
            const callIdx = p.toolCallId
                ? it.calls.findIndex(c => c.toolCallId === p.toolCallId)
                : it.calls.findIndex(c => c.output === undefined);
            if (callIdx < 0) continue;
            // Build the replacement from `it` (already narrowed to tool_use) — the
            // map callback's `entry` is the full union and TS can't see that
            // idx === i implies tool_use.
            const updated = {
                ...it,
                calls: it.calls.map((c, k) => (k !== callIdx ? c : { ...c, output: p.content, isError: p.isError })),
            };
            items = items.map((entry, idx) => (idx === i ? updated : entry));
            matched = true;
            break;
        }
        if (!matched) nextResults.push(p);
    }
    const nextPermissions: ChatItem[] = [];
    for (const p of s.pendingPermissions) {
        if (p.kind !== 'permission_request') {
            nextPermissions.push(p);
            continue;
        }
        let matched = false;
        for (let i = items.length - 1; i >= 0; i--) {
            const it = items[i];
            if (it.kind !== 'tool_use') continue;
            const callIdx = p.toolCallId ? it.calls.findIndex(c => c.toolCallId === p.toolCallId) : -1;
            if (callIdx < 0) continue;
            const newPermission = {
                requestId: p.requestId,
                toolName: p.toolName,
                input: p.input,
                options: p.options,
                ...(p.resolved ? { resolved: p.resolved } : {}),
            };
            const updated = {
                ...it,
                calls: it.calls.map((c, k) => (k !== callIdx ? c : { ...c, permission: newPermission })),
            };
            items = items.map((entry, idx) => (idx === i ? updated : entry));
            matched = true;
            break;
        }
        if (!matched) nextPermissions.push(p);
    }
    return { items, pendingResults: nextResults, pendingPermissions: nextPermissions };
}

/**
 * Fold a `tool_call` event: flush the streaming cursor, then update the trailing
 * tool_use in place (same toolCallId) or append a new call / tool_use block, and
 * re-scan the pending pools so out-of-order results/permissions reconcile.
 */
export function applyToolCall(
    s: PendingState,
    ev: { arguments?: unknown; toolName?: string; toolCallId?: string }
): PendingState {
    const items0 = flushStreamingCursor(s.items);
    const argsString = typeof ev.arguments === 'string' ? ev.arguments : JSON.stringify(ev.arguments, null, 2);
    const newCall: ToolCallInfo = {
        toolName: ev.toolName || 'tool',
        input: argsString,
    };
    if (ev.toolCallId) {
        newCall.toolCallId = ev.toolCallId;
    }
    const next = [...items0];
    const last = next[next.length - 1];
    if (last && last.kind === 'tool_use') {
        // Multiple tool_call events for the same toolCallId may arrive as more
        // data streams in. Update the existing call in place rather than
        // appending a duplicate so tool_result lands on the right call.
        const existingIdx = newCall.toolCallId ? last.calls.findIndex(c => c.toolCallId === newCall.toolCallId) : -1;
        if (existingIdx >= 0) {
            next[next.length - 1] = {
                ...last,
                calls: last.calls.map((c, idx) =>
                    idx === existingIdx
                        ? {
                              ...c,
                              toolName: newCall.toolName,
                              input: newCall.input,
                          }
                        : c
                ),
            };
        } else {
            next[next.length - 1] = {
                ...last,
                calls: [...last.calls, newCall],
            };
        }
    } else {
        next.push({
            id: cryptoId(),
            kind: 'tool_use',
            toolName: newCall.toolName,
            input: newCall.input,
            calls: [newCall],
            createdAt: Date.now(),
            ...(newCall.toolCallId ? { toolCallId: newCall.toolCallId } : {}),
        });
    }
    return tryAssignPending({
        items: next,
        pendingResults: s.pendingResults,
        pendingPermissions: s.pendingPermissions,
    });
}

/**
 * Fold a `tool_result` event: flush the streaming cursor, then attach the output
 * to the matching tool_use/call (reverse scan). If no tool_use exists yet, park
 * the result in pendingResults for a later tool_call to reconcile.
 */
export function applyToolResult(
    s: PendingState,
    ev: { text?: string; isError?: boolean; toolCallId?: string; toolName?: string }
): PendingState {
    const items = [...flushStreamingCursor(s.items)];
    let matched = false;
    for (let i = items.length - 1; i >= 0; i--) {
        const item = items[i];
        if (item.kind !== 'tool_use') continue;
        if (ev.toolCallId) {
            const callIdx = item.calls.findIndex(c => c.toolCallId === ev.toolCallId);
            if (callIdx >= 0) {
                items[i] = {
                    ...item,
                    calls: item.calls.map((c, idx) =>
                        idx === callIdx
                            ? {
                                  ...c,
                                  output: ev.text || '',
                                  isError: !!ev.isError,
                              }
                            : c
                    ),
                };
                matched = true;
                break;
            }
        }
        // No toolCallId: attach to the most recent call in the latest tool_use
        // that doesn't have output yet.
        const openCallIdx = item.calls.findIndex(c => c.output === undefined);
        const targetIdx = openCallIdx >= 0 ? openCallIdx : item.calls.length - 1;
        if (targetIdx >= 0) {
            items[i] = {
                ...item,
                calls: item.calls.map((c, idx) =>
                    idx === targetIdx
                        ? {
                              ...c,
                              output: ev.text || '',
                              isError: !!ev.isError,
                          }
                        : c
                ),
            };
            matched = true;
            break;
        }
    }
    if (!matched) {
        return {
            items,
            pendingResults: [
                ...s.pendingResults,
                {
                    id: cryptoId(),
                    kind: 'tool_result',
                    toolCallId: ev.toolCallId,
                    toolName: ev.toolName,
                    content: ev.text || '',
                    isError: !!ev.isError,
                    createdAt: Date.now(),
                },
            ],
            pendingPermissions: s.pendingPermissions,
        };
    }
    return { items, pendingResults: s.pendingResults, pendingPermissions: s.pendingPermissions };
}

/**
 * Fold a `permission_request` event: flush the streaming cursor, then nest the
 * permission inside the matching tool_use/call. If no tool_use exists yet, park
 * it in pendingPermissions for a later tool_call to reconcile.
 */
export function applyPermissionRequest(
    s: PendingState,
    ev: { arguments?: unknown; requestId?: string; toolName?: string; toolCallId?: string }
): PendingState {
    const argsString = typeof ev.arguments === 'string' ? ev.arguments : JSON.stringify(ev.arguments, null, 2);
    const requestId = ev.requestId || '';
    const toolCallId = ev.toolCallId;
    const toolName = ev.toolName || 'tool';
    const newPermission = {
        requestId,
        toolName,
        input: argsString,
        options: [] as Array<{ text: string; data: string }>,
    };
    const items = [...flushStreamingCursor(s.items)];
    let matched = false;
    if (toolCallId) {
        for (let i = items.length - 1; i >= 0; i--) {
            const item = items[i];
            if (item.kind !== 'tool_use') continue;
            const callIdx = item.calls.findIndex(c => c.toolCallId === toolCallId);
            if (callIdx >= 0) {
                items[i] = {
                    ...item,
                    calls: item.calls.map((c, idx) => (idx === callIdx ? { ...c, permission: newPermission } : c)),
                };
                matched = true;
                break;
            }
        }
    }
    if (!matched) {
        return {
            items,
            pendingResults: s.pendingResults,
            pendingPermissions: [
                ...s.pendingPermissions,
                {
                    id: cryptoId(),
                    kind: 'permission_request',
                    toolCallId,
                    requestId,
                    toolName,
                    input: argsString,
                    options: [],
                    createdAt: Date.now(),
                },
            ],
        };
    }
    return { items, pendingResults: s.pendingResults, pendingPermissions: s.pendingPermissions };
}

/**
 * Fold a `permission_timeout` event: flush the streaming cursor, mark the nested
 * permission as denied (collapses the inline UI), then append a timeout error.
 */
export function applyPermissionTimeout(
    items: ChatItem[],
    requestId: string | undefined,
    message: string | undefined
): ChatItem[] {
    const marked = flushStreamingCursor(items).map(it => {
        if (it.kind !== 'tool_use') return it;
        let touched = false;
        const calls = it.calls.map(c => {
            if (c.permission && c.permission.requestId === requestId) {
                touched = true;
                return { ...c, permission: { ...c.permission, resolved: 'deny' as const } };
            }
            return c;
        });
        return touched ? { ...it, calls } : it;
    });
    return [
        ...marked,
        {
            id: cryptoId(),
            kind: 'error',
            content: message || 'Permission request timed out.',
            createdAt: Date.now(),
        },
    ];
}

/** Flush the streaming cursor at end-of-turn (`done`). */
export function applyDone(items: ChatItem[]): ChatItem[] {
    return flushStreamingCursor(items);
}

/** Flush the streaming cursor, then append an error bubble. */
export function appendError(items: ChatItem[], message: string): ChatItem[] {
    return [
        ...flushStreamingCursor(items),
        {
            id: cryptoId(),
            kind: 'error',
            content: message,
            createdAt: Date.now(),
        },
    ];
}

// Resolve a permission decision to the collapsed allow/deny side, or null when
// the decision leaves the inline UI interactive (cancel).
export function resolvePermissionSide(
    decision: 'allow_once' | 'allow_always' | 'reject_once' | 'reject_always' | 'cancel'
): 'allow' | 'deny' | null {
    if (decision === 'allow_once' || decision === 'allow_always') return 'allow';
    if (decision === 'reject_once' || decision === 'reject_always') return 'deny';
    return null;
}

/**
 * Map a session's transient state to the sidebar status dot. A pending
 * permission (blocked on the user) outranks streaming (a turn in flight); when
 * neither applies, returns null so the sidebar falls back to the persisted
 * status (idle / completed / error).
 */
export function deriveLiveStatus(s: {
    items: ChatItem[];
    pendingPermissions: ChatItem[];
    typing: boolean;
}): ChatStatus | null {
    const hasPendingPermission =
        s.pendingPermissions.some(p => p.kind === 'permission_request' && !p.resolved) ||
        s.items.some(it => it.kind === 'tool_use' && it.calls.some(c => c.permission && !c.permission.resolved));
    if (hasPendingPermission) return 'awaiting_permission';
    if (s.typing) return 'streaming';
    return null;
}
