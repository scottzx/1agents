// Preact hooks wrapping the cc-connect bridge WebSocket.
//
// Owns one BridgeSocket per ChatSession; translates bridge events into
// a React-friendly stream of "messages" (assistant text, tool calls,
// permission requests, errors). The ChatPanel renders that stream.

import { useEffect, useRef, useState, useCallback } from 'preact/hooks';
import { BridgeSocket, getCcAuth, type BridgeEvent } from '../../services/ccconnectClient';
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
    respondPermission: (requestId: string, allow: boolean) => void;
}

/**
 * Connect to cc-connect's bridge WS for the given ChatSession, translate
 * events to ChatItems, and expose send / respondPermission helpers.
 *
 * History replay: not handled here. Use ccconnectClient.ccGetSession on
 * mount to load the last N history entries and seed `items` with them.
 */
export function useBridge(
    session: ChatSession | null,
    seed: ChatItem[] = [],
    pendingInitialMessage?: string | null,
    onClearPendingInitialMessage?: () => void
): UseBridgeState {
    const [items, setItems] = useState<ChatItem[]>(seed);
    const [connection, setConnection] = useState<ConnectionState>('idle');
    const [typing, setTyping] = useState(false);
    const sockRef = useRef<BridgeSocket | null>(null);
    const itemsRef = useRef<ChatItem[]>(seed);

    const pendingMsgRef = useRef(pendingInitialMessage);
    const onClearPendingRef = useRef(onClearPendingInitialMessage);

    useEffect(() => {
        pendingMsgRef.current = pendingInitialMessage;
    }, [pendingInitialMessage]);

    useEffect(() => {
        onClearPendingRef.current = onClearPendingInitialMessage;
    }, [onClearPendingInitialMessage]);

    // Keep ref in sync so the bridge callbacks see the latest items.
    useEffect(() => {
        itemsRef.current = items;
    }, [items]);

    const appendItem = useCallback((item: ChatItem) => {
        setItems(prev => [...prev, item]);
    }, []);

    useEffect(() => {
        if (!session) return;
        let cancelled = false;

        setConnection('connecting');
        setItems(seed);

        (async () => {
            try {
                const { token } = await getCcAuth(session.workspaceId);
                if (cancelled) return;

                const sock = new BridgeSocket({ token, platform: 'oneagents-web' });
                sockRef.current = sock;

                sock.on((ev: BridgeEvent) => {
                    switch (ev.type) {
                        case 'open':
                            setConnection('connected');
                            return;
                        case 'registered':
                            setConnection('connected');
                            if (pendingMsgRef.current && sockRef.current) {
                                const initialMsg = pendingMsgRef.current;
                                pendingMsgRef.current = null;
                                const msgId = cryptoId();
                                appendItem({
                                    id: msgId,
                                    kind: 'user',
                                    content: initialMsg,
                                    createdAt: Date.now(),
                                });
                                sockRef.current.sendMessage({
                                    msgId,
                                    sessionKey: session.sessionKey,
                                    userId: 'oneagents-user',
                                    userName: 'You',
                                    content: initialMsg,
                                });
                                setTyping(true);
                                onClearPendingRef.current?.();
                            }
                            return;
                        case 'close':
                            setConnection('reconnecting');
                            return;
                        case 'register_error':
                            setConnection('error');
                            appendItem({
                                id: cryptoId(),
                                kind: 'error',
                                content: `注册失败: ${ev.error}`,
                                createdAt: Date.now(),
                            });
                            return;
                        case 'stream': {
                            const delta = ev.payload.delta;
                            if (!delta) return;
                            // Append to the most recent assistant_text item,
                            // or create a new one.
                            setItems(prev => {
                                const next = [...prev];
                                const last = next[next.length - 1];
                                if (last && last.kind === 'assistant_text' && last.streaming) {
                                    next[next.length - 1] = {
                                        ...last,
                                        content: last.content + delta,
                                        streaming: !ev.payload.done,
                                    };
                                } else {
                                    next.push({
                                        id: cryptoId(),
                                        kind: 'assistant_text',
                                        content: delta,
                                        createdAt: Date.now(),
                                        streaming: !ev.payload.done,
                                    });
                                }
                                if (ev.payload.done) setTyping(false);
                                return next;
                            });
                            return;
                        }
                        case 'message': {
                            // Complete final reply.
                            setItems(prev => {
                                const next = [...prev];
                                const last = next[next.length - 1];
                                if (last && last.kind === 'assistant_text' && last.streaming) {
                                    next[next.length - 1] = {
                                        ...last,
                                        content: ev.payload.content,
                                        streaming: false,
                                    };
                                } else {
                                    next.push({
                                        id: cryptoId(),
                                        kind: 'assistant_text',
                                        content: ev.payload.content,
                                        createdAt: Date.now(),
                                        streaming: false,
                                    });
                                }
                                return next;
                            });
                            setTyping(false);
                            return;
                        }
                        case 'typing':
                            setTyping(ev.on);
                            return;
                        case 'permission_request': {
                            const buttons = ev.payload.buttons.flat();
                            const permButton = buttons.find(b => b.data.startsWith('perm:'));
                            if (!permButton) return;
                            // Extract request id from "perm:<req>:<allow|deny>".
                            const parts = permButton.data.split(':');
                            const requestId = parts[1] ?? '';
                            appendItem({
                                id: cryptoId(),
                                kind: 'permission',
                                requestId,
                                toolName: extractToolName(ev.payload.content),
                                input: ev.payload.content,
                                createdAt: Date.now(),
                                options: buttons,
                            });
                            return;
                        }
                        case 'error':
                            appendItem({
                                id: cryptoId(),
                                kind: 'error',
                                content: `${ev.code}: ${ev.message}`,
                                createdAt: Date.now(),
                            });
                            return;
                    }
                });

                sock.connect();
            } catch (e) {
                if (cancelled) return;
                setConnection('error');
                appendItem({
                    id: cryptoId(),
                    kind: 'error',
                    content: `连接失败: ${(e as Error).message}`,
                    createdAt: Date.now(),
                });
            }
        })();

        return () => {
            cancelled = true;
            sockRef.current?.disconnect();
            sockRef.current = null;
            setConnection('closed');
        };
        // seed intentionally not in deps — we consume it as initial state.
    }, [session?.id, session?.workspaceId, session?.sessionKey]);

    const send = useCallback(
        (content: string) => {
            if (!session || !sockRef.current) return;
            const msgId = cryptoId();
            appendItem({
                id: msgId,
                kind: 'user',
                content,
                createdAt: Date.now(),
            });
            sockRef.current.sendMessage({
                msgId,
                sessionKey: session.sessionKey,
                userId: 'oneagents-user',
                userName: 'You',
                content,
            });
            setTyping(true);
        },
        [session, appendItem]
    );

    const respondPermission = useCallback(
        (requestId: string, allow: boolean) => {
            if (!session || !sockRef.current) return;
            const action = `perm:${requestId}:${allow ? 'allow' : 'deny'}`;
            sockRef.current.sendCardAction({
                sessionKey: session.sessionKey,
                action,
            });
            // Mark the matching permission item as resolved locally.
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

    return { items, connection, typing, send, respondPermission };
}

function cryptoId(): string {
    if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
        return (crypto as Crypto).randomUUID();
    }
    return `id-${Math.random().toString(36).slice(2)}-${Date.now()}`;
}

function extractToolName(content: string): string {
    // Buttons content is typically "Allow tool execution: bash(rm -rf /tmp/old)?"
    const m = /^([a-zA-Z_][\w-]*)/.exec(content);
    return m ? m[1] : 'tool';
}
