import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback, useRef } from 'preact/hooks';

import { t, type Lang } from '../../../i18n';
import { sourceCliService, type CLIStatus } from '@1agents/core/services/sourceCliService';
import { sourceService, type CollectionView, type SyncRun } from '@1agents/core/services/sourceService';
import { ChatScopeModal } from './ChatScopeModal';

// 飞书数据源卡片 — shows three zones for the feishu source:
//   1. CLI lifecycle: lark-cli install/version/auth state, hints with copy buttons
//   2. Collection config: per-kind toggle + crawl parameters
//   3. History & stats: recent sync runs + per-kind "sync now" buttons

const TOOL = 'lark-cli';
const SOURCE = 'feishu';

// Minutes available as incremental frequency options.
const INTERVAL_OPTS = [15, 30, 60, 180, 360, 720, 1440];

// ── Helpers ──────────────────────────────────────────────────────────────────

function CopyIcon() {
    return (
        <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
        </svg>
    );
}

function CheckIcon() {
    return (
        <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="20 6 9 17 4 12" />
        </svg>
    );
}

function useCopyHint(): [string, (key: string, text: string) => void] {
    const [copiedKey, setCopiedKey] = useState('');
    const copy = (key: string, text: string) => {
        const done = () => {
            setCopiedKey(key);
            window.setTimeout(() => setCopiedKey(k => (k === key ? '' : k)), 1500);
        };
        if (navigator.clipboard?.writeText) {
            navigator.clipboard
                .writeText(text)
                .then(done)
                .catch(() => done());
        } else {
            done();
        }
    };
    return [copiedKey, copy];
}

// Absolute local timestamp — no "刚刚 / 即将..." labels. Relative labels lie for
// near-future timestamps (token expiries) and read as buggy; give the user the
// actual moment and let them judge.
function formatAbsTime(iso: string, language: Lang): string {
    return new Date(iso).toLocaleString(language);
}

// Parse the JSON result string from a SyncRun (e.g. '{"kind":"feishu_chat","collections":1,"changed":18}')
function parseSyncResult(result?: string): { changed?: number; collections?: number } {
    if (!result) return {};
    try {
        return JSON.parse(result) as { changed?: number; collections?: number };
    } catch {
        return {};
    }
}

function syncStatusClass(status: string): string {
    if (status === 'done' || status === 'completed') return 'done';
    if (status === 'running') return 'running';
    if (status === 'pending') return 'pending';
    return 'failed';
}

// Group CollectionViews by domain.
function groupByDomain(cols: CollectionView[]): Array<{ domain: string; items: CollectionView[] }> {
    const map = new Map<string, CollectionView[]>();
    for (const c of cols) {
        const arr = map.get(c.domain) ?? [];
        arr.push(c);
        map.set(c.domain, arr);
    }
    return Array.from(map.entries()).map(([domain, items]) => ({ domain, items }));
}

// ── Sub-zone: CLI lifecycle ───────────────────────────────────────────────────

interface CliZoneProps {
    language: Lang;
    // Which source CLI to probe. Defaults to lark-cli (飞书); Agent Mail passes
    // 'agently-cli' to reuse this same lifecycle card.
    tool?: string;
}

