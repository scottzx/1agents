import { h, Fragment } from 'preact';
import { useEffect } from 'preact/hooks';

import { PRIORITY_LABELS, STATUS_LABELS } from './constants';
import type { ProjectItem } from './types';

interface TaskPreviewDrawerProps {
    open: boolean;
    task: ProjectItem | null;
    onClose: () => void;
    // Receives the captured taskId so callers don't need to read it from a
    // state value that's about to be cleared by onClose().
    onOpenFull: (taskId: string) => void;
}

function fmtDateTime(s?: string): string {
    if (!s) return '';
    const d = new Date(s);
    if (isNaN(d.getTime())) return s;
    return `${d.toLocaleDateString()} ${d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
}

function fmtDate(s?: string): string {
    return s ? s.slice(0, 10) : '';
}

const TYPE_LABELS: Record<string, string> = {
    task: '任务',
    requirement: '需求',
    bug: '缺陷',
    discussion: '讨论',
};

export function TaskPreviewDrawer({ open, task, onClose, onOpenFull }: TaskPreviewDrawerProps) {
    // Esc closes the preview drawer; only attach while it's open.
    useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [open, onClose]);

    if (!open || !task) return null;

    const isIssue = task.type === 'requirement' || task.type === 'bug';
    const closed = task.issueState === 'closed';
    const prio = task.priority || 'medium';

    return (
        <Fragment>
            <div class="task-preview-backdrop" onClick={onClose} />
            <aside class="task-preview-drawer" role="dialog" aria-label={`任务详情预览：${task.title}`}>
                <header class="task-preview-head">
                    <div class="task-preview-head-main">
                        <div class="task-preview-badges">
                            {task.type && (
                                <span class={`task-type-badge type-${task.type}`}>
                                    {TYPE_LABELS[task.type] || task.type}
                                </span>
                            )}
                            {isIssue ? (
                                <span class={`issue-state-badge ${closed ? 'closed' : 'open'}`}>
                                    {closed ? '已关闭' : '开放'}
                                </span>
                            ) : (
                                <span class={`task-status-badge ${task.status}`}>
                                    {task.status === 'running' && <span class="pulse-indicator" />}
                                    {STATUS_LABELS[task.status] || task.status}
                                </span>
                            )}
                            <span class={`priority-badge priority-${prio}`}>{PRIORITY_LABELS[prio] || prio}</span>
                            {task.milestone && <span class="task-preview-milestone">🎯 {task.milestone}</span>}
                        </div>
                        <h3 class="task-preview-title">
                            {task.number ? <span class="task-number">#{task.number}</span> : null}
                            {task.title}
                        </h3>
                    </div>
                    <button class="task-preview-close" aria-label="关闭" onClick={onClose}>
                        ×
                    </button>
                </header>

                <div class="task-preview-body">
                    {task.description && (
                        <section class="task-preview-section">
                            <div class="task-preview-section-label">说明</div>
                            <div class="task-preview-desc">{task.description}</div>
                        </section>
                    )}

                    {task.summary && (
                        <section class="task-preview-section">
                            <div class="task-preview-section-label">最近结果</div>
                            <div class="task-preview-summary">{task.summary}</div>
                        </section>
                    )}

                    {(task.scheduledAt || task.plannedStart || task.plannedEnd) && (
                        <section class="task-preview-section">
                            <div class="task-preview-section-label">计划</div>
                            <div class="task-preview-meta-list">
                                {task.scheduledAt && (
                                    <div class="task-preview-meta-row">
                                        <span class="task-preview-meta-key">调度时间</span>
                                        <span class="task-preview-meta-val">{fmtDateTime(task.scheduledAt)}</span>
                                    </div>
                                )}
                                {task.plannedStart && (
                                    <div class="task-preview-meta-row">
                                        <span class="task-preview-meta-key">开始</span>
                                        <span class="task-preview-meta-val">{fmtDate(task.plannedStart)}</span>
                                    </div>
                                )}
                                {task.plannedEnd && (
                                    <div class="task-preview-meta-row">
                                        <span class="task-preview-meta-key">截止</span>
                                        <span class="task-preview-meta-val">{fmtDate(task.plannedEnd)}</span>
                                    </div>
                                )}
                            </div>
                        </section>
                    )}

                    <section class="task-preview-section">
                        <div class="task-preview-section-label">元数据</div>
                        <div class="task-preview-meta-list">
                            {task.assignee && (
                                <div class="task-preview-meta-row">
                                    <span class="task-preview-meta-key">执行者</span>
                                    <span class="task-preview-meta-val">{task.assignee}</span>
                                </div>
                            )}
                            {task.labels && task.labels.length > 0 && (
                                <div class="task-preview-meta-row">
                                    <span class="task-preview-meta-key">标签</span>
                                    <span class="task-preview-meta-val">
                                        {task.labels.map(l => (
                                            <span key={l} class="task-preview-label-chip">
                                                {l}
                                            </span>
                                        ))}
                                    </span>
                                </div>
                            )}
                            <div class="task-preview-meta-row">
                                <span class="task-preview-meta-key">创建</span>
                                <span class="task-preview-meta-val">{fmtDateTime(task.createdAt)}</span>
                            </div>
                            {task.startedAt && (
                                <div class="task-preview-meta-row">
                                    <span class="task-preview-meta-key">开始执行</span>
                                    <span class="task-preview-meta-val">{fmtDateTime(task.startedAt)}</span>
                                </div>
                            )}
                            {task.completedAt && (
                                <div class="task-preview-meta-row">
                                    <span class="task-preview-meta-key">完成</span>
                                    <span class="task-preview-meta-val">{fmtDateTime(task.completedAt)}</span>
                                </div>
                            )}
                            {task.githubUrl && (
                                <div class="task-preview-meta-row">
                                    <span class="task-preview-meta-key">GitHub</span>
                                    <a
                                        class="task-preview-meta-val task-preview-link"
                                        href={task.githubUrl}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        onClick={(e: Event) => e.stopPropagation()}
                                    >
                                        {task.githubUrl}
                                    </a>
                                </div>
                            )}
                        </div>
                    </section>
                </div>

                <footer class="task-preview-foot">
                    <button class="secondary" onClick={onClose}>
                        关闭
                    </button>
                    <button
                        class="primary"
                        onClick={() => {
                            // Pass task.id before onClose() clears it; the parent uses this
                            // id to navigate, and it doesn't need to read from the cleared
                            // signal.
                            const id = task.id;
                            onClose();
                            onOpenFull(id);
                        }}
                    >
                        打开完整详情 →
                    </button>
                </footer>
            </aside>
        </Fragment>
    );
}
