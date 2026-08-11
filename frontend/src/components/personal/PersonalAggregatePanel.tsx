/**
 * PersonalAggregatePanel (task #329) — the Personal Shell's cross-shell work
 * aggregation view. It renders the user's executable work across every
 * WorkCase / domain — running, awaiting human input/approval, failed, blocked,
 * and due-soon items — joined to each item's WorkCase (CaseRef) and the owning
 * domain's read-only summary (DomainRef).
 *
 * Data comes exclusively from GET /api/agent/personal/aggregate, a kernel-query
 * + domain-summary read model: this component never reads a domain table. A
 * domain summary that is not visible renders as a restricted placeholder. Every
 * row carries a deep link back to the owning shell / case / subject / task.
 */

import { h } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';

import { personalAggregateService } from '@1agents/core/services/personalAggregateService';
import { executionService } from '@1agents/core/services/executionService';
import type { ExecutionJob, ExecutionRun } from '@1agents/core/types/execution';
import type {
    AggregateWorkItem,
    AggBucketFilter,
    AggSortField,
    PersonalAggregateResponse,
} from '@1agents/core/types/workcase';

import { t, type Lang } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as taskNav from '../../stores/taskNavStore';
import * as shellStore from '../../stores/productShellStore';
import { EmptyState, ErrorState, InlineBadge, type ProcessStatus } from '../shared/primitives';

// ── bucket + sort metadata ───────────────────────────────────────────────────

const BUCKETS: readonly AggBucketFilter[] = ['all', 'awaiting', 'running', 'failed', 'blocked', 'due_soon', 'open'];

const SORTS: readonly { value: AggSortField; labelKey: string }[] = [
    { value: '', labelKey: 'personalAggregate.sort.salience' },
    { value: 'updated', labelKey: 'personalAggregate.sort.updated' },
    { value: 'due', labelKey: 'personalAggregate.sort.due' },
    { value: 'priority', labelKey: 'personalAggregate.sort.priority' },
];

/** Map a task status to a process-dot status for the row indicator. */
function statusDot(status: AggregateWorkItem['status']): ProcessStatus {
    switch (status) {
        case 'running':
        case 'queued':
            return 'running';
        case 'completed':
            return 'success';
        case 'failed':
            return 'error';
        case 'blocked':
        case 'not_ready':
        case 'awaiting_human':
        case 'pending_review':
            return 'waiting';
        default:
            return 'idle';
    }
}

/** Map a task status to an i18n label key. */
function statusLabelKey(status: AggregateWorkItem['status']): string {
    return `personalAggregate.status.${status}`;
}

function priorityBadgeVariant(p?: AggregateWorkItem['priority']): 'danger' | 'warning' | 'accent' | 'muted' {
    switch (p) {
        case 'urgent':
            return 'danger';
        case 'high':
            return 'warning';
        case 'medium':
            return 'accent';
        default:
            return 'muted';
    }
}

/** Format an ISO timestamp as a short local date(-time) string. */
function fmtDue(iso: string | undefined, lang: string): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    try {
        return d.toLocaleString(lang === 'zh-CN' ? 'zh-CN' : 'en-US', {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        });
    } catch {
        return d.toISOString().slice(0, 16);
    }
}

// ── component ────────────────────────────────────────────────────────────────

const PAGE_SIZE = 50;

