// Task board / issue-model service — typed wrappers around the 1agents backend
// /api/agent/{tasks,milestones} endpoints.
//
// Carved out of the web app's TaskList components (which used raw fetch) so the
// transport routes through apiFetch — relay-aware, so the same calls work from
// the 小程序 client (Taro.request) and over the relay bypass, not just on a
// same-origin web/Tauri host.

import { apiFetch } from './apiClient';
import { workspaceService } from './workspaceService';
import type { Task, Milestone, Reply, ReplyMode } from '../types/task';

/** Resolve a GitHub-style project#number reference to its task + workspace. */
export interface TaskRef {
    workspaceId: string;
    task: Task;
}

const q = encodeURIComponent;

async function ok(res: Response): Promise<Response> {
    if (!res.ok) throw new Error(await res.text());
    return res;
}

export const taskService = {
    // ---- tasks ----

    async list(workspaceId: string): Promise<Task[]> {
        const res = await apiFetch(`/agent/tasks?workspace_id=${q(workspaceId)}`);
        if (!res.ok) throw new Error(`Failed to load tasks: ${res.statusText}`);
        return (await res.json()) || [];
    },

    /**
     * Cross-project aggregate for the global board (issue #91): fan out
     * `list(workspaceId)` over every non-builtin workspace and stamp each task
     * with its owning workspace (client-only `workspaceId` / `workspaceName`).
     * Reuses the existing per-project endpoint — no new backend surface. A
     * single project failing to load is skipped, not fatal, so one bad meta
     * file can't blank the whole board.
     */
    async listAll(): Promise<Task[]> {
        const workspaces = (await workspaceService.list()).filter(ws => !ws.builtin);
        const perProject = await Promise.all(
            workspaces.map(async ws => {
                try {
                    const tasks = await this.list(ws.id);
                    return tasks.map(t => ({ ...t, workspaceId: ws.id, workspaceName: ws.name }));
                } catch {
                    return [] as Task[];
                }
            })
        );
        return perProject.flat();
    },

    /**
     * Raw JSON text for one task — lets pollers skip the re-render when the
     * payload is byte-identical to the last (Go's encoding is deterministic).
     */
    async getText(id: string): Promise<string> {
        const res = await apiFetch(`/agent/tasks/${q(id)}`);
        if (!res.ok) throw new Error(`Failed to load task: ${res.statusText}`);
        return res.text();
    },

    async get(id: string): Promise<Task> {
        return JSON.parse(await this.getText(id)) as Task;
    },

    async create(body: Record<string, unknown>): Promise<Task> {
        const res = await ok(
            await apiFetch('/agent/tasks', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            })
        );
        return res.json();
    },

    async patch(id: string, patch: Record<string, unknown>): Promise<Task> {
        const res = await ok(
            await apiFetch(`/agent/tasks/${q(id)}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(patch),
            })
        );
        return res.json();
    },

    async remove(id: string, workspaceId: string): Promise<void> {
        await ok(await apiFetch(`/agent/tasks/${q(id)}?workspace_id=${q(workspaceId)}`, { method: 'DELETE' }));
    },

    async resolve(project: string, number: number): Promise<TaskRef | null> {
        const res = await apiFetch(`/agent/tasks/resolve?project=${q(project)}&number=${number}`);
        if (!res.ok) return null;
        return res.json();
    },

    async addReply(id: string, text: string, mode: ReplyMode = 'pure_comment'): Promise<void> {
        await ok(
            await apiFetch(`/agent/tasks/${q(id)}/replies`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ text, mode }),
            })
        );
    },

    // ---- milestones ----

    async listMilestones(workspaceId: string): Promise<Milestone[]> {
        const res = await apiFetch(`/agent/milestones?workspace_id=${q(workspaceId)}`);
        if (!res.ok) throw new Error(`Failed to load milestones: ${res.statusText}`);
        return (await res.json()) || [];
    },

    async createMilestone(workspaceId: string, fields: Record<string, unknown>): Promise<void> {
        await ok(
            await apiFetch('/agent/milestones', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, ...fields }),
            })
        );
    },

    async patchMilestone(id: string, workspaceId: string, patch: Record<string, unknown>): Promise<void> {
        await ok(
            await apiFetch(`/agent/milestones/${q(id)}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, ...patch }),
            })
        );
    },

    async removeMilestone(id: string, workspaceId: string): Promise<void> {
        await ok(await apiFetch(`/agent/milestones/${q(id)}?workspace_id=${q(workspaceId)}`, { method: 'DELETE' }));
    },
};

export type { Task, Milestone, Reply };
