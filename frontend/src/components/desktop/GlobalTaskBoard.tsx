import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import { projectItemService } from '@1agents/core/services/taskService';
import type { ProjectItem } from '../drawer/TaskList/types';
import { TaskFilterBar } from '../drawer/TaskList/TaskFilterBar';
import type { TaskView } from '../drawer/TaskList/TaskFilterBar';
import { KanbanBoard } from '../drawer/TaskList/KanbanBoard';
import { CalendarBoard } from '../drawer/TaskList/CalendarBoard';
import * as taskNav from '../../stores/taskNavStore';

// Global board (跨项目) — issue #91.
//
// Reuses the single-project work-item views (KanbanBoard / CalendarBoard) and
// the shared TaskFilterBar verbatim, only feeding them a cross-project task set
// (projectItemService.listAll) annotated with each task's owning workspace. A project
// filter is layered on top of the existing search + status/priority filters, and
// clicking any card routes to the owning project's task detail via the same
// permalink navigation the single-project board uses.
//
// Board/calendar only: the list view's inline edit/delete are workspace-scoped
// write paths, out of scope for this read-across PMO view.

// Discussions and AI suggestions live in the same table but are board noise —
// drop them, matching the single-project split in TaskList/index.tsx.
function isBoardTask(t: ProjectItem): boolean {
    return t.type !== 'discussion' && t.source !== 'agent-suggested';
}

export function GlobalTaskBoard() {
    const [tasks, setTasks] = useState<ProjectItem[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');

    const search = useSignal('');
    const statusFilter = useSignal<string[]>([]);
    const priorityFilter = useSignal<string[]>([]);
    const assigneeFilter = useSignal<string[]>([]);
    const projectFilter = useSignal<string[]>([]);
    // Calendar is the natural default for a cross-project schedule view, but the
    // board gives the column rollup PMOs want first — start there.
    const taskView = useSignal<TaskView>('board');

    const fetchTasks = useCallback(async () => {
        setError('');
        try {
            setTasks((await projectItemService.listAll()).filter(isBoardTask));
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        fetchTasks();
        const timer = setInterval(fetchTasks, 5000);
        return () => clearInterval(timer);
    }, [fetchTasks]);

    const assignees = Array.from(new Set(tasks.map(t => t.assignee || 'claudecode')));
    const projects = Array.from(
        new Map(
            tasks.filter(t => t.workspaceId).map(t => [t.workspaceId!, t.workspaceName || t.workspaceId!])
        ).entries()
    );

    const q = search.value.trim().toLowerCase();
    const filtered = tasks.filter(t => {
        if (q && !(t.title.toLowerCase().includes(q) || (t.number ? `#${t.number}`.includes(q) : false))) return false;
        if (statusFilter.value.length && !statusFilter.value.includes(t.status)) return false;
        if (priorityFilter.value.length && !priorityFilter.value.includes(t.priority || 'medium')) return false;
        if (assigneeFilter.value.length && !assigneeFilter.value.includes(t.assignee || 'claudecode')) return false;
        if (projectFilter.value.length && !(t.workspaceId && projectFilter.value.includes(t.workspaceId))) return false;
        return true;
    });

    // Open the task in its owning project via the shared permalink navigation:
    // switches workspace, opens the task board, queues the selection. The cockpit
    // page leaves the big screen for the workbench on click, same as 派工 drill-down.
    const openTask = (taskId: string) => {
        const t = tasks.find(x => x.id === taskId);
        if (!t?.workspaceId) return;
        taskNav.openTaskById(t.workspaceId, taskId);
        window.location.href = window.location.origin + window.location.pathname;
    };

    return (
        <div class="global-board">
            <div class="task-view-shell">
                <TaskFilterBar
                    search={search}
                    statusFilter={statusFilter}
                    priorityFilter={priorityFilter}
                    assigneeFilter={assigneeFilter}
                    assignees={assignees}
                    taskView={taskView}
                    views={['board', 'calendar']}
                />

                {projects.length > 1 && (
                    <div class="global-board-projects">
                        <span class="global-board-projects-label">项目</span>
                        {projects.map(([id, name]) => (
                            <button
                                key={id}
                                class={`grid-filter-chip ${projectFilter.value.includes(id) ? 'active' : ''}`}
                                onClick={() =>
                                    (projectFilter.value = projectFilter.value.includes(id)
                                        ? projectFilter.value.filter(v => v !== id)
                                        : [...projectFilter.value, id])
                                }
                            >
                                {name}
                            </button>
                        ))}
                        {projectFilter.value.length > 0 && (
                            <button class="grid-filter-clear" onClick={() => (projectFilter.value = [])}>
                                清除
                            </button>
                        )}
                    </div>
                )}

                {error && <div class="task-error">{error}</div>}

                {taskView.value === 'calendar' ? (
                    <CalendarBoard tasks={filtered} loading={loading} onSelectTask={openTask} showProject />
                ) : (
                    <KanbanBoard
                        tasks={filtered}
                        loading={loading}
                        onSelectTask={openTask}
                        onStatusChange={() => undefined}
                        showProject
                    />
                )}
            </div>
        </div>
    );
}
