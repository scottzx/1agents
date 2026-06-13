import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import type { Session } from '../../types';
import { CreateTaskForm } from './CreateTaskForm';
import type { Task } from './types';
import { TaskDetail } from './TaskDetail';
import { TaskTable } from './TaskTable';

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
                <button class="create-task-btn-toggle" onClick={() => (showForm.value = !showForm.value)}>
                    {showForm.value ? '取消创建' : '+ 新建任务'}
                </button>
            </div>

            {showForm.value && (
                <CreateTaskForm workspaceId={workspaceId} tasks={tasks} onCreated={() => fetchTasks()} />
            )}

            {error && <div class="task-error">{error}</div>}

            <div class="task-table-scroller">
                <TaskTable
                    tasks={tasks}
                    loading={loading}
                    onSelectTask={setSelectedTaskId}
                    onDeleteTask={handleDeleteTask}
                />
            </div>
        </div>
    );
}
