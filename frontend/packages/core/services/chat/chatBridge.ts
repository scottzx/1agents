// ChatBridgeManager — framework-agnostic owner of the per-session chat
// WebSocket. Carved out of the web app's components/chat/hooks.ts (#216 §5) so
// the 小程序 client can reuse it: it owns the socket, listeners and reconnect
// bookkeeping; all conversation-state transforms live in the core reducer.
//
// Host couplings are injected via ChatBridgeOptions:
//   - directWsOrigin(): the ws(s):// origin for direct mode (web derives it
//     from window.location; weapp from a configured backend address).
//   - onStatus/onConnection: optional mirrors into a host store (sidebar dot /
//     header connection indicator).
// The transport itself goes through the platform bridge's connectSocket(), so
// direct mode uses the browser WebSocket on web/Tauri and Taro.connectSocket on
// the mini-program. Relay mode is unchanged (RelayChatSocket over socket.io).

import type {
    AuthMethod,
    AuthState,
    AvailableCommand,
    BackgroundTask,
    ChatItem,
    ConnectionState,
    PlanEntry,
    SessionConfigOption,
    SessionModesState,
    SessionUsage,
} from '../../protocol/types';
import type { ChatSession, ChatStatus, PermissionDecision, PermissionMode } from '../../types';
import { normalizePermissionMode } from '../../types';
import {
    cryptoId,
    hasRenderableArguments,
    normalizeHistory,
    applyTextDelta,
    applyPromptQueued,
    applyPromptCancelled,
    applyTurnState,
    selectOptimisticUsers,
    applyToolCall,
    applyToolResult,
    applyPermissionRequest,
    applyPermissionTimeout,
    applyAskUserQuestion,
    applyAskUserQuestionTimeout,
    resolveAskUserQuestion,
    applyExitPlanMode,
    applyExitPlanModeTimeout,
    resolveExitPlanMode,
    applyDone,
    appendError,
    resolvePermission,
    resolvePermissionSide,
    deriveLiveStatus,
} from '../../protocol/reducer';
import type { AskUserAnswerValue, AskUserOutcome, ExitPlanOutcome } from '../../protocol/types';
import {
    getHistoryAction,
    promptAction,
    cancelTurnAction,
    cancelQueuedAction,
    respondPermissionAction,
    respondAskUserQuestionAction,
    respondExitPlanModeAction,
    setPermissionModeAction,
    setSessionModeAction,
    setConfigOptionAction,
    authenticateAction,
    logoutAction,
    // #96 block A imports — staged here by a parallel session's wire-protocol
    // additions. Will be consumed by fork/delete/list case handlers; suppress
    // the unused-vars lint until that lands.
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    forkSessionAction,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    deleteSessionAction,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    listSessionsAction,
    type BridgeEventPayload,
} from '../../protocol/wireProtocol';
import { getPlatformBridge } from '../../platform/bridge';
import type { PlatformSocket } from '../../platform/socket';
import { apiFetch, backendTarget } from '../apiClient';
import { RelayChatSocket, type ChatTransport } from '../relay/relayChatSocket';

export const DEFAULT_PERMISSION_MODE: PermissionMode = 'approve-reads';

/** Mirror WebSocket.OPEN without referencing the (weapp-absent) global. */
const WS_OPEN = 1;
/** Mirror WebSocket.CLOSED without referencing the (weapp-absent) global. */
const WS_CLOSED = 3;

function bindEventTurn(previous: ChatItem[], next: ChatItem[], turnId: string | undefined): ChatItem[] {
    if (!turnId) return next;
    const previousById = new Map(previous.map(item => [item.id, item]));
    return next.map(item => {
        if (item.turnId) return item;
        const prior = previousById.get(item.id);
        return prior === item ? item : { ...item, turnId, turnStatus: 'running' };
    });
}

/** Host-specific dependencies the manager needs but core can't supply itself. */
export interface ChatBridgeOptions {
    /**
     * The ws:// or wss:// origin for the direct-mode chat WS (no trailing
     * slash), e.g. "wss://host". Web derives it from window.location; the
     * mini-program from its configured backend address.
     */
    directWsOrigin(): string;
    /** Mirror the derived live status into a host store (e.g. the sidebar dot). */
    onStatus?(sessionId: string, status: ChatStatus | null): void;
    /** Mirror the raw connection state into a host store (e.g. the header). */
    onConnection?(sessionId: string, conn: ConnectionState | null): void;
    /**
     * Mirror the session's auth state (authenticated | auth_required | logged_out)
     * plus the methods advertised by the agent. Drives the header badge and
     * any pre-emptive login entry. Cleared on destroy().
     */
    onAuthState?(sessionId: string, auth: AuthState | null): void;
    /** Mirror live ACP session capabilities into the host session index. */
    onSessionCapabilities?(sessionId: string, capabilities: { forkSupported: boolean }): void;
    /**
     * Bridge answered a `fork_session` request — the host store drops the
     * new ChatSession into the sidebar and runs the row-highlight animation.
     * `session` is the full ChatSessionRecord shape from the bridge; `parentId`
     * echoes the id we asked to fork. Undefined `session` is treated as a
     * silent no-op (the bridge answered with no payload).
     */
    onSessionForked?(parentId: string, session: unknown): void;
    /** Bridge answered a `delete_session` request — drop the row from the sidebar. */
    onSessionDeleted?(sessionId: string): void;
    /** Bridge answered a `list_sessions` request — refresh the cached session list. */
    onSessionsList?(workspaceId: string | undefined, sessions: unknown): void;
    /**
     * First non-thought text_delta of a new assistant_text block. Hosts use
     * this to bump lastEventAt so the sidebar sorts by last assistant reply.
     */
    onAssistantText?(sessionId: string): void;
}

