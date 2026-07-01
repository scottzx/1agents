import { apiFetch } from './apiClient';

// 对话历史全局搜索 — thin wrapper around the backend GET /api/search endpoint.
//
// Searches the structured metadata in meta.db (task title/description/summary/
// #number and chat session names). Chat message content is owned by 1acp and is
// NOT searchable here. The backend does a bounded LIKE scan; callers should
// debounce input to keep request volume low.

export interface TaskHit {
    id: string;
    projectId: string;
    projectName: string;
    number: number;
    title: string;
    status: string;
    type: string;
}

export interface SessionHit {
    id: string;
    projectId: string;
    projectName: string;
    taskId?: string;
    name: string;
    agentType: string;
}

export interface SearchResults {
    tasks: TaskHit[];
    sessions: SessionHit[];
}

interface RawTaskHit {
    id: string;
    project_id: string;
    project_name?: string;
    number?: number;
    title?: string;
    status?: string;
    type?: string;
}

interface RawSessionHit {
    id: string;
    project_id: string;
    project_name?: string;
    task_id?: string;
    name?: string;
    agent_type?: string;
}

const EMPTY: SearchResults = { tasks: [], sessions: [] };

export const searchService = {
    /**
     * GET /api/search?q=…
     * Returns matching tasks + sessions. An empty/blank query short-circuits to
     * empty results without hitting the network.
     */
    async search(query: string, limit = 30): Promise<SearchResults> {
        const q = query.trim();
        if (!q) return EMPTY;
        const res = await apiFetch(`/search?q=${encodeURIComponent(q)}&limit=${limit}`);
        if (!res.ok) throw new Error(await res.text());
        const data = (await res.json()) as { tasks?: RawTaskHit[]; sessions?: RawSessionHit[] };
        return {
            tasks: (data.tasks ?? []).map(t => ({
                id: String(t.id),
                projectId: String(t.project_id),
                projectName: String(t.project_name ?? ''),
                number: Number(t.number ?? 0),
                title: String(t.title ?? ''),
                status: String(t.status ?? ''),
                type: String(t.type ?? 'task'),
            })),
            sessions: (data.sessions ?? []).map(s => ({
                id: String(s.id),
                projectId: String(s.project_id),
                projectName: String(s.project_name ?? ''),
                taskId: s.task_id ? String(s.task_id) : undefined,
                name: String(s.name ?? ''),
                agentType: String(s.agent_type ?? ''),
            })),
        };
    },
};
