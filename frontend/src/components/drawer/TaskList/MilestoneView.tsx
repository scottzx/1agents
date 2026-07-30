import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { useEffect, useState } from 'preact/hooks';

import { featureCatalogService } from '@1agents/core/services/featureCatalogService';
import type { FeatureCatalog, GanttData } from '@1agents/core/types/featureCatalog';
import { Modal } from '../../modal';
import { GanttChart } from './GanttChart';
import { MilestoneForm } from './MilestoneForm';
import { TaskPreviewDrawer } from './TaskPreviewDrawer';
import { PRIORITY_LABELS, STATUS_LABELS } from './constants';
import { splitMilestones } from './milestoneModel';
import type { ProjectItem, Milestone } from './types';

interface MilestoneViewProps {
    workspaceId: string;
    tasks: ProjectItem[];
    milestones: Milestone[];
    featureCatalog?: FeatureCatalog;
    focusMilestoneId?: string;
    onOpenFeatureVersion?: (milestoneId: string) => void;
    onSelectTask: (taskId: string) => void;
    onPatchMilestone: (id: string, patch: Record<string, unknown>) => Promise<void>;
    onDeleteMilestone: (id: string) => Promise<void>;
}

// UNGROUPED is a synthetic node holding tasks with no milestone. It stays
// outside both the SemVer tree and the legacy history section.
const UNGROUPED = '__ungrouped__';
const COLLAPSED = '__collapsed__';

interface Node {
    id: string;
    name: string;
    milestone: Milestone | null;
    tasks: ProjectItem[];
    done: number;
    total: number;
    pct: number;
    complete: boolean;
}

function fmtDate(s?: string): string {
    return s ? s.slice(0, 10) : '';
}

