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
    /** Canonical SemVer for system-generated milestones; absent for legacy names. */
    version?: string;
    /** True when this milestone predates SemVer allocation and keeps a free-form name. */
    isLegacy?: boolean;
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

/**
 * ItemType — board-row discriminator (Epic #184 / #193 / #197).
 * Wire values stay task|requirement|bug|discussion; only `task` is scheduler-runnable.
 * TaskType is a transitional alias for gradual renames of call sites.
 * Removal target: after M6 (#196) closes (tracked by #197).
 */
export type ItemType = 'task' | 'requirement' | 'bug' | 'discussion';
/** @deprecated Prefer ItemType. Removal target: M6 close (#196 / #197). */
export type TaskType = ItemType;

/** Executor channel (名称定义表 §0.5). Field name stays executor — not AIWorkforce. */
export type TaskExecutor = 'agent' | 'function' | 'human';

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
    freq: 'daily' | 'weekly' | 'monthly' | 'yearly';
    interval?: number; // every N periods (default 1)
    weekday?: number; // weekly single-day (legacy)
    daysOfWeek?: number[]; // weekly multi-day, 0=Sunday…6 (e.g. [6,0] = Sat & Sun)
    monthday?: number; // monthly/yearly absolute day
    weekIndex?: number; // monthly/yearly relative: 1..4 / -1=last, with daysOfWeek
    month?: number; // yearly only: 1–12
    at?: string;
    until?: string; // stop after this date (RFC3339 or 'YYYY-MM-DD')
    count?: number; // stop after this many occurrences
}

// One entry of a task's embedded checklist — an ordered, individually-checkable
// sub-item held inside the task (distinct from a parent/child subtask).
export interface ChecklistItem {
    text: string;
    done?: boolean;
}

/**
 * ProjectItem — primary board-row type (Epic #184 / #197 M6-1).
 * Wire JSON field names stay stable; prefer ProjectItem / ItemType in new code.
 */
export interface ProjectItem {
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
    /**
     * Channel object for executor (名称定义表 §0.5):
     * agent → AgentType; human → user; function → function name.
     */
    assignee?: string;
    target?: {
        agent?: string;
        profile_id?: string;
        cwd?: string;
        capabilities?: string[];
    };
    /** Execution channel: agent | function | human (#192 / #193). */
    executor?: TaskExecutor;
    labels?: string[];
    createdBy?: string;
    parentId?: string;
    milestone?: string;
    type?: ItemType;
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
    // Embedded ordered progress ledger — items the executor ticks off as it works.
    checklist?: ChecklistItem[];
    maxRetries?: number;
    retryCount?: number;
    createdAt: string;
    updatedAt: string;
    startedAt?: string;
    completedAt?: string;
    summary?: string;
    closedBy?: {
        kind: string;
        taskRunId: string;
        turnId?: string;
        sessionId?: string;
        evidenceIds: string[];
        verdict: string;
        closedAt: string;
    };
    replies?: Reply[];
    sessions: SessionMetadata[];
    // Client-only annotations, set when tasks are aggregated across projects for
    // the global board (issue #91). Never sent to the backend; absent on the
    // single-project list/board/calendar. When present, the board surfaces a
    // project tag and routes clicks to the owning workspace.
    workspaceId?: string;
    workspaceName?: string;
}

/**
 * @deprecated Prefer ProjectItem. Transitional alias (Epic #197 / M6-1).
 * Removal target: delete when M6 (#196) closes (tracked by #197).
 */
export type Task = ProjectItem;
