// Preact hooks wrapping the backend chat WebSocket.
//
// Owns one WebSocket per Task session; translates events into
// a React-friendly stream of "messages" (assistant text, tool calls,
// permission requests, errors). The ChatPanel renders that stream.

import { useEffect, useState, useCallback } from 'preact/hooks';
import type { ChatSession, PermissionDecision, PermissionMode } from '../types';

export interface ToolCallInfo {
    id?: string;
    toolName: string;
    input: string;
    toolCallId?: string;
    output?: string;
    isError?: boolean;
    /**
     * Inline permission request that the runtime emitted for this tool call.
     * Lives as a sub-field (not a separate ChatItem) so the permission UI
     * stays nested inside its tool_use card across both real-time streaming
     * and history replay.
     */
    permission?: {
        requestId: string;
        toolName: string;
        input: string;
        options: Array<{ text: string; data: string }>;
        resolved?: 'allow' | 'deny';
    };
}

/**
 * Shape of each item sent in a `history_response`. Mirrors the kind union
 * the bridge-server produces when replaying an agent's native session
 * storage (e.g. Claude Code's ~/.claude/projects/.../<sessionId>.jsonl).
 */
export type HistoryItem =
    | { kind: 'user'; text: string; createdAt?: string }
    | { kind: 'assistant_text'; text: string; createdAt?: string }
    | { kind: 'thinking'; text: string; createdAt?: string }
    | {
          kind: 'tool_use';
          toolName: string;
          input: unknown;
          toolCallId?: string;
          createdAt?: string;
      }
    | {
          kind: 'tool_result';
          toolCallId?: string;
          content: string;
          isError: boolean;
          createdAt?: string;
      };

export type ChatItem =
    | { id: string; kind: 'user'; content: string; createdAt: number; queueStatus?: 'queued'; queueRequestId?: string }
    | { id: string; kind: 'assistant_text'; content: string; createdAt: number; streaming: boolean }
    | { id: string; kind: 'thinking'; content: string; createdAt: number }
    | {
          id: string;
          kind: 'tool_use';
          toolName: string;
          input: string;
          calls: ToolCallInfo[];
          createdAt: number;
          toolCallId?: string;
      }
    | {
          id: string;
          kind: 'tool_result';
          toolCallId?: string;
          /** Tool name echoed by the realtime event, when available.
           * Lets the "待分配" fallback group label orphan results with
           * the real tool instead of a generic placeholder. */
          toolName?: string;
          content: string;
          createdAt: number;
          isError: boolean;
      }
    | {
          id: string;
          kind: 'permission_request';
          toolCallId?: string;
          requestId: string;
          toolName: string;
          input: string;
          options: Array<{ text: string; data: string }>;
          createdAt: number;
          resolved?: 'allow' | 'deny';
      }
    | { id: string; kind: 'error'; content: string; createdAt: number };

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'closed' | 'error';

interface UseBridgeState {
    items: ChatItem[];
    connection: ConnectionState;
    typing: boolean;
    /**
     * True once the bridge has emitted `session_ready` for this session.
     * The UI uses this to gate the Composer and to render a "preparing
     * session" placeholder during the brief init window for new chats.
     */
    ready: boolean;
    permissionMode: PermissionMode;
    send: (content: string) => void;
    /**
     * Terminate the session. Cancels the running turn, drops every
     * queued prompt, and closes the underlying ACP runtime. After
     * this, the next `send` will re-initialize via `ensure_session`.
     * Distinct from `cancelQueued`, which only removes a single
     * queued entry.
     */
    cancel: () => void;
    cancelQueued: (requestId: string) => void;
    respondPermission: (requestId: string, decision: PermissionDecision) => void;
    setPermissionMode: (mode: PermissionMode) => void;
}

