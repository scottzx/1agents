// Platform-agnostic chat-protocol pure helpers.
//
// These are the side-effect-free building blocks the bridge folds events with —
// no `this`, no WebSocket, no preact, no DOM. Moved verbatim out of
// components/chat/hooks.ts (Phase 0 carve) so non-DOM hosts (Tauri Mobile, 小程序)
// can reuse the same conversion/identity logic.
//
// NOTE: the stateful event folds that live inside ChatBridgeManager's
// ws.onmessage switch (text_delta / tool_call / tool_result / permission folds)
// and the SessionBridgeState-coupled helpers (deriveLiveStatus, tryAssignPending)
// are intentionally NOT moved yet — extracting them into pure reducers is the
// higher-risk step that needs live streaming verification.

import type { ChatItem, HistoryItem, ToolCallInfo } from './types';

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
