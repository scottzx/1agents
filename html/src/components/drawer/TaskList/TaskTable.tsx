import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';

import { PRIORITY_RANK } from './constants';
import { GridCell } from './TaskGridCell';
import { TaskGridToolbar } from './TaskGridToolbar';
import type { ColState } from './TaskGridToolbar';
import { ALL_COLUMNS, compareTasks, groupValue, isSortable } from './gridConfig';
import type { ColumnDef, GroupKey } from './gridConfig';
import type { Task } from './types';

interface TaskTableProps {
    /** Tasks to render (already filtered by the shared TaskFilterBar). */
    tasks: Task[];
    /** Full task set for dependency resolution and empty-state messaging. */
    allTasks: Task[];
    loading: boolean;
    onSelectTask: (taskId: string) => void;
    onDeleteTask: (taskId: string) => void;
    onPatchTask: (taskId: string, patch: Record<string, unknown>) => Promise<void>;
}

const cellKey = (taskId: string, colKey: string) => `${taskId}:${colKey}`;

export function TaskTable({ tasks, allTasks, loading, onSelectTask, onDeleteTask, onPatchTask }: TaskTableProps) {
    const editingCell = useSignal<string | null>(null);
    const groupBy = useSignal<GroupKey>('none');
    const collapsed = useSignal<string[]>([]);
    const sort = useSignal<{ key: string; dir: 'asc' | 'desc' } | null>(null);
    const showHierarchy = useSignal(true);
    const columns = useSignal<ColState[]>(ALL_COLUMNS.map(c => ({ key: c.key, visible: true })));

    if (loading && allTasks.length === 0) {
        return <div class="task-loading">正在载入任务列表...</div>;
    }

    const colDefs = new Map(ALL_COLUMNS.map(c => [c.key, c]));
    const visibleCols = columns.value
        .filter(c => c.visible)
        .map(c => colDefs.get(c.key))
        .filter((c): c is ColumnDef => !!c);
    const colSpan = visibleCols.length + 1;

    const commit = async (taskId: string, patch: Record<string, unknown>) => {
        editingCell.value = null;
        try {
            await onPatchTask(taskId, patch);
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const renderCells = (task: Task, isChild: boolean) =>
        visibleCols.map(col => (
            <GridCell
                key={col.key}
                task={task}
                col={col}
                allTasks={allTasks}
                isChild={isChild}
                editing={editingCell.value === cellKey(task.id, col.key)}
                onStartEdit={() => (editingCell.value = cellKey(task.id, col.key))}
                onCommit={patch => commit(task.id, patch)}
                onCancel={() => (editingCell.value = null)}
                onOpenDetail={() => onSelectTask(task.id)}
            />
        ));

    const actionCell = (task: Task) => (
        <td class="col-actions">
            <button class="task-open-btn" onClick={() => onSelectTask(task.id)} title="打开详情">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="9 18 15 12 9 6" />
                </svg>
            </button>
            <button class="task-delete-btn" onClick={() => onDeleteTask(task.id)} title="删除任务">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="3 6 5 6 21 6" />
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                </svg>
            </button>
        </td>
    );

    const row = (task: Task, isChild: boolean) => {
        const closed = task.issueState === 'closed';
        return (
            <tr
                key={task.id}
                class={`task-row status-${task.status}${closed ? ' issue-closed' : ''}${
                    isChild ? ' task-row-child' : ''
                }`}
            >
                {renderCells(task, isChild)}
                {actionCell(task)}
            </tr>
        );
    };

    // Header-click sort: cycle off → asc → desc → off on the active column.
    const cycleSort = (key: string) => {
        const s = sort.value;
        if (!s || s.key !== key) sort.value = { key, dir: 'asc' };
        else if (s.dir === 'asc') sort.value = { key, dir: 'desc' };
        else sort.value = null;
    };

    // The active comparator: the chosen sort column when set, otherwise the
    // default (priority then creation time).
    const rank = (t: Task) => PRIORITY_RANK[t.priority || 'medium'] ?? 2;
    const cmp = (a: Task, b: Task): number => {
        const s = sort.value;
        if (!s) return rank(a) - rank(b) || a.createdAt.localeCompare(b.createdAt);
        return s.dir === 'asc' ? compareTasks(a, b, s.key) : -compareTasks(a, b, s.key);
    };

    // Hierarchy-aware order: top-level tasks sorted by `cmp`, each parent
    // immediately followed by its children sorted by `cmp` within the parent.
    const orderHierarchical = (list: Task[]): Array<{ task: Task; isChild: boolean }> => {
        const byParent = new Map<string, Task[]>();
        const tops: Task[] = [];
        for (const t of list) {
            if (t.parentId && list.some(p => p.id === t.parentId)) {
                (byParent.get(t.parentId) || byParent.set(t.parentId, []).get(t.parentId)!).push(t);
            } else {
                tops.push(t);
            }
        }
        tops.sort(cmp);
        const out: Array<{ task: Task; isChild: boolean }> = [];
        for (const t of tops) {
            out.push({ task: t, isChild: false });
            for (const c of (byParent.get(t.id) || []).slice().sort(cmp)) out.push({ task: c, isChild: true });
        }
        return out;
    };

    // Grouped buckets: members sorted by the active comparator. Hierarchy is
    // inherently flattened here (children may fall in different groups).
    const buildGroups = (): Array<[string, Task[]]> => {
        const ordered = [...tasks].sort(cmp);
        const map = new Map<string, Task[]>();
        for (const t of ordered) {
            const g = groupValue(t, groupBy.value);
            (map.get(g) || map.set(g, []).get(g)!).push(t);
        }
        return Array.from(map.entries());
    };

    const toggleGroup = (g: string) => {
        collapsed.value = collapsed.value.includes(g) ? collapsed.value.filter(x => x !== g) : [...collapsed.value, g];
    };

    return (
        <div class="task-grid">
            <TaskGridToolbar groupBy={groupBy} showHierarchy={showHierarchy} columns={columns} />

            <div class="task-table-scroller">
                <table class="task-table">
                    <thead>
                        <tr>
                            {visibleCols.map(col => {
                                const sortable = isSortable(col.key);
                                const active = sort.value?.key === col.key;
                                return (
                                    <th
                                        key={col.key}
                                        class={`col-${col.key}${sortable ? ' grid-sortable' : ''}${
                                            active ? ' sorted' : ''
                                        }`}
                                        style={{ minWidth: `${col.width}px` }}
                                        onClick={sortable ? () => cycleSort(col.key) : undefined}
                                        title={sortable ? '点击按此列排序' : undefined}
                                    >
                                        {col.label}
                                        {active && (
                                            <span class="sort-ind">{sort.value!.dir === 'asc' ? '▲' : '▼'}</span>
                                        )}
                                    </th>
                                );
                            })}
                            <th class="col-actions" />
                        </tr>
                    </thead>
                    <tbody>
                        {tasks.length === 0 && (
                            <tr class="task-empty-row">
                                <td colSpan={colSpan}>
                                    {allTasks.length === 0
                                        ? '暂无任务 —— 点击上方「+ 新建任务」创建第一个。'
                                        : '没有匹配筛选条件的任务。'}
                                </td>
                            </tr>
                        )}

                        {groupBy.value === 'none' &&
                            (showHierarchy.value
                                ? orderHierarchical(tasks).map(({ task, isChild }) => row(task, isChild))
                                : [...tasks].sort(cmp).map(task => row(task, false)))}

                        {groupBy.value !== 'none' &&
                            buildGroups().map(([g, members]) => {
                                const isCollapsed = collapsed.value.includes(g);
                                return (
                                    <Fragment key={g}>
                                        <tr class="task-group-header" onClick={() => toggleGroup(g)}>
                                            <td colSpan={colSpan}>
                                                <span class="group-caret">{isCollapsed ? '▶' : '▼'}</span>
                                                <span class="group-name">{g}</span>
                                                <span class="group-count">{members.length}</span>
                                            </td>
                                        </tr>
                                        {!isCollapsed && members.map(task => row(task, false))}
                                    </Fragment>
                                );
                            })}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
