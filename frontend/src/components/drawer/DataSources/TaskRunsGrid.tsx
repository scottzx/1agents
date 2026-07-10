import { h } from 'preact';
import { useState, useEffect, useCallback, useRef } from 'preact/hooks';

import { t, type Lang } from '../../../i18n';
import { sourceService, type SyncRun } from '@1agents/core/services/sourceService';
import { parseSyncResult, formatAbsTime } from './FeishuSourceCard';
import { ProcessBlock, type ProcessStatus, InlineBadge, ErrorState } from '../../shared/primitives';

// TaskRunsGrid — 「任务执行情况」子页面 (config subpage B). Each sync run is a
// collapsible ProcessBlock: summary in the header (kind + time), detail body shows
// change counts and, for failures, a foldable error + retry action. Auto-polls
// while any run is pending/running so 立即同步 shows its outcome without manual refresh.

const POLL_MS = 3000;
const POLL_MAX = 20;

function toProcessStatus(status: string): ProcessStatus {
    switch (status) {
        case 'running':
            return 'running';
        case 'pending':
            return 'waiting';
        case 'completed':
            return 'success';
        case 'failed':
        case 'error':
            return 'error';
        default:
            return 'idle';
    }
}

export function TaskRunsGrid({ source, language }: { source: string; language: Lang }) {
    const [runs, setRuns] = useState<SyncRun[] | null>(null);
    const [error, setError] = useState('');
    const [retryBusy, setRetryBusy] = useState<string | null>(null);
    const pollsLeft = useRef(POLL_MAX);

    const load = useCallback(async () => {
        setError('');
        try {
            setRuns(await sourceService.syncHistory(source));
        } catch (e) {
            setError((e as Error).message);
        }
    }, [source]);

    useEffect(() => {
        pollsLeft.current = POLL_MAX;
        load();
    }, [load]);

    useEffect(() => {
        if (!runs?.some(r => r.status === 'pending' || r.status === 'running')) return;
        if (pollsLeft.current <= 0) return;
        const id = window.setTimeout(() => {
            pollsLeft.current -= 1;
            load();
        }, POLL_MS);
        return () => window.clearTimeout(id);
    }, [runs, load]);

    const retryKind = async (kind: string) => {
        setRetryBusy(kind);
        try {
            await sourceService.syncNow(source, kind);
            pollsLeft.current = POLL_MAX;
            window.setTimeout(load, 800);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setRetryBusy(null);
        }
    };

    if (error) return <ErrorState title={t('datasource.runs.syncFailed', language)} message={error} onRetry={load} />;
    if (runs === null) return <div class="datasource-head-hint">…</div>;
    if (runs.length === 0) return <div class="contacts-empty">{t('datasource.collection.historyEmpty', language)}</div>;

    return (
        <div class="datasource-runs-list">
            {runs.slice(0, 100).map(run => {
                const ps = toProcessStatus(run.status);
                const stats = parseSyncResult(run.result);
                const timeStr = formatAbsTime(run.completedAt || run.createdAt, language);
                const isFailed = ps === 'error';

                // statusLabel: compact summary shown in the collapsed header
                let statusLabel = timeStr;
                if (ps === 'success' && stats.changed !== undefined) {
                    statusLabel = `${t('datasource.collection.changed', language, { n: stats.changed })} · ${timeStr}`;
                }

                return (
                    <ProcessBlock
                        key={run.taskId}
                        title={run.kind}
                        status={ps}
                        statusLabel={statusLabel}
                        defaultExpanded={isFailed || ps === 'running' || ps === 'waiting'}
                    >
                        <div class="datasource-run-detail">
                            {stats.changed !== undefined && (
                                <div class="datasource-run-stats">
                                    <InlineBadge variant="success">
                                        {t('datasource.collection.changed', language, { n: stats.changed })}
                                    </InlineBadge>
                                    {stats.collections !== undefined && (
                                        <InlineBadge variant="muted">
                                            {t('datasource.collection.collections', language, {
                                                n: stats.collections,
                                            })}
                                        </InlineBadge>
                                    )}
                                </div>
                            )}
                            {isFailed && (
                                <ErrorState
                                    title={t('datasource.runs.syncFailed', language)}
                                    detail={run.result ?? t('datasource.runs.unknownError', language)}
                                    onRetry={retryBusy === run.kind ? undefined : () => retryKind(run.kind)}
                                    retryLabel={t('datasource.runs.retry', language)}
                                />
                            )}
                        </div>
                    </ProcessBlock>
                );
            })}
        </div>
    );
}
