import { h } from 'preact';

import type { TaskListView } from '../../../stores/projectViewPrefs';

const BASE_VIEWS: ReadonlyArray<readonly [TaskListView, string]> = [
    ['overview', '总览'],
    ['discussion', '讨论'],
    ['requirements', '需求'],
    ['tasks', '任务'],
    ['milestone', '里程碑'],
];

export function taskListViewOptions(): ReadonlyArray<readonly [TaskListView, string]> {
    return BASE_VIEWS;
}

export function resolveTaskListView(activeView?: string): TaskListView {
    return BASE_VIEWS.find(([view]) => view === activeView)?.[0] ?? 'tasks';
}

interface TaskViewSwitcherProps {
    activeView: TaskListView;
    onSelect: (view: TaskListView) => void;
}

export function TaskViewSwitcher({ activeView, onSelect }: TaskViewSwitcherProps) {
    return (
        <div class="task-view-switcher">
            {taskListViewOptions().map(([key, label]) => (
                <button key={key} class={activeView === key ? 'active' : ''} onClick={() => onSelect(key)}>
                    {label}
                </button>
            ))}
        </div>
    );
}
