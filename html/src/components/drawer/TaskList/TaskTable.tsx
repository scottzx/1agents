import { h, Fragment } from 'preact';

import { PRIORITY_RANK } from './constants';
import { GridCell } from './TaskGridCell';
import { DataGrid, type GridColumn } from './DataGrid';
import { ALL_COLUMNS, compareTasks, groupValue, isSortable, GROUP_OPTIONS } from './gridConfig';
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

const colDefs = new Map(ALL_COLUMNS.map(c => [c.key, c]));
const TASK_COLUMNS: GridColumn[] = ALL_COLUMNS.map(c => ({
    key: c.key,
    label: c.label,
    width: c.width,
    locked: c.locked,
    groupable: c.groupable,
    sortable: isSortable(c.key),
}));

const rank = (t: Task) => PRIORITY_RANK[t.priority || 'medium'] ?? 2;

export function TaskTable({ tasks, allTasks, loading, onSelectTask, onDeleteTask, onPatchTask }: TaskTableProps) {
    return (
        <DataGrid<Task>
            rows={tasks}
            totalCount={allTasks.length}
            columns={TASK_COLUMNS}
            groupOptions={GROUP_OPTIONS as Array<[string, string]>}
            getRowKey={t => t.id}
            loading={loading}
            emptyAll="暂无任务 —— 点击上方「+ 新建任务」创建第一个。"
            emptyFiltered="没有匹配筛选条件的任务。"
            compare={compareTasks}
            defaultCompare={(a, b) => rank(a) - rank(b) || a.createdAt.localeCompare(b.createdAt)}
            groupValue={(t, key) => groupValue(t, key as Parameters<typeof groupValue>[1])}
            hierarchy={{
                parentId: t => t.parentId,
                label: '显示父子任务层级',
                hint: '排序时子任务只在父任务内排序',
            }}
            rowClass={(t, isChild) =>
                `task-row status-${t.status}${t.issueState === 'closed' ? ' issue-closed' : ''}${
                    isChild ? ' task-row-child' : ''
                }`
            }
            onPatchRow={onPatchTask}
            onOpenRow={t => onSelectTask(t.id)}
            renderCell={(task, col, helpers) => (
                <GridCell
                    key={col.key}
                    task={task}
                    col={colDefs.get(col.key)!}
                    allTasks={allTasks}
                    isChild={helpers.isChild}
                    editing={helpers.editing}
                    onStartEdit={helpers.startEdit}
                    onCommit={helpers.commit}
                    onCancel={helpers.cancel}
                    onOpenDetail={helpers.openDetail}
                />
            )}
            renderActions={task => (
                <Fragment>
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
                </Fragment>
            )}
        />
    );
}
