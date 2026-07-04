import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import { t, type Lang } from '../../../i18n';
import { sourceService, type CollectionView, type ScheduleRow } from '@1agents/core/services/sourceService';
import { INTERVAL_OPTS, groupByDomain, formatAbsTime, syncStatusClass } from './FeishuSourceCard';
import { ChatScopeModal } from './ChatScopeModal';

// ScheduleList — 「定时任务」子页面 (config subpage A). One reusable, source-agnostic
// editor: each crawlable kind is an enable toggle + crawl params (回溯/频率/条数) +
// 立即同步, and — layered on top — its live 定时任务 trigger state (已排程? 下次触发?
// 上次运行结果?) read from /schedules. Feishu's 群 selection is the one vendor
// extra, mounted per-row. Replaces the old per-vendor SourceConfigZone /
// CollectionsZone so every source configures its periodic sync the same way.

const FEISHU = 'feishu';

export function ScheduleList({ source, language }: { source: string; language: Lang }) {
    const [collections, setCollections] = useState<CollectionView[] | null>(null);
    const [schedules, setSchedules] = useState<Record<string, ScheduleRow>>({});
    const [error, setError] = useState('');
    const [msg, setMsg] = useState('');
    const [busyKind, setBusyKind] = useState<string | null>(null);
    const [modal, setModal] = useState<'view' | 'pick' | null>(null);

    const loadSchedules = useCallback(() => {
        sourceService
            .schedules(source)
            .then(rows => setSchedules(Object.fromEntries(rows.map(r => [r.kind, r]))))
            .catch(() => setSchedules({}));
    }, [source]);

    const load = useCallback(async () => {
        setError('');
        try {
            const cols = await sourceService.collections(source);
            setCollections(cols);
        } catch (e) {
            setError((e as Error).message);
            setCollections([]);
        }
        loadSchedules();
    }, [source, loadSchedules]);

    useEffect(() => {
        load();
    }, [load]);

    const save = async (col: CollectionView, patch: Partial<CollectionView>) => {
        setError('');
        const next = { ...col, ...patch };
        // Optimistic update.
        setCollections(prev => (prev ?? []).map(c => (c.kind === col.kind ? next : c)));
        try {
            const saved = await sourceService.setCollection(source, {
                kind: next.kind,
                enabled: next.enabled,
                initialLookbackDays: next.initialLookbackDays || 0,
                incrementalMinutes: next.incrementalMinutes || 60,
                pageSize: next.pageSize || 50,
            });
            // The PUT response is only the stored config (no label/domain/implemented),
            // so merge it into the view rather than replacing.
            setCollections(prev => (prev ?? []).map(c => (c.kind === col.kind ? { ...c, ...saved } : c)));
            loadSchedules(); // enabling a kind arms its recurring task
        } catch (e) {
            setError((e as Error).message);
            setCollections(prev => (prev ?? []).map(c => (c.kind === col.kind ? col : c))); // rollback
        }
    };

    const syncNow = async (kind: string) => {
        setBusyKind(kind);
        setError('');
        setMsg('');
        try {
            await sourceService.syncNow(source, kind);
            setMsg(t('datasource.collection.syncDispatched', language));
            window.setTimeout(loadSchedules, 800);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusyKind(null);
        }
    };

    if (collections === null) {
        return <div class="datasource-head-hint">…</div>;
    }
    if (collections.length === 0) {
        return <div class="contacts-empty">{t('datasource.schedule.empty', language)}</div>;
    }

    const domainLabel = (domain: string) => {
        const key = `datasource.collection.domain.${domain}`;
        return t(key, language) !== key ? t(key, language) : domain;
    };
    const groups = groupByDomain(collections);

    return (
        <div class="fscard-collections">
            {error && <div class="fscard-error">{error}</div>}
            {msg && <div class="fscard-toast">{msg}</div>}

            {groups.map(({ domain, items }) => (
                <div key={domain} class="fscard-domain-group">
                    {groups.length > 1 && <div class="fscard-domain-label">{domainLabel(domain)}</div>}
                    {items.map(col => (
                        <ScheduleRowItem
                            key={col.kind}
                            col={col}
                            sched={schedules[col.kind]}
                            source={source}
                            language={language}
                            busy={busyKind === col.kind}
                            onSave={save}
                            onSyncNow={syncNow}
                            onOpenChatModal={setModal}
                        />
                    ))}
                </div>
            ))}

            {modal && <ChatScopeModal mode={modal} onClose={() => setModal(null)} />}
        </div>
    );
}

