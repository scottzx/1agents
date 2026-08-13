import { apiFetch } from './apiClient';
import type {
    CreateJobInput,
    ExecutionJob,
    ExecutionRun,
    ExecutionTrigger,
    UpdateJobInput,
    UpsertTriggerInput,
} from '../types/execution';

async function expectJSON<T>(response: Response): Promise<T> {
    if (response.ok) return (await response.json()) as T;
    const body = await response.text();
    throw new Error(body || response.statusText);
}

/** Typed API client for the credential-free ExecutionJob control plane. */
export const executionService = {
    async listJobs(projectId?: string): Promise<ExecutionJob[]> {
        const query = projectId ? `?projectId=${encodeURIComponent(projectId)}` : '';
        const payload = await expectJSON<{ items: ExecutionJob[] }>(await apiFetch(`/execution-jobs${query}`));
        return payload.items || [];
    },

    async createJob(input: CreateJobInput): Promise<ExecutionJob> {
        return expectJSON(
            await apiFetch('/execution-jobs', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(input),
            })
        );
    },

    async updateJob(jobId: string, input: UpdateJobInput): Promise<ExecutionJob> {
        return expectJSON(
            await apiFetch(`/execution-jobs/${jobId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(input),
            })
        );
    },

    async pauseJob(jobId: string): Promise<void> {
        await expectJSON(await apiFetch(`/execution-jobs/${jobId}/pause`, { method: 'POST' }));
    },

    async resumeJob(jobId: string): Promise<void> {
        await expectJSON(await apiFetch(`/execution-jobs/${jobId}/resume`, { method: 'POST' }));
    },

    async deleteTrigger(jobId: string): Promise<void> {
        const response = await apiFetch(`/execution-jobs/${jobId}/trigger`, { method: 'DELETE' });
        if (!response.ok && response.status !== 204) {
            throw new Error((await response.text()) || response.statusText);
        }
    },

    async listRuns(jobId: string): Promise<ExecutionRun[]> {
        const payload = await expectJSON<{ items: ExecutionRun[] }>(await apiFetch(`/execution-jobs/${jobId}/runs`));
        return payload.items || [];
    },

    async runNow(jobId: string): Promise<void> {
        await expectJSON(await apiFetch(`/execution-jobs/${jobId}/run`, { method: 'POST' }));
    },

    async upsertTrigger(jobId: string, input: UpsertTriggerInput): Promise<ExecutionTrigger> {
        return expectJSON(
            await apiFetch(`/execution-jobs/${jobId}/trigger`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(input),
            })
        );
    },
};
