import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import type { Session } from '../../types';
import { CreateTaskForm } from './CreateTaskForm';
import type { Task } from './types';
import { TaskDetail } from './TaskDetail';
import { TaskTable } from './TaskTable';
import { KanbanBoard } from './KanbanBoard';
import { Overview } from './Overview';
import { MilestoneView } from './MilestoneView';
import { RequirementPool } from './RequirementPool';

export interface TaskListProps {
    workspaceId: string;
    onSelectSession?: (session: Session) => void;
}

export function TaskList({ workspaceId, onSelectSession }: TaskListProps) {
    const [tasks, setTasks] = useState<Task[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
    const showForm = useSignal(false);
    const view = useSignal<'table' | 'board' | 'overview' | 'milestone' | 'requirements'>('table');

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
    }, [workspaceId]);

    // Polling tasks status changes every 5 seconds
    useEffect(() => {
        fetchTasks();
        const timer = setInterval(() => {
            fetchTasks();
        }, 5000);
        return () => clearInterval(timer);
    }, [fetchTasks]);

    // Reset detail selection when switching workspaces
    useEffect(() => {
        setSelectedTaskId(null);
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

    if (selectedTaskId) {
        return (
            <TaskDetail
                workspaceId={workspaceId}
                taskId={selectedTaskId}
                allTasks={tasks}
                onBack={() => {
                    setSelectedTaskId(null);
                    fetchTasks();
                }}
                onDelete={handleDeleteTask}
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
                            ['table', '列表'],
                            ['board', '看板'],
                            ['overview', '总览'],
                            ['milestone', '里程碑'],
                            ['requirements', '需求池'],
                        ] as Array<[typeof view.value, string]>
                    ).map(([key, label]) => (
                        <button key={key} class={view.value === key ? 'active' : ''} onClick={() => (view.value = key)}>
                            {label}
                        </button>
                    ))}
                </div>
                <button class="create-task-btn-toggle" onClick={() => (showForm.value = !showForm.value)}>
                    {showForm.value ? '取消创建' : '+ 新建任务'}
                </button>
            </div>

            {showForm.value && (
                <CreateTaskForm workspaceId={workspaceId} tasks={tasks} onCreated={() => fetchTasks()} />
            )}

            {error && <div class="task-error">{error}</div>}

            {view.value === 'table' && (
                <div class="task-table-scroller">
                    <TaskTable
                        tasks={tasks}
                        loading={loading}
                        onSelectTask={setSelectedTaskId}
                        onDeleteTask={handleDeleteTask}
                    />
                </div>
            )}
            {view.value === 'board' && (
                <KanbanBoard
                    tasks={tasks}
                    loading={loading}
                    onSelectTask={setSelectedTaskId}
                    onStatusChange={handleStatusChange}
                />
            )}
            {view.value === 'overview' && <Overview tasks={tasks} />}
            {view.value === 'milestone' && <MilestoneView tasks={tasks} onSelectTask={setSelectedTaskId} />}
            {view.value === 'requirements' && <RequirementPool tasks={tasks} onSelectTask={setSelectedTaskId} />}
        </div>
    );
}
