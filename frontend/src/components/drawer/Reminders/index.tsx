import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import * as taskNav from '../../../stores/taskNavStore';
import { t } from '../../../i18n';
import type { CalendarItem } from './types';
import { AgendaCalendar } from './AgendaCalendar';
import { AgendaItemPopover, type ReminderDraft } from './AgendaItemPopover';

// The unified personal agenda (#192): aggregates personal reminders, scheduled/
// recurring project tasks, and milestone dates across ALL workspaces via the
// /api/agent/agenda endpoint, rendered on a month calendar. Click routing is
// kind-aware — reminders and milestones open a light popover; project tasks
// deep-link to their board detail.
export function RemindersPane() {
    const language = ui.language.value;
    const [items, setItems] = useState<CalendarItem[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [selected, setSelected] = useState<CalendarItem | null>(null);
    const [draft, setDraft] = useState<ReminderDraft | null>(null);

    const fetchAgenda = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            const res = await fetch('/api/agent/agenda');
            if (!res.ok) throw new Error(`Failed to load agenda: ${res.statusText}`);
            setItems((await res.json()) || []);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        fetchAgenda();
    }, [fetchAgenda]);

    const onSelectItem = (it: CalendarItem) => {
        if (it.kind === 'task') {
            // Project task → deep-link to its board detail in its own workspace.
            taskNav.openTaskById(it.workspaceId, it.id);
            return;
        }
        // Reminder or milestone → light popover.
        setDraft(null);
        setSelected(it);
    };

    const closePopover = (changed: boolean) => {
        setSelected(null);
        setDraft(null);
        if (changed) fetchAgenda();
    };

    return (
        <div class="reminders-pane">
            <div class="reminders-pane-header">
                <h2>{t('sidebar.navCtrl.scheduledTasks', language)}</h2>
                <div class="reminders-pane-actions">
                    <button
                        class="reminders-add"
                        onClick={() => {
                            setSelected(null);
                            setDraft({ presetDay: new Date() });
                        }}
                    >
                        + 新建提醒
                    </button>
                    <button class="reminders-refresh" onClick={fetchAgenda} title={t('common.refresh', language)}>
                        ↻
                    </button>
                </div>
            </div>

            {error ? (
                <div class="task-error">{error}</div>
            ) : (
                <AgendaCalendar
                    items={items}
                    loading={loading}
                    onSelectItem={onSelectItem}
                    onAddReminder={day => {
                        setSelected(null);
                        setDraft({ presetDay: day });
                    }}
                />
            )}

            {(selected || draft) && (
                <AgendaItemPopover item={selected || undefined} draft={draft || undefined} onClose={closePopover} />
            )}
        </div>
    );
}
