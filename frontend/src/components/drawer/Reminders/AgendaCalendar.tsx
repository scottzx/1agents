import { h } from 'preact';
import { useSignal } from '@preact/signals';

import type { CalendarItem } from './types';

interface AgendaCalendarProps {
    items: CalendarItem[];
    loading: boolean;
    onSelectItem: (item: CalendarItem) => void;
    onAddReminder: (day: Date) => void;
}

// Sunday-first, matching the traditional 万年历 layout.
const WEEKDAYS = ['日', '一', '二', '三', '四', '五', '六'];

const KIND_ICON: Record<CalendarItem['kind'], string> = {
    reminder: '🔔',
    task: '📋',
    milestone: '🚩',
};

function anchorDate(it: CalendarItem): Date | null {
    const d = new Date(it.date);
    return isNaN(d.getTime()) ? null : d;
}

function dayKey(year: number, month: number, day: number): string {
    return `${year}-${month}-${day}`;
}

function timeLabel(it: CalendarItem): string {
    if (!it.hasTime) return '';
    const d = new Date(it.date);
    if (isNaN(d.getTime())) return '';
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

// A perpetual month calendar for the unified personal agenda. Items are bucketed
// onto their anchor day and rendered with a kind-aware chip; clicking routes by
// kind (handled by the parent). Each day cell exposes a hover "+" to drop a new
// reminder on that date.
export function AgendaCalendar({ items, loading, onSelectItem, onAddReminder }: AgendaCalendarProps) {
    const today = new Date();
    const cursor = useSignal({ y: today.getFullYear(), m: today.getMonth() });

    if (loading && items.length === 0) {
        return <div class="task-loading">正在载入议程...</div>;
    }

    const { y, m } = cursor.value;

    const byDay = new Map<string, CalendarItem[]>();
    for (const it of items) {
        const d = anchorDate(it);
        if (!d) continue;
        const key = dayKey(d.getFullYear(), d.getMonth(), d.getDate());
        const arr = byDay.get(key);
        if (arr) arr.push(it);
        else byDay.set(key, [it]);
    }
    // Within a day, timed items first (by time), then untimed.
    for (const arr of byDay.values()) {
        arr.sort((a, b) => (a.hasTime === b.hasTime ? a.date.localeCompare(b.date) : a.hasTime ? -1 : 1));
    }

    const startOffset = new Date(y, m, 1).getDay();
    const daysInMonth = new Date(y, m + 1, 0).getDate();
    const totalCells = Math.ceil((startOffset + daysInMonth) / 7) * 7;
    const cells = Array.from({ length: totalCells }, (_, i) => new Date(y, m, 1 - startOffset + i));

    const todayKey = dayKey(today.getFullYear(), today.getMonth(), today.getDate());

    const goPrev = () => (cursor.value = m === 0 ? { y: y - 1, m: 11 } : { y, m: m - 1 });
    const goNext = () => (cursor.value = m === 11 ? { y: y + 1, m: 0 } : { y, m: m + 1 });
    const goToday = () => (cursor.value = { y: today.getFullYear(), m: today.getMonth() });

    return (
        <div class="task-calendar agenda-calendar">
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
                    const dayItems = byDay.get(key) || [];
                    const isOtherMonth = date.getMonth() !== m;
                    const isToday = key === todayKey;
                    return (
                        <div
                            key={key}
                            class={`calendar-cell${isOtherMonth ? ' is-other-month' : ''}${isToday ? ' is-today' : ''}`}
                        >
                            <div class="calendar-cell-date">
                                <span class="calendar-cell-day">{date.getDate()}</span>
                                <button class="calendar-cell-add" title="新建提醒" onClick={() => onAddReminder(date)}>
                                    +
                                </button>
                                {dayItems.length > 0 && <span class="calendar-cell-count">{dayItems.length}</span>}
                            </div>
                            <div class="calendar-cell-events">
                                {dayItems.map(it => {
                                    const done = it.status === 'completed';
                                    return (
                                        <button
                                            key={it.id}
                                            class={`calendar-event kind-${it.kind}${done ? ' is-done' : ''}`}
                                            onClick={() => onSelectItem(it)}
                                            title={`${it.workspaceName ? it.workspaceName + ' · ' : ''}${it.title}`}
                                        >
                                            <span class="calendar-event-icon">{KIND_ICON[it.kind]}</span>
                                            {it.hasTime && <span class="calendar-event-time">{timeLabel(it)}</span>}
                                            <span class="calendar-event-title">{it.title}</span>
                                            {it.recurrence && <span class="calendar-event-repeat">↻</span>}
                                        </button>
                                    );
                                })}
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
