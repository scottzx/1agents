// Preact hooks wrapping the backend chat WebSocket.
//
// The transport-owning ChatBridgeManager now lives in @1agents/core
// (services/chat/chatBridge) so the 小程序 client can reuse it. This file is
// the web host's thin layer: it constructs the manager with web-specific deps
// (window.location-derived WS origin, sessionStore mirrors) and exposes the
// preact `useBridge` hook that ChatPanel renders.

import { useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import type { ChatSession, PermissionDecision, PermissionMode } from '../types';
// Imported for its side-effecting setter only; referenced exclusively inside
// closures (never at module-eval time) so the sessionStore ⇄ hooks import cycle
// stays safe — see the cycle note in stores/sessionStore.ts.
import { setLiveSessionStatus, setLiveSessionConnection } from '../../stores/sessionStore';
import type { ChatItem, ConnectionState, SessionModesState, AvailableCommand } from '@1agents/core/protocol/types';
import { ChatBridgeManager, DEFAULT_PERMISSION_MODE } from '@1agents/core/services/chat/chatBridge';

export type { ToolCallInfo, HistoryItem, ChatItem, ConnectionState } from '@1agents/core/protocol/types';

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
    /**
     * NATIVE session modes the agent advertised (null until session_meta,
     * and forever null for mode-less agents — the Composer falls back to
     * the permissionMode picker in that case).
     */
    modes: SessionModesState | null;
    /** Slash commands the agent advertised (empty for agents with none). */
    availableCommands: AvailableCommand[];
    send: (content: string) => void;
    /**
     * Stop generating: cancels the running turn and drops every queued
     * prompt, but KEEPS the session alive so the user can continue
     * chatting immediately. Distinct from `cancelQueued`, which only
     * removes a single queued entry. (Full session teardown lives on the
     * sidebar archive action — bridgeManager.destroy.)
     */
    cancel: () => void;
    cancelQueued: (requestId: string) => void;
    respondPermission: (requestId: string, decision: PermissionDecision) => void;
    setPermissionMode: (mode: PermissionMode) => void;
    /** Switch the agent's native session mode (plan/acceptEdits/…). */
    setSessionMode: (modeId: string) => void;
    /** True when this connection was taken over by another tab/browser. */
    takenOver: boolean;
    /** Reconnect and reclaim ownership of the session (重试 button). */
    retry: () => void;
}

// The single web-host bridge manager. The store mirrors are wrapped in closures
// (not passed by reference) so they resolve at call time — preserving the
// "never reference sessionStore at module-eval" rule the import cycle needs.
export const globalBridgeManager = new ChatBridgeManager({
    directWsOrigin: () => `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`,
    onStatus: (sessionId, status) => setLiveSessionStatus(sessionId, status),
    onConnection: (sessionId, conn) => setLiveSessionConnection(sessionId, conn),
});

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
    const modes = state ? state.modes : null;
    const availableCommands = state ? state.availableCommands : [];
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

    const setSessionMode = useCallback(
        (modeId: string) => {
            if (!session) return;
            globalBridgeManager.setSessionMode(session, modeId);
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
        modes,
        availableCommands,
        send,
        cancel,
        cancelQueued,
        respondPermission,
        setPermissionMode,
        setSessionMode,
        takenOver,
        retry,
    };
}
