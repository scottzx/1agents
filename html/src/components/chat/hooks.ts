// Preact hooks wrapping the backend chat WebSocket.
//
// Owns one WebSocket per Task session; translates events into
// a React-friendly stream of "messages" (assistant text, tool calls,
// permission requests, errors). The ChatPanel renders that stream.

import { useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import type { ChatSession, PermissionDecision, PermissionMode } from '../types';
// Imported for its side-effecting setter only; referenced exclusively inside
// method bodies (never at module-eval time) so the sessionStore ⇄ hooks import
// cycle stays safe — see the cycle note in stores/sessionStore.ts.
import { setLiveSessionStatus, setLiveSessionConnection } from '../../stores/sessionStore';
// Protocol types moved to the platform-agnostic core (Phase 0 carve). Re-exported
// below so existing `./hooks` importers (MessageList, ChatPanel, …) stay
// unchanged.
import type { ChatItem, ConnectionState } from '../../core/protocol/types';
// Pure protocol folds + helpers carved into core (Phase 0). ChatBridgeManager is
// now a thin transport adapter over these — it owns the WebSocket, listeners and
// reconnect bookkeeping; all conversation-state transforms live in the reducer.
import {
    cryptoId,
    hasRenderableArguments,
    normalizeHistory,
    applyTextDelta,
    applyPromptQueued,
    applyPromptCancelled,
    applyToolCall,
    applyToolResult,
    applyPermissionRequest,
    applyPermissionTimeout,
    applyDone,
    appendError,
    resolvePermissionSide,
    deriveLiveStatus,
} from '../../core/protocol/reducer';
import {
    getHistoryAction,
    promptAction,
    closeSessionAction,
    cancelQueuedAction,
    respondPermissionAction,
    setPermissionModeAction,
    type BridgeEventPayload,
} from '../../core/protocol/wireProtocol';
// Backend transport mode (direct same-origin vs relay) + the relay chat transport.
// In relay mode the chat WS rides the relay (issue #17); terminal stays direct.
import { backendTarget } from '../../core/services/apiClient';
import { RelayChatSocket, type ChatTransport } from '../../core/services/relay/relayChatSocket';

export type { ToolCallInfo, HistoryItem, ChatItem, ConnectionState } from '../../core/protocol/types';

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
    /** True when this connection was taken over by another tab/browser. */
    takenOver: boolean;
    /** Reconnect and reclaim ownership of the session (重试 button). */
    retry: () => void;
}

export interface SessionBridgeState {
    /** The owning session's id — used to publish live status into sessionStore. */
    sessionId: string;
    items: ChatItem[];
    connection: ConnectionState;
    typing: boolean;
    ws: ChatTransport | null;
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
    /**
     * True once this session has reached `session_ready` at least once (never
     * reset). Distinguishes a live session that merely dropped — which should
     * reconnect indefinitely — from one that never connected (e.g. resuming a
     * transcript whose session is unrecoverable), which should give up after a
     * few attempts instead of looping forever and spamming the console.
     */
    everReady: boolean;
    /** Per-session permission policy mirrored from the backend record. */
    permissionMode: PermissionMode;
    /** Exponential backoff level — incremented on each reconnect attempt, reset on session_ready. */
    reconnectAttempt: number;
    /** Pending setTimeout handle for the next reconnect; null when idle. */
    reconnectTimer: ReturnType<typeof setTimeout> | null;
    /** True when destroy() was called; prevents the onclose handler from scheduling a reconnect. */
    closedByUser: boolean;
    /**
     * True once this connection was taken over by a newer one (the bridge
     * emitted `session_taken_over` right before closing us). Suppresses the
     * onclose auto-reconnect so two tabs don't ping-pong the bridge, and
     * drives the "session opened elsewhere" banner. Cleared on the next
     * connect() (i.e. when the user hits 重试).
     */
    takenOver: boolean;
}

const DEFAULT_PERMISSION_MODE: PermissionMode = 'approve-reads';

export class ChatBridgeManager {
    private sessions = new Map<string, SessionBridgeState>();

