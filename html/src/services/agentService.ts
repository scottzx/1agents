// Chat session index — thin fetch wrapper around the 1agents backend
// /api/agent/* endpoints. Does NOT talk to cc-connect directly.
//
// The actual cc-connect session lifecycle (create / send / receive
// events / delete) is owned by ccconnectClient.ts, which consumes
// /api/v1/* and /bridge/ws. This service only manages the 1agents-side
// metadata that the sidebar uses to list "my chat sessions".

import { AGENT_TYPES, type AgentType, type ChatSession } from '../components/types';

export interface IndexChatSessionRequest {
    workspace_id: string;
    name: string;
    agent_type: AgentType;
    /** Optional issue-model soft link — set for sessions spawned from a task timeline. */
    task_id?: string;
    /** cc-connect (IM) identifiers — empty for ACP-only sessions. */
    cc_project?: string;
    cc_session_id?: string;
    session_key?: string;
}

/** Default agent type used when a workspace has none configured. */
export const DEFAULT_AGENT_TYPE: AgentType = 'claudecode';

export const agentService = {
    /**
     * GET /api/agent/agent-types
     * Returns the canonical agent type list served by the backend.
     */
    async listAgentTypes(): Promise<AgentType[]> {
        const res = await fetch('/api/agent/agent-types');
        if (!res.ok) throw new Error(await res.text());
        const data = (await res.json()) as string[];
        // Defensive: backend may have a different list. Filter to the
        // ones we know how to render, then return backend's order.
        return data.filter((t): t is AgentType => (AGENT_TYPES as string[]).includes(t));
    },

    /**
     * GET /api/agent/sessions?workspace_id=…
     */
    async list(workspaceId: string): Promise<ChatSession[]> {
        const res = await fetch(`/api/agent/sessions?workspace_id=${encodeURIComponent(workspaceId)}`);
        if (!res.ok) throw new Error(await res.text());
        const data = (await res.json()) as RawChatSession[];
        return data.map(normalizeChatSession);
    },

    /**
     * GET /api/agent/sessions/{id}
     * Returns the indexed record, or null when the id is unknown.
     */
    async get(id: string): Promise<ChatSession | null> {
        const res = await fetch(`/api/agent/sessions/${encodeURIComponent(id)}`);
        if (res.status === 404) return null;
        if (!res.ok) throw new Error(await res.text());
        return normalizeChatSession((await res.json()) as RawChatSession);
    },

    /**
     * POST /api/agent/sessions
     *
     * Indexes an already-created cc-connect session. The caller MUST have
     * created the cc-connect session first (via ccconnectClient) so we
     * have cc_session_id + session_key to record.
     */
    async index(req: IndexChatSessionRequest): Promise<ChatSession> {
        const res = await fetch('/api/agent/sessions', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw new Error(await res.text());
        return normalizeChatSession((await res.json()) as RawChatSession);
    },

    /**
     * DELETE /api/agent/sessions/{id}
     *
     * Only removes the 1agents-side index. Does NOT touch cc-connect;
     * the caller should call ccconnectClient.ccDeleteSession first if
     * they want the underlying session gone too.
     */
    async delete(id: string): Promise<void> {
        const res = await fetch(`/api/agent/sessions/${encodeURIComponent(id)}`, {
            method: 'DELETE',
        });
        if (!res.ok) throw new Error(await res.text());
    },
};

interface RawChatSession {
    id: string | number;
    workspace_id: string | number;
    name?: string;
    agent_type?: string;
    task_id?: string;
    cc_project?: string;
    cc_session_id?: string;
    acp_session_id?: string;
    session_key?: string;
    status?: string;
    last_event_at?: string;
    active?: boolean;
}

/** Coerce unknown / missing fields into the canonical ChatSession shape. */
function normalizeChatSession(raw: RawChatSession): ChatSession {
    return {
        kind: 'chat',
        id: String(raw.id),
        workspaceId: String(raw.workspace_id),
        taskId: raw.task_id ? String(raw.task_id) : undefined,
        name: String(raw.name ?? ''),
        agentType: (raw.agent_type ?? DEFAULT_AGENT_TYPE) as AgentType,
        ccProject: String(raw.cc_project ?? ''),
        ccSessionId: String(raw.cc_session_id ?? ''),
        acpSessionId: raw.acp_session_id ? String(raw.acp_session_id) : undefined,
        sessionKey: String(raw.session_key ?? ''),
        status: (raw.status ?? 'idle') as ChatSession['status'],
        lastEventAt: raw.last_event_at || undefined,
        active: Boolean(raw.active),
    };
}
