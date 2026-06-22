/**
 * RelayChatSocket — a minimal WebSocket-shaped transport that carries the Agent
 * chat stream over the relay instead of a same-origin `wss://…/api/agent/chat/ws`
 * (issue #17). It mimics just the surface the ChatBridgeManager uses
 * (onopen/onmessage/onclose/onerror + send/close/readyState) so the chat manager
 * stays unchanged apart from choosing which socket to construct.
 *
 * Down (node → H5): the node bridge mirrors every Go chat event into a Happy
 * session message; we subscribe to the shared relay `socket.on('update')`, filter
 * by happySessionId, decrypt the content with the machine key, and re-emit the
 * verbatim Go `WsMessage` JSON via onmessage — so the existing parser is reused.
 * Up (H5 → node): send() forwards the raw action JSON over an RPC.
 *
 * Transient gaps (e.g. relay reconnect) self-heal: the manager reloads authoritative
 * history from the node on every turn `done`, so lost deltas are backfilled there.
 */
import type { Socket } from 'socket.io-client';
import { decrypt, decodeBase64 } from './crypto';
import { openChat, sendChat, closeChat, type RelayMachine, type RelayChatParams } from './relayClient';

/** The subset of the browser WebSocket API the chat manager relies on. */
export interface ChatTransport {
    onopen: ((ev?: unknown) => void) | null;
    onmessage: ((ev: { data: string }) => void) | null;
    onclose: ((ev?: unknown) => void) | null;
    onerror: ((ev?: unknown) => void) | null;
    readyState: number;
    send(data: string): void;
    close(): void;
}

// Mirror WebSocket.{CONNECTING,OPEN,CLOSED} numeric constants.
const CONNECTING = 0;
const OPEN = 1;
const CLOSED = 3;

export class RelayChatSocket implements ChatTransport {
    onopen: ((ev?: unknown) => void) | null = null;
    onmessage: ((ev: { data: unknown }) => void) | null = null;
    onclose: ((ev?: unknown) => void) | null = null;
    onerror: ((ev?: unknown) => void) | null = null;
    readyState = CONNECTING;

    private happySessionId: string | null = null;
    private updateHandler: ((data: unknown) => void) | null = null;
    private disconnectHandler: (() => void) | null = null;

    constructor(
        private socket: Socket,
        private machine: RelayMachine,
        private params: RelayChatParams
    ) {
        // start() resolves on a later tick, after the manager has assigned its
        // onopen/onmessage handlers synchronously following construction.
        void this.start();
    }

    private async start(): Promise<void> {
        // Subscribe to relay updates BEFORE opening the chat, so a message
        // emitted while the open RPC is still in flight isn't dropped. A warm
        // backend bridge re-emits `session_ready` synchronously on reconnect —
        // faster than the open RPC round-trip — so registering the listener
        // only after `await openChat` would miss it and leave the chat stuck
        // "initializing". (A direct WebSocket can't deliver a message before
        // its onmessage is assigned, so this race is unique to the relay path.)
        // Until our happySessionId is known we can't filter by it, so buffer
        // every update and replay it through the same filter once open resolves.
        const pending: unknown[] = [];
        this.updateHandler = (data: unknown) => {
            if (this.happySessionId === null) {
                pending.push(data);
                return;
            }
            this.handleUpdate(data);
        };
        this.socket.on('update', this.updateHandler);

        this.disconnectHandler = () => this.fail();
        this.socket.on('disconnect', this.disconnectHandler);

        try {
            const { happySessionId } = await openChat(this.socket, this.machine, this.params);
            if (this.readyState === CLOSED) return; // closed before open resolved
            this.happySessionId = happySessionId;

            this.readyState = OPEN;
            this.onopen?.();

            // Replay anything that landed during the open RPC round-trip.
            for (const data of pending) this.handleUpdate(data);
            pending.length = 0;
        } catch {
            this.fail();
        }
    }

    /** Filter a relay `update` down to this session and re-emit its Go WsMessage. */
    private handleUpdate(data: unknown): void {
        // new-message payload (happy-server eventRouter.buildNewMessageUpdate):
        //   { body: { t: 'new-message', sid, message: { content: { t, c } } } }
        const body = (data as { body?: Record<string, unknown> } | undefined)?.body;
        if (!body || body.t !== 'new-message') return;
        if (body.sid !== this.happySessionId) return;
        const message = body.message as { content?: { t?: string; c?: string } } | undefined;
        const content = message?.content;
        if (!content || content.t !== 'encrypted' || !content.c) return;
        void this.deliver(content.c);
    }

    private async deliver(c: string): Promise<void> {
        const obj = await decrypt(this.machine.encryptionKey, this.machine.variant, decodeBase64(c));
        if (obj === null || obj === undefined) return;
        // Bridge sentinel: the node-side Go WS closed → end this transport.
        if (typeof obj === 'object' && (obj as { event?: string }).event === '__relay_closed') {
            this.fail();
            return;
        }
        this.onmessage?.({ data: JSON.stringify(obj) });
    }

    send(data: string): void {
        if (this.readyState !== OPEN || !this.happySessionId) return;
        void sendChat(this.socket, this.machine, this.params.sessionId, data);
    }

    close(): void {
        if (this.readyState === CLOSED) return;
        this.readyState = CLOSED;
        this.cleanup();
        void closeChat(this.socket, this.machine, this.params.sessionId);
        this.onclose?.();
    }

    /** Unexpected end (open failed, relay dropped, or bridge sentinel). */
    private fail(): void {
        if (this.readyState === CLOSED) return;
        this.readyState = CLOSED;
        this.cleanup();
        this.onclose?.();
    }

    private cleanup(): void {
        if (this.updateHandler) {
            this.socket.off('update', this.updateHandler);
            this.updateHandler = null;
        }
        if (this.disconnectHandler) {
            this.socket.off('disconnect', this.disconnectHandler);
            this.disconnectHandler = null;
        }
    }
}
