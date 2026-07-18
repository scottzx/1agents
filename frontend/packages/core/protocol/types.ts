// Platform-agnostic chat protocol types.
//
// These describe the in-memory chat stream shape (ChatItem) and the wire
// shapes the bridge replays (HistoryItem) — pure TypeScript with zero
// preact / DOM / WebSocket dependencies so they can be reused across the
// web workbench, the Tauri desktop/mobile shell, and the 小程序 client.
//
// Moved verbatim out of components/chat/hooks.ts (Phase 0 — core carve).
// hooks.ts re-exports them so existing importers stay unchanged.

/** A file location a tool touched (ACP ToolCallLocation) — drives file links. */
export interface ToolCallLocation {
    path: string;
    line?: number;
}

/** A file diff a tool produced (ACP diff content block). */
export interface ToolCallDiff {
    path: string;
    oldText?: string;
    newText: string;
}

/**
 * ACP ToolCallStatus — authoritative lifecycle for a tool inside a prompt turn.
 * See https://agentclientprotocol.com/protocol/tool-calls#status
 * Distinct from the Composer follow-up prompt queue (`queueStatus` on user bubbles).
 */
export type ToolCallStatus = 'pending' | 'in_progress' | 'completed' | 'failed';

export interface ToolCallInfo {
    id?: string;
    toolName: string;
    input: string;
    toolCallId?: string;
    output?: string;
    isError?: boolean;
    /** ACP tool kind (read/edit/execute/…) — chooses the card icon. */
    kind?: string;
    /**
     * ACP tool-call lifecycle. When present, UI should prefer this over
     * heuristics (output presence / turn active).
     */
    status?: ToolCallStatus;
    /** Files the tool touched — rendered as clickable file links. */
    locations?: ToolCallLocation[];
    /** File diffs the tool produced — rendered inline in the card. */
    diffs?: ToolCallDiff[];
    /**
     * Inline permission request that the runtime emitted for this tool call.
     * Lives as a sub-field (not a separate ChatItem) so the permission UI
     * stays nested inside its tool_use card across both real-time streaming
     * and history replay.
     */
    permission?: {
        requestId: string;
        toolName: string;
        input: string;
        options: Array<{ text: string; data: string }>;
        resolved?: 'allow' | 'deny';
    };
}

/**
 * Shape of each item sent in a `history_response`. Mirrors the kind union
 * the bridge-server produces when replaying an agent's native session
 * storage (e.g. Claude Code's ~/.claude/projects/.../<sessionId>.jsonl).
 */
export type HistoryItem =
    | { kind: 'user'; text: string; createdAt?: string }
    | { kind: 'assistant_text'; text: string; createdAt?: string }
    | { kind: 'thinking'; text: string; createdAt?: string }
    | {
          kind: 'tool_use';
          toolName: string;
          input: unknown;
          toolCallId?: string;
          createdAt?: string;
      }
    | {
          kind: 'tool_result';
          toolCallId?: string;
          content: string;
          isError: boolean;
          createdAt?: string;
      };

export type ChatItem =
    | { id: string; kind: 'user'; content: string; createdAt: number; queueStatus?: 'queued'; queueRequestId?: string }
    | { id: string; kind: 'assistant_text'; content: string; createdAt: number; streaming: boolean }
    | { id: string; kind: 'thinking'; content: string; createdAt: number }
    | {
          id: string;
          kind: 'tool_use';
          toolName: string;
          input: string;
          calls: ToolCallInfo[];
          createdAt: number;
          toolCallId?: string;
      }
    | {
          id: string;
          kind: 'tool_result';
          toolCallId?: string;
          /** Tool name echoed by the realtime event, when available.
           * Lets the "待分配" fallback group label orphan results with
           * the real tool instead of a generic placeholder. */
          toolName?: string;
          content: string;
          createdAt: number;
          isError: boolean;
      }
    | {
          id: string;
          kind: 'permission_request';
          toolCallId?: string;
          requestId: string;
          toolName: string;
          input: string;
          options: Array<{ text: string; data: string }>;
          createdAt: number;
          resolved?: 'allow' | 'deny';
      }
    | { id: string; kind: 'error'; content: string; createdAt: number };

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'closed' | 'error';

/**
 * One NATIVE session mode the agent advertised over ACP (`session_meta` /
 * bridge `mode_changed`). Ids differ per agent (Claude Code:
 * default/acceptEdits/plan/…, Codex: read-only/agent/…), so pickers must
 * render data-driven from this list — never hardcode the id set.
 */
export interface SessionModeInfo {
    id: string;
    name: string;
    description?: string;
}

