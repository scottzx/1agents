import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal, signal } from '@preact/signals';

import type { Session } from '../../types';
import * as taskNav from '../../../stores/taskNavStore';
import * as sessionStore from '../../../stores/sessionStore';
import * as viewPrefs from '../../../stores/projectViewPrefs';
import * as projectConfig from '../../../stores/projectTabPrefs';
import * as workspaceStore from '../../../stores/workspaceStore';
import { agentService } from '../../../services/agentService';
import { projectItemService } from '@1agents/core/services/taskService';
import { featureCatalogService } from '@1agents/core/services/featureCatalogService';
import type { FeatureCatalog } from '@1agents/core/types/featureCatalog';
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
import { resolveTaskListView, TaskViewSwitcher } from './TaskViewSwitcher';
import { FeatureCatalogView } from './FeatureCatalogView';
import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';

const cachedTasks = signal<Record<string, ProjectItem[]>>({});
const cachedMilestones = signal<Record<string, Milestone[]>>({});
const EMPTY_FEATURE_CATALOG: FeatureCatalog = { nodes: [], links: [] };
const cachedFeatureCatalogs = signal<Record<string, FeatureCatalog>>({});

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
    const [featureCatalog, setFeatureCatalogState] = useState<FeatureCatalog>(
        cachedFeatureCatalogs.value[workspaceId] || EMPTY_FEATURE_CATALOG
    );
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [internalSelectedTaskId, setInternalSelectedTaskId] = useState<string | null>(null);
    const [milestoneFocusId, setMilestoneFocusId] = useState<string>();
    const [featureVersionFilterId, setFeatureVersionFilterId] = useState<string>();

    const isControlled = onTaskSelect !== undefined;
    const selectedTaskId = isControlled ? externalSelectedTaskId ?? null : internalSelectedTaskId;
    const setSelectedTaskId = useCallback(
        (id: string | null) => {
            if (onTaskSelect) onTaskSelect(id);
            else setInternalSelectedTaskId(id);
        },
        [onTaskSelect]
    );
    const showMsForm = useSignal(false); // create-milestone modal (small → stays a modal)
    const showSessions = useSignal(false); // sessions popup, opened from the 总览 会话 card
    const [sessionCount, setSessionCount] = useState(0);
    // Top-level view tab (overview/discussion/requirements/features/tasks/milestone) is
    // per-workspace so each project remembers where you left it.
    const view = useSignal<viewPrefs.TaskListView>(
        (viewPrefs.allPrefs.value[workspaceId]?.activeView as viewPrefs.TaskListView) || 'tasks'
    );

    // Re-init the view from the new workspace's stored prefs when switching
    // projects; otherwise we'd carry the prior project's tab across.
    useEffect(() => {
        view.value = (viewPrefs.allPrefs.value[workspaceId]?.activeView as viewPrefs.TaskListView) || 'tasks';
        setMilestoneFocusId(undefined);
        setFeatureVersionFilterId(undefined);
    }, [workspaceId]);

    useEffect(() => {
        viewPrefs.updatePrefs(workspaceId, { activeView: view.value });
    }, [workspaceId, view.value]);

    const workspacePath = workspaceStore.findWorkspaceAnyStatus(workspaceId)?.path ?? '';
    useEffect(() => {
        void projectConfig.ensureLoaded(workspaceId, workspacePath);
    }, [workspaceId, workspacePath]);
    const featureCatalogEnabled = projectConfig.getFeatureCatalogEnabled(workspaceId);
    const configStatus = projectConfig.getProjectConfigStatus(workspaceId);
    const activeView = resolveTaskListView(view.value, featureCatalogEnabled, configStatus.loaded);

    // A persisted `features` view is legal only for projects whose config has
    // finished loading and has the capability enabled. Waiting for the load
    // prevents an enabled project from briefly overwriting its saved view.
    useEffect(() => {
        if (activeView !== view.value) view.value = activeView;
    }, [workspaceId, activeView]);

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

    const setFeatureCatalog = useCallback(
        (next: FeatureCatalog) => {
            setFeatureCatalogState(next);
            cachedFeatureCatalogs.value = { ...cachedFeatureCatalogs.value, [workspaceId]: next };
        },
        [workspaceId]
    );

    // Only the bare picker sentinel "oneshot" has no projects row.
    // Real kind=tmp workspaces (tmp-…) resolve path and load like any other.
    const bareOneshotSentinel = workspaceId === 'oneshot';

    const fetchMilestones = useCallback(async () => {
        if (!workspaceId || workspaceId === 'oneshot') return;
        try {
            setMilestones(await projectItemService.listMilestones(workspaceId));
        } catch {
            // milestones are non-critical; the task list still renders
        }
    }, [workspaceId, setMilestones]);

    const fetchTasks = useCallback(async () => {
        if (!workspaceId || workspaceId === 'oneshot') return;
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

    const fetchFeatureCatalog = useCallback(async () => {
        if (!workspaceId || workspaceId === 'oneshot') return;
        try {
            setFeatureCatalog(await featureCatalogService.get(workspaceId));
        } catch {
            // Traceability is supplementary; task views continue to render.
        }
    }, [workspaceId, setFeatureCatalog]);

    const closeTaskDetail = useCallback(() => {
        setSelectedTaskId(null);
        void fetchTasks();
    }, [fetchTasks, setSelectedTaskId]);

    // Standalone project/assistant task details publish their back action to
    // the global header. Controlled TaskList instances are owned by RightPanel,
    // which also preserves its task-to-task history stack.
    useEffect(() => {
        if (!selectedTaskId || isControlled) return;
        return taskNav.registerHeaderBackAction(
            `task-detail:${workspaceId}`,
            closeTaskDetail,
            taskNav.HEADER_BACK_PRIORITY.detail
        );
    }, [selectedTaskId, isControlled, workspaceId, closeTaskDetail]);

    // The 总览 会话 card shows the live count of active (non-archived) sessions.
    const fetchSessionCount = useCallback(async () => {
        if (!workspaceId || workspaceId === 'oneshot') return;
        try {
            const data = await agentService.list(workspaceId);
            setSessionCount(data.length);
        } catch {
            // session count is non-critical; the overview still renders
        }
    }, [workspaceId]);

    // Polling tasks status changes every 5 seconds
    useEffect(() => {
        if (workspaceId === 'oneshot') {
            setLoading(false);
            setError('');
            setTasksState([]);
            setMilestonesState([]);
            setFeatureCatalogState(EMPTY_FEATURE_CATALOG);
            setSessionCount(0);
            return;
        }
        fetchTasks();
        fetchMilestones();
        fetchFeatureCatalog();
        fetchSessionCount();
        const timer = setInterval(() => {
            fetchTasks();
            fetchMilestones();
            fetchFeatureCatalog();
            fetchSessionCount();
        }, 5000);
        return () => clearInterval(timer);
    }, [workspaceId, fetchTasks, fetchMilestones, fetchFeatureCatalog, fetchSessionCount]);

    // Reset detail selection and load cached data when switching workspaces
    useEffect(() => {
        setSelectedTaskId(null);
        setTasksState(cachedTasks.value[workspaceId] || []);
        setMilestonesState(cachedMilestones.value[workspaceId] || []);
        setFeatureCatalogState(cachedFeatureCatalogs.value[workspaceId] || EMPTY_FEATURE_CATALOG);
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
            await Promise.all([fetchTasks(), fetchFeatureCatalog()]);
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

    // Bare picker sentinel has no projects row; real tmp-* workspaces load normally.
    if (bareOneshotSentinel) {
        return (
            <div class="task-dashboard-container task-oneshot-empty">
                <div class="task-oneshot-empty-inner">
                    <p class="task-oneshot-empty-title">{t('tasks.oneshot.emptyTitle', ui.language.value)}</p>
                    <p class="task-oneshot-empty-desc">{t('tasks.oneshot.emptyDesc', ui.language.value)}</p>
                </div>
            </div>
        );
    }

    if (selectedTaskId) {
        return (
            <TaskDetail
                workspaceId={workspaceId}
                taskId={selectedTaskId}
                allTasks={tasks}
                featureCatalog={featureCatalog}
                onBack={isControlled ? undefined : closeTaskDetail}
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
                <TaskViewSwitcher
                    activeView={activeView}
                    featureCatalogEnabled={featureCatalogEnabled}
                    onSelect={next => (view.value = next)}
                />
                {/* Tasks/requirements/bugs are created only by agents (via MCP tools),
                    never through a human form; AI 建议 likewise comes from agents.
                    Discussions and milestones are the two user-creatable items. When
                    hosted in the panel these "+" actions move to panel-tabs-header (see
                    the taskAddAction bridge); standalone keeps them inline here. */}
                {!isControlled && (
                    <div class="task-header-actions">
                        {activeView === 'discussion' && (
                            <button class="task-add-icon-btn" title="新建讨论" onClick={startDiscussionWithPM}>
                                +
                            </button>
                        )}
                        {activeView === 'milestone' && (
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

            {activeView === 'tasks' && (
                <TasksView
                    workspaceId={workspaceId}
                    tasks={workItems}
                    loading={loading}
                    onSelectTask={setSelectedTaskId}
                    onDeleteTask={handleDeleteTask}
                    onPatchTask={handlePatchTask}
                    onStatusChange={handleStatusChange}
                    featureCatalog={featureCatalog}
                />
            )}
            {activeView === 'overview' && (
                <Overview
                    tasks={boardTasks}
                    sessionCount={sessionCount}
                    onOpenSessions={() => (showSessions.value = true)}
                />
            )}
            {activeView === 'discussion' && <DiscussionView tasks={discussions} onSelectTask={setSelectedTaskId} />}
            {activeView === 'features' && featureCatalogEnabled && (
                <FeatureCatalogView
                    workspaceId={workspaceId}
                    items={tasks}
                    milestones={milestones}
                    versionFilterMilestoneId={featureVersionFilterId}
                    onClearVersionFilter={() => setFeatureVersionFilterId(undefined)}
                    onOpenMilestone={milestoneId => {
                        setMilestoneFocusId(milestoneId);
                        view.value = 'milestone';
                    }}
                    onCatalogChange={setFeatureCatalog}
                    onItemsChange={fetchTasks}
                />
            )}
            {activeView === 'milestone' &&
                (() => {
                    try {
                        return (
                            <MilestoneView
                                workspaceId={workspaceId}
                                tasks={boardTasks}
                                milestones={milestones}
                                featureCatalog={featureCatalog}
                                focusMilestoneId={milestoneFocusId}
                                onOpenFeatureVersion={
                                    featureCatalogEnabled
                                        ? milestoneId => {
                                              setFeatureVersionFilterId(milestoneId);
                                              view.value = 'features';
                                          }
                                        : undefined
                                }
                                onSelectTask={setSelectedTaskId}
                                onPatchMilestone={patchMilestone}
                                onDeleteMilestone={deleteMilestone}
                            />
                        );
                    } catch (e) {
                        console.error('[sidebar] failed to render MilestoneView:', e);
                        return null;
                    }
                })()}
            {activeView === 'requirements' && (
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
                            onSelectSession={async s => {
                                showSessions.value = false;
                                await onSelectSession?.(s);
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
