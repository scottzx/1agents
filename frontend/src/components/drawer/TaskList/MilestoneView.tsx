import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';

import { Modal } from '../../modal';
import { MilestoneForm } from './MilestoneForm';
import { PRIORITY_LABELS, STATUS_LABELS } from './constants';
import type { ProjectItem, Milestone } from './types';

interface MilestoneViewProps {
    tasks: ProjectItem[];
    milestones: Milestone[];
    onSelectTask: (taskId: string) => void;
    onPatchMilestone: (id: string, patch: Record<string, unknown>) => Promise<void>;
    onDeleteMilestone: (id: string) => Promise<void>;
}

// UNGROUPED is a synthetic node holding tasks with no milestone. Rendered as a
// detached root at the tail of the tree; carries no entity (no CRUD).
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
    children: Node[];
}

function fmtDate(s?: string): string {
    return s ? s.slice(0, 10) : '';
}

export function MilestoneView({
    tasks,
    milestones,
    onSelectTask,
    onPatchMilestone,
    onDeleteMilestone,
}: MilestoneViewProps) {
    const expandedId = useSignal<string | null>(null); // null=default(current), COLLAPSED, or id
    const editing = useSignal<Milestone | null>(null); // milestone being edited (modal)
    // Which item kind the roadmap lens shows. 任务 is the default (executable
    // work); 需求/缺陷 switch the tree + detail to the issue items sharing that
    // milestone. Progress/"done" then means closed for issues, completed for tasks.
    const typeFilter = useSignal<'task' | 'requirement' | 'bug'>('task');
    const isTask = typeFilter.value === 'task';
    const isDone = (t: ProjectItem) => (isTask ? t.status === 'completed' : t.issueState === 'closed');

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
        const total = isTask ? m.total || items.length : items.length;
        const done = isTask ? m.completed || items.filter(isDone).length : items.filter(isDone).length;
        return {
            id: m.id,
            name: m.name,
            milestone: m,
            tasks: items,
            done,
            total,
            pct: total ? Math.round((done / total) * 100) : 0,
            complete: total > 0 && done >= total,
            children: [],
        };
    };

    // Build the forest from predecessorId. Siblings (and roots) are ordered by
    // milestone position.
    const ordered = [...milestones].sort((a, b) => a.position - b.position);
    const nodeMap = new Map<string, Node>();
    ordered.forEach(m => nodeMap.set(m.id, mkNode(m)));
    const roots: Node[] = [];
    for (const m of ordered) {
        const node = nodeMap.get(m.id)!;
        const parent = m.predecessorId ? nodeMap.get(m.predecessorId) : undefined;
        if (parent && parent !== node) parent.children.push(node);
        else roots.push(node);
    }
    const ungrouped = byName.get(UNGROUPED) || [];
    if (ungrouped.length) {
        const done = ungrouped.filter(isDone).length;
        roots.push({
            id: UNGROUPED,
            name: '未分组',
            milestone: null,
            tasks: ungrouped,
            done,
            total: ungrouped.length,
            pct: Math.round((done / ungrouped.length) * 100),
            complete: done >= ungrouped.length,
            children: [],
        });
    }

    // "当前" = first non-complete milestone in position order (未分组 excluded).
    const currentId = ordered.find(m => !nodeMap.get(m.id)!.complete)?.id ?? null;

    const allNodes = [...nodeMap.values()];
    const findNode = (id: string | null) =>
        allNodes.find(n => n.id === id) || (id === UNGROUPED ? roots.find(r => r.id === UNGROUPED) : undefined);

    const effectiveId =
        expandedId.value === null ? currentId : expandedId.value === COLLAPSED ? null : expandedId.value;
    const expanded = effectiveId ? findNode(effectiveId) : undefined;
    const today = new Date().toISOString().slice(0, 10);

    if (milestones.length === 0 && ungrouped.length === 0) {
        return <div class="task-loading">还没有里程碑。用右上角「+ 新建里程碑」开始规划路线图。</div>;
    }

    function renderCard(n: Node) {
        const isCurrent = n.id === currentId;
        const state = n.id === UNGROUPED ? 'ungrouped' : isCurrent ? 'current' : n.complete ? 'past' : 'future';
        const isExpanded = expanded?.id === n.id;
        const overdue = !!n.milestone?.targetDate && !n.complete && fmtDate(n.milestone.targetDate) < today;
        return (
            <div
                class={`ms-card state-${state}${isExpanded ? ' expanded' : ''}${n.children.length ? ' has-children' : ''}`}
                onClick={() => (expandedId.value = isExpanded ? COLLAPSED : n.id)}
            >
                {isCurrent && <span class="ms-card-flag">当前</span>}
                <div class="ms-card-head">
                    <span class="ms-card-dot">{isCurrent && <span class="pulse-indicator" />}</span>
                    <span class="ms-card-name">{n.name}</span>
                </div>
                <div class="ms-card-bar">
                    <div class="ms-card-fill" style={{ width: `${n.pct}%` }} />
                </div>
                <div class="ms-card-meta">
                    <span class="ms-card-count">{`${n.done}/${n.total}`}</span>
                    {n.milestone?.targetDate && (
                        <span class={`ms-card-date${overdue ? ' overdue' : ''}`}>
                            🎯 {fmtDate(n.milestone.targetDate)}
                        </span>
                    )}
                </div>
            </div>
        );
    }

    // Recursive branch render (left→right). visited guards against cycles.
    function renderBranch(n: Node, visited: Set<string>) {
        const safeChildren = n.children.filter(c => !visited.has(c.id));
        const next = new Set(visited).add(n.id);
        return (
            <div class="ms-branch" key={n.id}>
                {renderCard(n)}
                {safeChildren.length > 0 && (
                    <div class="ms-children">
                        {safeChildren.map(c => (
                            <div class="ms-subtree" key={c.id}>
                                {renderBranch(c, next)}
                            </div>
                        ))}
                    </div>
                )}
            </div>
        );
    }

    const predecessorName = (m: Milestone | null) =>
        m?.predecessorId ? milestones.find(x => x.id === m.predecessorId)?.name : undefined;

    return (
        <div class="milestone-view">
            <div class="ms-tree">{roots.map(r => renderBranch(r, new Set()))}</div>

            {expanded && (
                <div class="milestone-detail">
                    <div class="milestone-detail-head">
                        <div class="milestone-detail-context">
                            {(() => {
                                const prev = predecessorName(expanded.milestone);
                                return (
                                    <Fragment>
                                        {prev && <span class="milestone-ctx prev">◀ {prev}</span>}
                                        <span class="milestone-ctx cur">{expanded.name}</span>
                                        {expanded.children.map(c => (
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
                                    <button onClick={() => (editing.value = expanded.milestone)}>编辑</button>
                                    <button
                                        class="danger"
                                        onClick={async () => {
                                            if (!confirm(`删除里程碑「${expanded.name}」？任务会回到未分组。`)) return;
                                            try {
                                                await onDeleteMilestone(expanded.milestone!.id);
                                                if (expandedId.value === expanded.id) expandedId.value = COLLAPSED;
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
                                        onClick={() => onSelectTask(task.id)}
                                    >
                                        {isTask ? (
                                            <span class={`task-status-badge ${task.status}`}>
                                                {task.status === 'running' && <span class="pulse-indicator" />}
                                                {STATUS_LABELS[task.status] || task.status}
                                            </span>
                                        ) : (
                                            <span class={`issue-state-badge ${closed ? 'closed' : 'open'}`}>
                                                {closed ? '已关闭' : '开放'}
                                            </span>
                                        )}
                                        <span class="milestone-task-title">
                                            {task.parentId && <span class="subtask-indent">└─</span>}
                                            {task.number ? <span class="task-number">#{task.number}</span> : null}
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
                </div>
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