    getOrCreate(session: ChatSession): SessionBridgeState {
        let state = this.sessions.get(session.id);
        if (!state) {
            state = {
                sessionId: session.id,
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
                everReady: false,
                // The list endpoint (GET /api/agent/sessions?workspace_id=…)
                // already serializes ChatSessionRecord.PermissionMode onto
                // the ChatSession object, so we can trust the field
                // verbatim instead of doing a second GET per session.
                permissionMode: session.permissionMode ?? DEFAULT_PERMISSION_MODE,
                reconnectAttempt: 0,
                reconnectTimer: null,
                closedByUser: false,
                takenOver: false,
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
            setLiveSessionStatus(sessionId, null);
            setLiveSessionConnection(sessionId, null);
        }
    }

    /**
     * Reclaim a session that was taken over by another tab/browser. Used by
     * the "重试" button on the takeover banner — reconnecting makes THIS
     * connection the newest, so the bridge hands ownership back here (and the
     * other tab gets its own takeover banner). connect() clears `takenOver`.
     */
    retry(session: ChatSession) {
        const state = this.sessions.get(session.id);
        if (!state) return;
        this.connect(session, state);
    }

    private connect(session: ChatSession, state: SessionBridgeState) {
        state.connection = 'connecting';
        // Reset `ready` on every (re)connect. The bridge-server emits a
        // fresh `session_ready` after each `ensure_session`, so we wait
        // for the new confirmation before letting the user act again.
        state.ready = false;
        // A fresh connect (incl. the user hitting 重试 after being taken
        // over) reclaims ownership, so clear the takeover flag/banner.
        state.takenOver = false;
        this.notify(state);

        // Pick the transport by how the backend is reached: relay mode tunnels the
        // chat stream over the relay (issue #17); direct mode keeps the same-origin WS.
        const target = backendTarget.value;
        let ws: ChatTransport;
        if (target.mode === 'relay') {
            ws = new RelayChatSocket(target.socket, target.machine, {
                workspaceId: session.workspaceId,
                taskId: session.taskId,
                sessionId: session.id,
                agentType: session.agentType,
                replyId: session.replyId,
            });
        } else {
            const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const taskId = session.taskId || '';
            const replyId = session.replyId || '';
            const wsUrl = `${wsProto}//${window.location.host}/api/agent/chat/ws?workspace_id=${encodeURIComponent(session.workspaceId)}&task_id=${encodeURIComponent(taskId)}&session_id=${encodeURIComponent(session.id)}&agent_type=${encodeURIComponent(session.agentType)}&reply_id=${encodeURIComponent(replyId)}`;
            console.log('[useBridgeManager] Connecting to backend websocket:', wsUrl);
            ws = new WebSocket(wsUrl) as unknown as ChatTransport;
        }
        state.ws = ws;

        ws.onopen = () => {
            state.connection = 'connected';
            this.notify(state);
            ws.send(
                JSON.stringify(
                    getHistoryAction({
                        sessionId: session.id,
                        agentType: session.agentType,
                        acpSessionId: session.acpSessionId,
                    })
                )
            );
        };

        ws.onmessage = e => {
            let payload: BridgeEventPayload;
            try {
                payload = JSON.parse(e.data) as BridgeEventPayload;
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
                    state.everReady = true;
                    this.notify(state);
                    break;
                case 'session_taken_over':
                    // A newer connection (another tab/browser) took over this
                    // session. The bridge closes us right after this event;
                    // mark takenOver so onclose skips the auto-reconnect (no
                    // ping-pong) and ChatPanel surfaces the banner.
                    state.takenOver = true;
                    state.ready = false;
                    if (state.reconnectTimer) {
                        clearTimeout(state.reconnectTimer);
                        state.reconnectTimer = null;
                    }
                    this.notify(state);
                    break;
                case 'prompt_queued': {
                    // Bridge accepted the prompt but couldn't start it because
                    // another turn is already running. Mark the most-recent user
                    // bubble as "queued" (the X button sends `requestId` back in
                    // cancel_queued; the next turn's first text_delta drains it).
                    if (!state.turnStarted) break;
                    const items = applyPromptQueued(state.items, payload.requestId);
                    if (items !== state.items) {
                        state.items = items;
                        this.notify(state);
                    }
                    break;
                }
                case 'prompt_cancelled': {
                    // Bridge dropped a queued prompt without ever starting it.
                    // Clear the queue badge from still-queued user bubbles — the
                    // cancelled turn never produces a text_delta to drain them.
                    const { items, mutated } = applyPromptCancelled(state.items);
                    if (mutated) {
                        state.items = items;
                        this.notify(state);
                    }
                    break;
                }
                case 'text_delta': {
                    if (!state.turnStarted) break;
                    const delta = payload.text;
                    if (!delta) break;
                    state.items = applyTextDelta(state.items, delta, payload.type || 'output');
                    this.notify(state);
                    break;
                }
                case 'tool_call': {
                    if (!state.turnStarted) break;
                    // Backend's SSE safety fallback may emit tool_call events
                    // without `arguments` (omitted) or with `arguments: {}` (the
                    // runtime's no-input placeholder); neither carries renderable
                    // data, so drop them before they reach the fold.
                    if (!hasRenderableArguments(payload.arguments)) break;
                    const next = applyToolCall(state, payload);
                    state.items = next.items;
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'tool_result': {
                    if (!state.turnStarted) break;
                    const next = applyToolResult(state, payload);
                    state.items = next.items;
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'permission_request': {
                    if (!state.turnStarted) break;
                    const next = applyPermissionRequest(state, payload);
                    state.items = next.items;
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'permission_timeout': {
                    if (!state.turnStarted) break;
                    state.items = applyPermissionTimeout(state.items, payload.requestId, payload.message);
                    this.notify(state);
                    break;
                }
                case 'done': {
                    state.items = applyDone(state.items);
                    state.typing = false;
                    state.turnStarted = false;
                    this.notify(state);
                    this.reloadHistory(session, state);
                    break;
                }
                case 'history_response': {
                    state.items = normalizeHistory(payload.items, payload.messages);
                    // History is authoritative: drop the realtime holding pools so
                    // the renderer stops showing the "待分配" group once the
                    // on-disk record has been replayed.
                    state.pendingResults = [];
                    state.pendingPermissions = [];
                    this.notify(state);
                    break;
                }
                case 'error': {
                    // SESSION_NOT_FOUND before `session_ready` is the bridge
                    // answering a control action fired during the init window
                    // (the UI gates input on `ready`, so the user can't trigger
                    // these). Swallow it so the stream doesn't flash a banner.
                    if (payload.code === 'SESSION_NOT_FOUND' && !state.ready) {
                        break;
                    }
                    state.items = appendError(state.items, payload.message || payload.code || 'Unknown error');
                    // Only reload history when the error came from inside a turn —
                    // that's when on-disk state may be authoritative over memory.
                    // Out-of-turn control errors don't touch history; reloading
                    // then turns one bad input into a console storm.
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
            // closedByUser: explicit destroy(). takenOver: a newer connection
            // claimed the session. Either way, do NOT auto-reconnect — the
            // takeover path is what would otherwise ping-pong two tabs.
            if (state.closedByUser || state.takenOver) {
                state.connection = 'closed';
                this.notify(state);
                return;
            }
            // A session that never connected (e.g. "查看详情" on a transcript the
            // backend can't resume) keeps failing the open handshake. Give up
            // after a few tries with a clear unavailable state instead of an
            // endless reconnect loop. A session that *was* live (everReady) only
            // dropped — keep reconnecting indefinitely.
            if (!state.everReady && state.reconnectAttempt >= 4) {
                state.connection = 'error';
                state.typing = false;
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
        state.ws.send(JSON.stringify(promptAction(session.id, content)));
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
        state.ws.send(JSON.stringify(closeSessionAction(session.id)));
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
        state.ws.send(JSON.stringify(cancelQueuedAction(session.id, requestId)));
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
        state.ws.send(JSON.stringify(respondPermissionAction(session.id, requestId, toolCallId, decision)));
        // `cancel` leaves the inline UI interactive so the user can re-decide
        // (the runtime will time the request out on its own if nothing
        // else happens). The four real decisions collapse the inline
        // permission into a one-line summary keyed on the allow/deny side.
        const resolved = resolvePermissionSide(decision);
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
        // setPermissionModeAction keeps the `permissionMode` field name aligned
        // with the Go WsMessage JSON tag (see wireProtocol.ts).
        if (state.ws && state.ws.readyState === WebSocket.OPEN) {
            state.ws.send(JSON.stringify(setPermissionModeAction(session.id, mode)));
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
                JSON.stringify(
                    getHistoryAction({
                        sessionId: session.id,
                        agentType: session.agentType,
                        acpSessionId: session.acpSessionId,
                    })
                )
            );
        }
    }

    private notify(state: SessionBridgeState) {
        // Publish the derived live status into sessionStore so the sidebar dot
        // tracks this session in real time, then repaint the chat subscribers.
        setLiveSessionStatus(state.sessionId, deriveLiveStatus(state));
        // Also mirror the raw WS connection state so the workspace header can
        // show the active session's connection status (the chat status bar
        // that used to own this was merged into the header).
        setLiveSessionConnection(state.sessionId, state.connection);
        for (const listener of state.listeners) {
            listener();
        }
    }
}

export const globalBridgeManager = new ChatBridgeManager();

export function useBridge(session: ChatSession | null, seed: ChatItem[] = []): UseBridgeState {
    // Re-render via a signal, NOT useState. Under @preact/signals a plain
    // useState forceUpdate silently fails to repaint a component that lives in
    // a static subtree (e.g. the 副屏 AI 项目经理 panel) — leaving the composer
    // stuck disabled even after the bridge connects. Reading `rev.value` in
    // render subscribes this component so each listener bump repaints reliably.
    const rev = useSignal(0);
    const bump = () => {
        rev.value++;
    };

    useEffect(() => {
        if (!session) return;

        const state = globalBridgeManager.getOrCreate(session);
        state.listeners.add(bump);

        bump();

        return () => {
            state.listeners.delete(bump);
        };
    }, [session?.id, session?.workspaceId, session?.taskId]);

    // Subscribe to bridge-state changes (see note above): reading `.value`
    // registers this render with the signal so listener bumps repaint it.
    // eslint-disable-next-line no-unused-expressions
    rev.value;

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
    const takenOver = state ? state.takenOver : false;

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

    const retry = useCallback(() => {
        if (!session) return;
        globalBridgeManager.retry(session);
    }, [session]);

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
        takenOver,
        retry,
    };
}
