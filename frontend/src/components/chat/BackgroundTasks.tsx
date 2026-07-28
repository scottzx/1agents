import { h } from 'preact';
import { useState } from 'preact/hooks';
import { t, getLang } from '../../i18n';
import type { BackgroundTask } from './hooks';

function StatusIcon({ status }: { status: BackgroundTask['status'] }) {
    if (status === 'completed') {
        return (
            <svg
                viewBox="0 0 24 24"
                width="14"
                height="14"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
            >
                <path d="M20 6 9 17l-5-5" />
            </svg>
        );
    }
    if (status === 'running') {
        return (
            <svg
                viewBox="0 0 24 24"
                width="14"
                height="14"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                aria-hidden="true"
            >
                <circle cx="12" cy="12" r="9" opacity="0.3" />
                <path d="M12 3a9 9 0 0 1 9 9" stroke-linecap="round" />
            </svg>
        );
    }
    return (
        <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            aria-hidden="true"
        >
            <circle cx="12" cy="12" r="8" stroke-dasharray="2 3" />
        </svg>
    );
}

interface BackgroundTasksProps {
    tasks: BackgroundTask[];
}

export function BackgroundTasks({ tasks }: BackgroundTasksProps) {
    const lang = getLang();
    // Collapsed by default; expand to see details.
    const [expanded, setExpanded] = useState(false);

    const running = tasks.filter(t => t.status === 'running').length;
    const completed = tasks.filter(t => t.status === 'completed').length;
    const failed = tasks.filter(t => t.status === 'failed' || t.status === 'cancelled' || t.status === 'killed').length;
    const total = tasks.length;

    return (
        <div class="chat-background-tasks" data-expanded={expanded ? 'true' : undefined}>
            <button
                type="button"
                class="chat-background-tasks-header"
                onClick={() => setExpanded(v => !v)}
                aria-expanded={expanded}
            >
                <svg
                    class="chat-background-tasks-caret"
                    data-expanded={expanded ? 'true' : 'false'}
                    viewBox="0 0 24 24"
                    width="12"
                    height="12"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    aria-hidden="true"
                >
                    <path d="m9 18 6-6-6-6" />
                </svg>
                <span class="chat-plan-title">{t('chat.background.title', lang)}</span>
                <span class="chat-plan-progress-text">
                    {completed}/{total}
                </span>
                {running > 0 && <span class="chat-task-running-badge">{running} running</span>}
                {failed > 0 && <span class="chat-task-failed-badge">{failed} failed</span>}
            </button>
            {expanded && (
                <div class="chat-background-tasks-list">
                    {tasks.map((task, i) => (
                        <div key={i} class="chat-background-task-item" data-status={task.status}>
                            <StatusIcon status={task.status} />
                            <div class="chat-background-task-content">
                                <div class="chat-background-task-id">#{task.id}</div>
                                <div class="chat-background-task-command">{task.command}</div>
                                {task.duration && <div class="chat-background-task-duration">{task.duration}</div>}
                                {task.exitCode !== undefined && (
                                    <div class="chat-background-task-exit">exit code: {task.exitCode}</div>
                                )}
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}
