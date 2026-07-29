import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import type { FeatureCatalog } from '@1agents/core/types/featureCatalog';

import { CalendarBoard } from './CalendarBoard';
import { KanbanBoard } from './KanbanBoard';
import { TaskFilterBar } from './TaskFilterBar';
import type { TaskView } from './TaskFilterBar';
import { TaskTable } from './TaskTable';
import type { ProjectItem } from './types';
import * as viewPrefs from '../../../stores/projectViewPrefs';

interface TasksViewProps {
    workspaceId: string;
    tasks: ProjectItem[];
    loading: boolean;
    onSelectTask: (taskId: string) => void;
    onDeleteTask: (taskId: string) => void;
    onPatchTask: (taskId: string, patch: Record<string, unknown>) => Promise<void>;
    onStatusChange: (taskId: string, status: 'completed' | 'cancelled') => void;
    featureCatalog: FeatureCatalog;
}

// The 任务 tab: a list↔board↔calendar view switch over one shared filter
// (search + status/priority/assignee). All UI state is per-workspace so each
// project remembers its view, sort and filters across reloads.
export function TasksView({
    workspaceId,
    tasks,
    loading,
    onSelectTask,
    onDeleteTask,
    onPatchTask,
    onStatusChange,
    featureCatalog,
}: TasksViewProps) {
    const initial = viewPrefs.allPrefs.value[workspaceId] || viewPrefs.DEFAULT_PREFS;
    const search = useSignal(initial.search);
    const statusFilter = useSignal<string[]>(initial.statusFilter);
    const priorityFilter = useSignal<string[]>(initial.priorityFilter);
    const assigneeFilter = useSignal<string[]>(initial.assigneeFilter);
    const taskView = useSignal<TaskView>(initial.taskView);

    // When switching projects, re-init all signals from the new workspace's
    // stored prefs so we don't bleed one project's view state into another.
    useEffect(() => {
        const p = viewPrefs.allPrefs.value[workspaceId] || viewPrefs.DEFAULT_PREFS;
        search.value = p.search;
        statusFilter.value = p.statusFilter;
        priorityFilter.value = p.priorityFilter;
        assigneeFilter.value = p.assigneeFilter;
        taskView.value = p.taskView;
    }, [workspaceId]);

    // Persist on every change. The deps include workspaceId so we write into
    // the right slot after a project switch.
    useEffect(() => {
        viewPrefs.updatePrefs(workspaceId, {
            search: search.value,
            statusFilter: statusFilter.value,
            priorityFilter: priorityFilter.value,
            assigneeFilter: assigneeFilter.value,
            taskView: taskView.value,
        });
    }, [workspaceId, search.value, statusFilter.value, priorityFilter.value, assigneeFilter.value, taskView.value]);

    const assignees = Array.from(new Set(tasks.map(t => t.assignee || 'claudecode')));

    const q = search.value.trim().toLowerCase();
    const filtered = tasks.filter(t => {
        if (q && !(t.title.toLowerCase().includes(q) || (t.number ? `#${t.number}`.includes(q) : false))) return false;
        if (statusFilter.value.length && !statusFilter.value.includes(t.status)) return false;
        if (priorityFilter.value.length && !priorityFilter.value.includes(t.priority || 'medium')) return false;
        if (assigneeFilter.value.length && !assigneeFilter.value.includes(t.assignee || 'claudecode')) return false;
        return true;
    });

    return (
        <div class="task-view-shell">
            <TaskFilterBar
                search={search}
                statusFilter={statusFilter}
                priorityFilter={priorityFilter}
                assigneeFilter={assigneeFilter}
                assignees={assignees}
                taskView={taskView}
            />

            {taskView.value === 'list' && (
                <TaskTable
                    workspaceId={workspaceId}
                    tasks={filtered}
                    allTasks={tasks}
                    loading={loading}
                    onSelectTask={onSelectTask}
                    onDeleteTask={onDeleteTask}
                    onPatchTask={onPatchTask}
                    featureCatalog={featureCatalog}
                />
            )}
            {taskView.value === 'board' && (
                <KanbanBoard
                    tasks={filtered}
                    loading={loading}
                    onSelectTask={onSelectTask}
                    onStatusChange={onStatusChange}
                />
            )}
            {taskView.value === 'calendar' && (
                <CalendarBoard tasks={filtered} loading={loading} onSelectTask={onSelectTask} />
            )}
        </div>
    );
}
