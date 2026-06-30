import { apiFetch } from './apiClient';

// PMO 跨项目对话式需求分发层 (#61) — dispatch a clarified requirement into a
// target project's pool. Mirrors backend/internal/meta (PMOStore +
// PMODispatchHandler). The PMO never schedules; it only dispatches into a
// project, where that project's AI PM (#49/#50) takes over.

export interface DispatchTarget {
    projectId: string;
    name: string;
    workspacePath: string;
}

export interface DispatchInput {
    projectId: string;
    title: string;
    description?: string;
    priority?: string;
    /** Originating inbox_item id; recorded as a backlink + marks it read. */
    fromInbox?: string;
}

export const pmoService = {
    /** GET /api/pmo/dispatch — the projects the PMO may dispatch into. */
    async targets(): Promise<DispatchTarget[]> {
        const res = await apiFetch('/pmo/dispatch');
        if (!res.ok) throw new Error(await res.text());
        const body = (await res.json()) as { targets: DispatchTarget[] };
        return body.targets || [];
    },

    /** POST /api/pmo/dispatch — write a requirement into a project's pool. */
    async dispatch(input: DispatchInput): Promise<void> {
        const res = await apiFetch('/pmo/dispatch', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
    },
};