function ScheduleRowItem({
    col,
    sched,
    source,
    language,
    busy,
    onSave,
    onSyncNow,
    onOpenChatModal,
}: {
    col: CollectionView;
    sched?: ScheduleRow;
    source: string;
    language: Lang;
    busy: boolean;
    onSave: (col: CollectionView, patch: Partial<CollectionView>) => void;
    onSyncNow: (kind: string) => void;
    onOpenChatModal: (mode: 'view' | 'pick') => void;
}) {
    const isFeishu = source === FEISHU;
    return (
        <div class="fscard-collection-row">
            <div class="fscard-collection-main">
                <label class="contacts-channels-toggle contacts-channels-toggle-sm">
                    <input
                        type="checkbox"
                        checked={col.enabled}
                        disabled={!col.implemented}
                        onChange={(e: Event) => onSave(col, { enabled: (e.target as HTMLInputElement).checked })}
                    />
                    <span class="fscard-collection-label">{col.label || col.kind}</span>
                </label>
                {!col.implemented && (
                    <span class="fscard-badge muted">{t('datasource.collection.comingSoon', language)}</span>
                )}
                {col.implemented && col.enabled && <ScheduleStatus sched={sched} language={language} />}
            </div>

            {col.enabled && col.implemented && (
                <div class="fscard-collection-controls">
                    <div class="fscard-field">
                        <label>{t('datasource.collection.lookbackDays', language)}</label>
                        <input
                            type="number"
                            min={1}
                            max={3650}
                            value={col.initialLookbackDays}
                            onBlur={(e: Event) =>
                                onSave(col, { initialLookbackDays: Number((e.target as HTMLInputElement).value) })
                            }
                        />
                    </div>
                    <div class="fscard-field">
                        <label>{t('datasource.collection.interval', language)}</label>
                        <select
                            value={String(col.incrementalMinutes)}
                            onChange={(e: Event) =>
                                onSave(col, { incrementalMinutes: Number((e.target as HTMLSelectElement).value) })
                            }
                        >
                            {INTERVAL_OPTS.map(m => (
                                <option key={m} value={String(m)}>
                                    {t(`contacts.channels.interval.${m}`, language)}
                                </option>
                            ))}
                        </select>
                    </div>
                    <div class="fscard-field">
                        <label>{t('datasource.collection.pageSize', language)}</label>
                        <input
                            type="number"
                            min={10}
                            max={1000}
                            value={col.pageSize}
                            onBlur={(e: Event) =>
                                onSave(col, { pageSize: Number((e.target as HTMLInputElement).value) })
                            }
                        />
                    </div>
                    {/* Feishu 群 extras: 群列表 browses the cache; perChat kinds (群消息) pick
                        their scope from the same cache — one lark-cli pull serves both. */}
                    {isFeishu && col.kind === 'feishu_chat' && (
                        <button class="fscard-sync-btn" onClick={() => onOpenChatModal('view')}>
                            {t('datasource.chats.viewBtn', language)}
                        </button>
                    )}
                    {isFeishu && col.perChat && (
                        <button class="fscard-sync-btn" onClick={() => onOpenChatModal('pick')}>
                            {t('datasource.chats.pickBtn', language)}
                        </button>
                    )}
                    <button class="fscard-sync-btn" disabled={busy} onClick={() => onSyncNow(col.kind)}>
                        {busy
                            ? t('datasource.collection.syncing', language)
                            : t('datasource.collection.syncNow', language)}
                    </button>
                </div>
            )}
        </div>
    );
}

// ScheduleStatus renders the live 定时任务 trigger badge + next/last run, from the
// ScheduleRow. The collection row already carries enable/cadence; this is purely
// the runtime "task system" slice (是否触发/触发状态).
function ScheduleStatus({ sched, language }: { sched?: ScheduleRow; language: Lang }) {
    const statusLabel = (status?: string) => {
        if (!status) return '';
        const norm = status === 'completed' ? 'done' : status;
        const key = `datasource.collection.status.${norm}`;
        return t(key, language) !== key ? t(key, language) : status;
    };
    return (
        <span class="datasource-schedule-status">
            <span class={`fscard-badge ${sched?.recurring ? 'ok' : 'muted'}`}>
                {sched?.recurring
                    ? t('datasource.schedule.armed', language)
                    : t('datasource.schedule.notArmed', language)}
            </span>
            {sched?.recurring && sched.nextRunAt && (
                <span class="datasource-schedule-meta">
                    {t('datasource.schedule.nextRun', language)} · {formatAbsTime(sched.nextRunAt, language)}
                </span>
            )}
            {sched?.lastStatus && (
                <span class="datasource-schedule-meta">
                    {t('datasource.schedule.lastRun', language)} ·{' '}
                    <span class={`fscard-history-status ${syncStatusClass(sched.lastStatus)}`}>
                        {statusLabel(sched.lastStatus)}
                    </span>
                    {sched.lastRunAt && <Fragment> · {formatAbsTime(sched.lastRunAt, language)}</Fragment>}
                </span>
            )}
        </span>
    );
}