export function PersonalAggregatePanel() {
    const language = ui.language.value;

    const [bucket, setBucket] = useState<AggBucketFilter>('all');
    const [sort, setSort] = useState<AggSortField>('');
    const [caseFilter, setCaseFilter] = useState<string>('');
    const [items, setItems] = useState<AggregateWorkItem[]>([]);
    const [counts, setCounts] = useState<Record<string, number>>({});
    const [total, setTotal] = useState(0);
    const [offset, setOffset] = useState(0);
    const [hasMore, setHasMore] = useState(false);
    const [loading, setLoading] = useState(true);
    const [loadingMore, setLoadingMore] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const load = useCallback(
        async (nextOffset: number, append: boolean) => {
            if (append) setLoadingMore(true);
            else setLoading(true);
            setError(null);
            try {
                const resp: PersonalAggregateResponse = await personalAggregateService.fetch({
                    bucket,
                    sort,
                    case: caseFilter || undefined,
                    limit: PAGE_SIZE,
                    offset: nextOffset,
                });
                setItems(prev => (append ? [...prev, ...resp.items] : resp.items));
                setCounts(resp.counts || {});
                setTotal(resp.total);
                setOffset(nextOffset + resp.items.length);
                setHasMore(resp.hasMore);
            } catch (err) {
                setError(String(err));
                if (!append) setItems([]);
            } finally {
                setLoading(false);
                setLoadingMore(false);
            }
        },
        [bucket, sort, caseFilter]
    );

    // Reset + reload whenever a controlling filter changes (load is re-created
    // when bucket / sort / caseFilter change).
    useEffect(() => {
        setItems([]);
        setOffset(0);
        void load(0, false);
    }, [load]);

    // ── deep-link handlers ───────────────────────────────────────────────────

    const openTask = useCallback((item: AggregateWorkItem) => {
        const dl = item.deepLink;
        if (dl.taskWorkspaceId && dl.taskId) {
            taskNav.openTaskById(dl.taskWorkspaceId, dl.taskId);
        }
    }, []);

    const openSubjectShell = useCallback(
        (item: AggregateWorkItem) => {
            const shell = item.deepLink.subjectShell;
            if (shell) {
                shellStore.setActiveShell(shell);
            } else {
                ui.showToast(t('personalAggregate.noShell', language));
            }
        },
        [language]
    );

    const focusCase = useCallback((item: AggregateWorkItem) => {
        const caseId = item.caseRef ? item.caseRef.split(':')[2] : '';
        setCaseFilter(caseId || '');
    }, []);

    const clearCaseFilter = useCallback(() => setCaseFilter(''), []);

    // ── render ───────────────────────────────────────────────────────────────

    return (
        <div class="personal-aggregate">
            <div class="personal-aggregate-header">
                <h2 class="personal-aggregate-title">{t('personalAggregate.title', language)}</h2>
                <div class="personal-aggregate-controls">
                    <select
                        class="personal-aggregate-sort"
                        value={sort}
                        onChange={e => setSort((e.currentTarget as HTMLSelectElement).value as AggSortField)}
                        aria-label={t('personalAggregate.sort.label', language)}
                    >
                        {SORTS.map(s => (
                            <option key={s.value} value={s.value}>
                                {t(s.labelKey, language)}
                            </option>
                        ))}
                    </select>
                    <button
                        type="button"
                        class="personal-aggregate-refresh"
                        onClick={() => void load(0, false)}
                        title={t('personalAggregate.refresh', language)}
                    >
                        ⟳
                    </button>
                </div>
            </div>

            <div class="personal-aggregate-buckets" role="tablist">
                {BUCKETS.map(b => {
                    const active = bucket === b;
                    const count = b === 'all' ? counts['all'] ?? total : counts[b] ?? 0;
                    return (
                        <button
                            key={b}
                            type="button"
                            role="tab"
                            aria-selected={active}
                            class={`personal-aggregate-bucket${active ? ' is-active' : ''}`}
                            onClick={() => setBucket(b)}
                        >
                            {t(`personalAggregate.bucket.${b}`, language)}
                            {typeof count === 'number' && <span class="personal-aggregate-count">{count}</span>}
                        </button>
                    );
                })}
            </div>

            <ExecutionOverview language={language} />

            {caseFilter && (
                <div class="personal-aggregate-casefilter">
                    <InlineBadge
                        variant="accent"
                        title={t('personalAggregate.clearCase', language)}
                        onClick={clearCaseFilter}
                    >
                        {t('personalAggregate.case', language)}: {caseFilter} ✕
                    </InlineBadge>
                </div>
            )}

            <div class="personal-aggregate-body">
                {loading ? (
                    <div class="personal-aggregate-loading">{t('personalAggregate.loading', language)}</div>
                ) : error ? (
                    <ErrorState
                        title={t('personalAggregate.error.title', language)}
                        message={error}
                        onRetry={() => void load(0, false)}
                        retryLabel={t('personalAggregate.retry', language)}
                    />
                ) : items.length === 0 ? (
                    <EmptyState
                        title={t('personalAggregate.empty.title', language)}
                        description={t('personalAggregate.empty.desc', language)}
                    />
                ) : (
                    <ul class="personal-aggregate-list">
                        {items.map(item => (
                            <AggregateRow
                                key={item.id}
                                item={item}
                                language={language}
                                onOpenTask={() => openTask(item)}
                                onOpenSubject={() => openSubjectShell(item)}
                                onFocusCase={() => focusCase(item)}
                            />
                        ))}
                    </ul>
                )}

                {!loading && !error && hasMore && (
                    <div class="personal-aggregate-footer">
                        <button
                            type="button"
                            class="personal-aggregate-more"
                            onClick={() => void load(offset, true)}
                            disabled={loadingMore}
                        >
                            {loadingMore
                                ? t('personalAggregate.loading', language)
                                : t('personalAggregate.loadMore', language)}
                        </button>
                    </div>
                )}
                {!loading && !error && items.length > 0 && (
                    <div class="personal-aggregate-total">{t('personalAggregate.total', language, { n: total })}</div>
                )}
            </div>
        </div>
    );
}

