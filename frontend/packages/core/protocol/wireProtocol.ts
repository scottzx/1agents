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

import type { HistoryItem } from './types';
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
    | 'done'
    | 'history_response'
    | 'error';

/** Union of every field the bridge may put on an inbound event. */
export interface BridgeEventPayload {
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