export interface SessionBridgeState {
    items: ChatItem[];
    connection: ConnectionState;
    typing: boolean;
    ws: WebSocket | null;
    listeners: Set<() => void>;
    turnStarted: boolean;
    /**
     * Real-time-only holding pen for tool_result and permission_request
     * events that arrived before (or without) their matching tool_call.
     * Each new tool_call re-scans these lists and folds any matching
     * entries into the call's tool_use; leftover entries are surfaced
     * by the renderer as a "待分配" tool_group so the data is never
     * silently dropped. The pool is cleared on history reload because
     * the historical record is authoritative.
     */
    pendingResults: ChatItem[];
    pendingPermissions: ChatItem[];
    /**
     * True once the bridge-server confirms the session is initialized
     * (`session_ready` event). New sessions sit at `ready = false` for a
     * brief window while the bridge spawns the agent process; during
     * that window, prompt/cancel/set_permission_mode would all bounce
     * with SESSION_NOT_FOUND, so the UI must gate input on this flag.
     */
    ready: boolean;
    /** Per-session permission policy mirrored from the backend record. */
    permissionMode: PermissionMode;
    /** Exponential backoff level — incremented on each reconnect attempt, reset on session_ready. */
    reconnectAttempt: number;
    /** Pending setTimeout handle for the next reconnect; null when idle. */
    reconnectTimer: ReturnType<typeof setTimeout> | null;
    /** True when destroy() was called; prevents the onclose handler from scheduling a reconnect. */
    closedByUser: boolean;
}

const DEFAULT_PERMISSION_MODE: PermissionMode = 'approve-reads';

export class ChatBridgeManager {
    private sessions = new Map<string, SessionBridgeState>();

    getOrCreate(session: ChatSession): SessionBridgeState {
        let state = this.sessions.get(session.id);
        if (!state) {
            state = {
                items: [],
                connection: 'idle',
                typing: false,
                ws: null,
                listeners: new Set(),
                turnStarted: false,
                pendingResults: [],
                pendingPermissions: [],
                // New sessions stay `ready: false` until the bridge-server
                // emits `session_ready`; the UI gates input on this so we
                // don't bounce prompts with SESSION_NOT_FOUND during the
                // brief window the agent process is spawning.
                ready: false,
                // The list endpoint (GET /api/agent/sessions?workspace_id=…)
                // already serializes ChatSessionRecord.PermissionMode onto
                // the ChatSession object, so we can trust the field
                // verbatim instead of doing a second GET per session.
                permissionMode: session.permissionMode ?? DEFAULT_PERMISSION_MODE,
                reconnectAttempt: 0,
                reconnectTimer: null,
                closedByUser: false,
            };
            this.sessions.set(session.id, state);
            this.connect(session, state);
        }
        return state;
    }

    destroy(sessionId: string) {
        const state = this.sessions.get(sessionId);
        if (state) {
            state.closedByUser = true;
            if (state.reconnectTimer) {
                clearTimeout(state.reconnectTimer);
                state.reconnectTimer = null;
            }
            if (state.ws) {
                if (state.ws.readyState === WebSocket.OPEN) {
                    state.ws.send(JSON.stringify({ action: 'close_session', sessionId }));
                }
                state.ws.close();
            }
            this.sessions.delete(sessionId);
        }
    }

