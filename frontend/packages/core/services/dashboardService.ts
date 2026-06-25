import { apiFetch } from './apiClient';

// Company cockpit (公司驾驶舱) Phase 1 — read-only cross-project aggregate.
// Mirrors the backend DashboardResponse shape (backend/internal/agent/dashboard.go).

/** Board-level rollup of a project's health. */
export type ProjectHealth = 'running' | 'blocked' | 'stalled' | 'done' | 'idle';

export interface DashboardProject {
    id: string;
    name: string;
    defaultAgent?: string;
    health: ProjectHealth;
    progressPercent: number;
    totalTasks: number;
    completedTasks: number;
    runningTasks: number;
    blockedTasks: number;
    activeSessions: number;
    agentSessions: number;
    lastEventAt?: string;
}

export interface DashboardSummary {
    totalProjects: number;
    runningProjects: number;
    blockedProjects: number;
    doneProjects: number;
    activeAgents: number;
    deliveredTasks: number;
}

export interface DashboardData {
    summary: DashboardSummary;
    projects: DashboardProject[];
    generatedAt: string;
}

export const dashboardService = {
    /**
     * GET /api/agent/dashboard — cross-project cockpit aggregate (read-only).
     * Backend摊开 every project on one board, derives per-project health and
     * sorts blocked / stalled projects to the top.
     */
    async get(): Promise<DashboardData> {
        const res = await apiFetch('/agent/dashboard');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as DashboardData;
    },
};
