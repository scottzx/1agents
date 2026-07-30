import { h } from 'preact';
import type { GanttData } from '@1agents/core/types/featureCatalog';

interface GanttUnscheduledTasksProps {
    tasks: GanttData['unscheduled'];
    onTaskClick: (taskId: string) => void;
}

export function GanttUnscheduledTasks({ tasks, onTaskClick }: GanttUnscheduledTasksProps) {
    if (!tasks || tasks.length === 0) return null;

    return (
        <details class="gantt-unscheduled">
            <summary>
                <span>{tasks.length} 个任务尚未排期</span>
                <small>它们不会出现在时间轴中</small>
                <strong>查看</strong>
            </summary>
            <div class="gantt-unscheduled-list">
                {tasks.map(task => (
                    <button
                        type="button"
                        key={task.id}
                        class="gantt-unscheduled-item"
                        onClick={() => onTaskClick(task.id)}
                    >
                        <span>{`#${task.number} ${task.title}`}</span>
                        <span class="gantt-badge">{task.status}</span>
                    </button>
                ))}
            </div>
        </details>
    );
}
