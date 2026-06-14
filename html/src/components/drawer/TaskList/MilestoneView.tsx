import { h } from 'preact';

import { PRIORITY_LABELS, STATUS_LABELS } from './constants';
import type { Task } from './types';

interface MilestoneViewProps {
    tasks: Task[];
    onSelectTask: (taskId: string) => void;
}

// Group tasks under their milestone (a lightweight 阶段性目标 grouping). Order
// follows first appearance; tasks without a milestone fall into 未分组.
export function MilestoneView({ tasks, onSelectTask }: MilestoneViewProps) {
    const order: string[] = [];
    const groups = new Map<string, Task[]>();
    for (const t of tasks) {
        const m = t.milestone || '未分组';
        if (!groups.has(m)) {
            groups.set(m, []);
            order.push(m);
        }
        groups.get(m)!.push(t);
    }

    if (tasks.length === 0) {
        return <div class="task-loading">暂无任务。</div>;
    }

    return (
        <div class="milestone-view">
            {order.map(m => {
                const items = groups.get(m)!;
                const done = items.filter(t => t.status === 'completed').length;
                const pct = Math.round((done / items.length) * 100);
                return (
                    <div key={m} class="milestone-group">
                        <div class="milestone-group-header">
                            <span class="milestone-group-name">{m}</span>
                            <div class="milestone-group-bar">
                                <div class="milestone-group-fill" style={{ width: `${pct}%` }} />
                            </div>
                            <span class="milestone-group-count">{`${done}/${items.length}`}</span>
                        </div>
                        <div class="milestone-group-body">
                            {items.map(task => {
                                const prio = task.priority || 'medium';
                                return (
                                    <div
                                        key={task.id}
                                        class={`milestone-task-row status-${task.status}`}
                                        onClick={() => onSelectTask(task.id)}
                                    >
                                        <span class={`task-status-badge ${task.status}`}>
                                            {task.status === 'running' && <span class="pulse-indicator" />}
                                            {STATUS_LABELS[task.status] || task.status}
                                        </span>
                                        <span class="milestone-task-title">
                                            {task.parentId && <span class="subtask-indent">└─</span>}
                                            {task.title}
                                        </span>
                                        <span class={`priority-badge priority-${prio}`}>
                                            {PRIORITY_LABELS[prio] || prio}
                                        </span>
                                    </div>
                                );
                            })}
                        </div>
                    </div>
                );
            })}
        </div>
    );
}
