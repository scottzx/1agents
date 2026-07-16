import { h } from 'preact';
import { useSignal } from '@preact/signals';
import type { Signal } from '@preact/signals';

import { PRIORITY_LABELS, STATUS_LABELS } from './constants';

export type TaskView = 'list' | 'board' | 'calendar';

// View-toggle icons (feather-style, currentColor). Shown on mobile where the
// toggle collapses to icons-only; desktop keeps the text labels.
const ListIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <line x1="8" y1="6" x2="21" y2="6" />
        <line x1="8" y1="12" x2="21" y2="12" />
        <line x1="8" y1="18" x2="21" y2="18" />
        <line x1="3" y1="6" x2="3.01" y2="6" />
        <line x1="3" y1="12" x2="3.01" y2="12" />
        <line x1="3" y1="18" x2="3.01" y2="18" />
    </svg>
);

const BoardIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <rect x="3" y="3" width="7" height="18" rx="1" />
        <rect x="14" y="3" width="7" height="11" rx="1" />
    </svg>
);

const CalendarIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <rect x="3" y="4" width="18" height="18" rx="2" />
        <line x1="16" y1="2" x2="16" y2="6" />
        <line x1="8" y1="2" x2="8" y2="6" />
        <line x1="3" y1="10" x2="21" y2="10" />
    </svg>
);

interface FilterBarProps {
    search: Signal<string>;
    statusFilter: Signal<string[]>;
    priorityFilter: Signal<string[]>;
    assigneeFilter: Signal<string[]>;
    assignees: string[];
    taskView: Signal<TaskView>;
    /** Which view toggles to show. Defaults to all three; the global board (#91)
     *  omits 'list' since cross-project inline edit/delete are out of scope. */
    views?: TaskView[];
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
    views = ['list', 'board', 'calendar'],
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
        <div class="data-grid-toolbar task-filter-bar">
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
                {views.includes('list') && (
                    <button
                        class={taskView.value === 'list' ? 'active' : ''}
                        title="列表"
                        onClick={() => (taskView.value = 'list')}
                    >
                        <span class="tvt-icon">
                            <ListIcon />
                        </span>
                        <span class="tvt-label">列表</span>
                    </button>
                )}
                {views.includes('board') && (
                    <button
                        class={taskView.value === 'board' ? 'active' : ''}
                        title="看板"
                        onClick={() => (taskView.value = 'board')}
                    >
                        <span class="tvt-icon">
                            <BoardIcon />
                        </span>
                        <span class="tvt-label">看板</span>
                    </button>
                )}
                {views.includes('calendar') && (
                    <button
                        class={taskView.value === 'calendar' ? 'active' : ''}
                        title="日历"
                        onClick={() => (taskView.value = 'calendar')}
                    >
                        <span class="tvt-icon">
                            <CalendarIcon />
                        </span>
                        <span class="tvt-label">日历</span>
                    </button>
                )}
            </div>
        </div>
    );
}
