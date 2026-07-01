import { h } from 'preact';
import { useState } from 'preact/hooks';

import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';
import { imessageService, channelModuleService, type ChannelModule } from '@1agents/core/services/contactService';

// iMessage 子模块 — reads the local chat.db (plaintext, gated by the user's Full
// Disk Access grant; no decryption, no AI). Shows the FDA guidance, a manual sync,
// and the crawl rules (time window + attachments + frequency). Assumes the parent
// has already cleared the consent gate.

const WINDOWS = [90, 365, 0]; // days; 0 = 全部
const FREQS = [0, 180, 1440]; // minutes; 0 = manual

export function IMessageSection({ module, onChange }: { module: ChannelModule; onChange: () => void }) {
    const language = ui.language.value;
    const r = module.rules || {};
    const [timeWindowDays, setTimeWindowDays] = useState<number>(
        typeof r.timeWindowDays === 'number' ? (r.timeWindowDays as number) : 90
    );
    const [includeAttachments, setIncludeAttachments] = useState<boolean>(r.includeAttachments === true);
    const [intervalMinutes, setIntervalMinutes] = useState<number>(module.intervalMinutes || 0);
    const [error, setError] = useState('');
    const [toast, setToast] = useState('');
    const [busy, setBusy] = useState(false);

    const saveRules = async (next: { tw?: number; att?: boolean; iv?: number }) => {
        const tw = next.tw ?? timeWindowDays;
        const att = next.att ?? includeAttachments;
        const iv = next.iv ?? intervalMinutes;
        setTimeWindowDays(tw);
        setIncludeAttachments(att);
        setIntervalMinutes(iv);
        setError('');
        try {
            await channelModuleService.setRules(module.id, {
                autoSync: iv > 0,
                intervalMinutes: iv,
                rules: { timeWindowDays: tw, includeAttachments: att },
            });
            onChange();
        } catch (e) {
            setError((e as Error).message);
        }
    };

    const syncNow = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            const res = await imessageService.sync();
            setToast(t('contacts.imsg.syncDone', language, { inserted: res.inserted, fetched: res.fetched }));
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <div class="contacts-icloud-setup">
            <div class="contacts-icloud-steps" style="padding-left:0;">
                {t('contacts.imsg.fdaHint', language)}
            </div>
            <div class="contacts-modrules">
                <label class="contacts-field">
                    <span>{t('contacts.imsg.timeWindow', language)}</span>
                    <select
                        value={String(timeWindowDays)}
                        onChange={(e: Event) => saveRules({ tw: Number((e.target as HTMLSelectElement).value) })}
                    >
                        {WINDOWS.map(d => (
                            <option key={d} value={String(d)}>
                                {t(`contacts.imsg.window.${d}`, language)}
                            </option>
                        ))}
                    </select>
                </label>
                <label class="contacts-field">
                    <span>{t('contacts.imsg.attachments', language)}</span>
                    <select
                        value={includeAttachments ? '1' : '0'}
                        onChange={(e: Event) => saveRules({ att: (e.target as HTMLSelectElement).value === '1' })}
                    >
                        <option value="0">{t('contacts.imsg.exclude', language)}</option>
                        <option value="1">{t('contacts.imsg.include', language)}</option>
                    </select>
                </label>
                <label class="contacts-field">
                    <span>{t('contacts.channels.interval', language)}</span>
                    <select
                        value={String(intervalMinutes)}
                        onChange={(e: Event) => saveRules({ iv: Number((e.target as HTMLSelectElement).value) })}
                    >
                        {FREQS.map(m => (
                            <option key={m} value={String(m)}>
                                {m === 0
                                    ? t('contacts.imsg.manual', language)
                                    : t(`contacts.channels.interval.${m}`, language)}
                            </option>
                        ))}
                    </select>
                </label>
            </div>

            {error && <div class="contacts-error">{error}</div>}
            {toast && <div class="contacts-channels-toast">{toast}</div>}

            <div class="contacts-modrules-actions">
                <button class="contacts-btn contacts-btn-primary contacts-btn-sm" disabled={busy} onClick={syncNow}>
                    {busy ? t('contacts.imsg.syncing', language) : t('contacts.imsg.syncNow', language)}
                </button>
            </div>
        </div>
    );
}
