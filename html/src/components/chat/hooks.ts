// Preact hooks wrapping the backend chat WebSocket.
//
// Owns one WebSocket per Task session; translates events into
// a React-friendly stream of "messages" (assistant text, tool calls,
// permission requests, errors). The ChatPanel renders that stream.

import { useEffect, useState, useCallback } from 'preact/hooks';
import type { ChatSession } from '../types';

export interface ToolCallInfo {
    toolName: string;
    input: string;
    toolCallId?: string;
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
    | {
          id: string;
          kind: 'permission';
          requestId: string;
          toolName: string;
          input: string;
          createdAt: number;
          options: Array<{ text: string; data: string }>;
          resolved?: 'allow' | 'deny';
      }
    | { id: string; kind: 'error'; content: string; createdAt: number };

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'closed' | 'error';

interface UseBridgeState {
    items: ChatItem[];
    connection: ConnectionState;
    typing: boolean;
    send: (content: string) => void;
    cancel: () => void;
    respondPermission: (requestId: string, allow: boolean) => void;
}

export interface SessionBridgeState {
    items: ChatItem[];
    connection: ConnectionState;
    typing: boolean;
    ws: WebSocket | null;
    listeners: Set<() => void>;
    turnStarted: boolean;
}

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
                        next[next.length - 1] = {
                            ...last,
                            calls: [...last.calls, newCall],
                        };
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
                case 'permission_request': {
                    if (!state.turnStarted) break;
                    const argsString =
                        typeof payload.arguments === 'string'
                            ? payload.arguments
                            : JSON.stringify(payload.arguments, null, 2);
                    state.items = [
                        ...state.items,
                        {
                            id: cryptoId(),
                            kind: 'permission',
                            requestId: payload.requestId || '',
                            toolName: payload.toolName || 'tool',
                            input: argsString,
                            createdAt: Date.now(),
                            options: [],
                        },
                    ];
                    this.notify(state);
                    break;
                }
                case 'permission_timeout': {
                    if (!state.turnStarted) break;
                    state.items = state.items.map(it =>
                        it.kind === 'permission' && it.requestId === payload.requestId
                            ? { ...it, resolved: 'deny' }
                            : it
                    );
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
                    state.items = [
                        ...state.items,
                        {
                            id: cryptoId(),
                            kind: 'error',
                            content: payload.message || payload.code || 'Unknown error',
                            createdAt: Date.now(),
                        },
                    ];
                    state.typing = false;
                    state.turnStarted = false;
                    this.notify(state);
                    this.reloadHistory(session, state);
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

    respondPermission(session: ChatSession, requestId: string, allow: boolean) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
        state.ws.send(
            JSON.stringify({
                action: 'respond_permission',
                sessionId: session.id,
                requestId,
                behavior: allow ? 'allow' : 'deny',
            })
        );
        state.items = state.items.map(it =>
            it.kind === 'permission' && it.requestId === requestId ? { ...it, resolved: allow ? 'allow' : 'deny' } : it
        );
        this.notify(state);
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
        (requestId: string, allow: boolean) => {
            if (!session) return;
            globalBridgeManager.respondPermission(session, requestId, allow);
        },
        [session]
    );

    return { items, connection, typing, send, cancel, respondPermission };
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
