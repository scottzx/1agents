import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback, useRef } from 'preact/hooks';

import { t, type Lang } from '../../../i18n';
import { sourceService, type SyncRun } from '@1agents/core/services/sourceService';
import { parseSyncResult, syncStatusClass, formatAbsTime } from './FeishuSourceCard';

// TaskRunsGrid — 「任务执行情况」子页面 (config subpage B). A schema-fixed 多维表格
// over the source's work-order sync runs (/history): one row per single run with
// 采集项 / 状态 / 结果(变更条数·集合数) / 时间. Same task system that powers every
// scheduled sync, filtered to this source. Polls while any run is still
// pending/running so 立即同步 shows its outcome without a manual refresh.

const POLL_MS = 3000;
const POLL_MAX = 20;

export function TaskRunsGrid({ source, language }: { source: string; language: Lang }) {
    const [runs, setRuns] = useState<SyncRun[] | null>(null);
    const [error, setError] = useState('');
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

    const statusLabel = (status: string) => {
        const norm = status === 'completed' ? 'done' : status;
        const key = `datasource.collection.status.${norm}`;
        return t(key, language) !== key ? t(key, language) : status;
    };

    if (error) return <div class="fscard-error">{error}</div>;
    if (runs === null) return <div class="datasource-head-hint">…</div>;
    if (runs.length === 0) return <div class="contacts-empty">{t('datasource.collection.historyEmpty', language)}</div>;

    return (
        <div class="datasource-runs-grid">
            <table class="datasource-runs-table">
                <thead>
                    <tr>
                        <th>{t('datasource.runs.col.kind', language)}</th>
                        <th>{t('datasource.runs.col.status', language)}</th>
                        <th>{t('datasource.runs.col.result', language)}</th>
                        <th>{t('datasource.runs.col.time', language)}</th>
                    </tr>
                </thead>
                <tbody>
                    {runs.slice(0, 100).map(run => {
                        const stats = parseSyncResult(run.result);
                        return (
                            <tr key={run.taskId}>
                                <td>{run.kind}</td>
                                <td>
                                    <span class={`fscard-history-status ${syncStatusClass(run.status)}`}>
                                        {statusLabel(run.status)}
                                    </span>
                                </td>
                                <td>
                                    {stats.changed !== undefined ? (
                                        <span class="fscard-history-stats">
                                            {t('datasource.collection.changed', language, { n: stats.changed })}
                                            {stats.collections !== undefined && (
                                                <Fragment>
                                                    {' · '}
                                                    {t('datasource.collection.collections', language, {
                                                        n: stats.collections,
                                                    })}
                                                </Fragment>
                                            )}
                                        </span>
                                    ) : (
                                        '—'
                                    )}
                                </td>
                                <td>{formatAbsTime(run.completedAt || run.createdAt, language)}</td>
                            </tr>
                        );
                    })}
                </tbody>
            </table>
        </div>
    );
}
