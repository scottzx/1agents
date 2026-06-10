import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import type { ChatSession, Session, AgentType } from '../types';

interface SessionMetadata {
    id: string;
    kind: 'chat';
    name: string;
    agentType: string;
    status: 'idle' | 'running';
    summary?: string;
    createdAt: string;
}

interface Task {
    id: string;
    title: string;
    status: 'pending' | 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'blocked';
    scheduleType: 'immediate' | 'scheduled';
    scheduledAt?: string;
    dependsOn?: string[];
    createdAt: string;
    updatedAt: string;
    startedAt?: string;
    completedAt?: string;
    summary?: string;
    sessions: SessionMetadata[];
}

interface TaskListProps {
    workspaceId: string;
    onSelectSession?: (session: Session) => void;
}

export function TaskList({ workspaceId, onSelectSession }: TaskListProps) {
    const [tasks, setTasks] = useState<Task[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');

    // Form state for creating a new task
    const [showForm, setShowForm] = useState(false);
    const [title, setTitle] = useState('');
    const [scheduleType, setScheduleType] = useState<'immediate' | 'scheduled'>('immediate');
    const [scheduledAt, setScheduledAt] = useState('');
    const [dependsOn, setDependsOn] = useState<string[]>([]);

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

    const handleCreateTask = async (e: Event) => {
        e.preventDefault();
        if (!title.trim()) return;

        try {
            const res = await fetch('/api/agent/tasks', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    workspace_id: workspaceId,
                    title: title.trim(),
                    scheduleType,
                    scheduledAt:
                        scheduleType === 'scheduled' && scheduledAt ? new Date(scheduledAt).toISOString() : null,
                    dependsOn,
                }),
            });
            if (!res.ok) {
                throw new Error('Failed to create task');
            }
            setTitle('');
            setScheduleType('immediate');
            setScheduledAt('');
            setDependsOn([]);
            setShowForm(false);
            fetchTasks();
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const handleDeleteTask = async (taskId: string) => {
        if (!confirm('确定要删除该任务吗？')) return;
        try {
            const res = await fetch(`/api/agent/tasks/${taskId}?workspace_id=${encodeURIComponent(workspaceId)}`, {
                method: 'DELETE',
            });
            if (!res.ok) {
                throw new Error('Failed to delete task');
            }
            fetchTasks();
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const handleToggleDependency = (taskId: string) => {
        setDependsOn(prev => (prev.includes(taskId) ? prev.filter(id => id !== taskId) : [...prev, taskId]));
    };

    const startNewSession = (task: Task) => {
        if (!onSelectSession) return;
        const sessionId = `sess-${Math.random().toString(36).slice(2, 10)}-${Date.now()}`;
        const newSession: ChatSession = {
            kind: 'chat',
            id: sessionId,
            workspaceId,
            taskId: task.id,
            name: `${task.title} - 智能体`,
            agentType: 'claudecode',
            ccProject: '',
            ccSessionId: '',
            sessionKey: '',
            status: 'idle',
            active: true,
        };
        onSelectSession(newSession);
    };

    const openExistingSession = (task: Task, sess: SessionMetadata) => {
        if (!onSelectSession) return;
        const chatSession: ChatSession = {
            kind: 'chat',
            id: sess.id,
            workspaceId,
            taskId: task.id,
            name: `${task.title} - 智能体`,
            agentType: sess.agentType as AgentType,
            ccProject: '',
            ccSessionId: '',
            sessionKey: '',
            status: sess.status === 'running' ? 'streaming' : 'idle',
            active: true,
        };
        onSelectSession(chatSession);
    };

    const getStatusLabel = (status: string) => {
        switch (status) {
            case 'pending':
                return '等待中';
            case 'queued':
                return '排队中';
            case 'running':
                return '执行中';
            case 'completed':
                return '已完成';
            case 'failed':
                return '失败';
            case 'cancelled':
                return '已取消';
            case 'blocked':
                return '受阻';
            default:
                return status;
        }
    };

    return (
        <div class="task-dashboard-container">
            <div class="task-dashboard-header">
                <button class="create-task-btn-toggle" onClick={() => setShowForm(!showForm)}>
                    {showForm ? '取消创建' : '+ 新建任务'}
                </button>
            </div>

            {showForm && (
                <form class="create-task-form" onSubmit={handleCreateTask}>
                    <div class="form-group">
                        <label>任务标题</label>
                        <input
                            type="text"
                            placeholder="如: 完成新模块开发"
                            value={title}
                            onInput={(e: Event) => setTitle((e.target as HTMLInputElement).value)}
                            required
                        />
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label>调度方式</label>
                            <select
                                value={scheduleType}
                                onChange={(e: Event) =>
                                    setScheduleType((e.target as HTMLSelectElement).value as 'immediate' | 'scheduled')
                                }
                            >
                                <option value="immediate">立即排队</option>
                                <option value="scheduled">定时排队</option>
                            </select>
                        </div>

                        {scheduleType === 'scheduled' && (
                            <div class="form-group">
                                <label>执行时间</label>
                                <input
                                    type="datetime-local"
                                    value={scheduledAt}
                                    onChange={(e: Event) => setScheduledAt((e.target as HTMLInputElement).value)}
                                    required
                                />
                            </div>
                        )}
                    </div>

                    {tasks.length > 0 && (
                        <div class="form-group">
                            <label>前置依赖任务</label>
                            <div class="dependency-checklist">
                                {tasks.map(t => (
                                    <label key={t.id} class="checkbox-label">
                                        <input
                                            type="checkbox"
                                            checked={dependsOn.includes(t.id)}
                                            onChange={() => handleToggleDependency(t.id)}
                                        />
                                        <span>{t.title}</span>
                                    </label>
                                ))}
                            </div>
                        </div>
                    )}

                    <button type="submit" class="submit-task-btn">
                        创建任务
                    </button>
                </form>
            )}

            {error && <div class="task-error">{error}</div>}

            <div class="task-list-scroller">
                {loading && tasks.length === 0 ? (
                    <div class="task-loading">正在载入任务看板...</div>
                ) : tasks.length === 0 ? (
                    <div class="task-empty-state">暂无任务，请点击上方按钮创建。</div>
                ) : (
                    tasks.map(task => {
                        const dependents = tasks.filter(t => task.dependsOn?.includes(t.id));
                        return (
                            <div key={task.id} class={`task-card-item status-${task.status}`}>
                                <div class="task-card-header">
                                    <div class="task-title-group">
                                        <span class={`task-status-badge ${task.status}`}>
                                            {task.status === 'running' && <span class="pulse-indicator" />}
                                            {getStatusLabel(task.status)}
                                        </span>
                                        <h4 class="task-title">{task.title}</h4>
                                    </div>
                                    <button
                                        class="task-delete-btn"
                                        onClick={() => handleDeleteTask(task.id)}
                                        title="删除任务"
                                    >
                                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                            <polyline points="3 6 5 6 21 6" />
                                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                        </svg>
                                    </button>
                                </div>

                                <div class="task-card-details">
                                    {task.scheduleType === 'scheduled' && task.scheduledAt && (
                                        <div class="detail-row">
                                            <span class="detail-label">调度于:</span>
                                            <span class="detail-value">
                                                {new Date(task.scheduledAt).toLocaleString()}
                                            </span>
                                        </div>
                                    )}
                                    {dependents.length > 0 && (
                                        <div class="detail-row">
                                            <span class="detail-label">前置依赖:</span>
                                            <div class="detail-value dependency-tags">
                                                {dependents.map(d => (
                                                    <span key={d.id} class="dep-tag">
                                                        {d.title}
                                                    </span>
                                                ))}
                                            </div>
                                        </div>
                                    )}
                                    {task.completedAt && (
                                        <div class="detail-row">
                                            <span class="detail-label">完成时间:</span>
                                            <span class="detail-value">
                                                {new Date(task.completedAt).toLocaleString()}
                                            </span>
                                        </div>
                                    )}
                                </div>

                                <div class="task-sessions-section">
                                    <div class="sessions-header">
                                        <h5>执行会话列表</h5>
                                        <button
                                            class="new-session-btn"
                                            onClick={() => startNewSession(task)}
                                            disabled={task.status === 'completed' || task.status === 'blocked'}
                                        >
                                            + 新建服务会话
                                        </button>
                                    </div>

                                    {task.sessions && task.sessions.length > 0 ? (
                                        <div class="sessions-list">
                                            {task.sessions.map(s => (
                                                <div key={s.id} class={`session-item-row status-${s.status}`}>
                                                    <div class="session-info">
                                                        <div class="session-meta">
                                                            <span class={`status-dot ${s.status}`} />
                                                            <span class="agent-type-badge">{s.agentType}</span>
                                                            <span class="session-time">
                                                                {new Date(s.createdAt).toLocaleTimeString()}
                                                            </span>
                                                        </div>
                                                        {s.summary && (
                                                            <div class="session-summary-text" title={s.summary}>
                                                                {s.summary}
                                                            </div>
                                                        )}
                                                    </div>
                                                    <button
                                                        class="enter-session-btn"
                                                        onClick={() => openExistingSession(task, s)}
                                                    >
                                                        进入
                                                    </button>
                                                </div>
                                            ))}
                                        </div>
                                    ) : (
                                        <div class="no-sessions-hint">暂无启动的智能体服务会话。</div>
                                    )}
                                </div>
                            </div>
                        );
                    })
                )}
            </div>
        </div>
    );
}
