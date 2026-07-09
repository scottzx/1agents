import { h } from 'preact';

import type { ProjectItem } from './types';

interface DiscussionViewProps {
    tasks: ProjectItem[];
    onSelectTask: (taskId: string) => void;
}

// The 讨论区: free-form conceptual/directional posts (type === 'discussion').
// They never get scheduled; they may later be refined into a requirement
// via the "讨论需求" action on the detail page (which opens a PM conversation).
export function DiscussionView({ tasks, onSelectTask }: DiscussionViewProps) {
    const posts = tasks.filter(t => t.type === 'discussion');

    if (posts.length === 0) {
        return (
            <div class="requirement-pool-empty">
                讨论区为空 —— 点击右上角「+ 新建讨论」，和 PM 聊聊还没想清楚的想法或方向；聊清楚有了明确交付物，PM
                会帮你转成需求，否则先留一张讨论卡片，以后再聊。
            </div>
        );
    }

    return (
        <div class="requirement-pool">
            {posts.map(task => (
                <div key={task.id} class="requirement-card type-discussion" onClick={() => onSelectTask(task.id)}>
                    <div class="requirement-card-top">
                        <span class="requirement-type-badge type-discussion">讨论</span>
                    </div>
                    <div class="requirement-card-title">
                        {task.number ? <span class="task-number">#{task.number}</span> : null}
                        {task.title}
                    </div>
                    {task.description && <div class="requirement-card-desc">{task.description}</div>}
                </div>
            ))}
        </div>
    );
}
