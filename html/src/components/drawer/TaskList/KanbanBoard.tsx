import { h } from 'preact';
import { useState } from 'preact/hooks';

import { PRIORITY_LABELS, STATUS_LABELS } from './constants';
import type { Task } from './types';

interface KanbanBoardProps {
    tasks: Task[];
    loading: boolean;
    onSelectTask: (taskId: string) => void;
    /** Drag-to-retire a card. Only terminal states are reachable by drag. */
    onStatusChange: (taskId: string, status: 'completed' | 'cancelled') => void;
}

type DropStatus = 'completed' | 'cancelled';

interface Column {
    key: string;
    label: string;
    statuses: Array<Task['status']>;
    /** When set, the column accepts drops and moves the card to this status. */
    dropStatus?: DropStatus;
}

// Columns reflect runtime status. Only the two terminal columns accept drops —
// runnable lanes (待办/进行中/阻塞) stay scheduler-owned, so a drag can only
// retire a card (mark done / cancel), never arm execution.
const COLUMNS: Column[] = [
    { key: 'todo', label: '待办', statuses: ['pending', 'queued'] },
    { key: 'running', label: '进行中', statuses: ['running'] },
    { key: 'blocked', label: '阻塞', statuses: ['blocked'] },
    { key: 'done', label: '已完成', statuses: ['completed'], dropStatus: 'completed' },
    { key: 'retired', label: '失败/取消', statuses: ['failed', 'cancelled'], dropStatus: 'cancelled' },
];

export function KanbanBoard({ tasks, loading, onSelectTask, onStatusChange }: KanbanBoardProps) {
    const [draggedId, setDraggedId] = useState<string | null>(null);
    const [dragOverCol, setDragOverCol] = useState<string | null>(null);

    if (loading && tasks.length === 0) {
        return <div class="task-loading">正在载入任务列表...</div>;
    }

    const handleDrop = (col: Column) => {
        if (draggedId && col.dropStatus) {
            const t = tasks.find(x => x.id === draggedId);
            if (t && !col.statuses.includes(t.status)) {
                onStatusChange(draggedId, col.dropStatus);
            }
        }
        setDraggedId(null);
        setDragOverCol(null);
    };

    return (
        <div class="kanban-board">
            {COLUMNS.map(col => {
                const items = tasks.filter(t => col.statuses.includes(t.status));
                const isTarget = !!col.dropStatus && dragOverCol === col.key && draggedId !== null;
                return (
                    <div
                        key={col.key}
                        class={`kanban-column${isTarget ? ' is-drop-target' : ''}`}
                        onDragOver={(e: DragEvent) => {
                            if (!col.dropStatus) return;
                            e.preventDefault();
                            setDragOverCol(col.key);
                        }}
                        onDragLeave={() => dragOverCol === col.key && setDragOverCol(null)}
                        onDrop={(e: DragEvent) => {
                            e.preventDefault();
                            handleDrop(col);
                        }}
                    >
                        <div class="kanban-column-header">
                            <span class={`kanban-column-title col-${col.key}`}>{col.label}</span>
                            <span class="kanban-column-count">{items.length}</span>
                        </div>
                        <div class="kanban-column-body">
                            {items.map(task => (
                                <KanbanCard
                                    key={task.id}
                                    task={task}
                                    dragging={draggedId === task.id}
                                    onSelect={() => onSelectTask(task.id)}
                                    onDragStart={() => setDraggedId(task.id)}
                                    onDragEnd={() => {
                                        setDraggedId(null);
                                        setDragOverCol(null);
                                    }}
                                />
                            ))}
                            {items.length === 0 && <div class="kanban-column-empty">—</div>}
                        </div>
                    </div>
                );
            })}
        </div>
    );
}

interface KanbanCardProps {
    task: Task;
    dragging: boolean;
    onSelect: () => void;
    onDragStart: () => void;
    onDragEnd: () => void;
}

function KanbanCard({ task, dragging, onSelect, onDragStart, onDragEnd }: KanbanCardProps) {
    const prio = task.priority || 'medium';
    return (
        <div
            class={`kanban-card status-${task.status}${dragging ? ' dragging' : ''}`}
            draggable
            onClick={onSelect}
            onDragStart={(e: DragEvent) => {
                if (e.dataTransfer) {
                    e.dataTransfer.effectAllowed = 'move';
                    e.dataTransfer.setData('text/plain', task.id);
                }
                onDragStart();
            }}
            onDragEnd={onDragEnd}
        >
            <div class="kanban-card-top">
                <span class={`priority-badge priority-${prio}`}>{PRIORITY_LABELS[prio] || prio}</span>
                {task.milestone && <span class="kanban-card-milestone">{task.milestone}</span>}
            </div>
            <div class="kanban-card-title">{task.title}</div>
            <div class="kanban-card-foot">
                <span class={`task-status-badge ${task.status}`}>
                    {task.status === 'running' && <span class="pulse-indicator" />}
                    {STATUS_LABELS[task.status] || task.status}
                </span>
                {(task.dependsOn?.length ?? 0) > 0 && (
                    <span class="kanban-card-deps" title="前置依赖数">{`⛓ ${task.dependsOn!.length}`}</span>
                )}
                <span class="kanban-card-assignee">{task.assignee || 'claudecode'}</span>
            </div>
        </div>
    );
}
