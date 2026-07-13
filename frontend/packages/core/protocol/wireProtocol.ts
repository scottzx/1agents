// The chat WebSocket wire contract — the single source of truth for the JSON
// shapes exchanged with the Go backend (backend/internal/agent.WsMessage) and,
// through it, the bridge-server.
//
// Transport convention: the Go relay forwards frames as RAW BYTES and only
// peeks into them for its interception branches (done/error/session_ready/
// text_delta/prompt), so unknown fields survive the hop. New events put their
// structured data under a single `payload` object — Go must never need to
// parse `payload`. Top-level fields shared with Go's peek struct
// (WsMessage) must still match its JSON tags exactly.

import type { HistoryItem, ToolCallDiff, ToolCallLocation } from './types';
import type { PermissionDecision, PermissionMode } from './permission';

// ── Inbound: events the bridge emits ───────────────────────────────────────

export type BridgeEvent =
    | 'session_ready'
    | 'session_taken_over'
    | 'session_meta'
    | 'mode_changed'
    | 'available_commands_update'
    | 'usage'
    | 'plan'
    | 'prompt_queued'
    | 'prompt_cancelled'
    | 'text_delta'
    | 'tool_call'
    | 'tool_result'
    | 'permission_request'
    | 'permission_timeout'
    | 'auth_required'
    | 'auth_completed'
    | 'logged_out'
    | 'done'
    | 'history_response'
    | 'error'
    | 'session_forked'
    | 'session_deleted'
    | 'sessions_list';

/** Union of every field the bridge may put on an inbound event. */
export interface BridgeEventPayload {
    event: string;
    text?: string;
    type?: string;
    arguments?: unknown;
    requestId?: string;
    /** Session id on cross-session events (session_deleted, session_forked, …). */
    sessionId?: string;
    message?: string;
    code?: string;
    toolName?: string;
    toolCallId?: string;
    isError?: boolean;
    /** ACP tool metadata on tool_call events (Phase 6). */
    kind?: string;
    locations?: ToolCallLocation[];
    diffs?: ToolCallDiff[];
    messages?: Array<{ role: string; text: string }>;
    items?: HistoryItem[];
    /**
     * Structured event data (session_meta / mode_changed). Opaque to the Go
     * relay by convention; each consumer narrows it per event.
     */
    payload?: unknown;
}

// ── Outbound: actions the client sends ──────────────────────────────────────

export function getHistoryAction(args: { sessionId: string; agentType: string; acpSessionId?: string }) {
    return {
        action: 'get_history',
        sessionId: args.sessionId,
        agentType: args.agentType,
        acpSessionId: args.acpSessionId,
    };
}

export function promptAction(sessionId: string, text: string) {
    return { action: 'prompt', sessionId, text };
}

// Stop the active turn (and drop any queued prompts) WITHOUT tearing the
// session down — the user can immediately keep chatting in the same session.
// Distinct from close_session, which fully terminates the session and is only
// reached via the sidebar's archive action (bridgeManager.destroy).
export function cancelTurnAction(sessionId: string) {
    return { action: 'cancel_turn', sessionId };
}

export function closeSessionAction(sessionId: string) {
    return { action: 'close_session', sessionId };
}

export function cancelQueuedAction(sessionId: string, requestId: string) {
    return { action: 'cancel_queued', sessionId, requestId };
}

export function respondPermissionAction(
    sessionId: string,
    requestId: string,
    toolCallId: string | undefined,
    behavior: PermissionDecision
) {
    return { action: 'respond_permission', sessionId, requestId, toolCallId, behavior };
}

// Field name is `permissionMode` (NOT `mode`) to match the JSON tag on
// backend/internal/agent.WsMessage.PermissionMode — otherwise the Go struct
// drops the field on the ReadJSON → WriteJSON forward.
export function setPermissionModeAction(sessionId: string, permissionMode: PermissionMode) {
    return { action: 'set_permission_mode', sessionId, permissionMode };
}

// Switch the agent's NATIVE session mode (ACP session/set_mode — e.g. Claude
// Code's default/acceptEdits/plan, Codex's read-only/agent). Distinct from
// setPermissionModeAction, which only moves the bridge's own permission gate.
export function setSessionModeAction(sessionId: string, modeId: string) {
    return { action: 'set_session_mode', sessionId, payload: { modeId } };
}

// Switch a NATIVE session config option (ACP session/set_config_option — e.g.
// model, reasoning effort). The "mode" option has its own set_session_mode.
export function setConfigOptionAction(sessionId: string, key: string, value: string) {
    return { action: 'set_config_option', sessionId, payload: { key, value } };
}

// Submit credentials for a method the bridge advertised in `auth_required`
// (or, on first session_meta, in `authMethods`). The bridge forwards to the
// agent's authenticate call and answers with `auth_completed` or an `error`
// whose `code === 'auth_failed'`. `credentials` is opaque to the wire layer
// — the agent decides what fields it accepts per method.
export function authenticateAction(sessionId: string, methodId: string, credentials?: Record<string, string>) {
    return { action: 'authenticate', sessionId, methodId, credentials };
}

// Drop the agent's stored credentials and reset the session's auth state.
// The bridge answers with `logged_out`; a later `auth_required` re-prompts.
export function logoutAction(sessionId: string) {
    return { action: 'logout', sessionId };
}

// Issue #96 block A: ask the bridge-server to fork the given session into a
// new one (new id, snapshot of conversation so far). The bridge answers with
// `session_forked`, carrying the new session record under `payload.session`
// (and the parent id under `payload.parentSessionId` for the caller's
// convenience — the row highlight + scroll then runs on the new id).
export function forkSessionAction(sessionId: string) {
    return { action: 'fork_session', sessionId };
}

// Permanently delete a session (and its underlying agent conversation). The
// bridge tears down the live ACP session first, then answers with
// `session_deleted` carrying `sessionId` so the sidebar can drop the row.
export function deleteSessionAction(sessionId: string) {
    return { action: 'delete_session', sessionId };
}

// Pull the full session list for the current workspace. Used by the sidebar
// "Switch Session" popover to enumerate every session without an extra REST
// round trip. The bridge answers with `sessions_list` carrying an array of
// session records under `payload.sessions`. `workspaceId` is optional — when
// omitted the bridge uses the WS connection's workspace.
export function listSessionsAction(workspaceId?: string) {
    return { action: 'list_sessions', workspaceId };
}
