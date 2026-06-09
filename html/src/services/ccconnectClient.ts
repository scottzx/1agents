// Direct client for cc-connect.
//
// Talks to cc-connect's existing public surface:
//   - REST: /api/v1/projects/{project}/sessions[...] (and friends)
//   - WS:   /bridge/ws (the bridge protocol, see modules/cc-connect/docs/bridge-protocol.md)
//
// Authentication: every request carries the ManagementToken, fetched
// from the 1agents backend's existing /api/cc-connect/url endpoint.
// WS connections also pass it as ?token=… (browsers can't set headers).
//
// IMPORTANT: wire types here mirror cc-connect's bridge-protocol.md
// 1:1. If cc-connect renames a field, this file needs to follow.

import type { AgentType } from '../components/types';

export interface CcAuth {
    token: string;
}

export interface CcSessionMeta {
    id: string;
    name?: string;
    history_count?: number;
}

export interface CcSessionList {
    sessions: CcSessionMeta[];
    active_session_id?: string;
}

export interface CcHistoryEntry {
    role: 'user' | 'assistant';
    content: string;
}

export interface CcSessionWithHistory {
    id: string;
    name?: string;
    history: CcHistoryEntry[];
}

export interface CreateCcSessionRequest {
    session_key: string;
    name?: string;
}

export interface CreateCcSessionResponse {
    id: string;
    name?: string;
    message?: string;
}

// ── Auth ──────────────────────────────────────────────────────────────

let _authCache: { token: string; fetchedAt: number } | null = null;
const AUTH_TTL_MS = 5 * 60 * 1000;

/**
 * Fetch the cc-connect ManagementToken from the 1agents backend.
 *
 * The backend exposes /api/cc-connect/url which returns a full login
 * URL; the token is embedded in that URL. We extract it from the
 * `token=` query parameter.
 */
export async function getCcAuth(workspaceId: string): Promise<CcAuth> {
    if (_authCache && Date.now() - _authCache.fetchedAt < AUTH_TTL_MS) {
        return { token: _authCache.token };
    }
    const res = await fetch('/api/cc-connect/url', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspace: workspaceId, theme: 'light', lang: 'zh' }),
    });
    if (!res.ok) throw new Error(`failed to fetch cc-connect auth: ${res.status}`);
    const { url } = (await res.json()) as { url: string };
    const m = /[?&]token=([^&]+)/.exec(url);
    if (!m) throw new Error('cc-connect auth url did not include a token');
    const token = decodeURIComponent(m[1]);
    _authCache = { token, fetchedAt: Date.now() };
    return { token };
}

/** Clear the cached token (e.g. after a 401). */
export function clearCcAuthCache(): void {
    _authCache = null;
}

// ── REST helpers ──────────────────────────────────────────────────────

/**
 * Build a project name to use for /api/v1/projects/{project}/...
 *
 * cc-connect project names are globally unique per cc-connect instance.
 * We compose `<workspaceNameSlug>__<agentType>` to disambiguate per-agent
 * projects within a single workspace. (We don't actually create the
 * project here — cc-connect's workspace→project sync in
 * backend/internal/ccconnect/runner.go registers `claudecode` projects
 * automatically. For other agent types the user has to configure them
 * in the cc-connect admin UI; this function is just the convention.)
 */
export function ccProjectName(workspaceName: string, agentType: AgentType): string {
    const slug = workspaceName.replace(/[^a-zA-Z0-9_-]+/g, '_').slice(0, 32) || 'ws';
    return `${slug}__${agentType}`;
}

async function ccFetch<T>(project: string, path: string, init: RequestInit & { token: string }): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set('Authorization', `Bearer ${init.token}`);
    if (init.body && !headers.has('Content-Type')) {
        headers.set('Content-Type', 'application/json');
    }
    const url = path.startsWith('/')
        ? `/api/v1/projects/${encodeURIComponent(project)}${path}`
        : `/api/v1/projects/${encodeURIComponent(project)}/${path}`;
    const res = await fetch(url, { ...init, headers });
    if (!res.ok) {
        const body = await res.text().catch(() => '');
        throw new Error(`cc-connect ${init.method ?? 'GET'} ${url} → ${res.status}: ${body}`);
    }
    return (await res.json()) as T;
}

