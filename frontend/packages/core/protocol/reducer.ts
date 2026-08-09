// Platform-agnostic chat-protocol pure reducers.
//
// Side-effect-free folds the bridge applies to conversation state — no `this`,
// no WebSocket, no preact, no DOM. Moved verbatim out of
// components/chat/hooks.ts (Phase 0 carve) so non-DOM hosts (Tauri Mobile, 小程序)
// reuse the exact same conversion + folding logic; the host layer keeps only the
// transport + notify orchestration.

import type {
    AskUserAnswerValue,
    AskUserOption,
    AskUserOutcome,
    AskUserQuestionItem,
    AskUserQuestionState,
    ChatItem,
    ExitPlanModeState,
    ExitPlanOutcome,
    TurnAwareHistoryItem,
    ToolCallInfo,
    ToolCallStatus,
} from './types';
import type { ChatStatus } from './session';

/** Normalize agent/runtime status strings onto the ACP ToolCallStatus set. */
export function normalizeToolCallStatus(raw: string | undefined | null): ToolCallStatus | undefined {
    if (!raw) return undefined;
    switch (raw) {
        case 'pending':
        case 'in_progress':
        case 'completed':
        case 'failed':
            return raw;
        // Pre-ACP / legacy aliases some bridges still emit.
        case 'success':
            return 'completed';
        case 'error':
            return 'failed';
        case 'running':
            return 'in_progress';
        default:
            return undefined;
    }
}

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
export function historyItemToChatItem(it: TurnAwareHistoryItem, index: number): ChatItem {
    const createdAt = parseCreatedAt(it.createdAt);
    const turn = it.turnId ? { turnId: it.turnId } : {};
    switch (it.kind) {
        case 'user':
            return { id: `h-${index}`, kind: 'user', content: it.text, createdAt, ...turn };
        case 'assistant_text':
            return {
                id: `h-${index}`,
                kind: 'assistant_text',
                content: it.text,
                createdAt,
                streaming: false,
                ...turn,
            };
        case 'thinking':
            return { id: `h-${index}`, kind: 'thinking', content: it.text, createdAt, ...turn };
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
                ...turn,
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
                ...turn,
            };
    }
}

/**
 * Normalize a `history_response` payload into ChatItems. Prefers the structured
 * `items` array; falls back to the legacy `{role,text}[]` messages shape.
 */
export function normalizeHistory(
    items: TurnAwareHistoryItem[] | undefined,
    messages: Array<{ role: string; text: string }> | undefined
): ChatItem[] {
    const historyItems: TurnAwareHistoryItem[] =
        items && items.length > 0
            ? items
            : (messages || []).map(m => ({
                  kind: (m.role === 'user' ? 'user' : 'assistant_text') as 'user' | 'assistant_text',
                  text: m.text,
              }));
    return historyItems.map((it, idx) => historyItemToChatItem(it, idx));
}

/**
 * Among live items, keep only the user bubbles that represent turns still in
 * flight (queued/running) and NOT yet present in the persisted history — those
 * are optimistic prompts that must survive a reconnect mid-turn.
 *
 * History items carry the RUNTIME request id as their turnId (the bridge's
 * `turn_results` key), while live bubbles carry the canonical Turn id — so a
 * bubble counts as persisted when EITHER id appears in `persistedTurnIds`.
 * Without the clientRequestId check, an interrupted turn (e.g. a backend/1ACP
 * restart) keeps a duplicate `running` user bubble appended at the end of the
 * timeline on every history reload, dragging that turn's receipts down with it.
 */
export function selectOptimisticUsers(items: ChatItem[], persistedTurnIds: ReadonlySet<string>): ChatItem[] {
    return items.filter(
        item =>
            item.kind === 'user' &&
            !!item.turnId &&
            (item.turnStatus === 'queued' || item.turnStatus === 'running') &&
            !persistedTurnIds.has(item.turnId) &&
            !(item.clientRequestId && persistedTurnIds.has(item.clientRequestId))
    );
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
    if (last && last.kind === 'subagent_turn' && last.streaming) {
        return [...items.slice(0, -1), { ...last, streaming: false }];
    }
    return items;
}

