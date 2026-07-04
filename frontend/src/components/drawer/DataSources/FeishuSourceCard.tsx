import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import { t, type Lang } from '../../../i18n';
import { sourceCliService, type CLIStatus } from '@1agents/core/services/sourceCliService';
import type { CollectionView } from '@1agents/core/services/sourceService';

// 飞书数据源卡片 — the shared CLI-lifecycle zone (lark-cli / agently-cli) plus a
// handful of small helpers reused by the unified ScheduleList / TaskRunsGrid.
// Collection config + history moved into those reusable components; this file
// keeps only the CLI card and the parsing/formatting helpers.

const TOOL = 'lark-cli';

// Minutes available as incremental frequency options (shared by ScheduleList).
export const INTERVAL_OPTS = [15, 30, 60, 180, 360, 720, 1440];

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
export function formatAbsTime(iso: string, language: Lang): string {
    return new Date(iso).toLocaleString(language);
}

// Parse the JSON result string from a SyncRun (e.g. '{"kind":"feishu_chat","collections":1,"changed":18}')
export function parseSyncResult(result?: string): { changed?: number; collections?: number } {
    if (!result) return {};
    try {
        return JSON.parse(result) as { changed?: number; collections?: number };
    } catch {
        return {};
    }
}

export function syncStatusClass(status: string): string {
    if (status === 'done' || status === 'completed') return 'done';
    if (status === 'running') return 'running';
    if (status === 'pending') return 'pending';
    return 'failed';
}

// Group CollectionViews by domain.
export function groupByDomain(cols: CollectionView[]): Array<{ domain: string; items: CollectionView[] }> {
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
