import { h } from 'preact';
import { signal, useSignal } from '@preact/signals';

import { CalendarBoard } from './CalendarBoard';
import { KanbanBoard } from './KanbanBoard';
import { TaskFilterBar } from './TaskFilterBar';
import type { TaskView } from './TaskFilterBar';
import { TaskTable } from './TaskTable';
import type { ProjectItem } from './types';

interface TasksViewProps {
    tasks: ProjectItem[];
    loading: boolean;
    onSelectTask: (taskId: string) => void;
    onDeleteTask: (taskId: string) => void;
    onPatchTask: (taskId: string, patch: Record<string, unknown>) => Promise<void>;
    onStatusChange: (taskId: string, status: 'completed' | 'cancelled') => void;
}

// Module-level so the chosen view (列表/看板/日历) survives the unmount when a
// task detail opens — returning from detail keeps you on the same view.
const taskView = signal<TaskView>('list');

// The 任务 tab: a list↔board view switch over one shared filter (search +
// status/priority/assignee). Filtering happens here so both views agree.
export function TasksView({ tasks, loading, onSelectTask, onDeleteTask, onPatchTask, onStatusChange }: TasksViewProps) {
    const search = useSignal('');
    const statusFilter = useSignal<string[]>([]);
    const priorityFilter = useSignal<string[]>([]);
    const assigneeFilter = useSignal<string[]>([]);

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
        <div class="task-grid">
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
                    tasks={filtered}
                    allTasks={tasks}
                    loading={loading}
                    onSelectTask={onSelectTask}
                    onDeleteTask={onDeleteTask}
                    onPatchTask={onPatchTask}
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