export interface SessionBridgeState {
    /** The owning session's id — used to publish live status into the host store. */
    sessionId: string;
    items: ChatItem[];
    connection: ConnectionState;
    typing: boolean;
    ws: ChatTransport | null;
    listeners: Set<() => void>;
    notifyTimer: ReturnType<typeof setTimeout> | null;
    lastNotifyAt: number;
    turnStarted: boolean;
    activeTurnId: string | null;
    lastTurnSequence: Record<string, number>;
    /**
     * Per host turn: the first agentTurnId seen (grok `_meta.promptId`).
     * The main agent's turn sets the entry; any later agentTurnId that
     * differs belongs to a spawned subagent and renders in its own card.
     */
    mainAgentTurnByHost: Record<string, string>;
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
    /**
     * NATIVE session modes the agent advertised (session_meta snapshot,
     * kept current by mode_changed). null until the first session_meta —
     * and forever null for mode-less agents, which is what the Composer
     * uses to fall back to the permissionMode picker.
     */
    modes: SessionModesState | null;
    /**
     * Slash commands the agent advertised (session_meta snapshot, kept
     * current by available_commands_update). Empty until the first
     * session_meta and for agents that advertise none.
     */
    availableCommands: AvailableCommand[];
    /**
     * NATIVE config options the agent advertised (model, reasoning effort, …),
     * excluding the mode select. session_meta snapshot, replaced wholesale;
     * empty for agents that advertise none.
     */
    configOptions: SessionConfigOption[];
    /**
     * Latest token/context usage + cost from the bridge `usage` event.
     * Live-only (never in history): null until the first usage_update, then
     * kept as the last-known value (including across reconnects) so the badge
     * doesn't flicker off between turns.
     */
    usage: SessionUsage | null;
    /**
     * The agent's current execution plan (TodoWrite / Codex plan) from the
     * bridge `plan` event. Full list on every update — replaced wholesale.
     * null when the agent has no active plan. Live-only, never in history.
     */
    plan: PlanEntry[] | null;
    /**
     * Current background tasks (from runtime `background_task` / system-reminder
     * events). Live-only, never in history. Drives the top-of-chat window.
     */
    backgroundTasks: BackgroundTask[] | null;
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
    /**
     * True from the moment the user hits 停止 (cancel_turn) until the turn
     * actually ends (`done`/`error`). While set, streaming events are dropped
     * instead of rendered — the agent may keep emitting a few deltas before it
     * honors the cancel, and we must not let those (or the adopt path below)
     * resurrect a turn the user just stopped. Reset on the next prompt/connect.
     */
    cancelling: boolean;
    /**
     * Last bridge `error` event surfaced for the persistent top-of-composer
     * banner. Persists across re-renders and reconnects — only cleared by an
     * explicit user dismiss (the banner × button) or a full page reload, per
     * the requested "page-persistent, F5 / manual close only" UX. Newer
     * errors overwrite older ones so the banner always reflects the latest
     * upstream failure.
     */
    lastError: { message: string; code: string } | null;
    /**
     * Live auth state for the badge + re-auth modal. `null` until the bridge
     * has spoken — the UI hides the badge entirely in that case (so agents
     * that never require auth don't add visual noise). The agent's
     * `authMethods` are mirrored here as soon as `session_meta` carries them,
     * even before the first `auth_required`, so the header can show a
     * pre-emptive "登录" entry.
     */
    auth: AuthState | null;
    /** Whether ACP advertised session fork support; null until session_meta. */
    forkSupported: boolean | null;
}

export class ChatBridgeManager {
    private sessions = new Map<string, SessionBridgeState>();

