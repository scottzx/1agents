import { PRIORITY_LABELS, PRIORITY_RANK, STATUS_LABELS, TYPE_LABELS } from './constants';
import type { Task } from './types';

// Airtable-style column model. `type` drives which inline editor renders;
// `editable` is the BASE flag — `status` is editable but scheduler-constrained
// (see TaskGridCell), and system-owned columns (实际完成/依赖) are read-only.
export type ColType =
    | 'id'
    | 'text'
    | 'priority'
    | 'status'
    | 'assignee'
    | 'milestone'
    | 'labels'
    | 'date'
    | 'type'
    | 'deps';

export interface ColumnDef {
    key: string;
    label: string;
    type: ColType;
    editable: boolean;
    width: number;
    /** column can be used as a group-by key */
    groupable?: boolean;
    /** pinned column: always first, cannot be reordered or hidden */
    locked?: boolean;
}

export const ALL_COLUMNS: ColumnDef[] = [
    { key: 'id', label: 'ID', type: 'id', editable: false, width: 64, locked: true },
    { key: 'priority', label: '优先级', type: 'priority', editable: true, width: 92, groupable: true },
    { key: 'status', label: '状态', type: 'status', editable: true, width: 112, groupable: true },
    { key: 'title', label: '任务', type: 'text', editable: true, width: 260 },
    { key: 'assignee', label: '执行', type: 'assignee', editable: true, width: 112, groupable: true },
    { key: 'milestone', label: '里程碑', type: 'milestone', editable: true, width: 130, groupable: true },
    { key: 'labels', label: '标签', type: 'labels', editable: true, width: 170 },
    { key: 'type', label: '类型', type: 'type', editable: true, width: 88, groupable: true },
    { key: 'plannedStart', label: '计划开始', type: 'date', editable: true, width: 112 },
    { key: 'plannedEnd', label: '计划完成', type: 'date', editable: true, width: 112 },
    { key: 'completedAt', label: '实际完成', type: 'date', editable: false, width: 112 },
    { key: 'deps', label: '前置依赖', type: 'deps', editable: false, width: 170 },
];

export type GroupKey = 'none' | 'priority' | 'status' | 'assignee' | 'milestone';

export const GROUP_OPTIONS: Array<[GroupKey, string]> = [
    ['none', '不分组'],
    ['milestone', '里程碑'],
    ['status', '状态'],
    ['assignee', '执行'],
    ['priority', '优先级'],
];

/** Stable, human-readable group bucket for a task under the active group key. */
export function groupValue(task: Task, key: GroupKey): string {
    switch (key) {
        case 'milestone':
            return task.milestone || '（无里程碑）';
        case 'status':
            return STATUS_LABELS[task.status] || task.status;
        case 'assignee':
            return task.assignee || 'claudecode';
        case 'priority':
            return PRIORITY_LABELS[task.priority || 'medium'] || (task.priority ?? 'medium');
        default:
            return '';
    }
}

// ── Column sorting (header click) ──
// Ascending comparator per column. `desc` is the negation. Missing values
// (no date / no milestone) sort last in ascending order. `deps` is not sortable.
const STATUS_ORDER = ['running', 'queued', 'pending', 'blocked', 'failed', 'completed', 'cancelled'];
const TYPE_ORDER = ['task', 'requirement', 'bug'];

const ts = (iso?: string): number => (iso ? new Date(iso).getTime() : Number.POSITIVE_INFINITY);

export function compareTasks(a: Task, b: Task, key: string): number {
    switch (key) {
        case 'id':
            return (a.number || 0) - (b.number || 0);
        case 'priority':
            return (PRIORITY_RANK[a.priority || 'medium'] ?? 2) - (PRIORITY_RANK[b.priority || 'medium'] ?? 2);
        case 'status':
            return STATUS_ORDER.indexOf(a.status) - STATUS_ORDER.indexOf(b.status);
        case 'type':
            return TYPE_ORDER.indexOf(a.type || 'task') - TYPE_ORDER.indexOf(b.type || 'task');
        case 'title':
            return a.title.localeCompare(b.title);
        case 'assignee':
            return (a.assignee || 'claudecode').localeCompare(b.assignee || 'claudecode');
        case 'milestone':
            // Empty milestones sort last (treat blank as a high code point).
            return (a.milestone || '￿').localeCompare(b.milestone || '￿');
        case 'labels':
            return (a.labels || []).join(',').localeCompare((b.labels || []).join(','));
        case 'plannedStart':
            return ts(a.plannedStart) - ts(b.plannedStart);
        case 'plannedEnd':
            return ts(a.plannedEnd) - ts(b.plannedEnd);
        case 'completedAt':
            return ts(a.completedAt) - ts(b.completedAt);
        default:
            return 0;
    }
}

export const isSortable = (key: string): boolean => key !== 'deps';

export { TYPE_LABELS };
