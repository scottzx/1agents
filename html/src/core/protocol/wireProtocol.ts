// The chat WebSocket wire contract — the single source of truth for the JSON
// shapes exchanged with the Go backend (backend/internal/agent.WsMessage) and,
// through it, the bridge-server.
//
// CRITICAL: field names here must match the JSON tags on the Go WsMessage struct
// exactly. The Go relay does ReadJSON → WriteJSON and silently DROPS unknown
// keys, so a renamed field (e.g. `mode` instead of `permissionMode`) vanishes
// before it reaches the bridge-server. Keep this aligned across
// frontend → Go → bridge-server.

import type { HistoryItem } from './types';
import type { PermissionDecision, PermissionMode } from './permission';

// ── Inbound: events the bridge emits ───────────────────────────────────────

export type BridgeEvent =
    | 'session_ready'
    | 'session_taken_over'
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
