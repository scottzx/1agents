// Preact hooks wrapping the backend chat WebSocket.
//
// Owns one WebSocket per Task session; translates events into
// a React-friendly stream of "messages" (assistant text, tool calls,
// permission requests, errors). The ChatPanel renders that stream.

import { useEffect, useRef, useState, useCallback } from 'preact/hooks';
import type { ChatSession } from '../types';

export type ChatItem =
    | { id: string; kind: 'user'; content: string; createdAt: number }
    | { id: string; kind: 'assistant_text'; content: string; createdAt: number; streaming: boolean }
    | { id: string; kind: 'thinking'; content: string; createdAt: number }
    | { id: string; kind: 'tool_use'; toolName: string; input: string; createdAt: number; toolCallId?: string }
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

export function useBridge(session: ChatSession | null, seed: ChatItem[] = []): UseBridgeState {
    const [items, setItems] = useState<ChatItem[]>(seed);
    const [connection, setConnection] = useState<ConnectionState>('idle');
    const [typing, setTyping] = useState(false);
    const wsRef = useRef<WebSocket | null>(null);

    const appendItem = useCallback((item: ChatItem) => {
        setItems(prev => [...prev, item]);
    }, []);

    useEffect(() => {
        if (!session) return;
        let cancelled = false;

        setConnection('connecting');
        setItems([]);

        const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const taskId = session.taskId || '';
        const wsUrl = `${wsProto}//${window.location.host}/api/agent/chat/ws?workspace_id=${encodeURIComponent(session.workspaceId)}&task_id=${encodeURIComponent(taskId)}&session_id=${encodeURIComponent(session.id)}&agent_type=${encodeURIComponent(session.agentType)}`;

        console.log('[useBridge] Connecting to backend websocket:', wsUrl);
        const ws = new WebSocket(wsUrl);
        wsRef.current = ws;

        ws.onopen = () => {
            if (cancelled) return;
            setConnection('connected');
            ws.send(JSON.stringify({ action: 'get_history', sessionId: session.id }));
        };

        ws.onmessage = e => {
            if (cancelled) return;
            let payload: {
                event: string;
                text?: string;
                type?: string;
                arguments?: unknown;
                requestId?: string;
                message?: string;
                code?: string;
                toolName?: string;
                messages?: Array<{ role: string; text: string }>;
            };
            try {
                payload = JSON.parse(e.data) as typeof payload;
            } catch (err) {
                console.error('[useBridge] Failed to parse message:', err);
                return;
            }

            const event = payload.event;
            console.log('[useBridge] Received event:', event, payload);

            switch (event) {
                case 'session_ready':
                    break;
                case 'text_delta': {
                    const delta = payload.text;
                    const type = payload.type || 'output';
                    if (!delta) return;

                    setItems(prev => {
                        const next = [...prev];
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
                        return next;
                    });
                    break;
                }
                case 'tool_call': {
                    const argsString =
                        typeof payload.arguments === 'string'
                            ? payload.arguments
                            : JSON.stringify(payload.arguments, null, 2);
                    appendItem({
                        id: cryptoId(),
                        kind: 'tool_use',
                        toolName: payload.toolName || 'tool',
                        input: argsString,
                        createdAt: Date.now(),
                    });
                    break;
                }
                case 'permission_request': {
                    const argsString =
                        typeof payload.arguments === 'string'
                            ? payload.arguments
                            : JSON.stringify(payload.arguments, null, 2);
                    appendItem({
                        id: cryptoId(),
                        kind: 'permission',
                        requestId: payload.requestId || '',
                        toolName: payload.toolName || 'tool',
                        input: argsString,
                        createdAt: Date.now(),
                        options: [],
                    });
                    break;
                }
                case 'permission_timeout': {
                    setItems(prev =>
                        prev.map(it =>
                            it.kind === 'permission' && it.requestId === payload.requestId
                                ? { ...it, resolved: 'deny' }
                                : it
                        )
                    );
                    appendItem({
                        id: cryptoId(),
                        kind: 'error',
                        content: payload.message || 'Permission request timed out.',
                        createdAt: Date.now(),
                    });
                    break;
                }
                case 'done':
                    setItems(prev => {
                        const next = [...prev];
                        const last = next[next.length - 1];
                        if (last && last.kind === 'assistant_text' && last.streaming) {
                            next[next.length - 1] = {
                                ...last,
                                streaming: false,
                            };
                        }
                        return next;
                    });
                    setTyping(false);
                    break;
                case 'history_response': {
                    const historyMessages = payload.messages || [];
                    const converted: ChatItem[] = historyMessages.map(m => ({
                        id: cryptoId(),
                        kind: (m.role === 'user' ? 'user' : 'assistant_text') as 'user' | 'assistant_text',
                        content: m.text,
                        createdAt: Date.now(),
                        streaming: false,
                    }));
                    setItems(converted);
                    break;
                }
                case 'error':
                    appendItem({
                        id: cryptoId(),
                        kind: 'error',
                        content: payload.message || payload.code || 'Unknown error',
                        createdAt: Date.now(),
                    });
                    setTyping(false);
                    break;
            }
        };

        ws.onclose = () => {
            if (cancelled) return;
            setConnection('closed');
            setTyping(false);
        };

        ws.onerror = () => {
            if (cancelled) return;
            setConnection('error');
            setTyping(false);
        };

        return () => {
            cancelled = true;
            if (wsRef.current) {
                if (wsRef.current.readyState === WebSocket.OPEN) {
                    wsRef.current.send(JSON.stringify({ action: 'close_session', sessionId: session.id }));
                }
                wsRef.current.close();
                wsRef.current = null;
            }
            setConnection('closed');
        };
    }, [session?.id, session?.workspaceId, session?.taskId]);

    const send = useCallback(
        (content: string) => {
            if (!session || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
            const msgId = cryptoId();
            appendItem({
                id: msgId,
                kind: 'user',
                content,
                createdAt: Date.now(),
            });
            wsRef.current.send(
                JSON.stringify({
                    action: 'prompt',
                    sessionId: session.id,
                    text: content,
                })
            );
            setTyping(true);
        },
        [session, appendItem]
    );

    const cancel = useCallback(() => {
        if (!session || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
        wsRef.current.send(
            JSON.stringify({
                action: 'cancel',
                sessionId: session.id,
            })
        );
        setTyping(false);
    }, [session]);

    const respondPermission = useCallback(
        (requestId: string, allow: boolean) => {
            if (!session || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
            wsRef.current.send(
                JSON.stringify({
                    action: 'respond_permission',
                    sessionId: session.id,
                    requestId,
                    behavior: allow ? 'allow' : 'deny',
                })
            );
            setItems(prev =>
                prev.map(it =>
                    it.kind === 'permission' && it.requestId === requestId
                        ? { ...it, resolved: allow ? 'allow' : 'deny' }
                        : it
                )
            );
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
