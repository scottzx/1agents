import { apiFetch } from './apiClient';
import type { Task } from '../types/task';

// 个人任务 + 立项 (#67) — the no-project backlog that lives in the reserved
// `__personal__` bucket, plus the 立项 (incubate) gate that promotes a personal
// task into a real long-term Project. Mirrors backend/internal/meta
// (PersonalStore + PersonalTasksHandler / PersonalTaskItemHandler).
//
// Backlinks travel on Task.labels: `captured-from:<inboxItemId>` records the
// Inbox origin (#60 收尾), `incubated-from:<personalTaskId>` records the seed on
// promotion. The pane reads these to render the source trail.

export interface PersonalCaptureInput {
    title: string;
    description?: string;
    /** Inbox item id this task was captured from (#60 → #67). */
    fromInbox?: string;
}

export interface IncubateInput {
    projectName: string;
    workspacePath: string;
    /** Roadmap milestone names seeded in order (deduped, blanks skipped). */
    milestones?: string[];
}

/** The new Project created by 立项. Shape mirrors meta.Project's JSON. */
export interface IncubateProject {
    id: string;
    name: string;
    workspacePath: string;
    status: string;
}

export interface IncubateResult {
    project: IncubateProject;
    task: Task;
}

export const personalTaskService = {
    /** GET /api/personal-tasks — the personal backlog, oldest-first. */
    async list(): Promise<Task[]> {
        const res = await apiFetch('/personal-tasks');
        if (!res.ok) throw new Error(await res.text());
        const data = (await res.json()) as { tasks?: Task[] };
        return data.tasks || [];
    },

    /** POST /api/personal-tasks — land a lightweight personal task. */
    async capture(input: PersonalCaptureInput): Promise<Task> {
        const res = await apiFetch('/personal-tasks', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as Task;
    },

    /** POST /api/personal-tasks/{id}/incubate — promote into a fresh Project. */
    async incubate(id: string, input: IncubateInput): Promise<IncubateResult> {
        const res = await apiFetch(`/personal-tasks/${id}/incubate`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as IncubateResult;
    },
};
