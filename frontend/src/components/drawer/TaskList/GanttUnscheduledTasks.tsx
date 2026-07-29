import { h } from 'preact';
import type { GanttData } from '@1agents/core/types/featureCatalog';

interface GanttUnscheduledTasksProps {
    tasks: GanttData['unscheduled'];
    onTaskClick: (taskId: string) => void;
}

export function GanttUnscheduledTasks({ tasks, onTaskClick }: GanttUnscheduledTasksProps) {
    if (!tasks || tasks.length === 0) return null;

    return (
        <div class="gantt-unscheduled">
            <h4>未排期任务 ({tasks.length})</h4>
            <div class="gantt-unscheduled-list">
                {tasks.map(task => (
                    <div key={task.id} class="gantt-unscheduled-item" onClick={() => onTaskClick(task.id)}>
                        <span>{`#${task.number} ${task.title}`}</span>
                        <span class="gantt-badge">{task.status}</span>
                    </div>
                ))}
            </div>
        </div>
    );
}
