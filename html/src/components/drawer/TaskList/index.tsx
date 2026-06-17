import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal, signal } from '@preact/signals';

import type { Session } from '../../types';
import { Modal } from '../../modal';
import { MilestoneForm } from './MilestoneForm';
import type { MilestoneFields } from './MilestoneForm';
import type { Task, Milestone } from './types';
import { TaskDetail } from './TaskDetail';
import { TasksView } from './TasksView';
import { Overview } from './Overview';
import { MilestoneView } from './MilestoneView';
import { RequirementPool } from './RequirementPool';
import { SessionsView } from './SessionsView';

const cachedTasks = signal<Record<string, Task[]>>({});
const cachedMilestones = signal<Record<string, Milestone[]>>({});

export interface TaskListProps {
    workspaceId: string;
    onSelectSession?: (session: Session) => void;
    /** Optional controlled state: when provided, TaskList uses these instead of internal state. */
    selectedTaskId?: string | null;
    onTaskSelect?: (taskId: string | null) => void;
}

export function TaskList({
    workspaceId,
    onSelectSession,
    selectedTaskId: externalSelectedTaskId,
    onTaskSelect,
}: TaskListProps) {
    const [tasks, setTasksState] = useState<Task[]>(cachedTasks.value[workspaceId] || []);
    const [milestones, setMilestonesState] = useState<Milestone[]>(cachedMilestones.value[workspaceId] || []);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [internalSelectedTaskId, setInternalSelectedTaskId] = useState<string | null>(null);

    const isControlled = onTaskSelect !== undefined;
    const selectedTaskId = isControlled ? externalSelectedTaskId ?? null : internalSelectedTaskId;
    const setSelectedTaskId = isControlled ? (id: string | null) => onTaskSelect(id) : setInternalSelectedTaskId;
    const showMsForm = useSignal(false); // create-milestone modal (small → stays a modal)
    const view = useSignal<'overview' | 'tasks' | 'requirements' | 'sessions' | 'milestone'>('tasks');

    const setTasks = useCallback(
        (newTasks: Task[]) => {
            setTasksState(newTasks);
            cachedTasks.value = {
                ...cachedTasks.value,
                [workspaceId]: newTasks,
            };
        },
        [workspaceId]
    );

    const setMilestones = useCallback(
        (next: Milestone[]) => {
            setMilestonesState(next);
            cachedMilestones.value = { ...cachedMilestones.value, [workspaceId]: next };
        },
        [workspaceId]
    );

    const fetchMilestones = useCallback(async () => {
        if (!workspaceId) return;
        try {
            const res = await fetch(`/api/agent/milestones?workspace_id=${encodeURIComponent(workspaceId)}`);
            if (!res.ok) return;
            setMilestones((await res.json()) || []);
        } catch {
            // milestones are non-critical; the task list still renders
        }
    }, [workspaceId, setMilestones]);

    const fetchTasks = useCallback(async () => {
        if (!workspaceId) return;
        setLoading(true);
        setError('');
        try {
            const res = await fetch(`/api/agent/tasks?workspace_id=${encodeURIComponent(workspaceId)}`);
            if (!res.ok) {
                throw new Error(`Failed to load tasks: ${res.statusText}`);
            }
            const data = await res.json();
            setTasks(data || []);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [workspaceId, setTasks]);

    // Polling tasks status changes every 5 seconds
    useEffect(() => {
        fetchTasks();
        fetchMilestones();
        const timer = setInterval(() => {
            fetchTasks();
            fetchMilestones();
        }, 5000);
        return () => clearInterval(timer);
    }, [fetchTasks, fetchMilestones]);

    // Reset detail selection and load cached data when switching workspaces
    useEffect(() => {
        setSelectedTaskId(null);
        setTasksState(cachedTasks.value[workspaceId] || []);
        setMilestonesState(cachedMilestones.value[workspaceId] || []);
    }, [workspaceId]);

    // Drag-to-retire on the Kanban board. The backend only accepts terminal
    // states here, so this can mark a card done or cancelled but never run it.
    const handleStatusChange = useCallback(
        async (taskId: string, status: 'completed' | 'cancelled') => {
            try {
                const res = await fetch(`/api/agent/tasks/${taskId}`, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ status }),
                });
                if (!res.ok) throw new Error(await res.text());
                fetchTasks();
            } catch (err) {
                alert((err as Error).message);
            }
        },
        [fetchTasks]
    );

    // Inline grid edit: PATCH a single task and splice the response back into
    // the cached list (no full refetch, so the edit lands instantly even
    // between the 5s polls). The backend rejects scheduler-owned fields.
    const handlePatchTask = useCallback(
        async (taskId: string, patch: Record<string, unknown>) => {
            const res = await fetch(`/api/agent/tasks/${taskId}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(patch),
            });
            if (!res.ok) throw new Error(await res.text());
            const updated = (await res.json()) as Task;
            const cur = cachedTasks.value[workspaceId] || [];
            setTasks(cur.map(t => (t.id === taskId ? updated : t)));
        },
        [workspaceId, setTasks]
    );

    const handleDeleteTask = async (taskId: string) => {
        if (!confirm('确定要删除该任务吗？')) return;
        try {
            const res = await fetch(`/api/agent/tasks/${taskId}?workspace_id=${encodeURIComponent(workspaceId)}`, {
                method: 'DELETE',
            });
            if (!res.ok) {
                throw new Error('Failed to delete task');
            }
            if (selectedTaskId === taskId) setSelectedTaskId(null);
            fetchTasks();
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const createMilestone = useCallback(
        async (fields: MilestoneFields) => {
            const res = await fetch('/api/agent/milestones', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, ...fields }),
            });
            if (!res.ok) throw new Error(await res.text());
            await fetchMilestones();
        },
        [workspaceId, fetchMilestones]
    );

    const patchMilestone = useCallback(
        async (id: string, patch: Record<string, unknown>) => {
            const res = await fetch(`/api/agent/milestones/${id}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, ...patch }),
            });
            if (!res.ok) throw new Error(await res.text());
            await Promise.all([fetchMilestones(), fetchTasks()]);
        },
        [workspaceId, fetchMilestones, fetchTasks]
    );

    const deleteMilestone = useCallback(
        async (id: string) => {
            const res = await fetch(`/api/agent/milestones/${id}?workspace_id=${encodeURIComponent(workspaceId)}`, {
                method: 'DELETE',
            });
            if (!res.ok) throw new Error(await res.text());
            await Promise.all([fetchMilestones(), fetchTasks()]);
        },
        [workspaceId, fetchMilestones, fetchTasks]
    );

    if (selectedTaskId) {
        return (
            <TaskDetail
                workspaceId={workspaceId}
                taskId={selectedTaskId}
                allTasks={tasks}
                onBack={
                    isControlled
                        ? undefined
                        : () => {
                              setSelectedTaskId(null);
                              fetchTasks();
                          }
                }
                onDelete={handleDeleteTask}
                onNavigate={setSelectedTaskId}
                onSelectSession={onSelectSession}
            />
        );
    }

    return (
        <div class="task-dashboard-container">
            <div class="task-dashboard-header">
                <div class="task-view-switcher">
                    {(
                        [
                            ['overview', '总览'],
                            ['tasks', '任务'],
                            ['requirements', '需求'],
                            ['sessions', '会话'],
                            ['milestone', '里程碑'],
                        ] as Array<[typeof view.value, string]>
                    ).map(([key, label]) => (
                        <button key={key} class={view.value === key ? 'active' : ''} onClick={() => (view.value = key)}>
                            {label}
                        </button>
                    ))}
                </div>
                <div class="task-header-actions">
                    {/* Tasks are created only by agents (via MCP tools), never through a
                        human form. Only milestones are user-creatable here. */}
                    {view.value === 'milestone' && (
                        <button class="create-task-btn-toggle" onClick={() => (showMsForm.value = true)}>
                            + 新建里程碑
                        </button>
                    )}
                </div>
            </div>

            {error && <div class="task-error">{error}</div>}

            {view.value === 'tasks' && (
                <TasksView
                    tasks={tasks}
                    loading={loading}
                    onSelectTask={setSelectedTaskId}
                    onDeleteTask={handleDeleteTask}
                    onPatchTask={handlePatchTask}
                    onStatusChange={handleStatusChange}
                />
            )}
            {view.value === 'overview' && <Overview tasks={tasks} />}
            {view.value === 'sessions' && (
                <SessionsView
                    workspaceId={workspaceId}
                    onSelectSession={onSelectSession}
                    onSelectTask={setSelectedTaskId}
                />
            )}
            {view.value === 'milestone' && (
                <MilestoneView
                    tasks={tasks}
                    milestones={milestones}
                    onSelectTask={setSelectedTaskId}
                    onPatchMilestone={patchMilestone}
                    onDeleteMilestone={deleteMilestone}
                />
            )}
            {view.value === 'requirements' && <RequirementPool tasks={tasks} onSelectTask={setSelectedTaskId} />}

            <Modal show={showMsForm.value}>
                <MilestoneForm
                    milestones={milestones}
                    onClose={() => (showMsForm.value = false)}
                    onSubmit={createMilestone}
                />
            </Modal>
        </div>
    );
}