    constructor(private opts: ChatBridgeOptions) {}

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
                notifyTimer: null,
                lastNotifyAt: 0,
                turnStarted: false,
                activeTurnId: null,
                lastTurnSequence: {},
                mainAgentTurnByHost: {},
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
                // the ChatSession object. Normalized because old records may
                // carry the retired 'auto' value (native session modes own
                // that concept now).
                permissionMode: normalizePermissionMode(session.permissionMode),
                modes: null,
                availableCommands: [],
                configOptions: [],
                usage: null,
                plan: null,
                backgroundTasks: null,
                reconnectAttempt: 0,
                reconnectTimer: null,
                closedByUser: false,
                takenOver: false,
                cancelling: false,
                lastError: null,
                auth: null,
                forkSupported: session.forkSupported ?? (session.agentType === 'claudecode' ? true : null),
            };
            this.sessions.set(session.id, state);
            this.connect(session, state);
        } else if (
            state.listeners.size === 0 &&
            (state.connection === 'error' ||
                state.connection === 'closed' ||
                state.ws === null ||
                state.ws.readyState === WS_CLOSED)
        ) {
            // A failed bridge stays cached so its history and UI state survive
            // session switches. Re-entering that session must revive the
            // transport instead of returning the permanently-dead socket.
            if (state.reconnectTimer) {
                clearTimeout(state.reconnectTimer);
                state.reconnectTimer = null;
            }
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
            if (state.notifyTimer) {
                clearTimeout(state.notifyTimer);
                state.notifyTimer = null;
            }
            if (state.ws) {
                if (state.ws.readyState === WS_OPEN) {
                    state.ws.send(JSON.stringify({ action: 'close_session', sessionId }));
                }
                state.ws.close();
            }
            state.listeners.clear();
            state.items = [];
            state.pendingResults = [];
            state.pendingPermissions = [];
            this.sessions.delete(sessionId);
            this.opts.onStatus?.(sessionId, null);
            this.opts.onConnection?.(sessionId, null);
            this.opts.onAuthState?.(sessionId, null);
        }
    }

    /**
     * Force-reconnect a session. Used by the takeover banner "重试" button
     * (reclaim ownership from another tab) and the header refresh button
     * (recover a stuck/failed session without a full page reload). connect()
     * clears `takenOver`. A missing bridge is created + connected; a live
     * one gets a fresh reconnect budget and a clean socket swap so the old
     * onclose handler cannot race the new connection.
     */
    retry(session: ChatSession) {
        const state = this.sessions.get(session.id);
        if (!state) {
            this.getOrCreate(session);
            return;
        }
        if (state.reconnectTimer) {
            clearTimeout(state.reconnectTimer);
            state.reconnectTimer = null;
        }
        // User-initiated refresh: reset the auto-reconnect give-up counter so
        // a previously failed "会话不可用" session can try again.
        state.reconnectAttempt = 0;
        // Detach the previous socket so its onclose cannot schedule a competing
        // reconnect or overwrite the new transport's handlers/state.
        if (state.ws) {
            const prev = state.ws;
            prev.onopen = null;
            prev.onmessage = null;
            prev.onerror = null;
            prev.onclose = null;
            try {
                prev.close();
            } catch {
                // ignore close races on already-dead sockets
            }
            state.ws = null;
        }
        this.connect(session, state);
    }

    /**
     * Clear the persistent error banner for `sessionId`. Called by the
     * banner's × button; reconciles via `notify(state)` so listeners
     * (and therefore the React render) re-evaluate visibility. Safe to
     * call when there is no error or no live state — both are no-ops.
     */
    dismissError(sessionId: string) {
        const state = this.sessions.get(sessionId);
        if (!state || !state.lastError) return;
        state.lastError = null;
        this.notify(state);
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
        // A reconnect re-attaches to whatever turn the session is in; clear any
        // stale cancel intent so acceptTurnEvent can adopt a live turn again.
        state.cancelling = false;
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
                profileId: session.profileId,
                replyId: session.replyId,
                agentRef: session.agentRef,
            });
        } else {
            const taskId = session.taskId || '';
            const replyId = session.replyId || '';
            const agentRef = session.agentRef || '';
            const profileId = session.profileId || '';
            const wsUrl = `${this.opts.directWsOrigin()}/api/agent/chat/ws?workspace_id=${encodeURIComponent(session.workspaceId)}&task_id=${encodeURIComponent(taskId)}&session_id=${encodeURIComponent(session.id)}&agent_type=${encodeURIComponent(session.agentType)}&profile_id=${encodeURIComponent(profileId)}&reply_id=${encodeURIComponent(replyId)}&agent_ref=${encodeURIComponent(agentRef)}`;
            console.log('[ChatBridgeManager] Connecting to backend websocket:', wsUrl);
            // Route through the platform transport seam so weapp uses
            // Taro.connectSocket while web/Tauri use the browser WebSocket.
            ws = new PlatformChatTransport(getPlatformBridge().connectSocket(wsUrl));
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
                        workspacePath: session.workspaceId,
                    })
                )
            );
        };

        ws.onmessage = e => {
            let payload: BridgeEventPayload;
            try {
                payload = JSON.parse(e.data) as BridgeEventPayload;
            } catch (err) {
                console.error('[ChatBridgeManager] Failed to parse message:', err);
                return;
            }

            const event = payload.event;
            console.log('[ChatBridgeManager] Received event:', event, payload);

            switch (event) {
                case 'session_ready': {
                    // Flip the gate so the Composer / mode toggle unblock.
                    // `state.typing` is intentionally untouched — the
                    // bridge signals per-turn activity with `done` / `error`,
                    // not with `session_ready`.
                    state.reconnectAttempt = 0;
                    state.ready = true;
                    state.everReady = true;
                    const agentSessionId = payload.agentSessionId || payload.acpSessionId;
                    if (agentSessionId) {
                        session.acpSessionId = agentSessionId;
                    }
                    this.notify(state);
                    // onopen may have requested history before ensure_session
                    // finished (no active handle/cwd yet → empty transcript).
                    // Re-fetch once ready so resume doesn't stay blank until
                    // the user hits header refresh.
                    this.reloadHistory(session, state);
                    break;
                }
                case 'profile_upgraded': {
                    state.items = appendError(
                        state.items,
                        payload.resumed
                            ? `Agent Profile 已升级到 ${payload.profileId || ''} revision ${payload.profileRevision || ''}，已恢复原 ACP 会话。`
                            : `Agent Profile 已升级到 ${payload.profileId || ''} revision ${payload.profileRevision || ''}；原 ACP 会话无法恢复，已创建新会话。`
                    );
                    this.notify(state);
                    break;
                }
                case 'session_meta': {
                    // Authoritative capability snapshot sent after every
                    // session_ready. Modes and commands are live-only state
                    // (never in history), so this re-send is what keeps them
                    // correct across reconnects and reaps.
                    const meta = payload.payload as
                        | {
                              modes?: SessionModesState;
                              availableCommands?: AvailableCommand[];
                              configOptions?: SessionConfigOption[];
                              authMethods?: AuthMethod[];
                              forkSupported?: boolean;
                          }
                        | undefined;
                    let changed = false;
                    if (meta?.modes && Array.isArray(meta.modes.availableModes)) {
                        state.modes = meta.modes;
                        changed = true;
                    }
                    if (Array.isArray(meta?.availableCommands)) {
                        state.availableCommands = meta.availableCommands;
                        changed = true;
                    }
                    if (Array.isArray(meta?.configOptions)) {
                        state.configOptions = meta.configOptions;
                        changed = true;
                    }
                    // Mirror authMethods even before any auth_required fires —
                    // a pre-emptive "登录" entry needs the method list, and a
                    // session whose agent doesn't require auth keeps
                    // `state.auth === null` (the badge stays hidden).
                    if (Array.isArray(meta?.authMethods)) {
                        const methods = meta.authMethods;
                        if (!state.auth) {
                            // First time we see the methods list. Status starts
                            // as 'authenticated' for sessions the bridge has
                            // already opened without an auth challenge; the
                            // bridge will downgrade to 'auth_required' if/when
                            // the agent actually demands credentials.
                            state.auth = {
                                status: 'authenticated',
                                methods,
                            };
                        } else {
                            state.auth = { ...state.auth, methods };
                        }
                        changed = true;
                    }
                    if (typeof meta?.forkSupported === 'boolean') {
                        state.forkSupported = meta.forkSupported;
                        this.opts.onSessionCapabilities?.(session.id, {
                            forkSupported: meta.forkSupported,
                        });
                        changed = true;
                    }
                    if (changed) {
                        this.notify(state);
                    }
                    break;
                }
                case 'available_commands_update': {
                    // Live mid-session refresh of the slash-command list.
                    const upd = payload.payload as { availableCommands?: AvailableCommand[] } | undefined;
                    if (Array.isArray(upd?.availableCommands)) {
                        state.availableCommands = upd.availableCommands;
                        this.notify(state);
                    }
                    break;
                }
                case 'usage': {
                    // Latest context/token usage + cost. Replace wholesale —
                    // each event is a fresh snapshot, not a delta.
                    const u = payload.payload as SessionUsage | undefined;
                    if (u && typeof u === 'object') {
                        state.usage = u;
                        this.notify(state);
                    }
                    break;
                }
                case 'plan': {
                    // Agent's execution plan. Full list on every update — replace
                    // wholesale; an empty list clears the checklist.
                    const p = payload.payload as { entries?: PlanEntry[] } | undefined;
                    if (Array.isArray(p?.entries)) {
                        state.plan = p!.entries.length > 0 ? p!.entries : null;
                        this.notify(state);
                    }
                    break;
                }
                case 'background_task': {
                    // Live background task status (running, completed, etc.).
                    // Full snapshot on every update — replace wholesale.
                    const b = payload.payload as { tasks?: BackgroundTask[] } | undefined;
                    if (Array.isArray(b?.tasks)) {
                        state.backgroundTasks = b!.tasks.length > 0 ? b!.tasks : null;
                        this.notify(state);
                    }
                    break;
                }
                case 'mode_changed': {
                    // Either our own set_session_mode ack or the agent
                    // switching itself (ExitPlanMode). Authoritative over any
                    // optimistic value.
                    const changed = payload.payload as { currentModeId?: string } | undefined;
                    if (changed?.currentModeId && state.modes) {
                        state.modes = { ...state.modes, currentModeId: changed.currentModeId };
                        this.notify(state);
                    }
                    break;
                }
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
                case 'turn_state': {
                    if (!payload.turnId || !payload.status) break;
                    const status = payload.status as 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
                    state.items = applyTurnState(state.items, {
                        turnId: payload.turnId,
                        requestId: payload.requestId,
                        status,
                        promptText: payload.promptText,
                        queuePosition: payload.queuePosition,
                    });
                    if (status === 'running') {
                        state.activeTurnId = payload.turnId;
                        state.turnStarted = true;
                        state.typing = true;
                    } else if (
                        (status === 'completed' || status === 'failed' || status === 'cancelled') &&
                        state.activeTurnId === payload.turnId
                    ) {
                        state.activeTurnId = null;
                        state.turnStarted = false;
                        state.typing = false;
                    }
                    this.notify(state);
                    break;
                }
                case 'turn_sync': {
                    let items = state.items;
                    if (payload.active) {
                        items = applyTurnState(items, {
                            ...payload.active,
                            turnId: payload.active.turnId ?? payload.active.id,
                            requestId: payload.active.requestId ?? payload.active.clientRequestId,
                        });
                        state.activeTurnId = payload.active.turnId ?? payload.active.id ?? null;
                    } else {
                        state.activeTurnId = null;
                    }
                    for (const [index, queued] of (payload.queued ?? []).entries()) {
                        items = applyTurnState(items, {
                            ...queued,
                            turnId: queued.turnId ?? queued.id,
                            requestId: queued.requestId ?? queued.clientRequestId,
                            queuePosition: index + 1,
                        });
                    }
                    state.items = items;
                    state.turnStarted = !!state.activeTurnId;
                    state.typing = !!state.activeTurnId;
                    this.notify(state);
                    break;
                }
                case 'turn_terminal': {
                    if (!payload.turnId || !payload.status || !this.acceptTurnEvent(state, payload)) break;
                    state.items = applyTurnState(state.items, {
                        turnId: payload.turnId,
                        status: payload.status as 'completed' | 'failed' | 'cancelled',
                    });
                    state.items = applyDone(state.items);
                    if (state.activeTurnId === payload.turnId) {
                        state.activeTurnId = null;
                        state.typing = false;
                        state.turnStarted = false;
                    }
                    state.cancelling = false;
                    this.notify(state);
                    this.reloadHistory(session, state);
                    break;
                }
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
                    if (!this.acceptTurnEvent(state, payload)) break;
                    const delta = payload.text;
                    if (!delta) break;
                    const deltaType = payload.type || 'output';
                    const agent = this.classifyAgentTurn(state, payload.turnId, payload.agentTurnId);
                    // First chunk of a new assistant text block (not thought,
                    // not an extension of an already-streaming block) → bump
                    // sidebar recency once per block. Subagent chatter is
                    // excluded — only main-agent replies should re-sort.
                    if (deltaType !== 'thought' && !agent.isSubagent) {
                        const last = state.items[state.items.length - 1];
                        const isNewTextBlock = !(last && last.kind === 'assistant_text' && last.streaming);
                        if (isNewTextBlock) {
                            this.opts.onAssistantText?.(state.sessionId);
                        }
                    }
                    state.items = applyTextDelta(
                        state.items,
                        delta,
                        deltaType,
                        payload.turnId,
                        agent.agentTurnId,
                        agent.isSubagent
                    );
                    this.notify(state);
                    break;
                }
                case 'tool_call': {
                    if (!this.acceptTurnEvent(state, payload)) break;
                    // Backend's SSE safety fallback may emit tool_call events
                    // without `arguments` (omitted) or with `arguments: {}` (the
                    // runtime's no-input placeholder); neither carries renderable
                    // data, so drop them before they reach the fold — UNLESS the
                    // event carries other renderable metadata (a diff/locations/
                    // kind streamed on a later tool_call_update, Phase 6).
                    // Status-only tool_call_update frames (in_progress / completed)
                    // carry no arguments — still fold them so UI tracks ACP lifecycle.
                    const hasMeta = !!(
                        payload.diffs?.length ||
                        payload.locations?.length ||
                        payload.kind ||
                        payload.status
                    );
                    if (!hasRenderableArguments(payload.arguments) && !hasMeta) break;
                    const agent = this.classifyAgentTurn(state, payload.turnId, payload.agentTurnId);
                    const next = applyToolCall(state, payload, agent.agentTurnId, agent.isSubagent);
                    state.items = bindEventTurn(state.items, next.items, payload.turnId);
                    state.pendingResults = bindEventTurn(state.pendingResults, next.pendingResults, payload.turnId);
                    state.pendingPermissions = bindEventTurn(
                        state.pendingPermissions,
                        next.pendingPermissions,
                        payload.turnId
                    );
                    this.notify(state);
                    break;
                }
                case 'tool_result': {
                    if (!this.acceptTurnEvent(state, payload)) break;
                    const agent = this.classifyAgentTurn(state, payload.turnId, payload.agentTurnId);
                    const next = applyToolResult(state, payload, agent.agentTurnId, agent.isSubagent);
                    state.items = bindEventTurn(state.items, next.items, payload.turnId);
                    state.pendingResults = bindEventTurn(state.pendingResults, next.pendingResults, payload.turnId);
                    state.pendingPermissions = bindEventTurn(
                        state.pendingPermissions,
                        next.pendingPermissions,
                        payload.turnId
                    );
                    this.notify(state);
                    break;
                }
                case 'permission_request': {
                    if (!this.acceptTurnEvent(state, payload)) break;
                    const next = applyPermissionRequest(state, payload);
                    state.items = bindEventTurn(state.items, next.items, payload.turnId);
                    state.pendingResults = bindEventTurn(state.pendingResults, next.pendingResults, payload.turnId);
                    state.pendingPermissions = bindEventTurn(
                        state.pendingPermissions,
                        next.pendingPermissions,
                        payload.turnId
                    );
                    this.notify(state);
                    break;
                }
                case 'permission_timeout': {
                    if (!state.turnStarted) break;
                    const next = applyPermissionTimeout(state, payload.requestId, payload.message);
                    state.items = next.items;
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'ask_user_question': {
                    if (!this.acceptTurnEvent(state, payload)) break;
                    const next = applyAskUserQuestion(state, payload);
                    state.items = bindEventTurn(state.items, next.items, payload.turnId);
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'ask_user_question_timeout': {
                    if (!state.turnStarted) break;
                    const next = applyAskUserQuestionTimeout(state, payload.requestId, payload.message);
                    state.items = next.items;
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'exit_plan_mode': {
                    if (!this.acceptTurnEvent(state, payload)) break;
                    const next = applyExitPlanMode(state, payload);
                    state.items = bindEventTurn(state.items, next.items, payload.turnId);
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'exit_plan_mode_timeout': {
                    if (!state.turnStarted) break;
                    const next = applyExitPlanModeTimeout(state, payload.requestId, payload.message);
                    state.items = next.items;
                    state.pendingResults = next.pendingResults;
                    state.pendingPermissions = next.pendingPermissions;
                    this.notify(state);
                    break;
                }
                case 'auth_required': {
                    // The agent detected an expired token (or never had one).
                    // The host's ReauthModal subscribes to this through
                    // modalStore; here we just keep the bridge-side mirror in
                    // sync so the header badge switches to red + 重新认证.
                    const methods =
                        (payload.payload as { methods?: AuthMethod[] } | undefined)?.methods ??
                        state.auth?.methods ??
                        [];
                    state.auth = {
                        status: 'auth_required',
                        methods,
                        message: payload.message,
                    };
                    this.notify(state);
                    break;
                }
                case 'auth_completed': {
                    // Credentials accepted — badge returns to "authenticated".
                    // Methods stay cached so a later `auth_required` can
                    // re-prompt without re-fetching the capability list.
                    state.auth = {
                        status: 'authenticated',
                        methods: state.auth?.methods ?? [],
                    };
                    this.notify(state);
                    break;
                }
                case 'logged_out': {
                    // Bridge cleared the agent's stored credentials. Keep the
                    // method list so the user can log back in from the same
                    // header entry.
                    state.auth = {
                        status: 'logged_out',
                        methods: state.auth?.methods ?? [],
                        message: payload.message,
                    };
                    this.notify(state);
                    break;
                }
                case 'session_forked': {
                    // Bridge answered a `fork_session` action. Hand the new
                    // session record (and the parent id we asked to fork) up
                    // to the host store, which is the single owner of the
                    // sidebar row list — we don't mirror it into the chat
                    // bridge state. Unknown parent (no `parentSessionId`
                    // echoed by the bridge) falls back to the WS-owning
                    // session id so the caller's request can still correlate.
                    const forked = payload.payload as { session?: unknown; parentSessionId?: string } | undefined;
                    if (forked?.session) {
                        this.opts.onSessionForked?.(forked.parentSessionId ?? session.id, forked.session);
                    }
                    break;
                }
                case 'session_deleted': {
                    // Bridge answered a `delete_session` action. The host
                    // store drops the row from `chatSessions` and removes the
                    // matching bridge state; we don't touch `state.items`
                    // because the WS is about to close.
                    const del = payload.payload as { sessionId?: string } | undefined;
                    const sid = del?.sessionId || payload.sessionId || session.id;
                    this.opts.onSessionDeleted?.(sid);
                    break;
                }
                case 'sessions_list': {
                    // Bridge answered a `list_sessions` action with the full
                    // session list for the requested workspace. Used by the
                    // sidebar's "Switch Session" popover — keeps the bridge
                    // as the single source of truth so the popover doesn't
                    // have to fall back to REST for archived sessions.
                    const list = payload.payload as { sessions?: unknown; workspaceId?: string } | undefined;
                    if (list && Array.isArray(list.sessions)) {
                        this.opts.onSessionsList?.(list.workspaceId, list.sessions);
                    }
                    break;
                }
                case 'done': {
                    state.items = applyDone(state.items);
                    state.typing = false;
                    state.turnStarted = false;
                    // Turn is over — a pending 停止 has been honored (or the turn
                    // finished on its own), so drop the cancel guard.
                    state.cancelling = false;
                    this.notify(state, true);
                    this.reloadHistory(session, state);
                    break;
                }
                case 'history_response': {
                    const history = normalizeHistory(payload.items, payload.messages);
                    const persistedTurnIds = new Set<string>(
                        history.map(item => item.turnId).filter((id): id is string => Boolean(id))
                    );
                    // History items carry the runtime request id as their turnId
                    // (the bridge's `turn_results` key), while live bubbles carry
                    // the canonical Turn id — selectOptimisticUsers matches on
                    // both id spaces so a stale in-flight copy from an
                    // interrupted turn (e.g. backend/1ACP restart) is dropped
                    // instead of being appended at the end of the timeline
                    // forever, dragging its turn's receipts down with it.
                    const optimistic = selectOptimisticUsers(state.items, persistedTurnIds);
                    state.items = [...history, ...optimistic];
                    // History is authoritative: drop the realtime holding pools so
                    // the renderer stops showing the "待分配" group once the
                    // on-disk record has been replayed.
                    state.pendingResults = [];
                    state.pendingPermissions = [];
                    this.notify(state, true);
                    break;
                }
                case 'protocol_error':
                case 'error': {
                    // SESSION_NOT_FOUND before `session_ready` is the bridge
                    // answering a control action fired during the init window
                    // (the UI gates input on `ready`, so the user can't trigger
                    // these). Swallow it so the stream doesn't flash a banner.
                    if (payload.code === 'SESSION_NOT_FOUND' && !state.ready) {
                        break;
                    }
                    // auth_failed belongs to the ReauthModal — drop the
                    // generic composer banner so the modal stays the single
                    // source of truth for auth errors (and the user's input
                    // isn't wiped by a hard-to-dismiss top banner).
                    if (payload.code === 'auth_failed') {
                        if (state.auth) {
                            state.auth = {
                                ...state.auth,
                                lastError: { message: payload.message || '', code: 'auth_failed' },
                            };
                            this.notify(state);
                        }
                        break;
                    }
                    // Stale respond_permission (double-click, already timed out /
                    // aborted, or UI still showing a pending Grok client-side
                    // confirm after the first click succeeded). Not a turn
                    // failure — never kill typing or reload history.
                    if (
                        payload.code === 'PERMISSION_NOT_FOUND' ||
                        payload.code === 'ASK_USER_NOT_FOUND' ||
                        payload.code === 'EXIT_PLAN_NOT_FOUND'
                    ) {
                        console.warn('[ChatBridgeManager] stale interactive response:', payload.code, payload.message);
                        break;
                    }
                    const errorMessage = payload.message || payload.code || 'Unknown error';
                    state.items = appendError(state.items, errorMessage);
                    // Surface as a page-persistent banner above the composer.
                    // Held until the user clicks × or refreshes the page —
                    // newer errors replace older ones, never stack.
                    state.lastError = { message: errorMessage, code: payload.code || '' };
                    const endsLegacyTurn =
                        event === 'error' &&
                        payload.terminal !== false &&
                        !payload.scope &&
                        !payload.turnId &&
                        state.turnStarted;
                    const endsExplicitTurn = payload.terminal === true && payload.scope === 'turn';
                    if (!endsLegacyTurn && !endsExplicitTurn) {
                        this.notify(state);
                        break;
                    }
                    // Only reload history when the error came from inside a turn —
                    // that's when on-disk state may be authoritative over memory.
                    // Out-of-turn control errors don't touch history; reloading
                    // then turns one bad input into a console storm.
                    const wasInTurn = state.turnStarted;
                    state.typing = false;
                    state.turnStarted = false;
                    state.activeTurnId = null;
                    state.cancelling = false;
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
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        // Refuse prompts sent before the bridge confirms initialization.
        // The bridge would answer with SESSION_NOT_FOUND and we'd be left
        // with an orphan user bubble in the stream.
        if (!state.ready) return;
        state.turnStarted = true;
        // A new prompt starts a fresh turn — clear any lingering cancel intent
        // from a previous 停止 so its streamed output isn't dropped.
        state.cancelling = false;
        const requestId = cryptoId();
        state.items = [
            ...state.items,
            {
                id: requestId,
                kind: 'user',
                content,
                createdAt: Date.now(),
                clientRequestId: requestId,
            },
        ];
        state.ws.send(JSON.stringify(promptAction(session.id, requestId, content)));
        state.typing = true;
        this.notify(state);
    }

    cancel(session: ChatSession) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        if (!state.ready) return;
        // The composer "停止" button targets only the active durable Turn and
        // keeps both the session and its queued Turns alive. Full teardown is
        // reached only through bridgeManager.destroy(). We optimistically clear
        // the running UI while retaining activeTurnId until turn_terminal
        // confirms cancellation.
        //
        // `cancelling` holds until that terminal arrives: the agent can still emit
        // a few frames before it honors the cancel, and without this guard
        // acceptTurnEvent would re-adopt them (turnStarted back to true) and the
        // stopped turn would visibly resume — the "点了停止还在继续" symptom.
        state.ws.send(JSON.stringify(cancelTurnAction(session.id, state.activeTurnId ?? undefined)));
        state.typing = false;
        state.turnStarted = false;
        state.cancelling = true;
        this.notify(state);
    }

    /**
     * Remove a single queued prompt (one the bridge has not started
     * yet). Distinct from `cancel`, which only stops the active turn;
     * the queue keeps running. Used by the X button on queued user
     * bubbles — `requestId` is the queue id echoed back in
     * `prompt_queued`.
     */
    cancelQueued(session: ChatSession, turnId: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        if (!state.ready) return;
        state.ws.send(JSON.stringify(cancelQueuedAction(session.id, turnId)));
        // Optimistically clear the badge so the user gets immediate
        // feedback; the bridge's `prompt_cancelled` event will
        // arrive later and be a no-op.
        state.items = state.items.map(it => {
            if (it.kind === 'user' && it.queueRequestId === turnId) {
                return { ...it, queueStatus: undefined, queueRequestId: undefined };
            }
            return it;
        });
        this.notify(state);
    }

    respondPermission(session: ChatSession, requestId: string, decision: PermissionDecision) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        if (!state.ready) return;
        // Find the permission (nested under tool_use OR parked in the
        // pendingPermissions holding pen) and capture the originating
        // toolCallId so the response can be linked back to the tool_use
        // block in the audit/log chain. Grok client-side fs/terminal
        // confirms almost always live only in pendingPermissions.
        let toolCallId: string | undefined;
        for (const it of state.items) {
            if (it.kind !== 'tool_use') continue;
            const match = it.calls.find(c => c.permission?.requestId === requestId);
            if (match) {
                toolCallId = match.toolCallId;
                break;
            }
        }
        if (toolCallId === undefined) {
            for (const p of state.pendingPermissions) {
                if (p.kind === 'permission_request' && p.requestId === requestId) {
                    toolCallId = p.toolCallId;
                    break;
                }
            }
        }
        state.ws.send(JSON.stringify(respondPermissionAction(session.id, requestId, toolCallId, decision)));
        // `cancel` leaves the inline UI interactive so the user can re-decide
        // (the runtime will time the request out on its own if nothing
        // else happens). The four real decisions collapse the inline
        // permission into a one-line summary keyed on the allow/deny side.
        // Must update pendingPermissions too — otherwise Grok client-side
        // confirms stay visible and a second click hits PERMISSION_NOT_FOUND.
        const resolved = resolvePermissionSide(decision);
        if (resolved) {
            const next = resolvePermission(state, requestId, resolved);
            state.items = next.items;
            state.pendingResults = next.pendingResults;
            state.pendingPermissions = next.pendingPermissions;
            this.notify(state);
        } else if (toolCallId === undefined) {
            // cancel with no known toolCallId — still fine; just log for
            // visibility in case a future event source breaks the invariant.
            console.warn('[ChatBridgeManager] respond_permission: no nested permission found for requestId', requestId);
        }
    }

    /**
     * Reply to a Grok ask_user_question questionnaire. Sends the wire action
     * and optimistically marks the nested/pending request as resolved so the
     * composer-adjacent prompt collapses immediately.
     */
    respondAskUserQuestion(
        session: ChatSession,
        requestId: string,
        outcome: AskUserOutcome,
        answers?: Record<string, AskUserAnswerValue>
    ) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        if (!state.ready) return;
        if (outcome === 'accepted' && (!answers || Object.keys(answers).length === 0)) {
            console.warn('[ChatBridgeManager] respond_ask_user_question: accepted without answers');
            return;
        }
        state.ws.send(
            JSON.stringify(
                respondAskUserQuestionAction({
                    sessionId: session.id,
                    requestId,
                    outcome,
                    answers,
                })
            )
        );
        const next = resolveAskUserQuestion(state, requestId, outcome, answers);
        state.items = next.items;
        state.pendingResults = next.pendingResults;
        state.pendingPermissions = next.pendingPermissions;
        this.notify(state);
    }

    /**
     * Reply to a Grok exit_plan_mode approval. Sends the wire action and
     * marks the nested/pending request as resolved. The mode change (if
     * approved or abandoned) is handled by the bridge's post-response
     * mode_changed event sent to the frontend.
     */
    respondExitPlanMode(session: ChatSession, requestId: string, outcome: ExitPlanOutcome, comments?: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        if (!state.ready) return;
        state.ws.send(
            JSON.stringify(
                respondExitPlanModeAction({
                    sessionId: session.id,
                    requestId,
                    outcome,
                    comments,
                })
            )
        );
        const next = resolveExitPlanMode(state, requestId, outcome, comments);
        state.items = next.items;
        state.pendingResults = next.pendingResults;
        state.pendingPermissions = next.pendingPermissions;
        this.notify(state);
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
        if (state.ws && state.ws.readyState === WS_OPEN) {
            state.ws.send(JSON.stringify(setPermissionModeAction(session.id, mode)));
        }
        void apiFetch(`/agent/sessions/${encodeURIComponent(session.id)}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ permission_mode: mode }),
        }).catch(err => {
            console.warn('[ChatBridgeManager] PATCH permission_mode failed:', err);
        });
    }

    /**
     * Switch the agent's NATIVE session mode (plan/acceptEdits/default/…).
     * Optimistic: the picker flips immediately; the bridge's `mode_changed`
     * ack (or the session_meta re-sent after a SET_MODE_FAILED) reconciles.
     * Persistence is free — 1acp stores desiredModeId and replays it onto
     * fresh ACP sessions after reap/resume, so no PATCH is needed here.
     */
    setSessionMode(session: ChatSession, modeId: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        if (!state.ready || !state.modes) return;
        if (state.modes.currentModeId === modeId) return;
        state.modes = { ...state.modes, currentModeId: modeId };
        this.notify(state);
        state.ws.send(JSON.stringify(setSessionModeAction(session.id, modeId)));
    }

    /**
     * Switch a NATIVE config option (model, reasoning effort, …). Optimistic:
     * the option's currentValue flips immediately; the session_meta the bridge
     * re-sends after set_config_option (success or SET_CONFIG_OPTION_FAILED)
     * reconciles. Persistence is free — 1acp stores the desired option and
     * replays it after reap/resume.
     */
    setConfigOption(session: ChatSession, key: string, value: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        if (!state.ready) return;
        const idx = state.configOptions.findIndex(o => o.id === key);
        if (idx < 0 || state.configOptions[idx].currentValue === value) return;
        state.configOptions = state.configOptions.map((o, i) => (i === idx ? { ...o, currentValue: value } : o));
        this.notify(state);
        state.ws.send(JSON.stringify(setConfigOptionAction(session.id, key, value)));
    }

    /**
     * Submit credentials for one of the methods the agent advertised. The
     * bridge answers with `auth_completed` (clears the badge) or an
     * `error` with code `auth_failed` (the modal keeps the user's input
     * and shows the error). Safe to call before `ready` — the bridge
     * accepts authenticate out-of-band once the session has been opened.
     */
    authenticate(session: ChatSession, methodId: string, credentials?: Record<string, string>) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        // Clear any stale error from the previous attempt so the modal can
        // re-show "submitting…" without flashing the prior failure.
        if (state.auth) {
            state.auth = { ...state.auth, lastError: null };
            this.notify(state);
        }
        state.ws.send(JSON.stringify(authenticateAction(session.id, methodId, credentials)));
    }

    /**
     * Drop the agent's stored credentials. The bridge answers with
     * `logged_out`; a later `auth_required` re-prompts with the cached
     * method list.
     */
    logout(session: ChatSession) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        state.ws.send(JSON.stringify(logoutAction(session.id)));
    }

    /**
     * Ask the bridge-server to fork `session` into a new ACP session.
     * The bridge answers with `session_forked`, which the manager fans
     * out via `opts.onSessionForked` so the host store can drop the new
     * row into the sidebar. Safe to call before `session_ready` — the
     * bridge queues control actions the same way it queues prompts.
     */
    forkSession(session: ChatSession) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        state.ws.send(JSON.stringify(forkSessionAction(session.id)));
    }

    /**
     * Permanently delete a session. The bridge tears down the live ACP
     * session first, then answers with `session_deleted`, which fans out
     * via `opts.onSessionDeleted` — the host store drops the row and the
     * bridge state. No-op when the WS isn't open.
     */
    deleteSession(session: ChatSession) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        state.ws.send(JSON.stringify(deleteSessionAction(session.id)));
    }

    /**
     * Pull the full session list for `workspaceId` (defaults to the
     * WS-owning session's workspace). The bridge answers with
     * `sessions_list`, fanned out via `opts.onSessionsList`. Used by
     * the sidebar's Switch Session popover to enumerate every session
     * without a REST round trip.
     */
    listSessions(session: ChatSession, workspaceId?: string) {
        const state = this.sessions.get(session.id);
        if (!state || !state.ws || state.ws.readyState !== WS_OPEN) return;
        state.ws.send(JSON.stringify(listSessionsAction(workspaceId ?? session.workspaceId)));
    }

    private reloadHistory(session: ChatSession, state: SessionBridgeState) {
        state.items = [];
        state.pendingResults = [];
        state.pendingPermissions = [];
        if (state.ws && state.ws.readyState === WS_OPEN) {
            state.ws.send(
                JSON.stringify(
                    getHistoryAction({
                        sessionId: session.id,
                        agentType: session.agentType,
                        acpSessionId: session.acpSessionId,
                        workspacePath: session.workspaceId,
                    })
                )
            );
        }
    }

    /**
     * Gate a streaming event (text_delta / tool_call / tool_result /
     * permission_request). Returns false when the event must be dropped.
     *
     * The old inline `if (!state.turnStarted) break` dropped every streamed
     * frame unless THIS client had itself sent the prompt (send() is the only
     * place that set turnStarted). That silently blanked any client which
     * JOINED a turn it didn't start — e.g. opening a running headless auto-run
     * from the task timeline: the bridge forwarded the deltas, but the observer
     * threw them all away. Here we instead ADOPT the in-flight turn (takeover
     * semantics — this connection now drives the session) so its output renders.
     *
     * Two cases still drop the event rather than adopt:
     *   - before session_ready (`!ready`): stray frames in the init window must
     *     not fabricate a turn;
     *   - after the user hit 停止 (`cancelling`): the agent may keep emitting a
     *     few deltas before it honors the cancel, and re-adopting them would
     *     resurrect a turn the user just stopped.
     */
    /**
     * Classify a chunk by its adapter-level turn id (grok `_meta.promptId`).
     * The first agentTurnId seen for a host turn is the MAIN agent's turn;
     * any different id belongs to a spawned subagent. Returns undefined for
     * agents that never stamp an id (Claude Code) — everything stays main.
     */
    private classifyAgentTurn(
        state: SessionBridgeState,
        turnId: string | undefined,
        agentTurnId: string | undefined
    ): { agentTurnId?: string; isSubagent?: boolean } {
        if (!agentTurnId) return {};
        if (!turnId) return { agentTurnId, isSubagent: false };
        const main = state.mainAgentTurnByHost[turnId];
        if (!main) {
            state.mainAgentTurnByHost[turnId] = agentTurnId;
            return { agentTurnId, isSubagent: false };
        }
        return { agentTurnId, isSubagent: agentTurnId !== main };
    }

    private acceptTurnEvent(state: SessionBridgeState, payload: BridgeEventPayload): boolean {
        if (payload.turnId) {
            if (state.activeTurnId && state.activeTurnId !== payload.turnId) {
                return false;
            }
            if (payload.sequence !== undefined) {
                const last = state.lastTurnSequence[payload.turnId] ?? 0;
                if (payload.sequence <= last) return false;
                state.lastTurnSequence[payload.turnId] = payload.sequence;
            }
            state.activeTurnId = payload.turnId;
        }
        if (state.turnStarted) return true;
        if (payload.event === 'turn_terminal' && state.cancelling) return true;
        if (!state.ready || state.cancelling) return false;
        state.turnStarted = true;
        state.typing = true;
        return true;
    }

    private notify(state: SessionBridgeState, immediate: boolean = false) {
        // High-priority metadata updates are published immediately to host callbacks
        this.opts.onStatus?.(state.sessionId, deriveLiveStatus(state));
        this.opts.onConnection?.(state.sessionId, state.connection);
        this.opts.onAuthState?.(state.sessionId, state.auth);

        if (immediate || !state.turnStarted) {
            this.flushNotifyListeners(state);
            return;
        }

        const now = Date.now();
        const elapsed = now - (state.lastNotifyAt || 0);
        if (elapsed >= 60) {
            this.flushNotifyListeners(state);
            return;
        }

        if (!state.notifyTimer) {
            state.notifyTimer = setTimeout(() => {
                state.notifyTimer = null;
                this.flushNotifyListeners(state);
            }, 60 - elapsed);
        }
    }

    private flushNotifyListeners(state: SessionBridgeState) {
        if (state.notifyTimer) {
            clearTimeout(state.notifyTimer);
            state.notifyTimer = null;
        }
        state.lastNotifyAt = Date.now();
        for (const listener of state.listeners) {
            listener();
        }
    }
}

/**
 * Adapt a PlatformSocket (onOpen/onMessage callbacks, string readyState) to the
 * browser-WebSocket-shaped ChatTransport the manager consumes. Handlers are
 * registered eagerly; the manager assigns its on* properties synchronously
 * after construction, before any async socket event can fire.
 */
class PlatformChatTransport implements ChatTransport {
    onopen: ((ev?: unknown) => void) | null = null;
    onmessage: ((ev: { data: string }) => void) | null = null;
    onclose: ((ev?: unknown) => void) | null = null;
    onerror: ((ev?: unknown) => void) | null = null;

    constructor(private sock: PlatformSocket) {
        sock.onOpen(() => this.onopen?.());
        sock.onMessage(data => this.onmessage?.({ data }));
        sock.onClose(info => this.onclose?.(info));
        sock.onError(err => this.onerror?.(err));
    }

    get readyState(): number {
        switch (this.sock.readyState) {
            case 'connecting':
                return 0;
            case 'open':
                return 1;
            case 'closing':
                return 2;
            default:
                return 3;
        }
    }

    send(data: string): void {
        this.sock.send(data);
    }

    close(): void {
        this.sock.close();
    }
}
