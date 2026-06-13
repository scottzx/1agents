// Chat session index — thin fetch wrapper around the 1agents backend
// /api/agent/* endpoints.
//
// Web chat runs purely on 1acp (via the /api/agent/chat/ws bridge). This
// service only manages the 1agents-side metadata that the sidebar uses to
// list "my chat sessions"; the live conversation is owned by 1acp.

import { AGENT_TYPES, type AgentType, type ChatSession } from '../components/types';

export interface IndexChatSessionRequest {
    workspace_id: string;
    name: string;
    agent_type: AgentType;
    /** Optional issue-model soft link — set for sessions spawned from a task timeline. */
    task_id?: string;
}

/** Default agent type used when a workspace has none configured. */
export const DEFAULT_AGENT_TYPE: AgentType = 'claudecode';

/** How cc-connect actually drives an agent today. */
export type CcTransport = 'acp' | 'cli-stream';

/** Per-host install + capability status for one agent application. */
export interface AgentStatus {
    type: AgentType;
    label: string;
    binary: string;
    installed: boolean;
    path?: string;
    /** Upstream app supports the ACP standard protocol. */
    acpCapable: boolean;
    /** Upstream app supports a CLI mode. */
    cliCapable: boolean;
    /** Transport cc-connect currently uses ('' when not integrated). */
    ccTransport: CcTransport | '';
    /** Whether this backend can drive the agent (only these reach the picker). */
    integrated: boolean;
    /** Terminal command to install the agent (shown when not installed). */
    installCommand?: string;
}

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
     * GET /api/agent/catalog
     *
     * Returns the per-host install + capability status for every real agent
     * application. Pass refresh=true to force a fresh PATH re-probe on the
     * backend (?refresh=1).
     */
    async getCatalog(refresh = false): Promise<AgentStatus[]> {
        const res = await fetch(`/api/agent/catalog${refresh ? '?refresh=1' : ''}`);
        if (!res.ok) throw new Error(await res.text());
        const data = (await res.json()) as RawAgentStatus[];
        return data.map(normalizeAgentStatus);
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
     * Registers a chat session in the 1agents index. Web sessions are
     * ACP-only — the live conversation runs on 1acp via the chat WS bridge.
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
     * Removes the 1agents-side index. The live 1acp session is torn down
     * separately via the chat WS bridge (globalBridgeManager.destroy).
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

interface RawAgentStatus {
    type?: string;
    label?: string;
    binary?: string;
    installed?: boolean;
    path?: string;
    acp_capable?: boolean;
    cli_capable?: boolean;
    cc_transport?: string;
    integrated?: boolean;
    install_command?: string;
}

/** Coerce the snake_case wire shape into the canonical AgentStatus. */
function normalizeAgentStatus(raw: RawAgentStatus): AgentStatus {
    return {
        type: (raw.type ?? '') as AgentType,
        label: String(raw.label ?? raw.type ?? ''),
        binary: String(raw.binary ?? ''),
        installed: Boolean(raw.installed),
        path: raw.path || undefined,
        acpCapable: Boolean(raw.acp_capable),
        cliCapable: Boolean(raw.cli_capable),
        ccTransport: (raw.cc_transport ?? '') as CcTransport | '',
        integrated: Boolean(raw.integrated),
        installCommand: raw.install_command || undefined,
    };
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
