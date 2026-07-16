import { h, Fragment } from 'preact';

import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';
import { PRIORITY_RANK } from './constants';
import { GridCell } from './TaskGridCell';
import { DataGrid, type GridColumn } from './DataGrid';
import { getAllColumns, compareTasks, groupValue, isSortable, getGroupOptions } from './gridConfig';
import type { ProjectItem } from './types';

interface TaskTableProps {
    /** Workspace owning the sort/groupBy/collapsed prefs being persisted. */
    workspaceId: string;
    /** Tasks to render (already filtered by the shared TaskFilterBar). */
    tasks: ProjectItem[];
    /** Full task set for dependency resolution and empty-state messaging. */
    allTasks: ProjectItem[];
    loading: boolean;
    onSelectTask: (taskId: string) => void;
    onDeleteTask: (taskId: string) => void;
    onPatchTask: (taskId: string, patch: Record<string, unknown>) => Promise<void>;
}

const rank = (task: ProjectItem) => PRIORITY_RANK[task.priority || 'medium'] ?? 2;

export function TaskTable({
    workspaceId,
    tasks,
    allTasks,
    loading,
    onSelectTask,
    onDeleteTask,
    onPatchTask,
}: TaskTableProps) {
    const lang = ui.language.value;
    const cols = getAllColumns(lang);
    const colDefs = new Map(cols.map(c => [c.key, c]));
    const taskColumns: GridColumn[] = cols.map(c => ({
        key: c.key,
        label: c.label,
        width: c.width,
        locked: c.locked,
        groupable: c.groupable,
        sortable: isSortable(c.key),
    }));

    return (
        <DataGrid<ProjectItem>
            workspaceId={workspaceId}
            prefsSurface="tasks"
            rows={tasks}
            totalCount={allTasks.length}
            columns={taskColumns}
            groupOptions={getGroupOptions(lang) as Array<[string, string]>}
            getRowKey={task => task.id}
            loading={loading}
            emptyAll={t('task.table.emptyAll', lang)}
            emptyFiltered={t('task.table.emptyFiltered', lang)}
            compare={compareTasks}
            defaultCompare={(a, b) => rank(a) - rank(b) || a.createdAt.localeCompare(b.createdAt)}
            groupValue={(task, key) => groupValue(task, key as Parameters<typeof groupValue>[1], lang)}
            hierarchy={{
                parentId: task => task.parentId,
                label: t('task.table.hierarchyLabel', lang),
                hint: t('task.table.hierarchyHint', lang),
            }}
            rowClass={(task, isChild) =>
                `task-row status-${task.status}${task.issueState === 'closed' ? ' issue-closed' : ''}${
                    isChild ? ' task-row-child' : ''
                }`
            }
            onPatchRow={onPatchTask}
            onOpenRow={task => onSelectTask(task.id)}
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
                    lang={lang}
                />
            )}
            renderActions={task => (
                <Fragment>
                    <button
                        class="task-open-btn"
                        onClick={() => onSelectTask(task.id)}
                        title={t('task.table.openDetail', lang)}
                    >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6" />
                        </svg>
                    </button>
                    <button
                        class="task-delete-btn"
                        onClick={() => onDeleteTask(task.id)}
                        title={t('task.table.deleteTask', lang)}
                    >
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
