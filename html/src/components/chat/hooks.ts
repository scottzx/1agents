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
    | { id: string; kind: 'user'; content: string; createdAt: number }
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
    | { id: string; kind: 'tool_result'; toolCallId?: string; content: string; createdAt: number; isError: boolean }
    | { id: string; kind: 'error'; content: string; createdAt: number };

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'closed' | 'error';

interface UseBridgeState {
    items: ChatItem[];
    connection: ConnectionState;
    typing: boolean;
    permissionMode: PermissionMode;
    send: (content: string) => void;
    cancel: () => void;
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
    /** Per-session permission policy mirrored from the backend record. */
    permissionMode: PermissionMode;
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
                // The list endpoint (GET /api/agent/sessions?workspace_id=…)
                // already serializes ChatSessionRecord.PermissionMode onto
                // the ChatSession object, so we can trust the field
                // verbatim instead of doing a second GET per session.
                permissionMode: session.permissionMode ?? DEFAULT_PERMISSION_MODE,
            };
            this.sessions.set(session.id, state);
            this.connect(session, state);
        }
        return state;
    }

    destroy(sessionId: string) {
        const state = this.sessions.get(sessionId);
        if (state) {
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
        this.notify(state);

        const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const taskId = session.taskId || '';
        const wsUrl = `${wsProto}//${window.location.host}/api/agent/chat/ws?workspace_id=${encodeURIComponent(session.workspaceId)}&task_id=${encodeURIComponent(taskId)}&session_id=${encodeURIComponent(session.id)}&agent_type=${encodeURIComponent(session.agentType)}`;

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
                    break;
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
                    stopAssistantStreaming();
                    // The runtime may emit tool_call events for the same
                    // toolCallId both with and without rawInput:
                    //   - first an empty `{}` placeholder, then the real
                    //     arguments as they stream in
                    //   - after tool_result, a status-only update with no
                    //     rawInput at all (e.g. when the call finishes)
                    // Only the first shape carries new arguments; the
                    // second must not clobber the input we already have.
                    const hasArguments = payload.arguments !== undefined;
                    const argsString = hasArguments
                        ? typeof payload.arguments === 'string'
                            ? payload.arguments
                            : JSON.stringify(payload.arguments, null, 2)
                        : '';
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
                        // The runtime may emit multiple tool_call events for
                        // the same toolCallId as more data streams in (e.g.
                        // a placeholder `{}` first, then the real input).
                        // Update the existing call in place rather than
                        // appending a duplicate so tool_result lands on the
                        // right call and the tool_group stays tidy.
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
                                              // Preserve the streamed input
                                              // when this event is a
                                              // status-only update; refresh
                                              // it only when the event
                                              // actually carries new
                                              // arguments.
                                              ...(hasArguments ? { input: newCall.input } : {}),
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
                        // No tool_use was seen (e.g. result arrived first over
                        // a flaky socket). Synthesize a tool_use with just the
                        // result so the user still sees the output.
                        items.push({
                            id: cryptoId(),
                            kind: 'tool_use',
                            toolName: payload.toolName || 'tool',
                            input: '',
                            calls: [
                                {
                                    id: `call-${payload.toolCallId || cryptoId()}`,
                                    toolCallId: payload.toolCallId,
                                    toolName: payload.toolName || 'tool',
                                    input: '',
                                    output: payload.text || '',
                                    isError: !!payload.isError,
                                },
                            ],
                            createdAt: Date.now(),
                            ...(payload.toolCallId ? { toolCallId: payload.toolCallId } : {}),
                        });
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
                        // Stub: only the permission is known so far. tool_call
                        // (or a later tool_call_update) will hit the same
                        // toolCallId and fill in toolName / input.
                        const placeholder: ToolCallInfo = {
                            toolName,
                            input: '',
                            toolCallId,
                            permission: newPermission,
                        };
                        const last = items[items.length - 1];
                        if (last && last.kind === 'tool_use') {
                            items[items.length - 1] = {
                                ...last,
                                calls: [...last.calls, placeholder],
                            };
                        } else {
                            items.push({
                                id: cryptoId(),
                                kind: 'tool_use',
                                toolName,
                                input: '',
                                calls: [placeholder],
                                createdAt: Date.now(),
                                ...(toolCallId ? { toolCallId } : {}),
                            });
                        }
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
                    const converted: ChatItem[] = historyItems.map(it => historyItemToChatItem(it));
                    state.items = converted;
                    this.notify(state);
                    break;
                }
                case 'error': {
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
            state.connection = 'closed';
            state.typing = false;
            this.notify(state);
        };

        ws.onerror = () => {
            state.connection = 'error';
            state.typing = false;
            this.notify(state);
        };
    }

    send(session: ChatSession, content: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
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
        state.ws.send(
            JSON.stringify({
                action: 'cancel',
                sessionId: session.id,
            })
        );
        state.typing = false;
        state.turnStarted = false;
        this.notify(state);
    }

    respondPermission(session: ChatSession, requestId: string, decision: PermissionDecision) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
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

    const items = state ? state.items : seed;
    const connection = state ? state.connection : 'idle';
    const typing = state ? state.typing : false;
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

    return { items, connection, typing, permissionMode, send, cancel, respondPermission, setPermissionMode };
}

function cryptoId(): string {
    if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
        return (crypto as Crypto).randomUUID();
    }
    return `id-${Math.random().toString(36).slice(2)}-${Date.now()}`;
}

function parseCreatedAt(value: string | undefined): number {
    if (!value) return Date.now();
    const ts = new Date(value).getTime();
    return Number.isFinite(ts) ? ts : Date.now();
}

function historyItemToChatItem(it: HistoryItem): ChatItem {
    const createdAt = parseCreatedAt(it.createdAt);
    switch (it.kind) {
        case 'user':
            return { id: cryptoId(), kind: 'user', content: it.text, createdAt };
        case 'assistant_text':
            return {
                id: cryptoId(),
                kind: 'assistant_text',
                content: it.text,
                createdAt,
                streaming: false,
            };
        case 'thinking':
            return { id: cryptoId(), kind: 'thinking', content: it.text, createdAt };
        case 'tool_use': {
            const inputJson = typeof it.input === 'string' ? it.input : JSON.stringify(it.input ?? {}, null, 2);
            const call: ToolCallInfo = {
                toolName: it.toolName || 'tool',
                input: inputJson,
            };
            if (it.toolCallId) call.toolCallId = it.toolCallId;
            return {
                id: cryptoId(),
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
                id: cryptoId(),
                kind: 'tool_result',
                content: it.content,
                isError: !!it.isError,
                createdAt,
                ...(it.toolCallId ? { toolCallId: it.toolCallId } : {}),
            };
    }
}
