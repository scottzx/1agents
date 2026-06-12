import { h } from 'preact';

import { PRIORITY_LABELS, STATUS_LABELS } from './constants';
import type { Task } from './types';
import { fmtDateOnly, orderForTable, recurrenceLabel } from './utils';

interface TaskTableProps {
    tasks: Task[];
    loading: boolean;
    onSelectTask: (taskId: string) => void;
    onDeleteTask: (taskId: string) => void;
}

export function TaskTable({ tasks, loading, onSelectTask, onDeleteTask }: TaskTableProps) {
    if (loading && tasks.length === 0) {
        return <div class="task-loading">正在载入任务列表...</div>;
    }

    return (
        <table class="task-table">
            <thead>
                <tr>
                    <th class="col-priority">优先级</th>
                    <th class="col-status">状态</th>
                    <th class="col-issue" title="Issue 状态">
                        {'\u{1F513}'}
                    </th>
                    <th class="col-title">任务</th>
                    <th class="col-assignee">执行</th>
                    <th class="col-date">计划开始</th>
                    <th class="col-date">计划完成</th>
                    <th class="col-date">实际完成</th>
                    <th class="col-deps">前置依赖</th>
                    <th class="col-actions" />
                </tr>
            </thead>
            <tbody>
                {tasks.length === 0 && (
                    <tr class="task-empty-row">
                        <td colSpan={10}>暂无任务 —— 点击上方「+ 新建任务」创建第一个。</td>
                    </tr>
                )}
                {orderForTable(tasks).map(({ task, isChild }) => {
                    const deps = tasks.filter(t => task.dependsOn?.includes(t.id));
                    const closed = task.issueState === 'closed';
                    const prio = task.priority || 'medium';
                    return (
                        <tr
                            key={task.id}
                            class={`task-row status-${task.status}${closed ? ' issue-closed' : ''}${
                                isChild ? ' task-row-child' : ''
                            }`}
                            onClick={() => onSelectTask(task.id)}
                        >
                            <td class="col-priority">
                                <span class={`priority-badge priority-${prio}`}>{PRIORITY_LABELS[prio] || prio}</span>
                            </td>
                            <td class="col-status">
                                <span class={`task-status-badge ${task.status}`}>
                                    {task.status === 'running' && <span class="pulse-indicator" />}
                                    {STATUS_LABELS[task.status] || task.status}
                                </span>
                            </td>
                            <td class="col-issue">{closed ? '\u{1F512}' : '\u{1F513}'}</td>
                            <td class="col-title">
                                {isChild && <span class="subtask-indent">└─</span>}
                                <span class="task-row-title">{task.title}</span>
                                {(task.labels || []).map(l => (
                                    <span key={l} class="task-label-tag">
                                        {l}
                                    </span>
                                ))}
                                {task.recurrence && (
                                    <span class="task-recur-tag" title={recurrenceLabel(task.recurrence)}>
                                        🔁
                                    </span>
                                )}
                                {(task.replies?.length ?? 0) > 0 && (
                                    <span class="task-reply-count">💬 {task.replies!.length}</span>
                                )}
                            </td>
                            <td class="col-assignee">{task.assignee || 'claudecode'}</td>
                            <td class="col-date">{fmtDateOnly(task.plannedStart)}</td>
                            <td class="col-date">{fmtDateOnly(task.plannedEnd)}</td>
                            <td class="col-date">{fmtDateOnly(task.completedAt)}</td>
                            <td class="col-deps">
                                {deps.length > 0
                                    ? deps.map(d => (
                                          <span key={d.id} class="dep-tag">
                                              {d.status === 'completed' ? '✓ ' : ''}
                                              {d.title}
                                          </span>
                                      ))
                                    : '—'}
                            </td>
                            <td class="col-actions">
                                <button
                                    class="task-delete-btn"
                                    onClick={(e: Event) => {
                                        e.stopPropagation();
                                        onDeleteTask(task.id);
                                    }}
                                    title="删除任务"
                                >
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                        <polyline points="3 6 5 6 21 6" />
                                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                    </svg>
                                </button>
                            </td>
                        </tr>
                    );
                })}
            </tbody>
        </table>
    );
}
