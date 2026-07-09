import { h } from 'preact';

import { t, type Lang } from '../../../i18n';
import { AGENT_OPTIONS, getPriorityLabels, getStatusLabels, getTypeLabels } from './constants';
import type { ColumnDef } from './gridConfig';
import type { ProjectItem, TaskType } from './types';
import { fmtDateOnly, recurrenceLabel } from './utils';

interface GridCellProps {
    task: ProjectItem;
    col: ColumnDef;
    allTasks: ProjectItem[];
    isChild: boolean;
    editing: boolean;
    onStartEdit: () => void;
    onCommit: (patch: Record<string, unknown>) => void;
    onCancel: () => void;
    onOpenDetail: () => void;
    lang: Lang;
}

const toDateInput = (iso?: string): string => {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${d.getFullYear()}-${m}-${day}`;
};

const focusRef = (el: HTMLInputElement | HTMLSelectElement | null) => {
    if (el) {
        el.focus();
        if (el instanceof HTMLInputElement && el.type === 'text') el.select();
    }
};

export function GridCell({
    task,
    col,
    allTasks,
    isChild,
    editing,
    onStartEdit,
    onCommit,
    onCancel,
    onOpenDetail,
    lang,
}: GridCellProps) {
    const prio = task.priority || 'medium';
    const priorityLabels = getPriorityLabels(lang);
    const statusLabels = getStatusLabels(lang);
    const typeLabels = getTypeLabels(lang);

    // ── Edit mode ──────────────────────────────────────────────────────────
    if (editing) {
        switch (col.type) {
            case 'text':
                return (
                    <input
                        ref={focusRef}
                        class="grid-edit-input"
                        type="text"
                        value={task.title}
                        onKeyDown={(e: KeyboardEvent) => {
                            const el = e.target as HTMLInputElement;
                            if (e.key === 'Enter') el.blur();
                            // Reset before blur so the blur handler sees no change → cancels.
                            else if (e.key === 'Escape') {
                                el.value = task.title;
                                el.blur();
                            }
                        }}
                        onBlur={(e: Event) => {
                            const v = (e.target as HTMLInputElement).value.trim();
                            if (v && v !== task.title) onCommit({ title: v });
                            else onCancel();
                        }}
                    />
                );
            case 'priority':
                return (
                    <select
                        ref={focusRef}
                        class="grid-edit-select"
                        value={prio}
                        onChange={(e: Event) => onCommit({ priority: (e.target as HTMLSelectElement).value })}
                        onBlur={onCancel}
                    >
                        {(['urgent', 'high', 'medium', 'low'] as const).map(p => (
                            <option key={p} value={p}>
                                {priorityLabels[p]}
                            </option>
                        ))}
                    </select>
                );
            case 'status':
                return (
                    <select
                        ref={focusRef}
                        class="grid-edit-select"
                        title={t('task.grid.statusHint', lang)}
                        value=""
                        onChange={(e: Event) => {
                            const v = (e.target as HTMLSelectElement).value;
                            if (v) onCommit({ status: v });
                            else onCancel();
                        }}
                        onBlur={onCancel}
                    >
                        <option value="">
                            {statusLabels[task.status] || task.status}
                            {t('task.grid.statusScheduler', lang)}
                        </option>
                        <option value="completed">{t('task.grid.markComplete', lang)}</option>
                        <option value="cancelled">{t('task.grid.markCancelled', lang)}</option>
                    </select>
                );
            case 'assignee':
                return (
                    <select
                        ref={focusRef}
                        class="grid-edit-select"
                        value={task.assignee || 'claudecode'}
                        onChange={(e: Event) => onCommit({ assignee: (e.target as HTMLSelectElement).value })}
                        onBlur={onCancel}
                    >
                        {AGENT_OPTIONS.map(a => (
                            <option key={a} value={a}>
                                {a}
                            </option>
                        ))}
                    </select>
                );
            case 'type':
                return (
                    <select
                        ref={focusRef}
                        class="grid-edit-select"
                        value={task.type || 'task'}
                        onChange={(e: Event) => onCommit({ type: (e.target as HTMLSelectElement).value })}
                        onBlur={onCancel}
                    >
                        {(['task', 'requirement', 'bug'] as TaskType[]).map(tp => (
                            <option key={tp} value={tp}>
                                {typeLabels[tp]}
                            </option>
                        ))}
                    </select>
                );
            case 'milestone':
                return (
                    <input
                        ref={focusRef}
                        class="grid-edit-input"
                        type="text"
                        placeholder={t('task.grid.milestonePlaceholder', lang)}
                        value={task.milestone || ''}
                        onKeyDown={(e: KeyboardEvent) => {
                            const el = e.target as HTMLInputElement;
                            if (e.key === 'Enter') el.blur();
                            else if (e.key === 'Escape') {
                                el.value = task.milestone || '';
                                el.blur();
                            }
                        }}
                        onBlur={(e: Event) => {
                            const v = (e.target as HTMLInputElement).value.trim();
                            if (v !== (task.milestone || '')) onCommit({ milestone: v });
                            else onCancel();
                        }}
                    />
                );
            case 'labels':
                return (
                    <input
                        ref={focusRef}
                        class="grid-edit-input"
                        type="text"
                        placeholder={t('task.grid.labelsPlaceholder', lang)}
                        value={(task.labels || []).join(', ')}
                        onKeyDown={(e: KeyboardEvent) => {
                            const el = e.target as HTMLInputElement;
                            if (e.key === 'Enter') el.blur();
                            else if (e.key === 'Escape') {
                                el.value = (task.labels || []).join(', ');
                                el.blur();
                            }
                        }}
                        onBlur={(e: Event) => {
                            const labels = (e.target as HTMLInputElement).value
                                .split(/[,，]/)
                                .map(s => s.trim())
                                .filter(Boolean);
                            if (labels.join(' ') !== (task.labels || []).join(' ')) onCommit({ labels });
                            else onCancel();
                        }}
                    />
                );
            case 'date': {
                const field = col.key as 'plannedStart' | 'plannedEnd';
                return (
                    <input
                        ref={focusRef}
                        class="grid-edit-input"
                        type="date"
                        value={toDateInput(task[field])}
                        onChange={(e: Event) => {
                            const v = (e.target as HTMLInputElement).value;
                            if (v) onCommit({ [field]: new Date(v).toISOString() });
                            else onCancel();
                        }}
                        onBlur={onCancel}
                    />
                );
            }
            default:
                onCancel();
                return null;
        }
    }

    // ── Display mode ───────────────────────────────────────────────────────
    const editableClass = col.editable ? ' grid-cell-editable' : '';
    const onDbl = col.editable ? onStartEdit : undefined;

    switch (col.type) {
        case 'id':
            return (
                <td class="col-id">
                    <button
                        class="task-number-link"
                        title={t('task.table.openDetail', lang)}
                        onClick={(e: Event) => {
                            e.stopPropagation();
                            onOpenDetail();
                        }}
                    >
                        {task.number ? `#${task.number}` : task.id.slice(0, 6)}
                    </button>
                </td>
            );
        case 'priority':
            return (
                <td class={`col-priority${editableClass}`} onDblClick={onDbl}>
                    <span class={`priority-badge priority-${prio}`}>{priorityLabels[prio] || prio}</span>
                </td>
            );
        case 'status':
            return (
                <td
                    class={'col-status grid-cell-editable'}
                    onDblClick={onStartEdit}
                    title={t('task.grid.statusHint', lang)}
                >
                    <span class={`task-status-badge ${task.status}`}>
                        {task.status === 'running' && <span class="pulse-indicator" />}
                        {statusLabels[task.status] || task.status}
                    </span>
                </td>
            );
        case 'text':
            return (
                <td class={`col-title${editableClass}`} onDblClick={onDbl}>
                    {isChild && <span class="subtask-indent">└─</span>}
                    <span class="task-row-title">{task.title}</span>
                    {task.recurrence && (
                        <span class="task-recur-tag" title={recurrenceLabel(task.recurrence)}>
                            🔁
                        </span>
                    )}
                    {(task.replies?.length ?? 0) > 0 && <span class="task-reply-count">💬 {task.replies!.length}</span>}
                </td>
            );
        case 'assignee':
            return (
                <td class={`col-assignee${editableClass}`} onDblClick={onDbl}>
                    {task.assignee || 'claudecode'}
                </td>
            );
        case 'type':
            return (
                <td class={`col-type${editableClass}`} onDblClick={onDbl}>
                    {typeLabels[task.type || 'task'] || task.type}
                </td>
            );
        case 'milestone':
            return (
                <td class={`col-milestone${editableClass}`} onDblClick={onDbl}>
                    {task.milestone ? <span class="milestone-chip">🏁 {task.milestone}</span> : '—'}
                </td>
            );
        case 'labels':
            return (
                <td class={`col-labels${editableClass}`} onDblClick={onDbl}>
                    {(task.labels || []).length > 0
                        ? (task.labels || []).map(l => (
                              <span key={l} class="task-label-tag">
                                  {l}
                              </span>
                          ))
                        : '—'}
                </td>
            );
        case 'date': {
            const field = col.key as 'plannedStart' | 'plannedEnd' | 'completedAt';
            return (
                <td class={`col-date${editableClass}`} onDblClick={onDbl}>
                    {fmtDateOnly(task[field])}
                </td>
            );
        }
        case 'deps': {
            const deps = allTasks.filter(t => task.dependsOn?.includes(t.id));
            return (
                <td class="col-deps">
                    {deps.length > 0
                        ? deps.map(d => (
                              <span key={d.id} class="dep-tag">
                                  {d.status === 'completed' ? '✓ ' : ''}
                                  {d.title}
                              </span>
                          ))
                        : '—'}
                </td>
            );
        }
        default:
            return <td />;
    }
}
