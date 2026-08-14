import { apiFetch } from './apiClient';
import type { TurnChangeFile, TurnChangeOp, TurnChangeReport, TurnChangeSource } from '../protocol/types';

export type { TurnChangeFile, TurnChangeOp, TurnChangeReport, TurnChangeSource };

export type AgentTurnStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
export type ProjectActivityStatus = 'succeeded' | 'rejected' | 'failed';

export interface AgentTurn {
    id: string;
    projectId: string;
    sessionId: string;
    clientRequestId?: string;
    runtimeRequestId?: string;
    initiatingReplyId?: string;
    agentType?: string;
    status: AgentTurnStatus;
    promptText?: string;
    finalAnswer?: string;
    errorCode?: string;
    errorText?: string;
    startedAt?: string;
    completedAt?: string;
    createdAt: string;
    updatedAt: string;
    changeReport?: TurnChangeReport;
}

export interface ProjectActivityTarget {
    type: string;
    id: string;
    operation: string;
}

export interface ProjectActivityEntry {
    id: string;
    projectId: string;
    groupKind: 'turn' | 'correlation' | 'event';
    turnId?: string;
    correlationId?: string;
    sessionId?: string;
    actorKind: string;
    actorName?: string;
    origin: string;
    status: ProjectActivityStatus;
    summary: string;
    count: number;
    eventIds: string[];
    targets: ProjectActivityTarget[];
    createdAt: string;
}

export interface CursorPage<T> {
    items: T[];
    nextCursor?: string;
    hasMore: boolean;
}

export interface CompletionEvidence {
    id: string;
    kind: string;
    summary: string;
    sessionId?: string;
    turnId?: string;
    createdAt: string;
}

export interface CompletionClosedBy {
    kind: string;
    taskRunId: string;
    turnId?: string;
    sessionId?: string;
    evidenceIds: string[];
    verdict: string;
    closedAt: string;
}

export interface TaskRunVerdict {
    pass: boolean;
    needsHuman?: boolean;
    summary?: string;
    attempt: number;
    verifier?: string;
    createdAt: string;
}

export interface TaskRun {
    id: string;
    projectId: string;
    taskId: string;
    originTurnId?: string;
    originSessionId?: string;
    sessionId?: string;
    kind: 'execution' | 'verification';
    status: 'running' | 'completed' | 'failed' | 'cancelled';
    attempt: number;
    evidence: CompletionEvidence[];
    verdict?: TaskRunVerdict;
    closedBy?: CompletionClosedBy;
    errorText?: string;
    startedAt: string;
    completedAt?: string;
    createdAt: string;
    updatedAt: string;
}

export interface TurnQuery {
    sessionId?: string;
    turnId?: string;
    status?: AgentTurnStatus;
    cursor?: string;
    limit?: number;
}

export interface ActivityQuery {
    sessionId?: string;
    turnId?: string;
    targetType?: string;
    targetId?: string;
    status?: ProjectActivityStatus;
    origin?: string;
    cursor?: string;
    limit?: number;
}

function queryString(workspaceId: string, fields: Record<string, string | number | undefined>): string {
    const params = new URLSearchParams({ workspace_id: workspaceId });
    for (const [key, value] of Object.entries(fields)) {
        if (value !== undefined && value !== '') params.set(key, String(value));
    }
    return params.toString();
}

async function page<T>(path: string): Promise<CursorPage<T>> {
    const response = await apiFetch(path, { cache: 'no-store' });
    if (!response.ok) throw new Error(await response.text());
    return (await response.json()) as CursorPage<T>;
}

export const activityService = {
    listTurns(workspaceId: string, query: TurnQuery = {}): Promise<CursorPage<AgentTurn>> {
        return page<AgentTurn>(
            `/agent/turns?${queryString(workspaceId, {
                session_id: query.sessionId,
                turn_id: query.turnId,
                status: query.status,
                cursor: query.cursor,
                limit: query.limit,
            })}`
        );
    },

    listActivity(workspaceId: string, query: ActivityQuery = {}): Promise<CursorPage<ProjectActivityEntry>> {
        return page<ProjectActivityEntry>(
            `/agent/activity?${queryString(workspaceId, {
                session_id: query.sessionId,
                turn_id: query.turnId,
                target_type: query.targetType,
                target_id: query.targetId,
                status: query.status,
                origin: query.origin,
                cursor: query.cursor,
                limit: query.limit,
            })}`
        );
    },

    async listTaskRuns(taskId: string): Promise<TaskRun[]> {
        const response = await apiFetch(`/agent/project-items/${encodeURIComponent(taskId)}/runs`, {
            cache: 'no-store',
        });
        if (!response.ok) throw new Error(await response.text());
        const data = (await response.json()) as { items: TaskRun[] };
        return data.items;
    },
};