    private connect(session: ChatSession, state: SessionBridgeState) {
        state.connection = 'connecting';
        // Reset `ready` on every (re)connect. The bridge-server emits a
        // fresh `session_ready` after each `ensure_session`, so we wait
        // for the new confirmation before letting the user act again.
        state.ready = false;
        this.notify(state);

        const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const taskId = session.taskId || '';
        const replyId = session.replyId || '';
        const wsUrl = `${wsProto}//${window.location.host}/api/agent/chat/ws?workspace_id=${encodeURIComponent(session.workspaceId)}&task_id=${encodeURIComponent(taskId)}&session_id=${encodeURIComponent(session.id)}&agent_type=${encodeURIComponent(session.agentType)}&reply_id=${encodeURIComponent(replyId)}`;

        console.log('[useBridgeManager] Connecting to backend websocket:', wsUrl);
        const ws = new WebSocket(wsUrl);
        state.ws = ws;

        // Flush the streaming cursor on the trailing assistant_text block. Called
        // whenever a non-text_delta event arrives, so the blink stops at the
        // boundary between text and whatever comes next (tool, permission, ...).
        const stopAssistantStreaming = () => {
            const items = state.items;
            if (items.length === 0) return;
            const last = items[items.length - 1];
            if (last && last.kind === 'assistant_text' && last.streaming) {
                state.items = [...items.slice(0, -1), { ...last, streaming: false }];
            }
        };

        ws.onopen = () => {
            state.connection = 'connected';
            this.notify(state);
            ws.send(
                JSON.stringify({
                    action: 'get_history',
                    sessionId: session.id,
                    agentType: session.agentType,
                    acpSessionId: session.acpSessionId,
                })
            );
        };

        ws.onmessage = e => {
            let payload: {
                event: string;
                text?: string;
                type?: string;
                arguments?: unknown;
                requestId?: string;
                message?: string;
                code?: string;
                toolName?: string;
                toolCallId?: string;
                isError?: boolean;
                messages?: Array<{ role: string; text: string }>;
                items?: HistoryItem[];
            };
            try {
                payload = JSON.parse(e.data) as typeof payload;
            } catch (err) {
                console.error('[useBridgeManager] Failed to parse message:', err);
                return;
            }

            const event = payload.event;
            console.log('[useBridgeManager] Received event:', event, payload);

            switch (event) {
                case 'session_ready':
                    // Flip the gate so the Composer / mode toggle unblock.
                    // `state.typing` is intentionally untouched — the
                    // bridge signals per-turn activity with `done` / `error`,
                    // not with `session_ready`.
                    state.reconnectAttempt = 0;
                    state.ready = true;
                    this.notify(state);
                    break;
                case 'prompt_queued': {
                    // Bridge accepted the prompt but couldn't start it
                    // because another turn is already running. Mark the
                    // most-recently-added user bubble as "queued" so the
                    // UI can render a queue badge + per-item cancel
                    // button on it. The bridge-supplied `requestId` is
                    // stored on the bubble so the X button knows what
                    // to send back in `cancel_queued`. When the next
                    // turn starts (first text_delta after the active one
                    // finishes), the FIFO drain in text_delta clears
                    // this status.
                    if (!state.turnStarted) break;
                    const items = state.items;
                    for (let i = items.length - 1; i >= 0; i--) {
                        const it = items[i];
                        if (it.kind !== 'user') continue;
                        if (it.queueStatus === 'queued') break;
                        state.items = [
                            ...items.slice(0, i),
                            { ...it, queueStatus: 'queued', queueRequestId: payload.requestId },
                            ...items.slice(i + 1),
                        ];
                        this.notify(state);
                        break;
                    }
                    break;
                }
                case 'prompt_cancelled': {
                    // Bridge dropped a queued prompt without ever starting
                    // it (e.g. user pressed cancel mid-flight). Clear
                    // the queue badge from any still-queued user bubbles
                    // — without this, the badge would linger forever
                    // because the cancelled turn never produced a
                    // text_delta to drain it.
                    let mutated = false;
                    const next = state.items.map(it => {
                        if (it.kind === 'user' && it.queueStatus === 'queued') {
                            mutated = true;
                            return { ...it, queueStatus: undefined };
                        }
                        return it;
                    });
                    if (mutated) {
                        state.items = next;
                        this.notify(state);
                    }
                    break;
                }
                case 'text_delta': {
                    if (!state.turnStarted) break;
                    const delta = payload.text;
                    const type = payload.type || 'output';
                    if (!delta) return;

                    const next = [...state.items];
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
                            // First text_delta for a freshly dequeued turn.
                            // Clear the queue badge from the oldest still-
                            // queued user bubble — the bridge drains its
                            // promptQueue FIFO, so the first remaining
                            // queued bubble is the one that just started.
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
                    state.items = next;
                    this.notify(state);
                    break;
                }
                case 'tool_call': {
                    if (!state.turnStarted) break;
                    // Backend's SSE safety fallback may emit tool_call events
                    // without `arguments` (omitted) or with `arguments: {}`
                    // (the runtime's no-input placeholder). Neither carries
                    // any data we can render, and acting on them would
                    // either synthesize a no-input call card or replace an
                    // existing call's streamed input. Drop them here so the
                    // next substantive event (or tool_result) is the only
                    // thing that can move the call's state forward.
                    if (!hasRenderableArguments(payload.arguments)) break;
                    stopAssistantStreaming();
                    const argsString =
                        typeof payload.arguments === 'string'
                            ? payload.arguments
                            : JSON.stringify(payload.arguments, null, 2);
                    const newCall: ToolCallInfo = {
                        toolName: payload.toolName || 'tool',
                        input: argsString,
                    };
                    if (payload.toolCallId) {
                        newCall.toolCallId = payload.toolCallId;
                    }
                    const next = [...state.items];
                    const last = next[next.length - 1];
                    if (last && last.kind === 'tool_use') {
                        // Multiple tool_call events for the same toolCallId
                        // may arrive as more data streams in. Update the
                        // existing call in place rather than appending a
                        // duplicate so tool_result lands on the right call
                        // and the tool_group stays tidy.
                        const existingIdx = newCall.toolCallId
                            ? last.calls.findIndex(c => c.toolCallId === newCall.toolCallId)
                            : -1;
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
                    state.items = next;
                    // Re-scan the pending result/permission pools and
                    // fold any entry that matches the new call (or
                    // earlier calls in this turn) onto its call. This
                    // is what reconciles out-of-order arrivals — e.g.
                    // a permission_request that beat the tool_call to
                    // the wire, or a tool_result that arrived before
                    // its call. Each scan is O(pending × items) but
                    // the pool is bounded by per-turn orphan count
                    // (a handful at most).
                    tryAssignPending(state);
                    this.notify(state);
                    break;
                }
                case 'tool_result': {
                    if (!state.turnStarted) break;
                    stopAssistantStreaming();

                    // Find the matching tool_use / call and fold the result onto
                    // it in place. This mirrors the rendered history shape (a
                    // call with both input and output attached) so the
                    // tool_group can display the body during streaming, not
                    // only after a history reload.
                    const items = [...state.items];
                    let matched = false;
                    for (let i = items.length - 1; i >= 0; i--) {
                        const item = items[i];
                        if (item.kind !== 'tool_use') continue;
                        if (payload.toolCallId) {
                            const callIdx = item.calls.findIndex(c => c.toolCallId === payload.toolCallId);
                            if (callIdx >= 0) {
                                items[i] = {
                                    ...item,
                                    calls: item.calls.map((c, idx) =>
                                        idx === callIdx
                                            ? {
                                                  ...c,
                                                  output: payload.text || '',
                                                  isError: !!payload.isError,
                                              }
                                            : c
                                    ),
                                };
                                matched = true;
                                break;
                            }
                        }
                        // No toolCallId: attach to the most recent call in the
                        // latest tool_use that doesn't have output yet.
                        const openCallIdx = item.calls.findIndex(c => c.output === undefined);
                        const targetIdx = openCallIdx >= 0 ? openCallIdx : item.calls.length - 1;
                        if (targetIdx >= 0) {
                            items[i] = {
                                ...item,
                                calls: item.calls.map((c, idx) =>
                                    idx === targetIdx
                                        ? {
                                              ...c,
                                              output: payload.text || '',
                                              isError: !!payload.isError,
                                          }
                                        : c
                                ),
                            };
                            matched = true;
                            break;
                        }
                    }
                    if (!matched) {
                        // No tool_use yet — park the result in the
                        // session-scoped pending pool. The next
                        // tool_call will re-scan the pool and fold any
                        // matching entry into its call. Leftover entries
                        // are surfaced by the renderer as a "待分配"
                        // tool_group so the data is never dropped.
                        state.items = items;
                        state.pendingResults = [
                            ...state.pendingResults,
                            {
                                id: cryptoId(),
                                kind: 'tool_result',
                                toolCallId: payload.toolCallId,
                                toolName: payload.toolName,
                                content: payload.text || '',
                                isError: !!payload.isError,
                                createdAt: Date.now(),
                            },
                        ];
                        this.notify(state);
                        break;
                    }
                    state.items = items;
                    this.notify(state);
                    break;
                }
                case 'permission_request': {
                    if (!state.turnStarted) break;
                    stopAssistantStreaming();
                    const argsString =
                        typeof payload.arguments === 'string'
                            ? payload.arguments
                            : JSON.stringify(payload.arguments, null, 2);
                    const requestId = payload.requestId || '';
                    const toolCallId = payload.toolCallId;
                    const toolName = payload.toolName || 'tool';
                    const newPermission = {
                        requestId,
                        toolName,
                        input: argsString,
                        options: [] as Array<{ text: string; data: string }>,
                    };

                    // Mirror tool_result's reverse-scan + synthesize-fallback:
                    // attach the permission to the matching tool_use so it
                    // renders nested inside the tool card. If the permission
                    // beat the tool_call over the wire, synthesize a stub
                    // tool_use and let the later tool_call merge into it.
                    const items = [...state.items];
                    let matched = false;
                    if (toolCallId) {
                        for (let i = items.length - 1; i >= 0; i--) {
                            const item = items[i];
                            if (item.kind !== 'tool_use') continue;
                            const callIdx = item.calls.findIndex(c => c.toolCallId === toolCallId);
                            if (callIdx >= 0) {
                                items[i] = {
                                    ...item,
                                    calls: item.calls.map((c, idx) =>
                                        idx === callIdx ? { ...c, permission: newPermission } : c
                                    ),
                                };
                                matched = true;
                                break;
                            }
                        }
                    }
                    if (!matched) {
                        // No tool_use yet — park the permission in the
                        // session-scoped pending pool. The next
                        // tool_call will re-scan and fold any matching
                        // entry into its call. Synthesizing a stub
                        // tool_use with `input: ''` here is what used
                        // to produce a phantom "permission" card that
                        // visually replaced the real call card.
                        state.items = items;
                        state.pendingPermissions = [
                            ...state.pendingPermissions,
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
                        ];
                        this.notify(state);
                        break;
                    }
                    state.items = items;
                    this.notify(state);
                    break;
                }
                case 'permission_timeout': {
                    if (!state.turnStarted) break;
                    stopAssistantStreaming();
                    const requestId = payload.requestId;
                    // Mark the nested permission as denied so the inline UI
                    // collapses, then surface the timeout as an error chip.
                    state.items = state.items.map(it => {
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
                    state.items = [
                        ...state.items,
                        {
                            id: cryptoId(),
                            kind: 'error',
                            content: payload.message || 'Permission request timed out.',
                            createdAt: Date.now(),
                        },
                    ];
                    this.notify(state);
                    break;
                }
                case 'done': {
                    const next = [...state.items];
                    const last = next[next.length - 1];
                    if (last && last.kind === 'assistant_text' && last.streaming) {
                        next[next.length - 1] = {
                            ...last,
                            streaming: false,
                        };
                    }
                    state.items = next;
                    state.typing = false;
                    state.turnStarted = false;
                    this.notify(state);
                    this.reloadHistory(session, state);
                    break;
                }
                case 'history_response': {
                    const historyItems: HistoryItem[] =
                        payload.items && payload.items.length > 0
                            ? payload.items
                            : (payload.messages || []).map(m => ({
                                  kind: (m.role === 'user' ? 'user' : 'assistant_text') as 'user' | 'assistant_text',
                                  text: m.text,
                              }));
                    const converted: ChatItem[] = historyItems.map((it, idx) => historyItemToChatItem(it, idx));
                    state.items = converted;
                    // History is authoritative: any entries the realtime
                    // pool was holding (e.g. a tool_result that arrived
                    // before the matching tool_call) are redundant now
                    // that the on-disk record has been replayed. Drop
                    // the pool so the renderer stops showing the
                    // "待分配" group once the real history is in.
                    state.pendingResults = [];
                    state.pendingPermissions = [];
                    this.notify(state);
                    break;
                }
                case 'error': {
                    // SESSION_NOT_FOUND arriving before `session_ready`
                    // is the bridge's answer to any control action we
                    // fired (or the Go backend re-fired) while the agent
                    // was still spawning. Since the UI now gates input
                    // on `ready`, the user can't trigger these anymore,
                    // but we still defensively swallow them so the chat
                    // stream doesn't get a misleading red banner during
                    // the init window.
                    if (payload.code === 'SESSION_NOT_FOUND' && !state.ready) {
                        break;
                    }
                    stopAssistantStreaming();
                    state.items = [
                        ...state.items,
                        {
                            id: cryptoId(),
                            kind: 'error',
                            content: payload.message || payload.code || 'Unknown error',
                            createdAt: Date.now(),
                        },
                    ];
                    // Only reload history when the error actually came from
                    // inside a turn — that's the case where on-disk state
                    // might be authoritative over what we have in memory.
                    // Errors from out-of-turn control actions
                    // (set_permission_mode, respond_permission for a stale
                    // requestId, etc.) don't touch history; reloading then
                    // just pulls a fresh history_response per error and
                    // turns one bad input into a console storm.
                    const wasInTurn = state.turnStarted;
                    state.typing = false;
                    state.turnStarted = false;
                    this.notify(state);
                    if (wasInTurn) {
                        this.reloadHistory(session, state);
                    }
                    break;
                }
            }
        };

        ws.onclose = () => {
            if (state.closedByUser) {
                state.connection = 'closed';
                this.notify(state);
                return;
            }
            state.connection = 'reconnecting';
            state.typing = false;
            this.notify(state);
            const delay = Math.min(30_000, 1_000 * Math.pow(2, state.reconnectAttempt));
            state.reconnectAttempt++;
            state.reconnectTimer = setTimeout(() => {
                state.reconnectTimer = null;
                this.connect(session, state);
            }, delay);
        };

        ws.onerror = () => {
            // onclose always fires after onerror in the browser WebSocket API;
            // let onclose own the reconnect logic to avoid double-scheduling.
        };
    }

    send(session: ChatSession, content: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
        // Refuse prompts sent before the bridge confirms initialization.
        // The bridge would answer with SESSION_NOT_FOUND and we'd be left
        // with an orphan user bubble in the stream.
        if (!state.ready) return;
        state.turnStarted = true;
        const msgId = cryptoId();
        state.items = [
            ...state.items,
            {
                id: msgId,
                kind: 'user',
                content,
                createdAt: Date.now(),
            },
        ];
        state.ws.send(
            JSON.stringify({
                action: 'prompt',
                sessionId: session.id,
                text: content,
            })
        );
        state.typing = true;
        this.notify(state);
    }

    cancel(session: ChatSession) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
        if (!state.ready) return;
        // `cancel` on the wire is mapped to terminate-session: the only
        // user-facing stop semantics are "终止对话" (cancels the active
        // turn, drops the queue, closes the session). Stopping the
        // current turn while letting the queue keep running isn't
        // exposed in the UI.
        state.ws.send(
            JSON.stringify({
                action: 'close_session',
                sessionId: session.id,
            })
        );
        state.typing = false;
        state.turnStarted = false;
        this.notify(state);
    }

    /**
     * Remove a single queued prompt (one the bridge has not started
     * yet). Distinct from `cancel`, which only stops the active turn;
     * the queue keeps running. Used by the X button on queued user
     * bubbles — `requestId` is the queue id echoed back in
     * `prompt_queued`.
     */
    cancelQueued(session: ChatSession, requestId: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
        if (!state.ready) return;
        state.ws.send(
            JSON.stringify({
                action: 'cancel_queued',
                sessionId: session.id,
                requestId,
            })
        );
        // Optimistically clear the badge so the user gets immediate
        // feedback; the bridge's `prompt_cancelled` event will
        // arrive later and be a no-op.
        state.items = state.items.map(it => {
            if (it.kind === 'user' && it.queueRequestId === requestId) {
                return { ...it, queueStatus: undefined, queueRequestId: undefined };
            }
            return it;
        });
        this.notify(state);
    }

    respondPermission(session: ChatSession, requestId: string, decision: PermissionDecision) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
        if (!state.ready) return;
        // Find the nested permission sub-item and capture the originating
        // toolCallId so the response can be linked back to the tool_use
        // block in the audit/log chain.
        let toolCallId: string | undefined;
        for (const it of state.items) {
            if (it.kind !== 'tool_use') continue;
            const match = it.calls.find(c => c.permission?.requestId === requestId);
            if (match) {
                toolCallId = match.toolCallId;
                break;
            }
        }
        state.ws.send(
            JSON.stringify({
                action: 'respond_permission',
                sessionId: session.id,
                requestId,
                toolCallId,
                behavior: decision,
            })
        );
        // `cancel` leaves the inline UI interactive so the user can re-decide
        // (the runtime will time the request out on its own if nothing
        // else happens). The four real decisions collapse the inline
        // permission into a one-line summary keyed on the allow/deny side.
        const resolved: 'allow' | 'deny' | null =
            decision === 'allow_once' || decision === 'allow_always'
                ? 'allow'
                : decision === 'reject_once' || decision === 'reject_always'
                  ? 'deny'
                  : null;
        if (resolved) {
            state.items = state.items.map(it => {
                if (it.kind !== 'tool_use') return it;
                let touched = false;
                const calls = it.calls.map(c => {
                    if (c.permission && c.permission.requestId === requestId) {
                        touched = true;
                        return { ...c, permission: { ...c.permission, resolved } };
                    }
                    return c;
                });
                return touched ? { ...it, calls } : it;
            });
            this.notify(state);
        } else if (toolCallId === undefined) {
            // Should not happen: every permission_request has a matching
            // toolCallId by the time the user clicks a button. Logged
            // for visibility in case a future event source breaks the
            // invariant.
            console.warn('[useBridgeManager] respond_permission: no nested permission found for requestId', requestId);
        }
    }

    setPermissionMode(session: ChatSession, mode: PermissionMode) {
        const state = this.sessions.get(session.id);
        if (!state) return;
        if (state.permissionMode === mode) return;
        state.permissionMode = mode;
        this.notify(state);
        // Notify the bridge-server immediately so the gate flips before
        // the next permission request; persist via the REST endpoint so
        // it survives reloads. Both calls are fire-and-forget — if PATCH
        // fails the local toggle still reflects the user intent for the
        // current process lifetime.
        // Field name is `permissionMode` (not `mode`) to match the JSON
        // tag on backend/internal/agent.WsMessage.PermissionMode —
        // otherwise the Go struct drops the field on the ReadJSON →
        // WriteJSON forward and the bridge-server sees a missing param.
        if (state.ws && state.ws.readyState === WebSocket.OPEN) {
            state.ws.send(
                JSON.stringify({
                    action: 'set_permission_mode',
                    sessionId: session.id,
                    permissionMode: mode,
                })
            );
        }
        void fetch(`/api/agent/sessions/${encodeURIComponent(session.id)}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ permission_mode: mode }),
        }).catch(err => {
            console.warn('[useBridgeManager] PATCH permission_mode failed:', err);
        });
    }

    private reloadHistory(session: ChatSession, state: SessionBridgeState) {
        if (state.ws && state.ws.readyState === WebSocket.OPEN) {
            state.ws.send(
                JSON.stringify({
                    action: 'get_history',
                    sessionId: session.id,
                    agentType: session.agentType,
                    acpSessionId: session.acpSessionId,
                })
            );
        }
    }

    private notify(state: SessionBridgeState) {
        for (const listener of state.listeners) {
            listener();
        }
    }
}

export const globalBridgeManager = new ChatBridgeManager();

export function useBridge(session: ChatSession | null, seed: ChatItem[] = []): UseBridgeState {
    const [, forceUpdate] = useState({});

    useEffect(() => {
        if (!session) return;

        const state = globalBridgeManager.getOrCreate(session);
        const listener = () => forceUpdate({});
        state.listeners.add(listener);

        forceUpdate({});

        return () => {
            state.listeners.delete(listener);
        };
    }, [session?.id, session?.workspaceId, session?.taskId]);

    const state = session ? globalBridgeManager.getOrCreate(session) : null;

    // The pending pools are flattened into the items stream so
    // MessageList.groupChatItems can attach them to a synthesized
    // "待分配" tool_group when no matching tool_use exists. Once
    // history is reloaded the pools are cleared (see
    // `history_response`) and this list collapses back to state.items.
    const items = state ? [...state.items, ...state.pendingResults, ...state.pendingPermissions] : seed;
    const connection = state ? state.connection : 'idle';
    const typing = state ? state.typing : false;
    // `ready` is only meaningful once a `SessionBridgeState` exists; for
    // a null session we report false so the UI treats it as "not yet".
    const ready = state ? state.ready : false;
    const permissionMode = state ? state.permissionMode : DEFAULT_PERMISSION_MODE;

    const send = useCallback(
        (content: string) => {
            if (!session) return;
            globalBridgeManager.send(session, content);
        },
        [session]
    );

    const cancel = useCallback(() => {
        if (!session) return;
        globalBridgeManager.cancel(session);
    }, [session]);

    const cancelQueued = useCallback(
        (requestId: string) => {
            if (!session) return;
            globalBridgeManager.cancelQueued(session, requestId);
        },
        [session]
    );

    const respondPermission = useCallback(
        (requestId: string, decision: PermissionDecision) => {
            if (!session) return;
            globalBridgeManager.respondPermission(session, requestId, decision);
        },
        [session]
    );

    const setPermissionMode = useCallback(
        (mode: PermissionMode) => {
            if (!session) return;
            globalBridgeManager.setPermissionMode(session, mode);
        },
        [session]
    );

    return {
        items,
        connection,
        typing,
        ready,
        permissionMode,
        send,
        cancel,
        cancelQueued,
        respondPermission,
        setPermissionMode,
    };
}

function cryptoId(): string {
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
function hasRenderableArguments(args: unknown): boolean {
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

function parseCreatedAt(value: string | undefined): number {
    if (!value) return Date.now();
    const ts = new Date(value).getTime();
    return Number.isFinite(ts) ? ts : Date.now();
}

// Walk the pending result/permission pools and fold any entries that
// now match an existing tool_use in `items` straight into the call.
// The number of pending entries is bounded by how many orphan
// tool_results / permission_requests arrive in a single turn (a few
// at most), so the linear scan is cheap. Returns the leftover
// entries that still don't have a matching call — those stay in the
// pool for the renderer to surface as a "待分配" tool_group.
function tryAssignPending(state: SessionBridgeState): void {
    if (state.pendingResults.length === 0 && state.pendingPermissions.length === 0) return;
    let items = state.items;
    const nextResults: ChatItem[] = [];
    for (const p of state.pendingResults) {
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
            // Build the replacement from `it` (already narrowed to
            // tool_use) — the map callback's `entry` is the full union
            // and TS can't see that idx === i implies tool_use.
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
    for (const p of state.pendingPermissions) {
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
    state.items = items;
    state.pendingResults = nextResults;
    state.pendingPermissions = nextPermissions;
}

// History ids are derived from the item's position (and toolCallId for
// tool_use) instead of cryptoId(). Each `done` triggers a history
// reload that rebuilds the whole list; random ids changed every React
// key on every reload, remounting every bubble (visible flicker, all
// expand/collapse state lost). Positional ids are stable between
// consecutive reloads of the same on-disk record.
function historyItemToChatItem(it: HistoryItem, index: number): ChatItem {
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
