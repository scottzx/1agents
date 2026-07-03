import { h } from 'preact';
import { useState } from 'preact/hooks';
import { t, getLang } from '../../i18n';
import type { PlanEntry } from '@1agents/core/protocol/types';

// Pinned checklist card for the agent's execution plan (ACP plan updates —
// Claude Code TodoWrite, Codex plan). Sits above the message list so the
// current task list stays glanceable while messages scroll underneath.
// Collapsed by default to a one-line progress bar; expands to the full list.
// Pure presentation — the plan is replaced wholesale by each `plan` event.

function StatusIcon({ status }: { status: PlanEntry['status'] }) {
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
    if (status === 'in_progress') {
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

interface PlanChecklistProps {
    entries: PlanEntry[];
}

export function PlanChecklist({ entries }: PlanChecklistProps) {
    const lang = getLang();
    // In-progress items are the most interesting, so start expanded when the
    // agent is actively working a step; otherwise collapse to save space.
    const [expanded, setExpanded] = useState(true);

    const done = entries.filter(e => e.status === 'completed').length;
    const total = entries.length;
    const percent = total > 0 ? Math.round((done / total) * 100) : 0;
    const allDone = done === total;

    return (
        <div class="chat-plan-checklist" data-complete={allDone ? 'true' : undefined}>
            <button
                type="button"
                class="chat-plan-checklist-header"
                onClick={() => setExpanded(v => !v)}
                aria-expanded={expanded}
            >
                <svg
                    class="chat-plan-caret"
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
                <span class="chat-plan-title">{t('chat.plan.title', lang)}</span>
                <span class="chat-plan-progress-text">
                    {done}/{total}
                </span>
                <span class="chat-plan-progress-track" aria-hidden="true">
                    <span class="chat-plan-progress-fill" style={{ width: `${percent}%` }} />
                </span>
            </button>
            {expanded && (
                <ul class="chat-plan-list">
                    {entries.map((entry, i) => (
                        <li key={i} class="chat-plan-item" data-status={entry.status}>
                            <span class="chat-plan-item-icon">
                                <StatusIcon status={entry.status} />
                            </span>
                            <span class="chat-plan-item-text">{entry.content}</span>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}
