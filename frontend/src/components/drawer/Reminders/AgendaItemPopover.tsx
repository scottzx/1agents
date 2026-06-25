import { h } from 'preact';
import type { ComponentChildren } from 'preact';
import { useState } from 'preact/hooks';

import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';
import type { CalendarItem } from './types';

type RepeatFreq = '' | 'daily' | 'weekly' | 'monthly';

// A draft for a new reminder carries the preset day the user clicked.
export interface ReminderDraft {
    presetDay: Date;
}

interface AgendaItemPopoverProps {
    // Exactly one of these is set: an existing item, or a new-reminder draft.
    item?: CalendarItem;
    draft?: ReminderDraft;
    // Called after a successful create/edit/done/delete (true) or a plain close
    // (false). The parent refetches the agenda when changed.
    onClose: (changed: boolean) => void;
}

const REMINDERS_WORKSPACE = 'default';

// Convert a UTC ISO string to the value a <input type="datetime-local"> expects
// (local "YYYY-MM-DDTHH:MM"), or '' when absent.
function toLocalInput(iso: string | undefined): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function presetInput(day: Date): string {
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${day.getFullYear()}-${pad(day.getMonth() + 1)}-${pad(day.getDate())}T09:00`;
}

// Build the recurrence object the backend expects from a freq + the chosen date.
function buildRecurrence(freq: RepeatFreq, when: Date | null): Record<string, unknown> | null {
    if (!freq) return null;
    const at = when ? `${String(when.getHours()).padStart(2, '0')}:${String(when.getMinutes()).padStart(2, '0')}` : '';
    const r: Record<string, unknown> = { freq, at };
    if (freq === 'weekly' && when) r.weekday = when.getDay();
    if (freq === 'monthly' && when) r.monthday = when.getDate();
    return r;
}

// Lightweight inline editor for an agenda item. Personal reminders are fully
// editable (title / time / repeat) with done + delete; milestones are read-only
// (the heavy project detail lives on the board). Project tasks never reach this
// popover — the parent deep-links them instead.
export function AgendaItemPopover({ item, draft, onClose }: AgendaItemPopoverProps) {
    const language = ui.language.value;
    const isMilestone = item?.kind === 'milestone';
    const isNew = !!draft;

    const initialWhen = draft ? presetInput(draft.presetDay) : item && item.hasTime ? toLocalInput(item.date) : '';
    const [title, setTitle] = useState(item?.title || '');
    const [when, setWhen] = useState(initialWhen);
    const [repeat, setRepeat] = useState<RepeatFreq>((item?.recurrence?.freq as RepeatFreq) || '');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    // Milestone: read-only card.
    if (isMilestone && item) {
        return (
            <Shell onClose={() => onClose(false)} title="🚩 里程碑">
                <div class="agenda-pop-readonly">
                    <div class="agenda-pop-title">{item.title}</div>
                    {item.workspaceName && <div class="agenda-pop-meta">项目：{item.workspaceName}</div>}
                    <div class="agenda-pop-meta">目标日期：{new Date(item.date).toLocaleDateString(language)}</div>
                </div>
            </Shell>
        );
    }

    const save = async () => {
        if (!title.trim()) {
            setError('标题不能为空');
            return;
        }
        setBusy(true);
        setError('');
        try {
            const whenDate = when ? new Date(when) : null;
            const recurrence = buildRecurrence(repeat, whenDate);
            if (isNew) {
                const body: Record<string, unknown> = {
                    workspace_id: REMINDERS_WORKSPACE,
                    title: title.trim(),
                    assignee: 'user',
                    type: 'task',
                };
                if (whenDate) {
                    body.scheduleType = 'scheduled';
                    body.scheduledAt = whenDate.toISOString();
                }
                if (recurrence) body.recurrence = recurrence;
                const res = await fetch('/api/agent/tasks', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                if (!res.ok) throw new Error(await res.text());
            } else if (item) {
                const body: Record<string, unknown> = { title: title.trim(), recurrence };
                if (whenDate) {
                    body.scheduleType = 'scheduled';
                    body.scheduledAt = whenDate.toISOString();
                }
                const res = await fetch(`/api/agent/tasks/${item.id}`, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                if (!res.ok) throw new Error(await res.text());
            }
            onClose(true);
        } catch (e) {
            setError((e as Error).message || '保存失败');
            setBusy(false);
        }
    };

    const markDone = async () => {
        if (!item) return;
        setBusy(true);
        try {
            const res = await fetch(`/api/agent/tasks/${item.id}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ status: 'completed' }),
            });
            if (!res.ok) throw new Error(await res.text());
            onClose(true);
        } catch (e) {
            setError((e as Error).message || '操作失败');
            setBusy(false);
        }
    };

    const remove = async () => {
        if (!item) return;
        setBusy(true);
        try {
            const res = await fetch(
                `/api/agent/tasks/${item.id}?workspace_id=${encodeURIComponent(item.workspaceId)}`,
                { method: 'DELETE' }
            );
            if (!res.ok) throw new Error(await res.text());
            onClose(true);
        } catch (e) {
            setError((e as Error).message || '删除失败');
            setBusy(false);
        }
    };

    const done = item?.status === 'completed';

    return (
        <Shell onClose={() => onClose(false)} title={isNew ? '🔔 新建提醒' : '🔔 提醒'}>
            <label class="agenda-pop-field">
                <span>标题</span>
                <input
                    type="text"
                    value={title}
                    placeholder="例如：海底捞吃饭"
                    onInput={e => setTitle((e.target as HTMLInputElement).value)}
                />
            </label>
            <label class="agenda-pop-field">
                <span>时间（可选）</span>
                <input
                    type="datetime-local"
                    value={when}
                    onInput={e => setWhen((e.target as HTMLInputElement).value)}
                />
            </label>
            <label class="agenda-pop-field">
                <span>重复</span>
                <select value={repeat} onChange={e => setRepeat((e.target as HTMLSelectElement).value as RepeatFreq)}>
                    <option value="">不重复</option>
                    <option value="daily">每天</option>
                    <option value="weekly">每周</option>
                    <option value="monthly">每月</option>
                </select>
            </label>

            {error && <div class="agenda-pop-error">{error}</div>}

            <div class="agenda-pop-actions">
                {!isNew && !done && (
                    <button class="agenda-pop-btn done" disabled={busy} onClick={markDone}>
                        ✓ 完成
                    </button>
                )}
                {!isNew && (
                    <button class="agenda-pop-btn danger" disabled={busy} onClick={remove}>
                        删除
                    </button>
                )}
                <span class="agenda-pop-spacer" />
                <button class="agenda-pop-btn" disabled={busy} onClick={() => onClose(false)}>
                    {t('common.cancel', language)}
                </button>
                <button class="agenda-pop-btn primary" disabled={busy} onClick={save}>
                    {isNew ? '创建' : '保存'}
                </button>
            </div>
        </Shell>
    );
}

// Shared modal shell: backdrop + card.
function Shell({ children, title, onClose }: { children: ComponentChildren; title: string; onClose: () => void }) {
    return (
        <div class="agenda-pop-backdrop" onClick={onClose}>
            <div class="agenda-pop-card" onClick={e => e.stopPropagation()}>
                <div class="agenda-pop-header">
                    <span>{title}</span>
                    <button class="agenda-pop-close" onClick={onClose}>
                        ✕
                    </button>
                </div>
                {children}
            </div>
        </div>
    );
}