/**
 * Shape of every response from /api/v1 — see docs/management-api.md.
 *   {"ok": true, "data": {...}} | {"ok": false, "error": "..."}
 */
interface CcEnvelope<T> {
    ok: boolean;
    data?: T;
    error?: string;
}

function unwrap<T>(env: CcEnvelope<T>): T {
    if (!env.ok || env.data === undefined) {
        throw new Error(env.error ?? 'cc-connect returned non-ok response');
    }
    return env.data;
}

// ── Project / session REST ───────────────────────────────────────────

export async function ccCreateSession(
    project: string,
    body: CreateCcSessionRequest,
    token: string
): Promise<CreateCcSessionResponse> {
    const env = await ccFetch<CcEnvelope<CreateCcSessionResponse>>(project, '/sessions', {
        method: 'POST',
        body: JSON.stringify(body),
        token,
    });
    return unwrap(env);
}

export async function ccListSessions(project: string, sessionKey: string, token: string): Promise<CcSessionList> {
    const env = await ccFetch<CcEnvelope<CcSessionList>>(
        project,
        `/sessions?session_key=${encodeURIComponent(sessionKey)}`,
        { method: 'GET', token }
    );
    return unwrap(env);
}

export async function ccGetSession(
    project: string,
    sessionId: string,
    sessionKey: string,
    historyLimit: number,
    token: string
): Promise<CcSessionWithHistory> {
    const env = await ccFetch<CcEnvelope<CcSessionWithHistory>>(
        project,
        `/sessions/${encodeURIComponent(sessionId)}?session_key=${encodeURIComponent(sessionKey)}&history_limit=${historyLimit}`,
        { method: 'GET', token }
    );
    return unwrap(env);
}

export async function ccDeleteSession(
    project: string,
    sessionId: string,
    sessionKey: string,
    token: string
): Promise<void> {
    const env = await ccFetch<CcEnvelope<{ message?: string }>>(
        project,
        `/sessions/${encodeURIComponent(sessionId)}?session_key=${encodeURIComponent(sessionKey)}`,
        { method: 'DELETE', token }
    );
    unwrap(env);
}

// ── Bridge WebSocket ─────────────────────────────────────────────────

/**
 * Bridge protocol message envelopes. See
 * modules/cc-connect/docs/bridge-protocol.md for the canonical spec.
 *
 * We declare a SUBSET of types — only the ones we need for chat. The
 * protocol uses single-frame newline-delimited JSON over WS text frames.
 */

export type BridgeOutbound =
    | { type: 'register'; platform: string; capabilities: string[]; metadata: Record<string, unknown> }
    | {
          type: 'message';
          msg_id: string;
          session_key: string;
          user_id: string;
          user_name?: string;
          content: string;
          reply_ctx: string;
          images?: Array<{ mime_type: string; data: string; file_name?: string }>;
          files?: Array<{ mime_type: string; data: string; file_name: string }>;
      }
    | { type: 'card_action'; session_key: string; action: string; reply_ctx: string }
    | { type: 'ping'; ts: number };

export type BridgeInbound =
    | { type: 'register_ack'; ok: boolean; error?: string }
    | { type: 'reply'; session_key: string; reply_ctx: string; content: string; format?: string }
    | {
          type: 'reply_stream';
          session_key: string;
          reply_ctx: string;
          delta: string;
          full_text: string;
          preview_handle?: string;
          done: boolean;
      }
    | { type: 'preview_start'; ref_id: string; session_key: string; reply_ctx: string; content: string }
    | { type: 'update_message'; session_key: string; preview_handle: string; content: string }
    | { type: 'delete_message'; session_key: string; preview_handle: string }
    | { type: 'card'; session_key: string; reply_ctx: string; card: unknown }
    | {
          type: 'buttons';
          session_key: string;
          reply_ctx: string;
          content: string;
          buttons: Array<Array<{ text: string; data: string }>>;
      }
    | { type: 'typing_start'; session_key: string; reply_ctx: string }
    | { type: 'typing_stop'; session_key: string; reply_ctx: string }
    | { type: 'image'; session_key: string; reply_ctx: string; data: string; mime_type: string; file_name?: string }
    | { type: 'file'; session_key: string; reply_ctx: string; data: string; mime_type: string; file_name: string }
    | { type: 'audio'; session_key: string; reply_ctx: string; data: string; format: string }
    | { type: 'pong'; ts: number }
    | { type: 'error'; code: string; message: string };

