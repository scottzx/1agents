import { h } from 'preact';

import type { Task } from './types';
import { TYPE_LABELS } from './constants';

interface SuggestionViewProps {
    // Already filtered to source === 'agent-suggested' by the parent.
    tasks: Task[];
    onSelectTask: (taskId: string) => void;
    // 采纳：clear the suggestion marker so the card joins the board as a real task.
    onAdopt: (taskId: string) => void;
    // 忽略：drop the suggestion (dismiss / withdraw).
    onDismiss: (taskId: string) => void;
}

// The AI 建议 view (issue #47, spawn_task model): cards an executing agent
// bubbled up as out-of-scope-but-worth-doing work. Each is self-contained
// (file paths + context in the description) and stays off the board until the
// user 采纳 (转正式任务) or 忽略 (dismiss) it.
export function SuggestionView({ tasks, onSelectTask, onAdopt, onDismiss }: SuggestionViewProps) {
    if (tasks.length === 0) {
        return (
            <div class="requirement-pool-empty">
                还没有 AI 建议。当执行中的 agent
                发现「值得做、但会让当前改动膨胀」的计划外问题（顺手看到的死代码、过期文档、缺测试、确认存在的
                TODO/FIXME 等）时，会在这里冒泡成一张建议卡片，由你一键采纳或忽略。
            </div>
        );
    }

    return (
        <div class="requirement-pool">
            {tasks.map(task => {
                const type = task.type && task.type !== 'discussion' ? task.type : 'task';
                return (
                    <div key={task.id} class={`requirement-card type-${type}`} onClick={() => onSelectTask(task.id)}>
                        <div class="requirement-card-top">
                            <span class="requirement-type-badge suggestion-badge">AI 建议</span>
                            <span class={`requirement-type-badge type-${type}`}>{TYPE_LABELS[type]}</span>
                        </div>
                        <div class="requirement-card-title">
                            {task.number ? <span class="task-number">#{task.number}</span> : null}
                            {task.title}
                        </div>
                        {task.description && <div class="requirement-card-desc">{task.description}</div>}
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
                    </div>
                );
            })}
        </div>
    );
}
