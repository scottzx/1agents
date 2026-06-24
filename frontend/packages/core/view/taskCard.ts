// Shared task-card view-model — the presentation logic both shells render from.
//
// Framework- and locale-agnostic by design: it maps a Task to semantic *tones*
// (color-token families) and status/priority/type *keys*, never to hex or to a
// localized string. Each shell then renders the tone via its own theme tokens
// (web SCSS `--{tone}-fg`; 小程序 its own palette) and the key via its own i18n.
// The web today encodes the same status→color mapping inline in SCSS class
// names; this lifts that single source of truth up so the 小程序 stays in sync.

import type { Task, TaskPriority, TaskType } from '../types/task';

/** Semantic color families — mirror the SCSS token families in index.scss. */
export type Tone = 'accent' | 'success' | 'danger' | 'warning' | 'orange' | 'purple' | 'muted';

// Status → tone. Lifted verbatim from `.task-card-item.status-*` in index.scss:
// running=success, completed=accent, failed/cancelled=danger, blocked=warning,
// queued/pending=muted.
const STATUS_TONE: Record<Task['status'], Tone> = {
    running: 'success',
    completed: 'accent',
    failed: 'danger',
    cancelled: 'danger',
    blocked: 'warning',
    queued: 'muted',
    pending: 'muted',
};

// Priority → tone. From `.priority-badge.priority-*`: urgent=danger, high=orange,
// medium=accent, low=muted.
const PRIORITY_TONE: Record<TaskPriority, Tone> = {
    urgent: 'danger',
    high: 'orange',
    medium: 'accent',
    low: 'muted',
};

/** Statuses past which a task no longer runs — drives dimming/strike-through. */
const TERMINAL: ReadonlySet<Task['status']> = new Set(['completed', 'cancelled', 'failed']);

export interface TaskCardVM {
    id: string;
    title: string;
    /** `#12` when the task has a number, else '' (suggestions before adoption). */
    numberLabel: string;
    type: TaskType;
    status: Task['status'];
    statusTone: Tone;
    /** Defaulted to 'medium' when the task omits priority (matches board sort). */
    priority: TaskPriority;
    priorityTone: Tone;
    /** source === 'agent-suggested' — held out of the board pending 采纳/忽略. */
    isSuggestion: boolean;
    isTerminal: boolean;
    labels: string[];
    assignee?: string;
    milestone?: string;
}

/** Derive the render-ready card descriptor for a task. Pure, no I/O. */
export function taskCardVM(task: Task): TaskCardVM {
    const priority: TaskPriority = task.priority || 'medium';
    return {
        id: task.id,
        title: task.title,
        numberLabel: task.number ? `#${task.number}` : '',
        type: task.type || 'task',
        status: task.status,
        statusTone: STATUS_TONE[task.status] || 'muted',
        priority,
        priorityTone: PRIORITY_TONE[priority] || 'muted',
        isSuggestion: task.source === 'agent-suggested',
        isTerminal: TERMINAL.has(task.status),
        labels: task.labels || [],
        assignee: task.assignee,
        milestone: task.milestone,
    };
}