export type BridgeEvent =
    | { type: 'open' }
    | { type: 'close'; code: number; reason: string }
    | { type: 'registered' }
    | { type: 'register_error'; error: string }
    | { type: 'message'; payload: Extract<BridgeInbound, { type: 'reply' }> }
    | { type: 'stream'; payload: Extract<BridgeInbound, { type: 'reply_stream' }> }
    | { type: 'typing'; on: boolean; session_key: string }
    | { type: 'permission_request'; payload: Extract<BridgeInbound, { type: 'buttons' }> }
    | { type: 'card'; payload: Extract<BridgeInbound, { type: 'card' }> }
    | { type: 'image'; payload: Extract<BridgeInbound, { type: 'image' }> }
    | { type: 'file'; payload: Extract<BridgeInbound, { type: 'file' }> }
    | { type: 'pong' }
    | { type: 'error'; code: string; message: string };

export type BridgeEventHandler = (ev: BridgeEvent) => void;

export interface BridgeSocketOptions {
    /** Bridge WebSocket URL. Defaults to `ws(s)://<current host>/bridge/ws`. */
    url?: string;
    /** Auth token, passed as `?token=…`. */
    token: string;
    /** Platform name to register under (cc-connect uses this in session_key scope). */
    platform?: string;
    /** Capabilities to advertise — must include "text" at minimum. */
    capabilities?: string[];
    /** Ping interval (ms). Default 25_000. */
    pingIntervalMs?: number;
    /** Auto-reconnect on close. Default true. */
    autoReconnect?: boolean;
}

/**
 * BridgeSocket is a thin client for the cc-connect bridge WebSocket.
 *
 * Lifecycle:
 *   1. connect() — opens the WS, sends `register` on open
 *   2. sendMessage / sendCardAction / sendPing — outbound helpers
 *   3. on(handler) — receive typed events
 *   4. disconnect() — close cleanly
 *
 * Reconnection: on close (when autoReconnect is true), schedules a
 * reconnect with exponential backoff (1s → 30s).
 */
export class BridgeSocket {
    private ws: WebSocket | null = null;
    private handlers: Set<BridgeEventHandler> = new Set();
    private pingTimer: ReturnType<typeof setInterval> | null = null;
    private reconnectAttempt = 0;
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    private closedByUser = false;
    private readonly opts: Required<BridgeSocketOptions>;

    constructor(opts: BridgeSocketOptions) {
        this.opts = {
            url: opts.url ?? defaultBridgeUrl(),
            token: opts.token,
            platform: opts.platform ?? 'oneagents-web',
            capabilities: opts.capabilities ?? [
                'text',
                'image',
                'file',
                'card',
                'buttons',
                'typing',
                'update_message',
                'preview',
            ],
            pingIntervalMs: opts.pingIntervalMs ?? 25_000,
            autoReconnect: opts.autoReconnect ?? true,
        };
    }

    connect(): void {
        this.closedByUser = false;
        const url = `${this.opts.url}?token=${encodeURIComponent(this.opts.token)}`;
        const ws = new WebSocket(url);
        this.ws = ws;

        ws.addEventListener('open', () => {
            this.reconnectAttempt = 0;
            this.send({
                type: 'register',
                platform: this.opts.platform,
                capabilities: this.opts.capabilities,
                metadata: {
                    protocol_version: 1,
                    source: 'oneagents-web',
                },
            });
            this.emit({ type: 'open' });
            this.startPing();
        });

        ws.addEventListener('message', e => {
            let parsed: BridgeInbound;
            try {
                parsed = JSON.parse(typeof e.data === 'string' ? e.data : '');
            } catch {
                return;
            }
            this.routeInbound(parsed);
        });

        ws.addEventListener('close', e => {
            this.stopPing();
            this.emit({ type: 'close', code: e.code, reason: e.reason });
            if (this.opts.autoReconnect && !this.closedByUser) {
                this.scheduleReconnect();
            }
        });

        ws.addEventListener('error', () => {
            // Browsers don't expose detail here. The close event will
            // follow and trigger reconnect.
        });
    }