/** Live session-mode state mirrored from the bridge's session_meta snapshot. */
export interface SessionModesState {
    currentModeId?: string;
    availableModes: SessionModeInfo[];
}

export interface SessionConfigOptionChoice {
    value: string;
    name: string;
    description?: string;
}

/**
 * A NATIVE session config option the agent advertised (ACP select — model,
 * reasoning effort, …). The "mode" option is excluded upstream (it has its own
 * SessionModePicker). Delivered via session_meta and switched with
 * `set_config_option`. `category` echoes the agent's grouping when present.
 */
export interface SessionConfigOption {
    id: string;
    name: string;
    category?: string;
    currentValue?: string;
    options: SessionConfigOptionChoice[];
}

export type PlanEntryStatus = 'pending' | 'in_progress' | 'completed';

/**
 * One entry of the agent's execution plan (ACP `PlanEntry` — Claude Code's
 * TodoWrite, Codex's plan). Delivered via the bridge `plan` event as the FULL
 * list on every update; the host replaces its checklist wholesale. Live-only,
 * never persisted to history.
 */
export interface PlanEntry {
    content: string;
    status: PlanEntryStatus;
    priority?: 'high' | 'medium' | 'low';
}

/**
 * Live token/context usage + cost surfaced by the bridge `usage` event
 * (from ACP `usage_update`). All fields optional — not every adapter reports
 * every field, so consumers treat missing values as "unknown", never zero.
 * `used`/`size` are the context-window occupancy (drives the % gauge); `cost`
 * is cumulative session USD; `breakdown` is the per-turn token split for the
 * hover detail. Live-only — never persisted into history.
 */
export interface SessionUsage {
    /** Context-window tokens currently occupied. */
    used?: number;
    /** Context-window capacity. */
    size?: number;
    cost?: { amount?: number; currency?: string };
    breakdown?: {
        inputTokens?: number;
        outputTokens?: number;
        cachedReadTokens?: number;
        cachedWriteTokens?: number;
        thoughtTokens?: number;
        totalTokens?: number;
    };
}

/**
 * A slash command the agent advertised (ACP `available_commands_update`,
 * normalized by 1acp to name/description/hasInput). Delivered via session_meta
 * and kept current by live command updates. Executed by sending the command as
 * ordinary prompt text (e.g. `/compact`) — no dedicated wire action.
 */
export interface AvailableCommand {
    name: string;
    description?: string;
    hasInput?: boolean;
}

/**
 * One authentication method the agent advertised (ACP authMethods). The
 * bridge delivers the full list in `session_meta.payload.authMethods` (so
 * the UI can render a pre-emptive "登录" entry even before the agent has
 * actually demanded auth) and re-broadcasts it in the `auth_required` event
 * when the agent requests credentials mid-session.
 *
 *   - `type: 'oauth'`     — click opens `authUrl` in a new tab; user pastes
 *                           the resulting code back into the form.
 *   - `type: 'api_key'`   — single secret string (PAT, API key).
 *   - `type: 'credentials'` — username + password (or multi-field secret).
 *
 * `name` is the agent-supplied display name (e.g. "GitHub", "Acme OAuth").
 */
export interface AuthMethod {
    id: string;
    name?: string;
    description?: string;
    type: 'oauth' | 'api_key' | 'credentials';
    /** OAuth only — URL the user opens in a browser to complete the flow. */
    authUrl?: string;
    /**
     * Credential field hints (credentials/api_key methods). Used to render
     * labeled inputs when the agent needs more than one value. Free-form
     * for the wire layer — the UI just renders `label` + `type='password'|'text'`
     * for each entry and POSTs the map back as `credentials`.
     */
    fields?: Array<{ name: string; label?: string; type?: 'text' | 'password' }>;
}

/** Live auth-state mirror tracked by the chat bridge. */
export type AuthStatus = 'authenticated' | 'auth_required' | 'logged_out';

/**
 * Snapshot of the auth state for one session. `status` drives the header
 * badge; `methods` is the list to render in the re-auth modal (empty when
 * the agent doesn't require auth — the badge stays hidden).
 */
export interface AuthState {
    status: AuthStatus;
    methods: AuthMethod[];
    /** Free-form reason from the bridge (e.g. "Token expired", shown in modal). */
    message?: string;
    /**
     * Last auth attempt's error (set when the bridge answers an `authenticate`
     * with code `auth_failed`). Cleared on every new submit so a retry
     * doesn't show a stale message. Lives on `auth` (not on `lastError`)
     * because auth failures should ONLY show inside the ReauthModal —
     * surfacing them as a generic banner would compete with the modal.
     */
    lastError?: { message: string; code: string } | null;
}