export function CliZone({ language, tool = TOOL }: CliZoneProps) {
    const [status, setStatus] = useState<CLIStatus | null>(null);
    const [rechecking, setRechecking] = useState(false);
    const [error, setError] = useState('');
    const [copiedKey, copy] = useCopyHint();

    const load = useCallback(async () => {
        setError('');
        try {
            setStatus(await sourceCliService.cliStatus(tool));
        } catch (e) {
            setError((e as Error).message);
        }
    }, [tool]);

    useEffect(() => {
        load();
    }, [load]);

    const recheck = async () => {
        setRechecking(true);
        setError('');
        try {
            setStatus(await sourceCliService.cliRecheck(tool));
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setRechecking(false);
        }
    };

    return (
        <div class="fscard-cli">
            <div class="fscard-cli-header">
                <div class="fscard-cli-title">
                    <span
                        class={`agent-catalog-dot ${status?.installed ? 'installed' : 'missing'}`}
                        aria-hidden="true"
                    />
                    <span>{tool}</span>
                    {status?.installed && status.version && (
                        <span class="fscard-cli-version">
                            {t('datasource.cli.version', language)} {status.version}
                            {status.updateAvailable && status.latestVersion && <> → {status.latestVersion}</>}
                        </span>
                    )}
                </div>
                <div class="fscard-cli-badges">
                    {status && (
                        <span class={`fscard-badge ${status.installed ? 'ok' : 'bad'}`}>
                            {status.installed
                                ? t('datasource.cli.installed', language)
                                : t('datasource.cli.notInstalled', language)}
                        </span>
                    )}
                    {status?.updateAvailable && (
                        <span class="fscard-badge warn">{t('datasource.cli.updateAvailable', language)}</span>
                    )}
                    {status?.installed && (
                        <span class={`fscard-badge ${status.authenticated ? 'ok' : 'bad'}`}>
                            {status.authenticated
                                ? t('datasource.cli.connected', language)
                                : t('datasource.cli.disconnected', language)}
                        </span>
                    )}
                    <button class="contacts-btn contacts-btn-sm" disabled={rechecking} onClick={recheck}>
                        {rechecking ? t('datasource.cli.rechecking', language) : t('datasource.cli.recheck', language)}
                    </button>
                </div>
            </div>

            {error && <div class="fscard-error">{error}</div>}

            {status?.installed && status.authenticated && (
                <Fragment>
                    {status.authAccount && (
                        <div class="fscard-cli-row">
                            <span class="fscard-cli-row-label">{t('datasource.cli.account', language)}</span>
                            <span class="fscard-cli-row-val">{status.authAccount}</span>
                        </div>
                    )}
                    {status.tokenStatus && (
                        <div class="fscard-cli-row">
                            <span class="fscard-cli-row-label">{t('datasource.cli.tokenStatus', language)}</span>
                            <span class="fscard-cli-row-val">{status.tokenStatus}</span>
                        </div>
                    )}
                    {status.authExpiresAt && (
                        <div class="fscard-cli-row">
                            <span class="fscard-cli-row-label">{t('datasource.cli.expiresAt', language)}</span>
                            <span class="fscard-cli-row-val">{formatAbsTime(status.authExpiresAt, language)}</span>
                        </div>
                    )}
                </Fragment>
            )}

            {status && !status.installed && status.installHint && (
                <div class="fscard-hint-row">
                    <span class="fscard-hint-label">{t('datasource.cli.installHint', language)}</span>
                    <code class="fscard-hint-code" title={status.installHint}>
                        {status.installHint}
                    </code>
                    <button
                        class="fscard-copy-btn"
                        title={
                            copiedKey === 'install'
                                ? t('datasource.cli.copied', language)
                                : t('datasource.cli.copy', language)
                        }
                        onClick={() => copy('install', status.installHint!)}
                    >
                        {copiedKey === 'install' ? <CheckIcon /> : <CopyIcon />}
                    </button>
                </div>
            )}

            {status?.installed && !status.authenticated && status.loginHint && (
                <div class="fscard-hint-row">
                    <span class="fscard-hint-label">{t('datasource.cli.loginHint', language)}</span>
                    <code class="fscard-hint-code" title={status.loginHint}>
                        {status.loginHint}
                    </code>
                    <button
                        class="fscard-copy-btn"
                        title={
                            copiedKey === 'login'
                                ? t('datasource.cli.copied', language)
                                : t('datasource.cli.copy', language)
                        }
                        onClick={() => copy('login', status.loginHint!)}
                    >
                        {copiedKey === 'login' ? <CheckIcon /> : <CopyIcon />}
                    </button>
                </div>
            )}

            {status?.updateAvailable && status.updateHint && (
                <div class="fscard-hint-row">
                    <span class="fscard-hint-label">{t('datasource.cli.updateHint', language)}</span>
                    <code class="fscard-hint-code" title={status.updateHint}>
                        {status.updateHint}
                    </code>
                    <button
                        class="fscard-copy-btn"
                        title={
                            copiedKey === 'update'
                                ? t('datasource.cli.copied', language)
                                : t('datasource.cli.copy', language)
                        }
                        onClick={() => copy('update', status.updateHint!)}
                    >
                        {copiedKey === 'update' ? <CheckIcon /> : <CopyIcon />}
                    </button>
                </div>
            )}

            {status?.checkedAt && (
                <div class="fscard-cli-row">
                    <span class="fscard-cli-row-label">{t('datasource.cli.checkedAt', language)}</span>
                    <span class="fscard-cli-row-val">{formatAbsTime(status.checkedAt, language)}</span>
                </div>
            )}
        </div>
    );
}