    disconnect(): void {
        this.closedByUser = true;
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
        this.stopPing();
        this.ws?.close();
        this.ws = null;
    }

    /** Subscribe to typed events. Returns an unsubscribe function. */
    on(handler: BridgeEventHandler): () => void {
        this.handlers.add(handler);
        return () => this.handlers.delete(handler);
    }

    sendMessage(args: {
        msgId: string;
        sessionKey: string;
        userId: string;
        userName?: string;
        content: string;
        images?: Array<{ mime_type: string; data: string; file_name?: string }>;
        files?: Array<{ mime_type: string; data: string; file_name: string }>;
    }): void {
        this.send({
            type: 'message',
            msg_id: args.msgId,
            session_key: args.sessionKey,
            user_id: args.userId,
            user_name: args.userName,
            content: args.content,
            reply_ctx: args.sessionKey, // 1agents uses session_key as reply_ctx
            images: args.images,
            files: args.files,
        });
    }

    sendCardAction(args: { sessionKey: string; action: string }): void {
        this.send({
            type: 'card_action',
            session_key: args.sessionKey,
            action: args.action,
            reply_ctx: args.sessionKey,
        });
    }

    sendPing(): void {
        this.send({ type: 'ping', ts: Date.now() });
    }

    // ── internals ─────────────────────────────────────────────────────

    private send(msg: BridgeOutbound): void {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
        this.ws.send(JSON.stringify(msg));
    }

    private startPing(): void {
        this.stopPing();
        this.pingTimer = setInterval(() => this.sendPing(), this.opts.pingIntervalMs);
    }

    private stopPing(): void {
        if (this.pingTimer) {
            clearInterval(this.pingTimer);
            this.pingTimer = null;
        }
    }

    private scheduleReconnect(): void {
        if (this.closedByUser) return;
        const delay = Math.min(30_000, 1000 * Math.pow(2, this.reconnectAttempt));
        this.reconnectAttempt++;
        this.reconnectTimer = setTimeout(() => this.connect(), delay);
    }

    private routeInbound(msg: BridgeInbound): void {
        switch (msg.type) {
            case 'register_ack':
                if (msg.ok) this.emit({ type: 'registered' });
                else this.emit({ type: 'register_error', error: msg.error ?? 'unknown' });
                return;
            case 'reply':
                this.emit({ type: 'message', payload: msg });
                return;
            case 'reply_stream':
                this.emit({ type: 'stream', payload: msg });
                return;
            case 'typing_start':
                this.emit({ type: 'typing', on: true, session_key: msg.session_key });
                return;
            case 'typing_stop':
                this.emit({ type: 'typing', on: false, session_key: msg.session_key });
                return;
            case 'buttons':
                // Detect permission buttons (data starts with "perm:")
                if (msg.buttons.some(row => row.some(b => b.data.startsWith('perm:')))) {
                    this.emit({ type: 'permission_request', payload: msg });
                }
                return;
            case 'card':
                this.emit({ type: 'card', payload: msg });
                return;
            case 'image':
                this.emit({ type: 'image', payload: msg });
                return;
            case 'file':
                this.emit({ type: 'file', payload: msg });
                return;
            case 'pong':
                this.emit({ type: 'pong' });
                return;
            case 'error':
                this.emit({ type: 'error', code: msg.code, message: msg.message });
                return;
            // preview_start / update_message / delete_message / audio are
            // currently ignored by the chat panel — the engine's preview
            // pipeline is for chat-streaming cards, not for tool messages.
        }
    }

    private emit(ev: BridgeEvent): void {
        for (const h of this.handlers) h(ev);
    }
}

function defaultBridgeUrl(): string {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}/bridge/ws`;
}
