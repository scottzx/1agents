export interface SessionMetadata {
    id: string;
    kind: 'chat';
    name: string;
    agentType: string;
    status: 'idle' | 'running';
    summary?: string;
    replyIds?: string[];
    createdAt: string;
}

export type ReplyMode = 'new' | 'follow_up' | 'pure_comment';

export interface Reply {
    id: string;
    author: { kind: 'user' | 'agent'; name: string };
    agentType?: string;
    text: string;
    sessionRef?: string;
    acpSessionId?: string;
    inReplyTo?: string;
    mode: ReplyMode;
    createdAt: string;
}

export type TaskPriority = 'urgent' | 'high' | 'medium' | 'low';

export type TaskType = 'task' | 'requirement' | 'bug';

export interface TaskRecurrence {
    freq: 'daily' | 'weekly' | 'monthly';
    weekday?: number;
    monthday?: number;
    at?: string;
}

export interface Task {
    id: string;
    title: string;
    description?: string;
    issueState?: 'open' | 'closed';
    status: 'pending' | 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'blocked';
    scheduleType: 'immediate' | 'scheduled';
    scheduledAt?: string;
    plannedStart?: string;
    plannedEnd?: string;
    dependsOn?: string[];
    priority?: TaskPriority;
    assignee?: string;
    labels?: string[];
    createdBy?: string;
    parentId?: string;
    milestone?: string;
    type?: TaskType;
    acceptanceCriteria?: string;
    recurrence?: TaskRecurrence | null;
    maxRetries?: number;
    retryCount?: number;
    createdAt: string;
    updatedAt: string;
    startedAt?: string;
    completedAt?: string;
    summary?: string;
    replies?: Reply[];
    sessions: SessionMetadata[];
}