// ── execution control plane ────────────────────────────────────────────────

function triggerLabel(job: ExecutionJob, language: Lang): string {
    const trigger = job.trigger;
    if (!trigger) return t('personalAggregate.execution.manual', language);
    if (trigger.kind === 'recurrence') {
        const every = Number(trigger.spec.everyMinutes);
        return Number.isFinite(every) && every > 0
            ? t('personalAggregate.execution.everyMinutes', language, { n: every })
            : t('personalAggregate.execution.recurrence', language);
    }
    return trigger.nextRunAt
        ? t('personalAggregate.execution.nextRun', language, { at: fmtDue(trigger.nextRunAt, language) })
        : t('personalAggregate.execution.scheduled', language);
}

function runStatusVariant(status: ExecutionRun['status']): 'success' | 'danger' | 'warning' | 'accent' {
    switch (status) {
        case 'completed':
            return 'success';
        case 'failed':
        case 'cancelled':
            return 'danger';
        case 'running':
            return 'accent';
        default:
            return 'warning';
    }
}

function ExecutionOverview({ language }: { language: Lang }) {
    const [tasks, setTasks] = useState<AggregateWorkItem[]>([]);
    const [taskCount, setTaskCount] = useState(0);
    const [jobs, setJobs] = useState<ExecutionJob[]>([]);
    const [runs, setRuns] = useState<Record<string, ExecutionRun[]>>({});
    const [expandedJob, setExpandedJob] = useState<string | null>(null);
    const [editingJob, setEditingJob] = useState<string | null>(null);
    const [retryMinutes, setRetryMinutes] = useState<Record<string, string>>({});
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [busyJob, setBusyJob] = useState<string | null>(null);

    const load = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            const [aggregate, allJobs] = await Promise.all([
                personalAggregateService.fetch({ limit: 200, sort: 'updated' }),
                executionService.listJobs(),
            ]);
            const executable = aggregate.items.filter(item => item.type === 'task');
            const taskIDs = new Set(executable.map(item => item.id));
            setTasks(executable);
            setTaskCount(aggregate.total);
            setJobs(allJobs.filter(job => taskIDs.has(job.workItemId)));
        } catch (err) {
            setError(String(err));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        void load();
    }, [load]);

    const toggleRuns = useCallback(
        async (jobId: string) => {
            if (expandedJob === jobId) {
                setExpandedJob(null);
                return;
            }
            setExpandedJob(jobId);
            if (runs[jobId]) return;
            try {
                const items = await executionService.listRuns(jobId);
                setRuns(previous => ({ ...previous, [jobId]: items }));
            } catch (err) {
                ui.showToast(t('personalAggregate.execution.runsFailed', language, { err: String(err) }));
            }
        },
        [expandedJob, language, runs]
    );

    const runNow = useCallback(
        async (jobId: string) => {
            setBusyJob(jobId);
            try {
                await executionService.runNow(jobId);
                ui.showToast(t('personalAggregate.execution.runAccepted', language));
                setRuns(previous => {
                    const next = { ...previous };
                    delete next[jobId];
                    return next;
                });
                await load();
            } catch (err) {
                ui.showToast(t('personalAggregate.execution.runFailed', language, { err: String(err) }));
            } finally {
                setBusyJob(null);
            }
        },
        [language, load]
    );

    const scheduleRetry = useCallback(
        async (job: ExecutionJob) => {
            const minutes = Number(retryMinutes[job.id] || '60');
            if (!Number.isInteger(minutes) || minutes < 1) {
                ui.showToast(t('personalAggregate.execution.invalidDelay', language));
                return;
            }
            setBusyJob(job.id);
            try {
                const at = new Date(Date.now() + minutes * 60_000).toISOString();
                await executionService.upsertTrigger(job.id, {
                    kind: 'at',
                    spec: { at },
                    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
                    misfirePolicy: 'run_once',
                    overlapPolicy: 'forbid',
                });
                ui.showToast(t('personalAggregate.execution.retryScheduled', language));
                setEditingJob(null);
                await load();
            } catch (err) {
                ui.showToast(t('personalAggregate.execution.scheduleFailed', language, { err: String(err) }));
            } finally {
                setBusyJob(null);
            }
        },
        [language, load, retryMinutes]
    );

    const taskByID = new Map(tasks.map(task => [task.id, task]));

    return (
        <section class="execution-overview" aria-label={t('personalAggregate.execution.title', language)}>
            <div class="execution-overview-header">
                <div>
                    <h3>{t('personalAggregate.execution.title', language)}</h3>
                    <p>{t('personalAggregate.execution.desc', language)}</p>
                </div>
                <button
                    type="button"
                    class="execution-overview-refresh"
                    onClick={() => void load()}
                    title={t('personalAggregate.refresh', language)}
                    disabled={loading}
                >
                    ⟳
                </button>
            </div>

            <div class="execution-overview-stats">
                <span>
                    <strong>{taskCount}</strong> {t('personalAggregate.execution.tasks', language)}
                </span>
                <span>
                    <strong>{jobs.length}</strong> {t('personalAggregate.execution.jobs', language)}
                </span>
            </div>

            {loading ? (
                <div class="execution-overview-empty">{t('personalAggregate.loading', language)}</div>
            ) : error ? (
                <div class="execution-overview-empty">
                    {t('personalAggregate.execution.loadFailed', language)}
                    <button type="button" onClick={() => void load()}>
                        {t('personalAggregate.retry', language)}
                    </button>
                </div>
            ) : jobs.length === 0 ? (
                <div class="execution-overview-empty">{t('personalAggregate.execution.noJobs', language)}</div>
            ) : (
                <div class="execution-overview-list">
                    {jobs.map(job => {
                        const task = taskByID.get(job.workItemId);
                        const failed = task?.run?.status === 'failed';
                        const isExpanded = expandedJob === job.id;
                        const isEditing = editingJob === job.id;
                        const isBusy = busyJob === job.id;
                        return (
                            <article key={job.id} class={`execution-overview-job${failed ? ' is-failed' : ''}`}>
                                <div class="execution-overview-job-main">
                                    <div>
                                        <span class="execution-overview-job-title">
                                            {task?.title || t('personalAggregate.execution.unknownTask', language)}
                                        </span>
                                        <span class="execution-overview-job-meta">
                                            <InlineBadge variant={failed ? 'danger' : 'accent'}>
                                                {job.executorKind}
                                            </InlineBadge>
                                            <InlineBadge variant={job.status === 'active' ? 'success' : 'muted'}>
                                                {job.status}
                                            </InlineBadge>
                                            <span>{triggerLabel(job, language)}</span>
                                        </span>
                                    </div>
                                    <div class="execution-overview-actions">
                                        <button type="button" onClick={() => void toggleRuns(job.id)}>
                                            {isExpanded
                                                ? t('personalAggregate.execution.hideRuns', language)
                                                : t('personalAggregate.execution.runs', language)}
                                        </button>
                                        {failed && (
                                            <button type="button" disabled={isBusy} onClick={() => void runNow(job.id)}>
                                                {isBusy
                                                    ? t('personalAggregate.execution.working', language)
                                                    : t('personalAggregate.execution.runNow', language)}
                                            </button>
                                        )}
                                        <button
                                            type="button"
                                            disabled={isBusy}
                                            onClick={() => setEditingJob(isEditing ? null : job.id)}
                                        >
                                            {job.trigger
                                                ? t('personalAggregate.execution.editTrigger', language)
                                                : t('personalAggregate.execution.setTrigger', language)}
                                        </button>
                                    </div>
                                </div>

                                {isEditing && (
                                    <div class="execution-overview-retry-form">
                                        <label>
                                            {t('personalAggregate.execution.retryAfter', language)}
                                            <input
                                                type="number"
                                                min="1"
                                                value={retryMinutes[job.id] || '60'}
                                                onInput={event =>
                                                    setRetryMinutes(previous => ({
                                                        ...previous,
                                                        [job.id]: (event.currentTarget as HTMLInputElement).value,
                                                    }))
                                                }
                                            />
                                            {t('personalAggregate.execution.minutes', language)}
                                        </label>
                                        <button type="button" disabled={isBusy} onClick={() => void scheduleRetry(job)}>
                                            {t('personalAggregate.execution.saveTrigger', language)}
                                        </button>
                                    </div>
                                )}

                                {isExpanded && (
                                    <div class="execution-overview-runs">
                                        {!runs[job.id] ? (
                                            <span>{t('personalAggregate.loading', language)}</span>
                                        ) : runs[job.id].length === 0 ? (
                                            <span>{t('personalAggregate.execution.noRuns', language)}</span>
                                        ) : (
                                            runs[job.id].map(run => (
                                                <div key={run.id} class="execution-overview-run">
                                                    <InlineBadge variant={runStatusVariant(run.status)}>
                                                        {run.status}
                                                    </InlineBadge>
                                                    <span>
                                                        {t('personalAggregate.execution.attempt', language, {
                                                            n: run.attempt,
                                                        })}
                                                    </span>
                                                    <span>{fmtDue(run.startedAt, language)}</span>
                                                    {run.errorText && (
                                                        <span class="execution-overview-run-error">
                                                            {run.errorText}
                                                        </span>
                                                    )}
                                                </div>
                                            ))
                                        )}
                                    </div>
                                )}
                            </article>
                        );
                    })}
                </div>
            )}
        </section>
    );
}

