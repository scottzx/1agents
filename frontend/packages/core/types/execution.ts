/** Public, credential-free read models for the ExecutionJob control plane. */

export type ExecutionJobStatus = 'active' | 'paused' | 'blocked' | 'completed' | 'archived';
export type ExecutionTriggerKind = 'at' | 'recurrence';
export type ExecutionTriggerStatus = 'armed' | 'paused' | 'exhausted';

export interface ExecutionTrigger {
    id: string;
    jobId: string;
    kind: ExecutionTriggerKind;
    spec: Record<string, unknown>;
    timezone?: string;
    misfirePolicy: 'skip' | 'run_once';
    overlapPolicy: 'forbid' | 'allow';
    status: ExecutionTriggerStatus;
    nextRunAt?: string;
    createdAt: string;
    updatedAt: string;
}

export interface ExecutionJob {
    id: string;
    projectId: string;
    workItemId: string;
    businessRef?: string;
    executorKind: 'agent' | 'function' | 'human';
    profileId?: string;
    legacyAgentType?: string;
    functionType?: string;
    preambleFunctionType?: string;
    cwd?: string;
    capabilities?: string[];
    status: ExecutionJobStatus;
    timeoutMinutes?: number;
    maxAttempts: number;
    blockedCode?: string;
    blockedReason?: string;
    createdAt: string;
    updatedAt: string;
    trigger?: ExecutionTrigger;
}

export interface CreateJobInput {
    projectId: string;
    workItemId: string;
    businessRef?: string;
    executorKind: 'agent' | 'function' | 'human';
    profileId?: string;
    legacyAgentType?: string;
    functionType?: string;
    preambleFunctionType?: string;
    cwd?: string;
    capabilities?: string[];
    timeoutMinutes?: number;
    maxAttempts?: number;
}

export interface UpdateJobInput {
    profileId?: string;
    functionType?: string;
    preambleFunctionType?: string;
    cwd?: string;
    capabilities?: string[];
    timeoutMinutes?: number;
    maxAttempts?: number;
}

export interface ExecutionRun {
    id: string;
    jobId?: string;
    kind: 'execution' | 'verification';
    status: 'running' | 'completed' | 'failed' | 'cancelled';
    attempt: number;
    errorText?: string;
    startedAt: string;
    completedAt?: string;
}

export interface UpsertTriggerInput {
    kind: ExecutionTriggerKind;
    spec: Record<string, unknown>;
    timezone?: string;
    misfirePolicy?: 'skip' | 'run_once';
    overlapPolicy?: 'forbid' | 'allow';
}
