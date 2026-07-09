import { h } from 'preact';
import { useSignal } from '@preact/signals';

import type { ProjectItem } from './types';

interface CalendarBoardProps {
    tasks: ProjectItem[];
    loading: boolean;
    onSelectTask: (taskId: string) => void;
    /** Global board (#91): prefix each event with its owning project tag. */
    showProject?: boolean;
}

// Sunday-first, matching the traditional 万年历 layout.
const WEEKDAYS = ['日', '一', '二', '三', '四', '五', '六'];

// Anchor a task to a single calendar day. Prefer the scheduled run time, then
// the planned start, falling back to creation time so every task lands somewhere.
function anchorDate(t: ProjectItem): Date | null {
    const raw = t.scheduledAt || t.plannedStart || t.createdAt;
    if (!raw) return null;
    const d = new Date(raw);
    return isNaN(d.getTime()) ? null : d;
}

function dayKey(year: number, month: number, day: number): string {
    return `${year}-${month}-${day}`;
}

// A perpetual month calendar (万年历): navigate any year/month, tasks anchored on
// their date, clicking a task jumps straight to its detail — same as list/board.
export function CalendarBoard({ tasks, loading, onSelectTask, showProject }: CalendarBoardProps) {
    const today = new Date();
    const cursor = useSignal({ y: today.getFullYear(), m: today.getMonth() });

    if (loading && tasks.length === 0) {
        return <div class="task-loading">正在载入任务列表...</div>;
    }

    const { y, m } = cursor.value;

    // Bucket every task onto its anchor day.
    const byDay = new Map<string, ProjectItem[]>();
    for (const t of tasks) {
        const d = anchorDate(t);
        if (!d) continue;
        const key = dayKey(d.getFullYear(), d.getMonth(), d.getDate());
        const arr = byDay.get(key);
        if (arr) arr.push(t);
        else byDay.set(key, [t]);
    }

    // Grid: leading blanks from the previous month, then enough whole weeks to
    // cover this month (5 or 6 rows depending on the offset).
    const startOffset = new Date(y, m, 1).getDay(); // 0 = Sunday
    const daysInMonth = new Date(y, m + 1, 0).getDate();
    const totalCells = Math.ceil((startOffset + daysInMonth) / 7) * 7;
    const cells = Array.from({ length: totalCells }, (_, i) => new Date(y, m, 1 - startOffset + i));

    const todayKey = dayKey(today.getFullYear(), today.getMonth(), today.getDate());

    const goPrev = () => (cursor.value = m === 0 ? { y: y - 1, m: 11 } : { y, m: m - 1 });
    const goNext = () => (cursor.value = m === 11 ? { y: y + 1, m: 0 } : { y, m: m + 1 });
    const goToday = () => (cursor.value = { y: today.getFullYear(), m: today.getMonth() });

    return (
        <div class="task-calendar">
            <div class="calendar-toolbar">
                <div class="calendar-title">
                    <span class="calendar-year">{y}年</span>
                    <span class="calendar-month">{m + 1}月</span>
                </div>
                <div class="calendar-nav">
                    <button onClick={goPrev} title="上个月">
                        ‹
                    </button>
                    <button class="calendar-today-btn" onClick={goToday}>
                        今天
                    </button>
                    <button onClick={goNext} title="下个月">
                        ›
                    </button>
                </div>
            </div>

            <div class="calendar-weekdays">
                {WEEKDAYS.map(w => (
                    <div key={w} class="calendar-weekday">
                        {w}
                    </div>
                ))}
            </div>

            <div class="calendar-grid">
                {cells.map(date => {
                    const key = dayKey(date.getFullYear(), date.getMonth(), date.getDate());
                    const items = byDay.get(key) || [];
                    const isOtherMonth = date.getMonth() !== m;
                    const isToday = key === todayKey;
                    return (
                        <div
                            key={key}
                            class={`calendar-cell${isOtherMonth ? ' is-other-month' : ''}${isToday ? ' is-today' : ''}`}
                        >
                            <div class="calendar-cell-date">
                                <span class="calendar-cell-day">{date.getDate()}</span>
                                {items.length > 0 && <span class="calendar-cell-count">{items.length}</span>}
                            </div>
                            <div class="calendar-cell-events">
                                {items.map(t => (
                                    <button
                                        key={t.id}
                                        class={`calendar-event status-${t.status}`}
                                        onClick={() => onSelectTask(t.id)}
                                        title={
                                            showProject && t.workspaceName ? `${t.workspaceName} · ${t.title}` : t.title
                                        }
                                    >
                                        {showProject && t.workspaceName ? (
                                            <span class="calendar-event-project">{t.workspaceName}</span>
                                        ) : t.number ? (
                                            <span class="calendar-event-num">#{t.number}</span>
                                        ) : null}
                                        <span class="calendar-event-title">{t.title}</span>
                                    </button>
                                ))}
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
