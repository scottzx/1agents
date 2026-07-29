import { h } from 'preact';

import type { TaskListView } from '../../../stores/projectViewPrefs';

const BASE_VIEWS: ReadonlyArray<readonly [TaskListView, string]> = [
    ['overview', '总览'],
    ['discussion', '讨论'],
    ['requirements', '需求'],
    ['tasks', '任务'],
    ['milestone', '里程碑'],
];

export function taskListViewOptions(featureCatalogEnabled: boolean): ReadonlyArray<readonly [TaskListView, string]> {
    if (!featureCatalogEnabled) return BASE_VIEWS;
    return [...BASE_VIEWS.slice(0, 3), ['features', '功能蓝图'], ...BASE_VIEWS.slice(3)];
}

export function resolveTaskListView(
    activeView: TaskListView,
    featureCatalogEnabled: boolean,
    configLoaded: boolean
): TaskListView {
    return configLoaded && !featureCatalogEnabled && activeView === 'features' ? 'tasks' : activeView;
}

interface TaskViewSwitcherProps {
    activeView: TaskListView;
    featureCatalogEnabled: boolean;
    onSelect: (view: TaskListView) => void;
}

export function TaskViewSwitcher({ activeView, featureCatalogEnabled, onSelect }: TaskViewSwitcherProps) {
    return (
        <div class="task-view-switcher">
            {taskListViewOptions(featureCatalogEnabled).map(([key, label]) => (
                <button key={key} class={activeView === key ? 'active' : ''} onClick={() => onSelect(key)}>
                    {label}
                </button>
            ))}
        </div>
    );
}
