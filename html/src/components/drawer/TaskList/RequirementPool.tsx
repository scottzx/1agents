import { h } from 'preact';

import { PRIORITY_LABELS, STATUS_LABELS, TYPE_LABELS } from './constants';
import type { Task } from './types';

interface RequirementPoolProps {
    tasks: Task[];
    onSelectTask: (taskId: string) => void;
}

// The 需求池: open-ended requirement/bug cards (type != 'task'). These are
// captured here, then refined into milestones + executable tasks downstream.
export function RequirementPool({ tasks, onSelectTask }: RequirementPoolProps) {
    const pool = tasks.filter(t => t.type === 'requirement' || t.type === 'bug');

    if (pool.length === 0) {
        return (
            <div class="requirement-pool-empty">
                需求池为空 —— 点击右上角「+ 新建任务」，类型选「需求」或「缺陷」即可在此提出开放性需求。
            </div>
        );
    }

    return (
        <div class="requirement-pool">
            {pool.map(task => {
                const prio = task.priority || 'medium';
                const type = task.type || 'task';
                return (
                    <div
                        key={task.id}
                        class={`requirement-card type-${type} status-${task.status}`}
                        onClick={() => onSelectTask(task.id)}
                    >
                        <div class="requirement-card-top">
                            <span class={`requirement-type-badge type-${type}`}>{TYPE_LABELS[type] || type}</span>
                            <span class={`priority-badge priority-${prio}`}>{PRIORITY_LABELS[prio] || prio}</span>
                        </div>
                        <div class="requirement-card-title">{task.title}</div>
                        {task.description && <div class="requirement-card-desc">{task.description}</div>}
                        <div class="requirement-card-foot">
                            <span class={`task-status-badge ${task.status}`}>
                                {STATUS_LABELS[task.status] || task.status}
                            </span>
                            {task.milestone && <span class="requirement-card-ms">{task.milestone}</span>}
                        </div>
                    </div>
                );
            })}
        </div>
    );
}
