import { h, Fragment } from 'preact';
import { useState, useEffect, useMemo } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import * as wsStore from '../../../stores/workspaceStore';
import { t, type Lang } from '../../../i18n';
import {
    sourceService,
    type GovDAG,
    type GovStep,
    type GovRun,
    type TemplateInfo,
    type SourceRecordRow,
} from '@1agents/core/services/sourceService';
import { AGENT_OPTIONS } from '../TaskList/constants';
import { DataGrid } from '../TaskList/DataGrid';
import { RecordModal } from './SourceDetail';
import {
    buildSourceColumns,
    buildGroupOptions,
    compareSource,
    sourceDefaultCompare,
    sourceGroupValue,
    renderSourceCell,
    searchText,
} from './sourceGrid';

// 数据治理 (governance) — the single view over the whole DAG. Where 数据接入 lands
// raw source data, 数据治理 is the two-tier transform of it: 清洗 (bronze→silver,
// source-faithful) and 融合 (silver→gold, cross-source). The overview is just the
// 治理表 cards (one per output table, grouped from the steps that write it) plus the
// global 全部重跑 / 从模板安装 actions. Clicking a card drills into that table, where
// its data, its dependency graph, and its execution log sit behind a secondary tab.

const GOLD_DOMAIN_TABLES: Record<string, string> = {
    contacts: 'contacts',
    messages: 'messages',
    calendar_events: 'events',
    todos: 'todos',
};

function tableLabel(table: string, language: Lang): string {
    // Only the fused gold entity tables get the friendly domain name (联系人/消息/…).
    // Silver + manifest tables show their real table name, so per-source tables
    // (silver_feishu_users vs silver_icloud_contacts, both domain=contacts) stay
    // distinct rather than collapsing to one label.
    const goldDomain = GOLD_DOMAIN_TABLES[table];
    if (goldDomain) {
        const key = `datasource.gold.domain.${goldDomain}`;
        const val = t(key, language);
        if (val !== key) return val;
    }
    return table;
}

function fmtTime(iso: string, language: Lang): string {
    if (!iso) return '';
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(language);
}

// One card = one output table, aggregating the steps that write it.
interface TableGroup {
    output: string;
    domain?: string;
    tier: string;
    steps: GovStep[];
    lastRun?: GovRun;
}

function groupByOutput(steps: GovStep[]): TableGroup[] {
    const order: string[] = [];
    const byOut = new Map<string, TableGroup>();
    for (const s of steps) {
        if (!s.output) continue;
        let g = byOut.get(s.output);
        if (!g) {
            g = { output: s.output, domain: s.domain, tier: s.tier, steps: [] };
            byOut.set(s.output, g);
            order.push(s.output);
        }
        g.steps.push(s);
        if (s.lastRun && (!g.lastRun || s.lastRun.ranAt > g.lastRun.ranAt)) g.lastRun = s.lastRun;
    }
    return order.map(o => byOut.get(o)!);
}

function StatusDot({ run, language }: { run?: GovRun; language: Lang }) {
    if (!run) return <span class="gov-dot gov-dot-idle" title={t('datasource.gov.neverRun', language)} />;
    const ok = run.status === 'success';
    return (
        <span
            class={`gov-dot ${ok ? 'gov-dot-ok' : 'gov-dot-fail'}`}
            title={`${ok ? t('datasource.gov.statusSuccess', language) : t('datasource.gov.statusFailed', language)} · ${fmtTime(run.ranAt, language)}${run.error ? ` · ${run.error}` : ''}`}
        />
    );
}