// ── Sub-zone: collection config ───────────────────────────────────────────────

interface CollectionsZoneProps {
    language: Lang;
    onSyncDispatched: () => void;
    onToast: (msg: string) => void;
}

export function CollectionsZone({ language, onSyncDispatched, onToast }: CollectionsZoneProps) {
    const [collections, setCollections] = useState<CollectionView[]>([]);
    const [error, setError] = useState('');
    const [busyKind, setBusyKind] = useState<string | null>(null);
    // Which chat modal is open: 'view' (群列表缓存) or 'pick' (群消息范围勾选).
    const [modal, setModal] = useState<'view' | 'pick' | null>(null);

    const load = useCallback(async () => {
        setError('');
        try {
            setCollections(await sourceService.collections(SOURCE));
        } catch (e) {
            setError((e as Error).message);
        }
    }, []);

    useEffect(() => {
        load();
    }, [load]);

    const save = async (col: CollectionView, patch: Partial<CollectionView>) => {
        setError('');
        const next = { ...col, ...patch };
        // Optimistic update
        setCollections(prev => prev.map(c => (c.kind === col.kind ? next : c)));
        try {
            const saved = await sourceService.setCollection(SOURCE, {
                kind: next.kind,
                enabled: next.enabled,
                initialLookbackDays: next.initialLookbackDays,
                incrementalMinutes: next.incrementalMinutes,
                pageSize: next.pageSize,
            });
            // MERGE the persisted config back into the existing view — the PUT
            // response is only the SourceCollectionConfig (no label / domain /
            // implemented / perChat), so replacing outright would wipe the view
            // fields and make the row render as an unlabeled "即将上线" stub.
            setCollections(prev => prev.map(c => (c.kind === col.kind ? { ...c, ...saved } : c)));
        } catch (e) {
            setError((e as Error).message);
            // Rollback
            setCollections(prev => prev.map(c => (c.kind === col.kind ? col : c)));
        }
    };

    const syncNow = async (kind: string) => {
        setBusyKind(kind);
        setError('');
        try {
            await sourceService.syncNow(SOURCE, kind);
            onToast(t('datasource.collection.syncDispatched', language));
            onSyncDispatched();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusyKind(null);
        }
    };

    const domainLabel = (domain: string) => {
        const key = `datasource.collection.domain.${domain}`;
        return t(key, language) !== key ? t(key, language) : domain;
    };

    const groups = groupByDomain(collections);

    return (
        <div class="fscard-collections">
            <div class="fscard-zone-title">{t('datasource.tab.config', language)}</div>

            {error && <div class="fscard-error">{error}</div>}

            {groups.map(({ domain, items }) => (
                <div key={domain} class="fscard-domain-group">
                    {groups.length > 1 && <div class="fscard-domain-label">{domainLabel(domain)}</div>}
                    {items.map(col => (
                        <div key={col.kind} class="fscard-collection-row">
                            <div class="fscard-collection-main">
                                <label class="contacts-channels-toggle contacts-channels-toggle-sm">
                                    <input
                                        type="checkbox"
                                        checked={col.enabled}
                                        disabled={!col.implemented}
                                        onChange={(e: Event) =>
                                            save(col, { enabled: (e.target as HTMLInputElement).checked })
                                        }
                                    />
                                    <span class="fscard-collection-label">{col.label}</span>
                                </label>
                                {!col.implemented && (
                                    <span class="fscard-badge muted">
                                        {t('datasource.collection.comingSoon', language)}
                                    </span>
                                )}
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
                                                save(col, {
                                                    initialLookbackDays: Number((e.target as HTMLInputElement).value),
                                                })
                                            }
                                        />
                                    </div>
                                    <div class="fscard-field">
                                        <label>{t('datasource.collection.interval', language)}</label>
                                        <select
                                            value={String(col.incrementalMinutes)}
                                            onChange={(e: Event) =>
                                                save(col, {
                                                    incrementalMinutes: Number((e.target as HTMLSelectElement).value),
                                                })
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
                                                save(col, {
                                                    pageSize: Number((e.target as HTMLInputElement).value),
                                                })
                                            }
                                        />
                                    </div>
                                    {/* 群列表 browses the cache; perChat kinds (群消息) pick
                                        their scope from the same cache — one lark-cli pull
                                        serves both, no duplicate fetching. */}
                                    {col.kind === 'feishu_chat' && (
                                        <button class="fscard-sync-btn" onClick={() => setModal('view')}>
                                            {t('datasource.chats.viewBtn', language)}
                                        </button>
                                    )}
                                    {col.perChat && (
                                        <button class="fscard-sync-btn" onClick={() => setModal('pick')}>
                                            {t('datasource.chats.pickBtn', language)}
                                        </button>
                                    )}
                                    <button
                                        class="fscard-sync-btn"
                                        disabled={busyKind === col.kind}
                                        onClick={() => syncNow(col.kind)}
                                    >
                                        {busyKind === col.kind
                                            ? t('datasource.collection.syncing', language)
                                            : t('datasource.collection.syncNow', language)}
                                    </button>
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            ))}

            {modal && <ChatScopeModal mode={modal} onClose={() => setModal(null)} />}
        </div>
    );
}

// ── Sub-zone: sync history ────────────────────────────────────────────────────

interface HistoryZoneProps {
    language: Lang;
    refreshTick: number;
}

// Poll cadence while a dispatched work-order is still pending/running, and a
// per-dispatch budget so a stuck executor can't keep the poll alive forever.
const HISTORY_POLL_MS = 3000;
const HISTORY_POLL_MAX = 20;

export function HistoryZone({ language, refreshTick }: HistoryZoneProps) {
    // null = still loading — renders nothing instead of flashing the empty hint.
    const [runs, setRuns] = useState<SyncRun[] | null>(null);
    const [error, setError] = useState('');
    const pollsLeft = useRef(HISTORY_POLL_MAX);

    const load = useCallback(async () => {
        setError('');
        try {
            setRuns(await sourceService.syncHistory(SOURCE));
        } catch (e) {
            setError((e as Error).message);
        }
    }, []);

    useEffect(() => {
        pollsLeft.current = HISTORY_POLL_MAX; // each dispatch gets a fresh budget
        load();
    }, [load, refreshTick]);

    // While any run is non-terminal, re-fetch so 立即同步 shows its outcome
    // (pending → running → done/failed) without a manual refresh.
    useEffect(() => {
        if (!runs?.some(r => r.status === 'pending' || r.status === 'running')) return;
        if (pollsLeft.current <= 0) return;
        const id = window.setTimeout(() => {
            pollsLeft.current -= 1;
            load();
        }, HISTORY_POLL_MS);
        return () => window.clearTimeout(id);
    }, [runs, load]);

    const statusLabel = (status: string) => {
        const norm = status === 'completed' ? 'done' : status;
        const key = `datasource.collection.status.${norm}`;
        const val = t(key, language);
        return val !== key ? val : status;
    };

    return (
        <div class="fscard-history">
            <div class="fscard-zone-title">{t('datasource.collection.historyTitle', language)}</div>

            {error && <div class="fscard-error">{error}</div>}

            {runs !== null && runs.length === 0 && !error && (
                <div class="contacts-empty">{t('datasource.collection.historyEmpty', language)}</div>
            )}

            {(runs ?? []).slice(0, 20).map(run => {
                const stats = parseSyncResult(run.result);
                const cls = syncStatusClass(run.status);
                return (
                    <div key={run.taskId} class="fscard-history-run">
                        <span class="fscard-history-kind">{run.kind}</span>
                        <span class={`fscard-history-status ${cls}`}>{statusLabel(run.status)}</span>
                        {stats.changed !== undefined && (
                            <span class="fscard-history-stats">
                                {t('datasource.collection.changed', language, { n: stats.changed })}
                                {stats.collections !== undefined && (
                                    <> · {t('datasource.collection.collections', language, { n: stats.collections })}</>
                                )}
                            </span>
                        )}
                        <span class="fscard-history-time">
                            {formatAbsTime(run.completedAt || run.createdAt, language)}
                        </span>
                    </div>
                );
            })}
        </div>
    );
}

// The three zones above (CliZone / CollectionsZone / HistoryZone) are composed by
// FeishuSourcePanel under the source's top-nav tabs — this file exports the zones,
// not a combined card.
