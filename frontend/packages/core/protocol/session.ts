// Platform-agnostic chat session + agent-type wire shapes.
//
// Moved verbatim out of components/types.ts (Phase 0 — core carve).
// components/types.ts re-exports these so existing importers stay unchanged.

import type { PermissionMode } from './permission';

/**
 * Agent plugin names registered in cc-connect. Keep in sync with
 * backend/internal/agent/types.go SupportedAgentTypes.
 */
export type AgentType =
    | 'claudecode'
    | 'codex'
    | 'acp'
    | 'gemini'
    | 'cursor'
    | 'devin'
    | 'iflow'
    | 'kimi'
    | 'opencode'
    | 'pi'
    | 'qoder'
    | 'tmux'
    // Detection-only frameworks (settings catalog; not yet drivable in chat).
    | 'antigravity'
    | 'openhands'
    | 'trae'
    | 'openclaw'
    | 'hermes';

export const AGENT_TYPES: AgentType[] = [
    'claudecode',
    'codex',
    'acp',
    'gemini',
    'cursor',
    'devin',
    'iflow',
    'kimi',
    'opencode',
    'pi',
    'qoder',
    'tmux',
];

/** Human-readable labels for the agent-type picker. */
export const AGENT_TYPE_LABELS: Record<AgentType, string> = {
    claudecode: 'Claude',
    codex: 'Codex',
    acp: 'ACP (通用)',
    gemini: 'Gemini CLI',
    cursor: 'Cursor',
    devin: 'Devin',
    iflow: 'iFlow',
    kimi: 'Kimi',
    opencode: 'OpenCode',
    pi: 'Pi',
    qoder: 'Qoder',
    tmux: 'Tmux',
    antigravity: 'Antigravity',
    openhands: 'OpenHands',
    trae: 'Trae',
    openclaw: 'OpenClaw',
    hermes: 'Hermes',
};

export type ChatStatus = 'idle' | 'streaming' | 'awaiting_permission' | 'error';

/**
 * A chat session — backed by a cc-connect session. The actual
 * conversation lives in cc-connect; this is the 1agents-side index.
 *
 * Wire shape: mirrors backend/internal/agent.ChatSessionRecord.
 */
export interface ChatSession {
    kind: 'chat';
    id: string; // 1agents uuid
    workspaceId: string;
    taskId?: string; // New: task ID this session belongs to
    /**
     * Transient: the timeline reply that triggered this session (issue
     * model). Forwarded as reply_id on the chat WS so the backend can link
     * Reply.sessionRef and attribute the agent's write-back. Not persisted.
     */
    replyId?: string;
    /**
     * Transient: the team expert (a <ws>/.claude/agents/<name>.md file) chosen
     * to drive this conversation, forwarded as agent_ref on the chat WS so the
     * backend injects that persona. Empty = the project's primary agent. Only
     * meaningful on a new session's first connect; not persisted.
     */
    agentRef?: string;
    /**
     * Transient: a prompt to auto-send to the agent as soon as the session is
     * ready (issue-model 追问/启动新会话). Routed into `pendingInitialMessage`
     * by `selectSession`, then fired once by `ChatPanel`. Not persisted.
     */
    initialMessage?: string;
    name: string;
    agentType: AgentType;
    ccProject: string; // cc-connect project name
    ccSessionId: string; // cc-connect session id
    /**
     * ACP-side session id — the agent-managed identifier (e.g. Claude
     * Code's JSONL UUID). Populated by the bridge-server on first
     * session_ready and reused as resumeSessionId on subsequent opens.
     * Independent of ccSessionId, which is for the cc-connect / IM path.
     */
    acpSessionId?: string;
    sessionKey: string; // cc-connect bridge session_key
    status: ChatStatus;
    createdAt?: string; // ISO timestamp — when the session was indexed
    lastEventAt?: string; // ISO timestamp
    /** Soft-delete: ISO timestamp when archived (closed from the sidebar). Empty/undefined = active. */
    archivedAt?: string;
    /** Derived from archivedAt — true when the session has been archived. */
    archived?: boolean;
    active: boolean;
    /** Per-session permission policy. Persisted via PATCH /api/agent/sessions/{id}. */
    permissionMode?: PermissionMode;
    /** Transient ACP capability mirrored from the live bridge session. */
    forkSupported?: boolean;
    /**
     * Conversation role, declared at creation (New Conversation). Drives the
     * avatar's role ring and, for 'pm', the project-locked task tools + PM
     * system prompt server-side. Empty/'general' = ordinary chat. Persisted
     * server-side (ChatSessionRecord.Role). See {@link SessionRole}.
     */
    role?: string;
}

/**
 * Conversation roles surfaced in the UI. Declared at creation; classified
 * visually by the AgentAvatar role ring. Only 'pm' currently carries special
 * backend behavior (task tools + PM prompt); the others are visual/scope
 * declarations the backend stores verbatim.
 *   - pmo      cross-project manager (no single project)  → purple ring
 *   - pm       project manager (project-scoped)           → accent ring
 *   - executor task executor                              → orange ring
 *   - verifier task verifier                              → success ring
 *   - general  ordinary chat (baseline)                   → no ring
 */
export type SessionRole = 'pmo' | 'pm' | 'executor' | 'verifier' | 'general';

/** Roles that get a colored avatar ring (everything except the baseline). */
export const RINGED_ROLES: SessionRole[] = ['pmo', 'pm', 'executor', 'verifier'];