// GovernanceZone — the governance overview: just the 治理表 cards. The dependency
// graph + execution log now live inside each table's detail; the global 全部重跑 /
// 从模板安装 actions sit in the 治理表 header.
export function GovernanceZone({
    onOpenTable,
}: {
    onOpenTable: (table: string, title: string, domain?: string) => void;
}) {
    const language = ui.language.value;
    const [dag, setDag] = useState<GovDAG | null>(null);
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const [showTemplates, setShowTemplates] = useState(false);

    const load = () => {
        sourceService
            .governance()
            .then(setDag)
            .catch(e => setError((e as Error).message));
    };
    useEffect(load, []);

    const runAll = async () => {
        setBusy(true);
        try {
            await sourceService.runGovernance();
            load();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    const groups = useMemo(() => (dag ? groupByOutput(dag.steps) : []), [dag]);

    return (
        <div class="datasource-detail gov-view">
            {error && <div class="contacts-error">{error}</div>}
            {dag !== null && groups.length === 0 && !error && (
                <div class="contacts-empty">{t('datasource.gov.empty', language)}</div>
            )}
            {groups.length > 0 && (
                <div class="fscard-zone">
                    <div class="fscard-zone-title gov-zone-head">
                        <span>{t('datasource.gov.tablesTitle', language)}</span>
                        <span class="gov-zone-actions">
                            <button class="gov-btn gov-btn-primary" disabled={busy} onClick={runAll}>
                                {busy ? t('datasource.gov.running', language) : t('datasource.gov.runAll', language)}
                            </button>
                            <button class="gov-btn" onClick={() => setShowTemplates(true)}>
                                {t('datasource.gov.templates', language)}
                            </button>
                        </span>
                    </div>
                    <div class="bento-grid fscard-data-grid">
                        {groups.map(g => {
                            const title = tableLabel(g.output, language);
                            return (
                                <button
                                    key={g.output}
                                    class="bento-card gov-card gov-card-btn"
                                    onClick={() => onOpenTable(g.output, title, g.domain)}
                                >
                                    <span class="gov-card-head">
                                        <span class="bento-card-title">{title}</span>
                                        <span class={`gov-tier gov-tier-${g.tier}`}>
                                            {t(`datasource.gov.tier.${g.tier}`, language)}
                                        </span>
                                    </span>
                                    <span class="gov-card-langs">
                                        {g.steps.map(s => (
                                            <span key={s.name} class={`gov-lang gov-lang-${s.lang}`}>
                                                {t(`datasource.gov.lang.${s.lang}`, language)}
                                            </span>
                                        ))}
                                    </span>
                                    <span class="gov-card-foot">
                                        <span class="gov-card-run">
                                            <StatusDot run={g.lastRun} language={language} />
                                            <span class="gov-card-time">
                                                {g.lastRun
                                                    ? `${t('datasource.gov.rows', language, { n: g.lastRun.rows })} · ${fmtTime(g.lastRun.ranAt, language)}`
                                                    : t('datasource.gov.neverRun', language)}
                                            </span>
                                        </span>
                                        <span class="fscard-data-open">{t('datasource.data.open', language)} →</span>
                                    </span>
                                </button>
                            );
                        })}
                    </div>
                </div>
            )}

            {showTemplates && <TemplatesModal onClose={() => setShowTemplates(false)} onInstalled={load} />}
        </div>
    );
}

// ExecutionLog renders a step-run history (used per-table in the detail).
function ExecutionLog({ runs, language }: { runs: GovRun[]; language: Lang }) {
    if (runs.length === 0) return <div class="contacts-empty">{t('datasource.gov.neverRun', language)}</div>;
    return (
        <div class="datasource-runs-table gov-log-table">
            {runs.map((r, i) => (
                <div key={`${r.step}-${r.ranAt}-${i}`} class="gov-log-row">
                    <StatusDot run={r} language={language} />
                    <span class="gov-log-step">{r.step}</span>
                    <span class={`gov-lang gov-lang-${r.lang}`}>{t(`datasource.gov.lang.${r.lang}`, language)}</span>
                    <span class="gov-log-rows">{t('datasource.gov.rows', language, { n: r.rows })}</span>
                    <span class="gov-log-dur">{r.durationMs}ms</span>
                    <span class="gov-log-time">{fmtTime(r.ranAt, language)}</span>
                    {r.error && <span class="gov-log-err">{r.error}</span>}
                </div>
            ))}
        </div>
    );
}

// TableDeps — one table's dependency view: upstreams → this table → downstreams,
// plus the governing steps with per-step re-run / rebuild.
function TableDeps({
    title,
    upstreams,
    downstreams,
    steps,
    busy,
    onRerun,
    language,
}: {
    title: string;
    upstreams: string[];
    downstreams: string[];
    steps: GovStep[];
    busy: string;
    onRerun: (step: string, rebuild: boolean) => void;
    language: Lang;
}) {
    const running = busy !== '';
    const col = (titleKey: string, tables: string[]) => (
        <div class="gov-deps-col">
            <div class="gov-deps-col-title">{t(titleKey, language)}</div>
            {tables.length > 0 ? (
                tables.map(x => (
                    <div key={x} class="gov-dag-node">
                        {x.replace(/^bronze:/, '')}
                    </div>
                ))
            ) : (
                <div class="gov-deps-none">—</div>
            )}
        </div>
    );
    return (
        <div class="gov-detail-body">
            <div class="fscard-zone">
                <div class="fscard-zone-title">{t('datasource.gov.dagTitle', language)}</div>
                <div class="gov-deps-flow">
                    {col('datasource.gov.upstreams', upstreams)}
                    <div class="gov-deps-arrow">→</div>
                    <div class="gov-deps-col">
                        <div class="gov-deps-col-title">{t('datasource.gov.thisTable', language)}</div>
                        <div class="gov-dag-node gov-dag-node-self">{title}</div>
                    </div>
                    <div class="gov-deps-arrow">→</div>
                    {col('datasource.gov.downstream', downstreams)}
                </div>
            </div>
            <div class="fscard-zone">
                <div class="fscard-zone-title">{t('datasource.gov.steps', language)}</div>
                <div class="gov-steps-list">
                    {steps.map(s => (
                        <div key={s.name} class="gov-step-row">
                            <StatusDot run={s.lastRun} language={language} />
                            <span class="gov-log-step">{s.name}</span>
                            <span class={`gov-lang gov-lang-${s.lang}`}>
                                {t(`datasource.gov.lang.${s.lang}`, language)}
                            </span>
                            <span class="gov-step-actions">
                                <button class="gov-mini-btn" disabled={running} onClick={() => onRerun(s.name, false)}>
                                    {busy === s.name ? '…' : t('datasource.gov.rerun', language)}
                                </button>
                                {(s.lang === 'sql' || s.lang === 'python') && (
                                    <button
                                        class="gov-mini-btn gov-mini-danger"
                                        disabled={running}
                                        onClick={() => onRerun(s.name, true)}
                                    >
                                        {t('datasource.gov.rebuild', language)}
                                    </button>
                                )}
                            </span>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}

// TemplatesModal lists the embedded connector + governance templates and installs
// one (hot-loaded) on click — the 从模板一键安装 entry (#408).
function TemplatesModal({ onClose, onInstalled }: { onClose: () => void; onInstalled: () => void }) {
    const language = ui.language.value;
    const [list, setList] = useState<TemplateInfo[] | null>(null);
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');

    useEffect(() => {
        sourceService
            .templates()
            .then(setList)
            .catch(e => setError((e as Error).message));
    }, []);

    const install = async (ti: TemplateInfo) => {
        setBusy(ti.id);
        setError('');
        try {
            const done = await sourceService.installTemplate(ti.id);
            ui.showToast(t('datasource.gov.installOk', language, { name: done.label }));
            onInstalled();
            const fresh = await sourceService.templates();
            setList(fresh);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy('');
        }
    };

    return (
        <div class="ds-record-overlay" onClick={onClose}>
            <div class="ds-record-modal" role="dialog" aria-modal="true" onClick={(e: Event) => e.stopPropagation()}>
                <button class="ds-record-close" aria-label={t('contacts.close', language)} onClick={onClose}>
                    ×
                </button>
                <h3 class="ds-record-title">{t('datasource.gov.templatesTitle', language)}</h3>
                {error && <div class="contacts-error">{error}</div>}
                {list !== null && list.length === 0 && (
                    <div class="contacts-empty">{t('datasource.gov.templatesEmpty', language)}</div>
                )}
                <div class="gov-tpl-list">
                    {(list ?? []).map(ti => (
                        <div key={ti.id} class="gov-tpl-row">
                            <div class="gov-tpl-info">
                                <span class="gov-tpl-name">{ti.label}</span>
                                <span class="gov-tpl-kind">
                                    {ti.kind === 'connector'
                                        ? t('datasource.gov.tplConnector', language)
                                        : t('datasource.gov.tplGovernance', language)}
                                </span>
                            </div>
                            <button
                                class="gov-btn"
                                disabled={ti.installed || busy === ti.id}
                                onClick={() => install(ti)}
                            >
                                {ti.installed
                                    ? t('datasource.gov.installed', language)
                                    : busy === ti.id
                                      ? t('datasource.gov.installing', language)
                                      : t('datasource.gov.install', language)}
                            </button>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}

// GovernanceTableDetail — one governed output table drilled in, behind a secondary
// nav: 数据 (schema-free grid) · 依赖关系 (this table's upstreams/downstreams + its
// governing steps with re-run/rebuild) · 执行日志 (this table's run history).
export function GovernanceTableDetail({ table, title, domain }: { table: string; title: string; domain?: string }) {
    const language = ui.language.value;
    const [tab, setTab] = useState<'data' | 'deps' | 'log'>('data');
    const [rows, setRows] = useState<SourceRecordRow[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const selected = useSignal<SourceRecordRow | null>(null);
    const [promoting, setPromoting] = useState<{ id: string; title: string } | null>(null);
    const [steps, setSteps] = useState<GovStep[]>([]);
    const [upstreams, setUpstreams] = useState<string[]>([]);
    const [downstreams, setDownstreams] = useState<string[]>([]);
    const [runs, setRuns] = useState<GovRun[]>([]);
    const [busy, setBusy] = useState('');
    const isTodos = domain === 'todos' || table === 'todos';

    const loadData = () => {
        setLoading(true);
        setError('');
        sourceService
            .governanceTable(table, 1000)
            .then(setRows)
            .catch(e => setError((e as Error).message))
            .finally(() => setLoading(false));
    };
    const loadMeta = () => {
        sourceService
            .governance()
            .then(dag => {
                const mine = dag.steps.filter(s => s.output === table);
                setSteps(mine);
                setUpstreams([...new Set(mine.flatMap(s => s.upstreams))]);
                setDownstreams([...new Set(dag.edges.filter(e => e.from === table).map(e => e.to))]);
            })
            .catch(() => {});
        sourceService
            .governanceRuns('', 200)
            .then(all => setRuns(all.filter(r => r.outputTable === table)))
            .catch(() => {});
    };
    useEffect(() => {
        loadData();
        loadMeta();
    }, [table]);

    const rerun = async (step: string, rebuild: boolean) => {
        if (rebuild && !window.confirm(t('datasource.gov.rebuildConfirm', language))) return;
        setBusy(step + (rebuild ? ':rebuild' : ''));
        try {
            await sourceService.runGovernance(step, rebuild);
            loadData();
            loadMeta();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy('');
        }
    };

    const columns = useMemo(() => buildSourceColumns(rows, language), [rows, language]);
    const groupOptions = useMemo(() => buildGroupOptions(columns, language), [columns, language]);
    const colSig = columns.map(c => c.key).join('|');
    const q = search.trim().toLowerCase();
    const filtered = q ? rows.filter(r => searchText(r).includes(q)) : rows;

    const tabs: { id: 'data' | 'deps' | 'log'; label: string }[] = [
        { id: 'data', label: t('datasource.gov.tabData', language) },
        { id: 'deps', label: t('datasource.gov.dagTitle', language) },
        { id: 'log', label: t('datasource.gov.logTitle', language) },
    ];

    return (
        <div class="datasource-detail">
            <div class="datasource-subhead">
                <span class="datasource-subhead-title">{title}</span>
                {tab === 'data' && (
                    <Fragment>
                        <span class="datasource-subhead-count">
                            {t('datasource.records', language, { n: rows.length })}
                        </span>
                        <input
                            class="contacts-search datasource-search"
                            placeholder={t('contacts.searchPlaceholder', language)}
                            value={search}
                            onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                        />
                    </Fragment>
                )}
            </div>

            <div class="datasource-subnav gov-detail-tabs">
                {tabs.map(tb => (
                    <button
                        key={tb.id}
                        class={`datasource-subnav-tab${tab === tb.id ? ' is-active' : ''}`}
                        onClick={() => setTab(tb.id)}
                    >
                        {tb.label}
                    </button>
                ))}
            </div>

            {error && <div class="contacts-error">{error}</div>}

            {tab === 'data' && (
                <Fragment>
                    <DataGrid<SourceRecordRow>
                        key={colSig}
                        persistKey={`gov:cols:${table}`}
                        rows={filtered}
                        totalCount={rows.length}
                        columns={columns}
                        groupOptions={groupOptions}
                        getRowKey={r => `${r.collection}:${r.uid}`}
                        loading={loading}
                        emptyAll={t('datasource.gold.empty', language)}
                        emptyFiltered={t('contacts.emptyFiltered', language)}
                        compare={compareSource}
                        defaultCompare={sourceDefaultCompare}
                        groupValue={sourceGroupValue}
                        rowClass={r => `task-row${r.deleted ? ' ds-row-deleted' : ''}`}
                        onOpenRow={r => (selected.value = r)}
                        renderCell={(r, col, helpers) => renderSourceCell(r, col, helpers, language)}
                    />

                    {selected.value && (
                        <RecordModal
                            record={selected.value}
                            onClose={() => (selected.value = null)}
                            onPromote={
                                isTodos
                                    ? () => {
                                          const r = selected.value!;
                                          setPromoting({
                                              id: r.uid,
                                              title: (r.preview || r.uid).split('\n')[0].slice(0, 80),
                                          });
                                          selected.value = null;
                                      }
                                    : undefined
                            }
                        />
                    )}

                    {promoting && (
                        <PromoteTodoDialog
                            todo={promoting}
                            onClose={() => setPromoting(null)}
                            onDone={msg => {
                                ui.showToast(msg);
                                setPromoting(null);
                            }}
                        />
                    )}
                </Fragment>
            )}

            {tab === 'deps' && (
                <TableDeps
                    title={title}
                    upstreams={upstreams}
                    downstreams={downstreams}
                    steps={steps}
                    busy={busy}
                    onRerun={rerun}
                    language={language}
                />
            )}

            {tab === 'log' && <ExecutionLog runs={runs} language={language} />}
        </div>
    );
}

// PromoteTodoDialog — pick a target workspace + executor, then turn a fused to-do
// into a task via POST /api/data/gold/todos/promote.
function PromoteTodoDialog({
    todo,
    onClose,
    onDone,
}: {
    todo: { id: string; title: string };
    onClose: () => void;
    onDone: (msg: string) => void;
}) {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const [workspaceId, setWorkspaceId] = useState(workspaces[0]?.id ?? '');
    const [assignee, setAssignee] = useState('user');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    const submit = async () => {
        if (!workspaceId) {
            setError(t('datasource.gold.promoteNoWorkspace', language));
            return;
        }
        setBusy(true);
        setError('');
        try {
            const res = await sourceService.promoteTodo({ id: todo.id, workspaceId, assignee });
            onDone(
                res.alreadyLinked
                    ? t('datasource.gold.promoteAlready', language)
                    : t('datasource.gold.promoteOk', language)
            );
            onClose();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <div class="ds-record-overlay" onClick={onClose}>
            <div
                class="ds-record-modal ds-promote-modal"
                role="dialog"
                aria-modal="true"
                onClick={(e: Event) => e.stopPropagation()}
            >
                <button class="ds-record-close" aria-label={t('contacts.close', language)} onClick={onClose}>
                    ×
                </button>
                <h3 class="ds-record-title">{t('datasource.gold.promoteTitle', language)}</h3>
                <div class="ds-promote-todo-title">{todo.title}</div>

                <div class="form-group">
                    <label>{t('datasource.gold.promoteWorkspace', language)}</label>
                    <select
                        value={workspaceId}
                        onChange={(e: Event) => setWorkspaceId((e.target as HTMLSelectElement).value)}
                    >
                        {workspaces.length === 0 && <option value="">—</option>}
                        {workspaces.map(w => (
                            <option key={w.id} value={w.id}>
                                {w.name}
                            </option>
                        ))}
                    </select>
                </div>

                <div class="form-group">
                    <label>{t('datasource.gold.promoteAssignee', language)}</label>
                    <select
                        value={assignee}
                        onChange={(e: Event) => setAssignee((e.target as HTMLSelectElement).value)}
                    >
                        <option value="user">{t('datasource.gold.promoteAsUser', language)}</option>
                        {AGENT_OPTIONS.map(a => (
                            <option key={a} value={a}>
                                {a}
                            </option>
                        ))}
                    </select>
                </div>

                {error && <div class="contacts-error">{error}</div>}
                <div class="ds-promote-actions">
                    <button class="submit-task-btn" disabled={busy} onClick={submit}>
                        {busy ? t('datasource.gold.promoting', language) : t('datasource.gold.promote', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}