// ── row ──────────────────────────────────────────────────────────────────────

interface AggregateRowProps {
    item: AggregateWorkItem;
    language: Lang;
    onOpenTask: () => void;
    onOpenSubject: () => void;
    onFocusCase: () => void;
}

function AggregateRow({ item, language, onOpenTask, onOpenSubject, onFocusCase }: AggregateRowProps) {
    const subject = item.subject;
    const overdue = item.dueAt ? new Date(item.dueAt).getTime() < Date.now() && item.status !== 'completed' : false;

    return (
        <li class="personal-aggregate-row">
            <div
                class="personal-aggregate-row-main"
                role="button"
                tabIndex={0}
                onClick={onOpenTask}
                onKeyDown={e => {
                    if (e.key === 'Enter') onOpenTask();
                }}
            >
                <span class={`sp-dot sp-dot-${statusDot(item.status)}`} aria-hidden="true" />
                <div class="personal-aggregate-row-text">
                    <span class="personal-aggregate-row-title">{item.title}</span>
                    <span class="personal-aggregate-row-meta">
                        {item.workspaceName && <span>{item.workspaceName}</span>}
                        <InlineBadge variant={statusDot(item.status) === 'error' ? 'danger' : 'default'}>
                            {t(statusLabelKey(item.status), language)}
                        </InlineBadge>
                        {item.priority && (
                            <InlineBadge variant={priorityBadgeVariant(item.priority)}>{item.priority}</InlineBadge>
                        )}
                        {item.dueAt && (
                            <InlineBadge variant={overdue ? 'danger' : 'muted'}>
                                {overdue
                                    ? t('personalAggregate.overdue', language)
                                    : t('personalAggregate.due', language)}{' '}
                                {fmtDue(item.dueAt, language)}
                            </InlineBadge>
                        )}
                    </span>
                </div>
            </div>

            <div class="personal-aggregate-row-refs" onClick={e => e.stopPropagation()}>
                {item.caseRef && (
                    <InlineBadge variant="mono" title={item.caseTitle || item.caseRef} onClick={onFocusCase}>
                        {t('personalAggregate.case', language)}
                        {item.caseTitle ? `：${item.caseTitle}` : ''}
                    </InlineBadge>
                )}
                {subject &&
                    (subject.available ? (
                        <InlineBadge variant="accent" title={subject.ref} onClick={onOpenSubject}>
                            {subject.title || subject.ref}
                        </InlineBadge>
                    ) : (
                        <InlineBadge variant="muted" title={`${subject.ref} — ${subject.restrictedReason || ''}`}>
                            🔒 {t('personalAggregate.restricted', language)}
                        </InlineBadge>
                    ))}
            </div>
        </li>
    );
}
