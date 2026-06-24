import { h } from 'preact';
import { useState } from 'preact/hooks';

import { PRIORITY_LABELS, TYPE_LABELS } from './constants';
import { parseFrontmatter } from '../../../utils/frontmatter';
import type { Task } from './types';

interface RequirementPoolProps {
    // Board requirement/bug cards (non-suggestion); the parent passes boardTasks.
    tasks: Task[];
    // AI suggestions (source === 'agent-suggested') merged into this pool.
    suggestions: Task[];
    onSelectTask: (taskId: string) => void;
    // 采纳 / 忽略 for suggestion cards (issue #47).
    onAdopt: (taskId: string) => void;
    onDismiss: (taskId: string) => void;
}

type Filter = 'all' | 'requirement' | 'bug' | 'suggestion';

// The 需求池: open-ended requirement/bug cards (type != 'task'), with AI
// suggestions (issue #47) merged in. A filter bar narrows the grid — pick
// 「AI 建议」 to isolate the agent-emitted proposals, each adoptable/dismissable
// inline without leaving the pool.
export function RequirementPool({ tasks, suggestions, onSelectTask, onAdopt, onDismiss }: RequirementPoolProps) {
    const [filter, setFilter] = useState<Filter>('all');

    const reqBug = tasks.filter(t => t.type === 'requirement' || t.type === 'bug');
    const reqCount = reqBug.filter(t => t.type === 'requirement').length;
    const bugCount = reqBug.filter(t => t.type === 'bug').length;

    const filters: Array<[Filter, string, number]> = [
        ['all', '全部', reqBug.length + suggestions.length],
        ['requirement', '需求', reqCount],
        ['bug', '缺陷', bugCount],
        ['suggestion', 'AI 建议', suggestions.length],
    ];

    let visible: Task[];
    if (filter === 'requirement') visible = reqBug.filter(t => t.type === 'requirement');
    else if (filter === 'bug') visible = reqBug.filter(t => t.type === 'bug');
    else if (filter === 'suggestion') visible = suggestions;
    else visible = [...reqBug, ...suggestions].sort((a, b) => (a.number || 0) - (b.number || 0));

    return (
        <div class="requirement-pool-wrap">
            <div class="requirement-filter">
                {filters.map(([key, label, count]) => (
                    <button key={key} class={filter === key ? 'active' : ''} onClick={() => setFilter(key)}>
                        {label}
                        <span class="requirement-filter-count">{count}</span>
                    </button>
                ))}
            </div>

            {visible.length === 0 ? (
                <div class="requirement-pool-empty">
                    {filter === 'suggestion'
                        ? '还没有 AI 建议。当执行中的 agent 发现「值得做、但会让当前改动膨胀」的计划外问题时，会在这里冒泡成一张可一键采纳 / 忽略的建议卡片。'
                        : '需求池为空 —— 点击右上角「+ 新建任务」，类型选「需求」或「缺陷」即可在此提出开放性需求。'}
                </div>
            ) : (
                <div class="requirement-pool">
                    {visible.map(task => {
                        const isSuggestion = task.source === 'agent-suggested';
                        const prio = task.priority || 'medium';
                        const type = task.type && task.type !== 'discussion' ? task.type : 'task';
                        // Card content is frontmatter Markdown — preview the prose body only.
                        const descBody = parseFrontmatter(task.description).body;
                        return (
                            <div
                                key={task.id}
                                class={`requirement-card type-${type} status-${task.status}`}
                                onClick={() => onSelectTask(task.id)}
                            >
                                <div class="requirement-card-top">
                                    {isSuggestion && (
                                        <span class="requirement-type-badge suggestion-badge">AI 建议</span>
                                    )}
                                    <span class={`requirement-type-badge type-${type}`}>
                                        {TYPE_LABELS[type] || type}
                                    </span>
                                    {!isSuggestion && (
                                        <span class={`priority-badge priority-${prio}`}>
                                            {PRIORITY_LABELS[prio] || prio}
                                        </span>
                                    )}
                                </div>
                                <div class="requirement-card-title">
                                    {task.number ? <span class="task-number">#{task.number}</span> : null}
                                    {task.title}
                                </div>
                                {descBody && <div class="requirement-card-desc">{descBody}</div>}
                                {isSuggestion ? (
                                    <div class="suggestion-actions">
                                        <button
                                            class="suggestion-btn adopt"
                                            onClick={e => {
                                                e.stopPropagation();
                                                onAdopt(task.id);
                                            }}
                                        >
                                            采纳
                                        </button>
                                        <button
                                            class="suggestion-btn dismiss"
                                            onClick={e => {
                                                e.stopPropagation();
                                                onDismiss(task.id);
                                            }}
                                        >
                                            忽略
                                        </button>
                                    </div>
                                ) : (
                                    <div class="requirement-card-foot">
                                        {/* Requirements/bugs are open/closed issues, not executable
                                            tasks — no pending/running/failed status here. */}
                                        <span class={`issue-state-badge ${task.issueState || 'open'}`}>
                                            {task.issueState === 'closed' ? '已关闭' : '开放'}
                                        </span>
                                        <span class={`confirm-badge ${task.userConfirm ? 'confirmed' : 'pending'}`}>
                                            {task.userConfirm ? '已确认' : '待确认'}
                                        </span>
                                        {task.milestone && <span class="requirement-card-ms">{task.milestone}</span>}
                                    </div>
                                )}
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
}