type SubagentTurnItem = Extract<ChatItem, { kind: 'subagent_turn' }>;

/** Reverse-scan for the subagent card of a given agent turn (grok promptId). */
function findSubagentTurn(
    items: ChatItem[],
    agentTurnId: string
): { index: number; item: SubagentTurnItem } | undefined {
    for (let i = items.length - 1; i >= 0; i--) {
        const it = items[i];
        if (it.kind === 'subagent_turn' && it.agentTurnId === agentTurnId) {
            return { index: i, item: it };
        }
    }
    return undefined;
}

/** Fresh subagent card; `thinking`/`output` are filled in by the caller. */
function createSubagentTurn(agentTurnId: string): SubagentTurnItem {
    return {
        id: cryptoId(),
        kind: 'subagent_turn',
        agentTurnId,
        label: 'subagent',
        thinking: '',
        output: '',
        calls: [],
        streaming: true,
        createdAt: Date.now(),
    };
}

/** Fold one tool call into a call list (same merge rules as tool_use.calls). */
function foldToolCall(calls: ToolCallInfo[], newCall: ToolCallInfo): ToolCallInfo[] {
    if (newCall.toolCallId) {
        const idx = calls.findIndex(c => c.toolCallId === newCall.toolCallId);
        if (idx >= 0) {
            return calls.map((c, i) =>
                i === idx
                    ? {
                          ...c,
                          toolName: newCall.toolName,
                          // Preserve prior input when this event carried none.
                          input: newCall.input || c.input,
                          ...(newCall.kind ? { kind: newCall.kind } : {}),
                          ...(newCall.status ? { status: newCall.status } : {}),
                          ...(newCall.locations ? { locations: newCall.locations } : {}),
                          ...(newCall.diffs ? { diffs: newCall.diffs } : {}),
                      }
                    : c
            );
        }
    }
    return [...calls, newCall];
}

/**
 * Fold a `text_delta` into the items stream. `type === 'thought'` appends to /
 * extends a thinking block; otherwise it extends the trailing streaming
 * assistant_text, or starts a new one — clearing the queue badge off the oldest
 * still-queued user bubble (the bridge drains its promptQueue FIFO).
 *
 * When `isSubagent` is set the chunk belongs to a spawned subagent turn
 * (grok stamps `_meta.promptId` on each ACP prompt; subagent turns differ).
 * Subagent text never touches the main thinking/assistant blocks — it folds
 * into a dedicated `subagent_turn` card keyed by `agentTurnId`.
 */
