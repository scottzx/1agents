import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal, signal } from '@preact/signals';

import type { Session } from '../../types';
import * as taskNav from '../../../stores/taskNavStore';
import * as sessionStore from '../../../stores/sessionStore';
import { agentService } from '../../../services/agentService';
import { projectItemService } from '@1agents/core/services/taskService';
import { Modal } from '../../modal';
import { MilestoneForm } from './MilestoneForm';
import type { MilestoneFields } from './MilestoneForm';
import type { ProjectItem, Milestone } from './types';
import { TaskDetail } from './TaskDetail';
import { TasksView } from './TasksView';
import { Overview } from './Overview';
import { MilestoneView } from './MilestoneView';
import { RequirementPool } from './RequirementPool';
import { DiscussionView } from './DiscussionView';
import { SessionsView } from './SessionsView';

const cachedTasks = signal<Record<string, ProjectItem[]>>({});
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
    const [tasks, setTasksState] = useState<ProjectItem[]>(cachedTasks.value[workspaceId] || []);
    const [milestones, setMilestonesState] = useState<Milestone[]>(cachedMilestones.value[workspaceId] || []);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [internalSelectedTaskId, setInternalSelectedTaskId] = useState<string | null>(null);

    const isControlled = onTaskSelect !== undefined;
    const selectedTaskId = isControlled ? externalSelectedTaskId ?? null : internalSelectedTaskId;
    const setSelectedTaskId = isControlled ? (id: string | null) => onTaskSelect(id) : setInternalSelectedTaskId;
    const showMsForm = useSignal(false); // create-milestone modal (small → stays a modal)
    const showSessions = useSignal(false); // sessions popup, opened from the 总览 会话 card
    const [sessionCount, setSessionCount] = useState(0);
    const view = useSignal<'overview' | 'discussion' | 'requirements' | 'tasks' | 'milestone'>('tasks');

    const setTasks = useCallback(
        (newTasks: ProjectItem[]) => {
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
            setMilestones(await projectItemService.listMilestones(workspaceId));
        } catch {
            // milestones are non-critical; the task list still renders
        }
    }, [workspaceId, setMilestones]);

    const fetchTasks = useCallback(async () => {
        if (!workspaceId) return;
        setLoading(true);
        setError('');
        try {
            setTasks(await projectItemService.list(workspaceId));
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [workspaceId, setTasks]);

    // The 总览 会话 card shows the live count of active (non-archived) sessions.
    const fetchSessionCount = useCallback(async () => {
        if (!workspaceId) return;
        try {
            const data = await agentService.list(workspaceId);
            setSessionCount(data.length);
        } catch {
            // session count is non-critical; the overview still renders
        }
    }, [workspaceId]);

    // Polling tasks status changes every 5 seconds
    useEffect(() => {
        fetchTasks();
        fetchMilestones();
        fetchSessionCount();
        const timer = setInterval(() => {
            fetchTasks();
            fetchMilestones();
            fetchSessionCount();
        }, 5000);
        return () => clearInterval(timer);
    }, [fetchTasks, fetchMilestones, fetchSessionCount]);

    // Reset detail selection and load cached data when switching workspaces
    useEffect(() => {
        setSelectedTaskId(null);
        setTasksState(cachedTasks.value[workspaceId] || []);
        setMilestonesState(cachedMilestones.value[workspaceId] || []);
    }, [workspaceId]);

    // Permalink / autolink navigation: open the requested task once a request
    // targeting this workspace lands. Reading `.value` here subscribes the
    // component, so the effect re-fires when a navigation is queued.
    const pendingNav = taskNav.pendingTaskNav.value;
    useEffect(() => {
        if (pendingNav && pendingNav.workspaceId === workspaceId) {
            setSelectedTaskId(pendingNav.taskId);
            taskNav.consumePendingTaskNav();
        }
    }, [pendingNav, workspaceId]);

    // Drag-to-retire on the Kanban board. The backend only accepts terminal
    // states here, so this can mark a card done or cancelled but never run it.
    const handleStatusChange = useCallback(
        async (taskId: string, status: 'completed' | 'cancelled') => {
            try {
                await projectItemService.patch(taskId, { status });
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
            const updated = await projectItemService.patch(taskId, patch);
            const cur = cachedTasks.value[workspaceId] || [];
            setTasks(cur.map(t => (t.id === taskId ? updated : t)));
        },
        [workspaceId, setTasks]
    );

    const handleDeleteTask = async (taskId: string) => {
        if (!confirm('确定要删除该任务吗？')) return;
        try {
            await projectItemService.remove(taskId, workspaceId);
            if (selectedTaskId === taskId) setSelectedTaskId(null);
            fetchTasks();
        } catch (err) {
            alert((err as Error).message);
        }
    };

    // The 讨论区 doubles as a notepad / requirement-grooming space: jot down
    // fuzzy ideas and directions before they're clear enough to be a requirement.
    // New discussions are created by talking to the PM, not a form: it decides
    // through the dialogue whether to record a discussion card (still fuzzy) or
    // create a requirement (clear, with a deliverable).
    const startDiscussionWithPM = useCallback(() => {
        const prompt = [
            '我想和你聊一个新的想法 / 方向。',
            '',
            '请通过对话帮我厘清：如果目标清晰、有明确交付物，就用 create_task 建一条 requirement；如果还不够清晰，就用 create_discussion 建一张讨论卡片，留待以后继续讨论。先问我想聊什么。',
        ].join('\n');
        return sessionStore.createPMSession(workspaceId, '新讨论', prompt);
    }, [workspaceId]);

    // 采纳 an AI suggestion: clear its source marker so the card stops being a
    // suggestion and joins the board as a normal task (the scheduler can then
    // pick it up per its type/status). Reuses the inline PATCH path so the
    // change lands instantly in the cached list.
    const handleAdoptSuggestion = useCallback(
        (taskId: string) => handlePatchTask(taskId, { source: '' }),
        [handlePatchTask]
    );

    // 忽略 an AI suggestion: withdraw it entirely (dismiss). Suggestions are
    // disposable proposals, so this deletes rather than retiring to a board
    // column.
    const handleDismissSuggestion = useCallback(
        async (taskId: string) => {
            if (!confirm('忽略这条 AI 建议？将从建议列表中移除。')) return;
            try {
                await projectItemService.remove(taskId, workspaceId);
                fetchTasks();
            } catch (err) {
                alert((err as Error).message);
            }
        },
        [workspaceId, fetchTasks]
    );

    // When hosted inside the panel (controlled mode), publish the current view's
    // create action to the panel header instead of rendering an inline button
    // row. Standalone (ContentViewHost) keeps its own inline buttons. The AI 建议
    // view has no create action — suggestions are emitted by agents, not users.
    useEffect(() => {
        if (!isControlled) {
            taskNav.taskAddAction.value = null;
            return;
        }
        if (view.value === 'discussion') {
            taskNav.taskAddAction.value = { title: '新建讨论', run: () => void startDiscussionWithPM() };
        } else if (view.value === 'milestone') {
            taskNav.taskAddAction.value = { title: '新建里程碑', run: () => (showMsForm.value = true) };
        } else {
            taskNav.taskAddAction.value = null;
        }
        return () => {
            taskNav.taskAddAction.value = null;
        };
    }, [isControlled, view.value, startDiscussionWithPM, showMsForm]);

    const createMilestone = useCallback(
        async (fields: MilestoneFields) => {
            await projectItemService.createMilestone(workspaceId, fields as unknown as Record<string, unknown>);
            await fetchMilestones();
        },
        [workspaceId, fetchMilestones]
    );

    const patchMilestone = useCallback(
        async (id: string, patch: Record<string, unknown>) => {
            await projectItemService.patchMilestone(id, workspaceId, patch);
            await Promise.all([fetchMilestones(), fetchTasks()]);
        },
        [workspaceId, fetchMilestones, fetchTasks]
    );

    const deleteMilestone = useCallback(
        async (id: string) => {
            await projectItemService.removeMilestone(id, workspaceId);
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

    // Discussions (type === 'discussion', the notepad / grooming space) and AI
    // suggestions (source === 'agent-suggested') live in the same tasks table but
    // must never leak into the board/KPI views (scheduler, Kanban, Overview all
    // treat them as noise). Split once here: boardTasks feeds the work-item views,
    // discussions the 讨论 tab, suggestions are merged into the 需求 pool (filterable).
    const discussions = tasks.filter(t => t.type === 'discussion');
    const suggestions = tasks.filter(t => t.source === 'agent-suggested');
    const boardTasks = tasks.filter(t => t.type !== 'discussion' && t.source !== 'agent-suggested');
    // The 任务 tab shows executable work only. Requirements and bugs are
    // open/closed issues, not schedulable rows — they live in the 需求池. The
    // 里程碑 roadmap keeps all three kinds and switches between them via its own
    // lens, so it receives boardTasks directly.
    const workItems = boardTasks.filter(t => !t.type || t.type === 'task');

    return (
        <div class="task-dashboard-container">
            <div class="task-dashboard-header">
                <div class="task-view-switcher">
                    {(
                        [
                            ['overview', '总览'],
                            ['discussion', '讨论'],
                            ['requirements', '需求'],
                            ['tasks', '任务'],
                            ['milestone', '里程碑'],
                        ] as Array<[typeof view.value, string]>
                    ).map(([key, label]) => (
                        <button key={key} class={view.value === key ? 'active' : ''} onClick={() => (view.value = key)}>
                            {label}
                        </button>
                    ))}
                </div>
                {/* Tasks/requirements/bugs are created only by agents (via MCP tools),
                    never through a human form; AI 建议 likewise comes from agents.
                    Discussions and milestones are the two user-creatable items. When
                    hosted in the panel these "+" actions move to panel-tabs-header (see
                    the taskAddAction bridge); standalone keeps them inline here. */}
                {!isControlled && (
                    <div class="task-header-actions">
                        {view.value === 'discussion' && (
                            <button class="task-add-icon-btn" title="新建讨论" onClick={startDiscussionWithPM}>
                                +
                            </button>
                        )}
                        {view.value === 'milestone' && (
                            <button
                                class="task-add-icon-btn"
                                title="新建里程碑"
                                onClick={() => (showMsForm.value = true)}
                            >
                                +
                            </button>
                        )}
                    </div>
                )}
            </div>

            {error && <div class="task-error">{error}</div>}

            {view.value === 'tasks' && (
                <TasksView
                    tasks={workItems}
                    loading={loading}
                    onSelectTask={setSelectedTaskId}
                    onDeleteTask={handleDeleteTask}
                    onPatchTask={handlePatchTask}
                    onStatusChange={handleStatusChange}
                />
            )}
            {view.value === 'overview' && (
                <Overview
                    tasks={boardTasks}
                    sessionCount={sessionCount}
                    onOpenSessions={() => (showSessions.value = true)}
                />
            )}
            {view.value === 'discussion' && <DiscussionView tasks={discussions} onSelectTask={setSelectedTaskId} />}
            {view.value === 'milestone' && (
                <MilestoneView
                    tasks={boardTasks}
                    milestones={milestones}
                    onSelectTask={setSelectedTaskId}
                    onPatchMilestone={patchMilestone}
                    onDeleteMilestone={deleteMilestone}
                />
            )}
            {view.value === 'requirements' && (
                <RequirementPool
                    tasks={boardTasks}
                    suggestions={suggestions}
                    onSelectTask={setSelectedTaskId}
                    onAdopt={handleAdoptSuggestion}
                    onDismiss={handleDismissSuggestion}
                />
            )}

            <Modal show={showMsForm.value}>
                <MilestoneForm
                    milestones={milestones}
                    onClose={() => (showMsForm.value = false)}
                    onSubmit={createMilestone}
                />
            </Modal>

            {/* Sessions popup — opened from the 总览 会话 card. SessionsView is a
                wide DataGrid, so it uses its own roomy overlay rather than the
                narrow form Modal above. Backdrop / ✕ both close it. */}
            {showSessions.value && (
                <div class="sessions-modal-overlay" onClick={() => (showSessions.value = false)}>
                    <div class="sessions-modal-box" onClick={e => e.stopPropagation()}>
                        <div class="sessions-modal-header">
                            <span>会话</span>
                            <button
                                class="sessions-modal-close"
                                title="关闭"
                                onClick={() => (showSessions.value = false)}
                            >
                                ✕
                            </button>
                        </div>
                        <SessionsView
                            workspaceId={workspaceId}
                            onSelectSession={s => {
                                showSessions.value = false;
                                onSelectSession?.(s);
                            }}
                            onSelectTask={id => {
                                showSessions.value = false;
                                setSelectedTaskId(id);
                            }}
                        />
                    </div>
                </div>
            )}
        </div>
    );
}
