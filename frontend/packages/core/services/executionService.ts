import { apiFetch } from './apiClient';
import type { ExecutionJob, ExecutionRun, ExecutionTrigger, UpsertTriggerInput } from '../types/execution';

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
