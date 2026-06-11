import { h } from 'preact';
import { t, type Lang } from '../i18n';

interface TaskListProps {
    language: Lang;
}

export function TaskList({ language }: TaskListProps) {
    const items = [
        t('taskList.t1', language),
        t('taskList.t2', language),
        t('taskList.t3', language),
        t('taskList.t4', language),
        t('taskList.t5', language),
    ];
    return (
        <div class="task-list-container">
            {items.map((text, i) => (
                <div key={i} class="task-item completed">
                    <svg
                        class="check-icon"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <circle cx="12" cy="12" r="10" />
                        <polyline points="12 8 12 12 14 14" />
                    </svg>
                    <span>{text}</span>
                </div>
            ))}
        </div>
    );
}