export function MilestoneView({
    workspaceId,
    tasks,
    milestones,
    featureCatalog,
    focusMilestoneId,
    onOpenFeatureVersion,
    onSelectTask,
    onPatchMilestone,
    onDeleteMilestone,
}: MilestoneViewProps) {
    const expandedId = useSignal<string | null>(null); // null or COLLAPSED = closed, else milestone id
    const editing = useSignal<Milestone | null>(null); // milestone being edited (modal)
    const [viewMode, setViewMode] = useState<'roadmap' | 'gantt'>('roadmap');
    const [ganttData, setGanttData] = useState<GanttData | null>(null);
    const [ganttLoading, setGanttLoading] = useState(false);
    const [ganttError, setGanttError] = useState('');
    const [ganttMilestoneId, setGanttMilestoneId] = useState('');
    // The task the user clicked in the milestone drawer. Opens a bottom-up
    // preview drawer first; full task detail only happens if they hit "打开完整".
    const taskPreview = useSignal<ProjectItem | null>(null);
    // Which item kind the roadmap lens shows. 任务 is the default (executable
    // work); 需求/缺陷 switch the tree + detail to the issue items sharing that
    // milestone. Progress/"done" then means closed for issues, completed for tasks.
    const typeFilter = useSignal<'task' | 'requirement' | 'bug'>('task');
    const isTask = typeFilter.value === 'task';
    const isDone = (t: ProjectItem) => (isTask ? t.status === 'completed' : t.issueState === 'closed');

    useEffect(() => {
        setGanttData(null);
        setGanttError('');
        setViewMode('roadmap');
    }, [workspaceId]);

    useEffect(() => {
        if (viewMode !== 'gantt' || ganttData) return;
        let active = true;
        setGanttLoading(true);
        setGanttError('');
        featureCatalogService
            .gantt(workspaceId)
            .then(data => {
                if (active) setGanttData(data);
            })
            .catch(cause => {
                if (active) setGanttError(cause instanceof Error ? cause.message : String(cause));
            })
            .finally(() => {
                if (active) setGanttLoading(false);
            });
        return () => {
            active = false;
        };
    }, [viewMode, ganttData, workspaceId]);

    const focusedMilestone = focusMilestoneId
        ? milestones.find(milestone => milestone.id === focusMilestoneId)
        : undefined;
    useEffect(() => {
        if (!focusedMilestone) return;
        expandedId.value = focusedMilestone.id;
        setGanttMilestoneId(focusedMilestone.id);
        setViewMode('roadmap');
    }, [focusMilestoneId, focusedMilestone?.id, focusedMilestone?.name, focusedMilestone?.version]);

    // Group the selected kind by milestone name (kept in sync on rename, so name
    // is a stable key).
    const byName = new Map<string, ProjectItem[]>();
    for (const t of tasks) {
        if ((t.type || 'task') !== typeFilter.value) continue;
        const key = t.milestone || UNGROUPED;
        if (!byName.has(key)) byName.set(key, []);
        byName.get(key)!.push(t);
    }

    const mkNode = (m: Milestone): Node => {
        const items = byName.get(m.name) || [];
        // Server-side milestone totals count executable tasks, so only trust them
        // for the 任务 lens; for 需求/缺陷 derive counts from the filtered items.
        const progressItems = isTask ? items.filter(it => it.status !== 'cancelled') : items;
        const total = isTask ? m.total ?? progressItems.length : progressItems.length;
        const done = isTask ? m.completed ?? progressItems.filter(isDone).length : progressItems.filter(isDone).length;
        return {
            id: m.id,
            name: m.name,
            milestone: m,
            tasks: items,
            done,
            total,
            pct: total ? Math.round((done / total) * 100) : 0,
            complete: total > 0 && done >= total,
        };
    };

    const { versions, legacy } = splitMilestones(milestones);
    const versionNodes = versions.map(mkNode);
    const legacyNodes = legacy.map(mkNode);
    const allNodes = [...versionNodes, ...legacyNodes];
    const nodeMap = new Map(allNodes.map(node => [node.id, node]));
    const ungrouped = byName.get(UNGROUPED) || [];
    let ungroupedNode: Node | undefined;
    if (ungrouped.length) {
        const progress = ungrouped.filter(it => !isTask || it.status !== 'cancelled');
        const done = progress.filter(isDone).length;
        ungroupedNode = {
            id: UNGROUPED,
            name: '未分组',
            milestone: null,
            tasks: ungrouped,
            done,
            total: progress.length,
            pct: Math.round((done / ungrouped.length) * 100),
            complete: done >= ungrouped.length,
        };
    }

    // The newest incomplete version is the current recent plan. Legacy and
    // ungrouped milestones never receive the current-version marker.
    const currentId = versionNodes.find(node => !node.complete)?.id ?? null;
    const findNode = (id: string | null) => nodeMap.get(id || '') || (id === UNGROUPED ? ungroupedNode : undefined);

    const effectiveId = expandedId.value === COLLAPSED ? null : expandedId.value;
    const expanded = effectiveId ? findNode(effectiveId) : undefined;
    const today = new Date().toISOString().slice(0, 10);

    const predecessorName = (m: Milestone | null) =>
        m?.predecessorId ? nodeMap.get(m.predecessorId)?.name : undefined;
    const successors = (m: Milestone | null) =>
        m ? allNodes.filter(node => node.milestone?.predecessorId === m.id) : [];
    const featureCount = (milestoneId: string) =>
        featureCatalog?.nodes.filter(node => node.kind === 'feature' && node.targetMilestoneId === milestoneId)
            .length ?? 0;

    const openGantt = (milestone?: Milestone | null) => {
        setGanttMilestoneId(milestone?.id || '');
        setViewMode('gantt');
        expandedId.value = COLLAPSED;
    };

    function renderCard(n: Node, compact = false) {
        const isCurrent = n.id === currentId;
        const state = n.id === UNGROUPED ? 'ungrouped' : isCurrent ? 'current' : n.complete ? 'past' : 'future';
        const isExpanded = expanded?.id === n.id;
        const overdue = !!n.milestone?.targetDate && !n.complete && fmtDate(n.milestone.targetDate) < today;
        const predecessor = predecessorName(n.milestone);
        return (
            <button
                type="button"
                class={`ms-card state-${state}${isExpanded ? ' expanded' : ''}${compact ? ' compact' : ''}`}
                onClick={() => (expandedId.value = isExpanded ? COLLAPSED : n.id)}
            >
                {isCurrent && <span class="ms-card-flag">当前</span>}
                <div class="ms-card-head">
                    <span class="ms-card-dot">{isCurrent && <span class="pulse-indicator" />}</span>
                    <span class="ms-card-name">{n.milestone?.version || n.name}</span>
                    <span class="ms-card-progress-state">{n.complete ? '已完成' : `${n.pct}%`}</span>
                </div>
                {n.milestone?.description && <div class="ms-card-description">{n.milestone.description}</div>}
                <div class="ms-card-bar">
                    <div class="ms-card-fill" style={{ width: `${n.pct}%` }} />
                </div>
                <div class="ms-card-meta">
                    <span class="ms-card-count">{`进度 ${n.done} / ${n.total}`}</span>
                    {n.milestone && (
                        <span class={`ms-card-date${overdue ? ' overdue' : ''}`}>
                            {`目标日期 ${fmtDate(n.milestone.targetDate) || '—'}`}
                        </span>
                    )}
                </div>
                {!compact && predecessor && <span class="ms-card-predecessor">{`前序 ${predecessor}`}</span>}
            </button>
        );
    }

    return (
        <div class="milestone-view">
            <div class="milestone-view-toolbar" aria-label="里程碑视图">
                <div class="milestone-view-switcher">
                    <button
                        type="button"
                        class={viewMode === 'roadmap' ? 'active' : ''}
                        onClick={() => setViewMode('roadmap')}
                    >
                        版本路线图
                    </button>
                    <button type="button" class={viewMode === 'gantt' ? 'active' : ''} onClick={() => openGantt()}>
                        甘特图
                    </button>
                </div>
                <span>路线图回答交付什么，甘特图回答何时交付。</span>
            </div>

            {viewMode === 'gantt' ? (
                ganttLoading ? (
                    <div class="task-loading">正在加载甘特图…</div>
                ) : ganttError ? (
                    <div class="task-error">甘特图加载失败：{ganttError}</div>
                ) : ganttData ? (
                    <GanttChart
                        workspaceId={workspaceId}
                        data={ganttData}
                        initialMilestoneId={ganttMilestoneId}
                        onMilestoneChange={setGanttMilestoneId}
                    />
                ) : (
                    <div class="task-loading">暂无可展示的排期数据。</div>
                )
            ) : (
                <Fragment>
                    {milestones.length === 0 && ungrouped.length === 0 ? (
                        <div class="task-loading">还没有里程碑。用右上角「+ 新建里程碑」开始规划路线图。</div>
                    ) : versionNodes.length > 0 ? (
                        <div class="ms-tree" aria-label="语义化版本树">
                            {versionNodes.map((node, index) => {
                                const next = versionNodes[index + 1];
                                const linked = !!next && node.milestone?.predecessorId === next.id;
                                return (
                                    <div
                                        key={node.id}
                                        class={`ms-version-row${linked ? ' linked' : ''}${next ? '' : ' last'}`}
                                    >
                                        {renderCard(node)}
                                    </div>
                                );
                            })}
                        </div>
                    ) : (
                        <div class="ms-version-empty">还没有语义化版本。用右上角「+ 新建里程碑」创建第一个版本。</div>
                    )}

                    {legacyNodes.length > 0 && (
                        <details class="ms-legacy">
                            <summary>{`历史里程碑（${legacyNodes.length}）`}</summary>
                            <div class="ms-legacy-list">{legacyNodes.map(node => renderCard(node, true))}</div>
                        </details>
                    )}

                    {ungroupedNode && (
                        <section class="ms-ungrouped">
                            <h3>未分组任务</h3>
                            {renderCard(ungroupedNode, true)}
                        </section>
                    )}

                    {expanded && (
                        <Fragment>
                            <div class="milestone-drawer-backdrop" onClick={() => (expandedId.value = COLLAPSED)} />
                            <div class="milestone-detail">
                                <div class="milestone-detail-head">
                                    <div class="milestone-detail-context">
                                        {(() => {
                                            const prev = predecessorName(expanded.milestone);
                                            return (
                                                <Fragment>
                                                    {prev && <span class="milestone-ctx prev">◀ {prev}</span>}
                                                    <span class="milestone-ctx cur">{expanded.name}</span>
                                                    {successors(expanded.milestone).map(c => (
                                                        <span key={c.id} class="milestone-ctx next">
                                                            {c.name} ▶
                                                        </span>
                                                    ))}
                                                </Fragment>
                                            );
                                        })()}
                                    </div>
                                    <div class="milestone-detail-actions">
                                        {/* 需求/任务/缺陷 透镜切换 — sits left of 编辑/删除. Changing it
                                re-filters the whole roadmap while keeping this milestone open. */}
                                        <select
                                            class="ms-type-select"
                                            value={typeFilter.value}
                                            onChange={(e: Event) =>
                                                (typeFilter.value = (e.target as HTMLSelectElement).value as
                                                    | 'task'
                                                    | 'requirement'
                                                    | 'bug')
                                            }
                                        >
                                            <option value="task">任务</option>
                                            <option value="requirement">需求</option>
                                            <option value="bug">缺陷</option>
                                        </select>
                                        {expanded.milestone && (
                                            <Fragment>
                                                <button onClick={() => (editing.value = expanded.milestone)}>
                                                    编辑
                                                </button>
                                                <button
                                                    class="danger"
                                                    onClick={async () => {
                                                        if (
                                                            !confirm(
                                                                `删除里程碑「${expanded.name}」？任务会回到未分组。`
                                                            )
                                                        )
                                                            return;
                                                        try {
                                                            await onDeleteMilestone(expanded.milestone!.id);
                                                            if (expandedId.value === expanded.id)
                                                                expandedId.value = COLLAPSED;
                                                        } catch (err) {
                                                            alert((err as Error).message);
                                                        }
                                                    }}
                                                >
                                                    删除
                                                </button>
                                            </Fragment>
                                        )}
                                    </div>
                                </div>

                                {expanded.milestone?.description && (
                                    <div class="milestone-detail-desc">{expanded.milestone.description}</div>
                                )}

                                {expanded.milestone && (
                                    <div class="milestone-delivery-summary">
                                        <div>
                                            <span>目标日期</span>
                                            <strong>{fmtDate(expanded.milestone.targetDate) || '未指定'}</strong>
                                        </div>
                                        <div>
                                            <span>交付进度</span>
                                            <strong>
                                                {expanded.done}/{expanded.total} · {expanded.pct}%
                                            </strong>
                                        </div>
                                        <div>
                                            <span>功能范围</span>
                                            {onOpenFeatureVersion ? (
                                                <button
                                                    type="button"
                                                    onClick={() => onOpenFeatureVersion(expanded.milestone!.id)}
                                                >
                                                    {featureCount(expanded.milestone.id)} 个功能点 →
                                                </button>
                                            ) : (
                                                <strong>{featureCount(expanded.milestone.id)} 个功能点</strong>
                                            )}
                                        </div>
                                        <button
                                            type="button"
                                            class="milestone-open-gantt"
                                            onClick={() => openGantt(expanded.milestone)}
                                        >
                                            在甘特图中查看
                                        </button>
                                    </div>
                                )}

                                <div class="milestone-detail-body">
                                    {expanded.tasks.length === 0 ? (
                                        <div class="milestone-detail-empty">
                                            {`该里程碑下还没有${isTask ? '任务' : typeFilter.value === 'bug' ? '缺陷' : '需求'}。`}
                                        </div>
                                    ) : (
                                        expanded.tasks.map(task => {
                                            const prio = task.priority || 'medium';
                                            // Issue items (需求/缺陷) are open/closed, not executable —
                                            // show the issue state instead of the workflow status (#4).
                                            const closed = task.issueState === 'closed';
                                            return (
                                                <div
                                                    key={task.id}
                                                    class={`milestone-task-row status-${task.status}`}
                                                    onClick={() => {
                                                        // Click a task → bottom-up preview drawer (no navigation).
                                                        // The preview's "打开完整详情" button is what triggers the
                                                        // full task detail page.
                                                        taskPreview.value = task;
                                                    }}
                                                >
                                                    {isTask ? (
                                                        <span class={`task-status-badge ${task.status}`}>
                                                            {task.status === 'running' && (
                                                                <span class="pulse-indicator" />
                                                            )}
                                                            {STATUS_LABELS[task.status] || task.status}
                                                        </span>
                                                    ) : (
                                                        <span class={`issue-state-badge ${closed ? 'closed' : 'open'}`}>
                                                            {closed ? '已关闭' : '开放'}
                                                        </span>
                                                    )}
                                                    <span class="milestone-task-title">
                                                        {task.parentId && <span class="subtask-indent">└─</span>}
                                                        {task.number ? (
                                                            <span class="task-number">#{task.number}</span>
                                                        ) : null}
                                                        {task.title}
                                                    </span>
                                                    <span class={`priority-badge priority-${prio}`}>
                                                        {PRIORITY_LABELS[prio] || prio}
                                                    </span>
                                                </div>
                                            );
                                        })
                                    )}
                                </div>

                                <TaskPreviewDrawer
                                    open={!!taskPreview.value}
                                    task={taskPreview.value}
                                    onClose={() => (taskPreview.value = null)}
                                    onOpenFull={onSelectTask}
                                />
                            </div>
                        </Fragment>
                    )}
                </Fragment>
            )}

            <Modal show={!!editing.value}>
                {editing.value && (
                    <MilestoneForm
                        milestones={milestones}
                        initial={editing.value}
                        onClose={() => (editing.value = null)}
                        onSubmit={async fields => {
                            await onPatchMilestone(editing.value!.id, fields as unknown as Record<string, unknown>);
                        }}
                    />
                )}
            </Modal>
        </div>
    );
}
