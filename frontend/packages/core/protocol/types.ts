// Platform-agnostic chat protocol types.
//
// These describe the in-memory chat stream shape (ChatItem) and the wire
// shapes the bridge replays (HistoryItem) — pure TypeScript with zero
// preact / DOM / WebSocket dependencies so they can be reused across the
// web workbench, the Tauri desktop/mobile shell, and the 小程序 client.
//
// Moved verbatim out of components/chat/hooks.ts (Phase 0 — core carve).
// hooks.ts re-exports them so existing importers stay unchanged.

export interface ToolCallInfo {
    id?: string;
    toolName: string;
    input: string;
    toolCallId?: string;
    output?: string;
    isError?: boolean;
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
