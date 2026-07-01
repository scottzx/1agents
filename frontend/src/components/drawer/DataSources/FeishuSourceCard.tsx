import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { sourceCliService, type CLIStatus } from '@1agents/core/services/sourceCliService';
import { sourceService, type CollectionView, type SyncRun } from '@1agents/core/services/sourceService';

// 飞书数据源卡片 — shows three zones for the feishu source:
//   1. CLI lifecycle: lark-cli install/version/auth state, hints with copy buttons
//   2. Collection config: per-kind toggle + crawl parameters
//   3. History & stats: recent sync runs + per-kind "sync now" buttons

const TOOL = 'lark-cli';
const SOURCE = 'feishu';

// Minutes available as incremental frequency options (matches FeishuSection).
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

function formatRelTime(iso: string): string {
    const ms = Date.now() - new Date(iso).getTime();
    if (ms < 60_000) return '刚刚';
    if (ms < 3_600_000) return `${Math.floor(ms / 60_000)} 分钟前`;
    if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)} 小时前`;
    return `${Math.floor(ms / 86_400_000)} 天前`;
}

function formatAbsTime(iso: string, language: string): string {
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
    language: string;
}

function CliZone({ language }: CliZoneProps) {
    const [status, setStatus] = useState<CLIStatus | null>(null);
    const [rechecking, setRechecking] = useState(false);
    const [error, setError] = useState('');
    const [copiedKey, copy] = useCopyHint();

    const load = useCallback(async () => {
        setError('');
        try {
            setStatus(await sourceCliService.cliStatus(TOOL));
        } catch (e) {
            setError((e as Error).message);
        }
    }, []);

    useEffect(() => {
        load();
    }, [load]);

    const recheck = async () => {
        setRechecking(true);
        setError('');
        try {
            setStatus(await sourceCliService.cliRecheck(TOOL));
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
                    <span>lark-cli</span>
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
                            <span class="fscard-cli-row-val">
                                {formatRelTime(status.authExpiresAt)} ({formatAbsTime(status.authExpiresAt, language)})
                            </span>
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
                    <span class="fscard-cli-row-val">{formatRelTime(status.checkedAt)}</span>
                </div>
            )}
        </div>
    );
}

// ── Sub-zone: collection config ───────────────────────────────────────────────

interface CollectionsZoneProps {
    language: string;
    onSyncDispatched: () => void;
    onToast: (msg: string) => void;
}

function CollectionsZone({ language, onSyncDispatched, onToast }: CollectionsZoneProps) {
    const [collections, setCollections] = useState<CollectionView[]>([]);
    const [error, setError] = useState('');
    const [busyKind, setBusyKind] = useState<string | null>(null);

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
            setCollections(prev => prev.map(c => (c.kind === col.kind ? saved : c)));
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
                                    {col.perChat ? (
                                        <span class="fscard-perchat-hint">
                                            {t('datasource.collection.perChatHint', language)}
                                        </span>
                                    ) : (
                                        <Fragment>
                                            <div class="fscard-field">
                                                <label>{t('datasource.collection.lookbackDays', language)}</label>
                                                <input
                                                    type="number"
                                                    min={1}
                                                    max={3650}
                                                    value={col.initialLookbackDays}
                                                    onBlur={(e: Event) =>
                                                        save(col, {
                                                            initialLookbackDays: Number(
                                                                (e.target as HTMLInputElement).value
                                                            ),
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
                                                            incrementalMinutes: Number(
                                                                (e.target as HTMLSelectElement).value
                                                            ),
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
                                        </Fragment>
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
        </div>
    );
}

// ── Sub-zone: sync history ────────────────────────────────────────────────────

interface HistoryZoneProps {
    language: string;
    refreshTick: number;
}

function HistoryZone({ language, refreshTick }: HistoryZoneProps) {
    const [runs, setRuns] = useState<SyncRun[]>([]);
    const [error, setError] = useState('');

    const load = useCallback(async () => {
        setError('');
        try {
            setRuns(await sourceService.syncHistory(SOURCE));
        } catch (e) {
            setError((e as Error).message);
        }
    }, []);

    useEffect(() => {
        load();
    }, [load, refreshTick]);

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

            {runs.length === 0 && !error && (
                <div class="contacts-empty">{t('datasource.collection.historyEmpty', language)}</div>
            )}

            {runs.slice(0, 20).map(run => {
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
                            {run.completedAt ? formatRelTime(run.completedAt) : formatRelTime(run.createdAt)}
                        </span>
                    </div>
                );
            })}
        </div>
    );
}

// ── Main export ───────────────────────────────────────────────────────────────

export function FeishuSourceCard() {
    const language = ui.language.value;
    const [toast, setToast] = useState('');
    // Incrementing this causes HistoryZone to re-fetch after a syncNow.
    const [historyTick, setHistoryTick] = useState(0);

    const showToast = (msg: string) => {
        setToast(msg);
        window.setTimeout(() => setToast(''), 3000);
    };

    const onSyncDispatched = () => {
        // Give the backend a moment to record the run before we re-fetch history.
        window.setTimeout(() => setHistoryTick(n => n + 1), 800);
    };

    return (
        <div class="fscard">
            {toast && <div class="fscard-toast">{toast}</div>}
            <CliZone language={language} />
            <CollectionsZone language={language} onSyncDispatched={onSyncDispatched} onToast={showToast} />
            <HistoryZone language={language} refreshTick={historyTick} />
        </div>
    );
}
