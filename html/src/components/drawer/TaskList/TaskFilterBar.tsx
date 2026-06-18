import { h } from 'preact';
import { useSignal } from '@preact/signals';
import type { Signal } from '@preact/signals';

import { PRIORITY_LABELS, STATUS_LABELS } from './constants';

export type TaskView = 'list' | 'board' | 'calendar';

interface FilterBarProps {
    search: Signal<string>;
    statusFilter: Signal<string[]>;
    priorityFilter: Signal<string[]>;
    assigneeFilter: Signal<string[]>;
    assignees: string[];
    taskView: Signal<TaskView>;
}

const STATUS_KEYS = ['pending', 'queued', 'running', 'completed', 'failed', 'cancelled', 'blocked'];
const PRIORITY_KEYS = ['urgent', 'high', 'medium', 'low'];

function toggle(sig: Signal<string[]>, value: string) {
    sig.value = sig.value.includes(value) ? sig.value.filter(v => v !== value) : [...sig.value, value];
}

// Shared across the list and board task views: search + multi-condition filter
// (status / priority / assignee), plus the list↔board view toggle on the right.
export function TaskFilterBar({
    search,
    statusFilter,
    priorityFilter,
    assigneeFilter,
    assignees,
    taskView,
}: FilterBarProps) {
    const showFilters = useSignal(false);
    const activeFilterCount = statusFilter.value.length + priorityFilter.value.length + assigneeFilter.value.length;

    const chip = (sig: Signal<string[]>, value: string, label: string) => (
        <button
            key={value}
            class={`grid-filter-chip ${sig.value.includes(value) ? 'active' : ''}`}
            onClick={() => toggle(sig, value)}
        >
            {label}
        </button>
    );

    return (
        <div class="task-grid-toolbar">
            <div class="grid-toolbar-search">
                <input
                    type="text"
                    placeholder="🔍 搜索任务标题 / #编号…"
                    value={search.value}
                    onInput={(e: Event) => (search.value = (e.target as HTMLInputElement).value)}
                />
            </div>

            <div class="grid-toolbar-popover-host">
                <button
                    class={`grid-toolbar-btn ${activeFilterCount > 0 ? 'active' : ''}`}
                    onClick={() => (showFilters.value = !showFilters.value)}
                >
                    筛选{activeFilterCount > 0 ? ` (${activeFilterCount})` : ''}
                </button>
                {showFilters.value && (
                    <div class="grid-toolbar-popover">
                        <div class="grid-filter-section">
                            <div class="grid-filter-label">状态</div>
                            <div class="grid-filter-chips">
                                {STATUS_KEYS.map(s => chip(statusFilter, s, STATUS_LABELS[s] || s))}
                            </div>
                        </div>
                        <div class="grid-filter-section">
                            <div class="grid-filter-label">优先级</div>
                            <div class="grid-filter-chips">
                                {PRIORITY_KEYS.map(p => chip(priorityFilter, p, PRIORITY_LABELS[p] || p))}
                            </div>
                        </div>
                        <div class="grid-filter-section">
                            <div class="grid-filter-label">执行</div>
                            <div class="grid-filter-chips">{assignees.map(a => chip(assigneeFilter, a, a))}</div>
                        </div>
                        {activeFilterCount > 0 && (
                            <button
                                class="grid-filter-clear"
                                onClick={() => {
                                    statusFilter.value = [];
                                    priorityFilter.value = [];
                                    assigneeFilter.value = [];
                                }}
                            >
                                清除全部筛选
                            </button>
                        )}
                    </div>
                )}
            </div>

            <div class="task-view-toggle">
                <button class={taskView.value === 'list' ? 'active' : ''} onClick={() => (taskView.value = 'list')}>
                    列表
                </button>
                <button class={taskView.value === 'board' ? 'active' : ''} onClick={() => (taskView.value = 'board')}>
                    看板
                </button>
                <button
                    class={taskView.value === 'calendar' ? 'active' : ''}
                    onClick={() => (taskView.value = 'calendar')}
                >
                    日历
                </button>
            </div>
        </div>
    );
}
