// Task / issue-model domain types.
//
// Moved verbatim out of the web app's components/drawer/TaskList/types.ts so the
// task board logic (core/services/taskService) and the 小程序 client can share
// one source of truth. The old location now re-exports from here.

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

// Milestone is a first-class roadmap stage. Tasks link to it by the matching
// Task.milestone *name* (the backend keeps that label in sync on rename), so
// the roadmap groups tasks by name and uses this record only for ordering,
// target date, and description. total/completed are server-computed counts.
export interface Milestone {
    id: string;
    name: string;
    description?: string;
    targetDate?: string;
    position: number;
    // Optional parent milestone (前置里程碑). Milestones sharing a predecessor
    // fork into parallel branches on the roadmap; empty = root.
    predecessorId?: string;
    createdAt: string;
    updatedAt: string;
    total: number;
    completed: number;
}

export type TaskPriority = 'urgent' | 'high' | 'medium' | 'low';

export type TaskType = 'task' | 'requirement' | 'bug' | 'discussion';

// How a task entered the pool. '' (omitted) = normal user/PM-created task;
// 'agent-suggested' marks an AI suggestion (issue #47) — held out of the board
// until a human 采纳 (clears source) or 忽略 (deletes) it.
export type TaskSource = 'agent-suggested';

// A GitHub-style peer cross-reference between work items (not hierarchy —
// subtasks use parentId). target is the referenced task's id.
export type LinkRel = 'closes' | 'relates';

export interface TaskLink {
    target: string;
    rel: LinkRel;
}

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
    status: 'pending' | 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'blocked' | 'not_ready';
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
    source?: TaskSource;
    // Requirement/bug only: user has confirmed the issue is ready for the PM to
    // schedule (break into executable tasks). Non-executable items stay open/closed.
    userConfirm?: boolean;
    number?: number;
    links?: TaskLink[];
    // GitHub Issue/PR sync mapping (#74). The github* fields are the sync anchor
    // to one GitHub object — normally backfilled by the sync pass and shown
    // read-only. githubAssignees are GitHub login names (human collaborators),
    // distinct from `assignee` above, which selects the executing agent type.
    githubRepo?: string;
    githubKind?: 'issue' | 'pr';
    githubNumber?: number;
    githubNodeId?: string;
    githubUrl?: string;
    githubState?: string;
    githubAssignees?: string[];
    lastSyncedAt?: string;
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