export function applyTextDelta(
    items: ChatItem[],
    delta: string,
    type: string,
    turnId?: string,
    agentTurnId?: string,
    isSubagent?: boolean
): ChatItem[] {
    const next = [...items];
    if (isSubagent && agentTurnId) {
        const existing = findSubagentTurn(next, agentTurnId);
        if (existing) {
            next[existing.index] = {
                ...existing.item,
                thinking: type === 'thought' ? existing.item.thinking + delta : existing.item.thinking,
                output: type !== 'thought' ? existing.item.output + delta : existing.item.output,
                streaming: true,
            };
        } else {
            next.push({
                ...createSubagentTurn(agentTurnId),
                thinking: type === 'thought' ? delta : '',
                output: type !== 'thought' ? delta : '',
            });
        }
        return next;
    }
    const last = next[next.length - 1];
    if (type === 'thought') {
        if (last && last.kind === 'thinking' && (!turnId || last.turnId === turnId)) {
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
                ...(turnId ? { turnId } : {}),
            });
        }
    } else {
        if (last && last.kind === 'assistant_text' && last.streaming && (!turnId || last.turnId === turnId)) {
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
                if (it.kind === 'user' && it.queueStatus === 'queued' && (!turnId || it.turnId === turnId)) {
                    next[i] = { ...it, queueStatus: undefined, turnStatus: 'running' };
                    break;
                }
            }
            next.push({
                id: cryptoId(),
                kind: 'assistant_text',
                content: delta,
                createdAt: Date.now(),
                streaming: true,
                ...(turnId ? { turnId } : {}),
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

export interface RealtimeTurnState {
    id?: string;
    turnId?: string;
    clientRequestId?: string;
    requestId?: string;
    status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
    promptText?: string;
    queuePosition?: number;
}

export function applyTurnState(items: ChatItem[], state: RealtimeTurnState): ChatItem[] {
    const turnId = state.turnId ?? state.id;
    const clientRequestId = state.clientRequestId ?? state.requestId;
    if (!turnId) return items;

    let matched = false;
    let next = items.map(item => {
        const isOptimisticUser = item.kind === 'user' && !!clientRequestId && item.clientRequestId === clientRequestId;
        const belongsToTurn = item.turnId === turnId;
        if (!isOptimisticUser && !belongsToTurn) return item;
        matched = true;
        return {
            ...item,
            turnId,
            turnStatus: state.status,
            ...(item.kind === 'user'
                ? {
                      queueStatus: state.status === 'queued' ? ('queued' as const) : undefined,
                      queueRequestId: state.status === 'queued' ? turnId : undefined,
                      queuePosition: state.queuePosition,
                  }
                : {}),
        };
    });
    if (!matched && state.promptText) {
        next = [
            ...next,
            {
                id: `turn-${turnId}`,
                kind: 'user',
                content: state.promptText,
                createdAt: Date.now(),
                clientRequestId,
                turnId,
                turnStatus: state.status,
                queueStatus: state.status === 'queued' ? ('queued' as const) : undefined,
                queueRequestId: state.status === 'queued' ? turnId : undefined,
                queuePosition: state.queuePosition,
            },
        ];
    }
    return next;
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
                calls: it.calls.map((c, k) =>
                    k !== callIdx
                        ? c
                        : {
                              ...c,
                              output: p.content,
                              isError: p.isError,
                              status: (p.isError ? 'failed' : 'completed') as ToolCallStatus,
                          }
                ),
            };
            items = items.map((entry, idx) => (idx === i ? updated : entry));
            matched = true;
            break;
        }
        if (!matched) nextResults.push(p);
    }
    const nextPermissions: ChatItem[] = [];
    for (const p of s.pendingPermissions) {
        if (p.kind === 'ask_user_question') {
            let matched = false;
            for (let i = items.length - 1; i >= 0; i--) {
                const it = items[i];
                if (it.kind !== 'tool_use') continue;
                const callIdx = p.toolCallId ? it.calls.findIndex(c => c.toolCallId === p.toolCallId) : -1;
                if (callIdx < 0) continue;
                const askUser: AskUserQuestionState = {
                    requestId: p.requestId,
                    toolCallId: p.toolCallId,
                    mode: p.mode,
                    questions: p.questions,
                    ...(p.resolved ? { resolved: p.resolved } : {}),
                    ...(p.answers ? { answers: p.answers } : {}),
                };
                const updated = {
                    ...it,
                    calls: it.calls.map((c, k) => (k !== callIdx ? c : { ...c, askUser })),
                };
                items = items.map((entry, idx) => (idx === i ? updated : entry));
                matched = true;
                break;
            }
            if (!matched) nextPermissions.push(p);
            continue;
        }
        if (p.kind === 'exit_plan_mode') {
            let matched = false;
            for (let i = items.length - 1; i >= 0; i--) {
                const it = items[i];
                if (it.kind !== 'tool_use') continue;
                const callIdx = p.toolCallId ? it.calls.findIndex(c => c.toolCallId === p.toolCallId) : -1;
                if (callIdx < 0) continue;
                const exitPlan: ExitPlanModeState = {
                    requestId: p.requestId,
                    toolCallId: p.toolCallId,
                    planContent: p.planContent,
                    ...(p.resolved ? { resolved: p.resolved } : {}),
                    ...(p.comments ? { comments: p.comments } : {}),
                };
                const updated = {
                    ...it,
                    calls: it.calls.map((c, k) => (k !== callIdx ? c : { ...c, exitPlan })),
                };
                items = items.map((entry, idx) => (idx === i ? updated : entry));
                matched = true;
                break;
            }
            if (!matched) nextPermissions.push(p);
            continue;
        }
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
 *
 * Subagent tool calls (isSubagent, grok stamps the subagent's `_meta.promptId`)
 * fold into the matching `subagent_turn` card instead of the main message.
 */
export function applyToolCall(
    s: PendingState,
    ev: {
        arguments?: unknown;
        toolName?: string;
        toolCallId?: string;
        kind?: string;
        status?: string;
        locations?: ToolCallInfo['locations'];
        diffs?: ToolCallInfo['diffs'];
    },
    agentTurnId?: string,
    isSubagent?: boolean
): PendingState {
    const items0 = flushStreamingCursor(s.items);
    // A tool_call_update carrying only new metadata (kind/locations/diffs/status)
    // may omit arguments; keep `input` empty here and let the merge below preserve
    // any input a prior event set, rather than stomping it with "undefined".
    const hasArgs = hasRenderableArguments(ev.arguments);
    const argsString = !hasArgs
        ? ''
        : typeof ev.arguments === 'string'
          ? ev.arguments
          : JSON.stringify(ev.arguments, null, 2);
    const status = normalizeToolCallStatus(ev.status);
    const newCall: ToolCallInfo = {
        toolName: ev.toolName || 'tool',
        input: argsString,
        ...(ev.kind ? { kind: ev.kind } : {}),
        ...(status ? { status } : {}),
        ...(ev.locations && ev.locations.length ? { locations: ev.locations } : {}),
        ...(ev.diffs && ev.diffs.length ? { diffs: ev.diffs } : {}),
    };
    if (ev.toolCallId) {
        newCall.toolCallId = ev.toolCallId;
    }
    if (isSubagent && agentTurnId) {
        const existing = findSubagentTurn(items0, agentTurnId);
        if (existing) {
            const next = [...items0];
            next[existing.index] = {
                ...existing.item,
                calls: foldToolCall(existing.item.calls, newCall),
            };
            return tryAssignPending({
                items: next,
                pendingResults: s.pendingResults,
                pendingPermissions: s.pendingPermissions,
            });
        }
        // Tool call arrived before any subagent text (e.g. a tool-first
        // subagent) — create the card so the call has a home.
        return tryAssignPending({
            items: [
                ...items0,
                {
                    ...createSubagentTurn(agentTurnId),
                    calls: [newCall],
                },
            ],
            pendingResults: s.pendingResults,
            pendingPermissions: s.pendingPermissions,
        });
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
                              // Preserve prior input when this event carried none.
                              input: newCall.input || c.input,
                              ...(newCall.kind ? { kind: newCall.kind } : {}),
                              ...(newCall.status ? { status: newCall.status } : {}),
                              ...(newCall.locations ? { locations: newCall.locations } : {}),
                              ...(newCall.diffs ? { diffs: newCall.diffs } : {}),
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
 *
 * Subagent results attach to the subagent card's own calls by toolCallId.
 */
export function applyToolResult(
    s: PendingState,
    ev: { text?: string; isError?: boolean; toolCallId?: string; toolName?: string },
    agentTurnId?: string,
    isSubagent?: boolean
): PendingState {
    if (isSubagent && agentTurnId) {
        const items = [...flushStreamingCursor(s.items)];
        const existing = findSubagentTurn(items, agentTurnId);
        if (existing && ev.toolCallId) {
            const callIdx = existing.item.calls.findIndex(c => c.toolCallId === ev.toolCallId);
            if (callIdx >= 0) {
                items[existing.index] = {
                    ...existing.item,
                    calls: existing.item.calls.map((c, idx) =>
                        idx === callIdx
                            ? {
                                  ...c,
                                  output: ev.text || '',
                                  isError: !!ev.isError,
                                  status: (ev.isError ? 'failed' : 'completed') as ToolCallStatus,
                              }
                            : c
                    ),
                };
                return { items, pendingResults: s.pendingResults, pendingPermissions: s.pendingPermissions };
            }
        }
        // No matching call yet (result raced ahead of the call). Park it like
        // the main path does — tryAssignPending reconciles once the call lands.
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
                                  // tool_result is terminal — align ACP status even when
                                  // the agent only emitted a result without a status update.
                                  status: (ev.isError ? 'failed' : 'completed') as ToolCallStatus,
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
                              status: (ev.isError ? 'failed' : 'completed') as ToolCallStatus,
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
 * Mark a permission request as resolved wherever it may live:
 * nested under tool_use, as a top-level permission_request item, or parked in
 * the pendingPermissions holding pen (Grok client-side fs/terminal confirms
 * use synthetic acpx-client-* toolCallIds and almost always land there).
 */
export function resolvePermission(s: PendingState, requestId: string, resolved: 'allow' | 'deny'): PendingState {
    const items = s.items.map(it => {
        if (it.kind === 'tool_use') {
            let touched = false;
            const calls = it.calls.map(c => {
                if (c.permission && c.permission.requestId === requestId && !c.permission.resolved) {
                    touched = true;
                    return { ...c, permission: { ...c.permission, resolved } };
                }
                return c;
            });
            return touched ? { ...it, calls } : it;
        }
        if (it.kind === 'permission_request' && it.requestId === requestId && !it.resolved) {
            return { ...it, resolved };
        }
        return it;
    });
    const pendingPermissions = s.pendingPermissions.map(p => {
        if (p.kind === 'permission_request' && p.requestId === requestId && !p.resolved) {
            return { ...p, resolved };
        }
        return p;
    });
    return { items, pendingResults: s.pendingResults, pendingPermissions };
}

/**
 * Fold a `permission_timeout` event: flush the streaming cursor, mark the nested
 * /pending permission as denied (collapses the inline UI), then append a timeout error.
 */
export function applyPermissionTimeout(
    s: PendingState,
    requestId: string | undefined,
    message: string | undefined
): PendingState {
    const rid = requestId || '';
    const marked = resolvePermission(
        {
            items: flushStreamingCursor(s.items),
            pendingResults: s.pendingResults,
            pendingPermissions: s.pendingPermissions,
        },
        rid,
        'deny'
    );
    return {
        items: [
            ...marked.items,
            {
                id: cryptoId(),
                kind: 'error',
                content: message || 'Permission request timed out.',
                createdAt: Date.now(),
            },
        ],
        pendingResults: marked.pendingResults,
        pendingPermissions: marked.pendingPermissions,
    };
}

function parseAskUserOptions(raw: unknown): AskUserOption[] {
    if (!Array.isArray(raw)) return [];
    const out: AskUserOption[] = [];
    for (const item of raw) {
        if (!item || typeof item !== 'object') continue;
        const o = item as Record<string, unknown>;
        if (typeof o.label !== 'string' || typeof o.description !== 'string') continue;
        out.push({
            label: o.label,
            description: o.description,
            preview: (o.preview as string | null | undefined) ?? undefined,
        });
    }
    return out;
}

/** Normalize a bridge `ask_user_question` questions array into typed items. */
export function parseAskUserQuestions(raw: unknown): AskUserQuestionItem[] {
    if (!Array.isArray(raw)) return [];
    const out: AskUserQuestionItem[] = [];
    for (const item of raw) {
        if (!item || typeof item !== 'object') continue;
        const q = item as Record<string, unknown>;
        if (typeof q.question !== 'string') continue;
        const options = parseAskUserOptions(q.options);
        if (options.length === 0) continue;
        const multiRaw = 'multiSelect' in q ? q.multiSelect : q.multi_select;
        const multiSelect = multiRaw === undefined ? undefined : multiRaw === null ? null : Boolean(multiRaw);
        out.push({ question: q.question, options, multiSelect });
    }
    return out;
}

/**
 * Fold an `ask_user_question` event: nest on the matching tool_use call when
 * possible, otherwise park in pendingPermissions for later reconcile.
 * Reuses pendingPermissions as the holding pen (same lifecycle as permission).
 */
export function applyAskUserQuestion(
    s: PendingState,
    ev: {
        requestId?: string;
        toolCallId?: string;
        mode?: string;
        questions?: unknown;
    }
): PendingState {
    const requestId = ev.requestId || '';
    const toolCallId = ev.toolCallId;
    const questions = parseAskUserQuestions(ev.questions);
    if (!requestId || questions.length === 0) {
        return s;
    }
    const askUser: AskUserQuestionState = {
        requestId,
        toolCallId,
        mode: ev.mode,
        questions,
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
                    calls: item.calls.map((c, idx) => (idx === callIdx ? { ...c, askUser } : c)),
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
                    kind: 'ask_user_question',
                    requestId,
                    toolCallId,
                    mode: ev.mode,
                    questions,
                    createdAt: Date.now(),
                },
            ],
        };
    }
    return { items, pendingResults: s.pendingResults, pendingPermissions: s.pendingPermissions };
}

/**
 * Fold an `ask_user_question_timeout` event: mark nested/pending request as
 * cancelled and append a timeout error bubble.
 */
export function applyAskUserQuestionTimeout(
    s: PendingState,
    requestId: string | undefined,
    message: string | undefined
): PendingState {
    const rid = requestId || '';
    const items = flushStreamingCursor(s.items).map(it => {
        if (it.kind === 'tool_use') {
            let touched = false;
            const calls = it.calls.map(c => {
                if (c.askUser && c.askUser.requestId === rid && !c.askUser.resolved) {
                    touched = true;
                    return {
                        ...c,
                        askUser: { ...c.askUser, resolved: 'cancelled' as const },
                    };
                }
                return c;
            });
            return touched ? { ...it, calls } : it;
        }
        if (it.kind === 'ask_user_question' && it.requestId === rid && !it.resolved) {
            return { ...it, resolved: 'cancelled' as const };
        }
        return it;
    });
    const pendingPermissions = s.pendingPermissions.map(p => {
        if (p.kind === 'ask_user_question' && p.requestId === rid && !p.resolved) {
            return { ...p, resolved: 'cancelled' as const };
        }
        return p;
    });
    return {
        items: [
            ...items,
            {
                id: cryptoId(),
                kind: 'error',
                content: message || 'ask_user_question timed out.',
                createdAt: Date.now(),
            },
        ],
        pendingResults: s.pendingResults,
        pendingPermissions,
    };
}

/** Mark a nested/pending ask_user request as resolved after the user answers. */
export function resolveAskUserQuestion(
    s: PendingState,
    requestId: string,
    outcome: AskUserOutcome,
    answers?: Record<string, AskUserAnswerValue>
): PendingState {
    const items = s.items.map(it => {
        if (it.kind === 'tool_use') {
            let touched = false;
            const calls = it.calls.map(c => {
                if (c.askUser && c.askUser.requestId === requestId && !c.askUser.resolved) {
                    touched = true;
                    return {
                        ...c,
                        askUser: {
                            ...c.askUser,
                            resolved: outcome,
                            ...(answers ? { answers } : {}),
                        },
                    };
                }
                return c;
            });
            return touched ? { ...it, calls } : it;
        }
        if (it.kind === 'ask_user_question' && it.requestId === requestId && !it.resolved) {
            return {
                ...it,
                resolved: outcome,
                ...(answers ? { answers } : {}),
            };
        }
        return it;
    });
    const pendingPermissions = s.pendingPermissions.map(p => {
        if (p.kind === 'ask_user_question' && p.requestId === requestId && !p.resolved) {
            return {
                ...p,
                resolved: outcome,
                ...(answers ? { answers } : {}),
            };
        }
        return p;
    });
    return { items, pendingResults: s.pendingResults, pendingPermissions };
}

/**
 * Fold an `exit_plan_mode` event: nest on matching tool_use or park pending.
 */
export function applyExitPlanMode(
    s: PendingState,
    ev: { requestId?: string; toolCallId?: string; planContent?: string }
): PendingState {
    const requestId = ev.requestId || '';
    const toolCallId = ev.toolCallId;
    const planContent = typeof ev.planContent === 'string' ? ev.planContent : '';
    if (!requestId) {
        return s;
    }
    const exitPlan: ExitPlanModeState = {
        requestId,
        toolCallId,
        planContent,
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
                    calls: item.calls.map((c, idx) => (idx === callIdx ? { ...c, exitPlan } : c)),
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
                    kind: 'exit_plan_mode',
                    requestId,
                    toolCallId,
                    planContent,
                    createdAt: Date.now(),
                },
            ],
        };
    }
    return { items, pendingResults: s.pendingResults, pendingPermissions: s.pendingPermissions };
}

/** Fold `exit_plan_mode_timeout`: mark abandoned + error bubble. */
export function applyExitPlanModeTimeout(
    s: PendingState,
    requestId: string | undefined,
    message: string | undefined
): PendingState {
    const rid = requestId || '';
    const items = flushStreamingCursor(s.items).map(it => {
        if (it.kind === 'tool_use') {
            let touched = false;
            const calls = it.calls.map(c => {
                if (c.exitPlan && c.exitPlan.requestId === rid && !c.exitPlan.resolved) {
                    touched = true;
                    return {
                        ...c,
                        exitPlan: { ...c.exitPlan, resolved: 'abandoned' as const },
                    };
                }
                return c;
            });
            return touched ? { ...it, calls } : it;
        }
        if (it.kind === 'exit_plan_mode' && it.requestId === rid && !it.resolved) {
            return { ...it, resolved: 'abandoned' as const };
        }
        return it;
    });
    const pendingPermissions = s.pendingPermissions.map(p => {
        if (p.kind === 'exit_plan_mode' && p.requestId === rid && !p.resolved) {
            return { ...p, resolved: 'abandoned' as const };
        }
        return p;
    });
    return {
        items: [
            ...items,
            {
                id: cryptoId(),
                kind: 'error',
                content: message || 'exit_plan_mode timed out.',
                createdAt: Date.now(),
            },
        ],
        pendingResults: s.pendingResults,
        pendingPermissions,
    };
}

/** Mark nested/pending exit_plan_mode as resolved after user decision. */
export function resolveExitPlanMode(
    s: PendingState,
    requestId: string,
    outcome: ExitPlanOutcome,
    comments?: string
): PendingState {
    const items = s.items.map(it => {
        if (it.kind === 'tool_use') {
            let touched = false;
            const calls = it.calls.map(c => {
                if (c.exitPlan && c.exitPlan.requestId === requestId && !c.exitPlan.resolved) {
                    touched = true;
                    return {
                        ...c,
                        exitPlan: {
                            ...c.exitPlan,
                            resolved: outcome,
                            ...(comments ? { comments } : {}),
                        },
                    };
                }
                return c;
            });
            return touched ? { ...it, calls } : it;
        }
        if (it.kind === 'exit_plan_mode' && it.requestId === requestId && !it.resolved) {
            return {
                ...it,
                resolved: outcome,
                ...(comments ? { comments } : {}),
            };
        }
        return it;
    });
    const pendingPermissions = s.pendingPermissions.map(p => {
        if (p.kind === 'exit_plan_mode' && p.requestId === requestId && !p.resolved) {
            return {
                ...p,
                resolved: outcome,
                ...(comments ? { comments } : {}),
            };
        }
        return p;
    });
    return { items, pendingResults: s.pendingResults, pendingPermissions };
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
        s.pendingPermissions.some(
            p =>
                (p.kind === 'permission_request' && !p.resolved) ||
                (p.kind === 'ask_user_question' && !p.resolved) ||
                (p.kind === 'exit_plan_mode' && !p.resolved)
        ) ||
        s.items.some(
            it =>
                it.kind === 'tool_use' &&
                it.calls.some(
                    c =>
                        (c.permission && !c.permission.resolved) ||
                        (c.askUser && !c.askUser.resolved) ||
                        (c.exitPlan && !c.exitPlan.resolved)
                )
        ) ||
        s.items.some(
            it => (it.kind === 'ask_user_question' && !it.resolved) || (it.kind === 'exit_plan_mode' && !it.resolved)
        );
    if (hasPendingPermission) return 'awaiting_permission';
    if (s.typing) return 'streaming';
    return null;
}
