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
// source-faithful) and 融合 (silver→gold, cross-source). Every governed table —
// built-in Go, declarative SQL, Python script — shows as a card grouped by its
// output table, with the dependency graph (依赖关系) and the run log (执行日志)
// alongside. This replaces the old per-source 已治理 tab + the separate 数据融合
// layer: one place, table-centric, with re-run + rebuild + template install.

const GOLD_DOMAIN_TABLES: Record<string, string> = {
    contacts: 'contacts',
    messages: 'messages',
    calendar_events: 'events',
    todos: 'todos',
};

function tableLabel(table: string, domain: string | undefined, language: Lang): string {
    const d = domain || GOLD_DOMAIN_TABLES[table];
    if (d) {
        const key = `datasource.gold.domain.${d}`;
        const val = t(key, language);
        if (val !== key) return val;
    }
    return table; // silver_* / manifest outputs: the technical name is the honest label
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
        // keep the most recent lastRun across the group
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

// GovernanceZone — the governance overview: table cards + dependency board + log.
export function GovernanceZone({
    onOpenTable,
}: {
    onOpenTable: (table: string, title: string, domain?: string) => void;
}) {
    const language = ui.language.value;
    const [dag, setDag] = useState<GovDAG | null>(null);
    const [runs, setRuns] = useState<GovRun[]>([]);
    const [error, setError] = useState('');
    const [busy, setBusy] = useState('');
    const [showTemplates, setShowTemplates] = useState(false);

    const load = () => {
        sourceService
            .governance()
            .then(setDag)
            .catch(e => setError((e as Error).message));
        sourceService
            .governanceRuns('', 100)
            .then(setRuns)
            .catch(() => {});
    };
    useEffect(load, []);

    const runAll = async () => {
        setBusy('all');
        try {
            await sourceService.runGovernance();
            load();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy('');
        }
    };

    const rerun = async (step: string, rebuild: boolean) => {
        if (rebuild && !window.confirm(t('datasource.gov.rebuildConfirm', language))) return;
        setBusy(step + (rebuild ? ':rebuild' : ''));
        try {
            await sourceService.runGovernance(step, rebuild);
            load();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy('');
        }
    };

    const groups = useMemo(() => (dag ? groupByOutput(dag.steps) : []), [dag]);
    const running = busy !== '';

    return (
        <div class="datasource-detail gov-view">
            <div class="gov-toolbar">
                <button class="gov-btn gov-btn-primary" disabled={running} onClick={runAll}>
                    {busy === 'all' ? t('datasource.gov.running', language) : t('datasource.gov.runAll', language)}
                </button>
                <button class="gov-btn" onClick={() => setShowTemplates(true)}>
                    {t('datasource.gov.templates', language)}
                </button>
            </div>
            {error && <div class="contacts-error">{error}</div>}

            {dag !== null && groups.length === 0 && !error && (
                <div class="contacts-empty">{t('datasource.gov.empty', language)}</div>
            )}

            {groups.length > 0 && (
                <Fragment>
                    {/* 治理表卡片 */}
                    <div class="fscard-zone">
                        <div class="fscard-zone-title">{t('datasource.gov.tablesTitle', language)}</div>
                        <div class="bento-grid fscard-data-grid">
                            {groups.map(g => {
                                const title = tableLabel(g.output, g.domain, language);
                                const upstreams = [...new Set(g.steps.flatMap(s => s.upstreams))];
                                return (
                                    <div key={g.output} class="bento-card gov-card">
                                        <button
                                            class="gov-card-head"
                                            onClick={() => onOpenTable(g.output, title, g.domain)}
                                        >
                                            <span class="bento-card-title">{title}</span>
                                            <span class={`gov-tier gov-tier-${g.tier}`}>
                                                {t(`datasource.gov.tier.${g.tier}`, language)}
                                            </span>
                                        </button>
                                        <div class="gov-card-langs">
                                            {g.steps.map(s => (
                                                <span key={s.name} class={`gov-lang gov-lang-${s.lang}`}>
                                                    {t(`datasource.gov.lang.${s.lang}`, language)}
                                                </span>
                                            ))}
                                        </div>
                                        {upstreams.length > 0 && (
                                            <div class="gov-upstreams">
                                                <span class="gov-upstreams-label">
                                                    {t('datasource.gov.upstreams', language)}
                                                </span>
                                                {upstreams.map(u => (
                                                    <span key={u} class="gov-chip">
                                                        {u.replace(/^bronze:/, '')}
                                                    </span>
                                                ))}
                                            </div>
                                        )}
                                        <div class="gov-card-foot">
                                            <span class="gov-card-run">
                                                <StatusDot run={g.lastRun} language={language} />
                                                <span class="gov-card-time">
                                                    {g.lastRun
                                                        ? `${t('datasource.gov.rows', language, { n: g.lastRun.rows })} · ${fmtTime(g.lastRun.ranAt, language)}`
                                                        : t('datasource.gov.neverRun', language)}
                                                </span>
                                            </span>
                                            <span class="gov-card-actions">
                                                {g.steps.map(s => (
                                                    <button
                                                        key={s.name}
                                                        class="gov-mini-btn"
                                                        disabled={running}
                                                        title={s.name}
                                                        onClick={() => rerun(s.name, false)}
                                                    >
                                                        {busy === s.name ? '…' : t('datasource.gov.rerun', language)}
                                                    </button>
                                                ))}
                                                {/* rebuild only for declarative steps (safe to truncate) */}
                                                {g.steps.some(s => s.lang === 'sql' || s.lang === 'python') &&
                                                    g.steps.length === 1 && (
                                                        <button
                                                            class="gov-mini-btn gov-mini-danger"
                                                            disabled={running}
                                                            onClick={() => rerun(g.steps[0].name, true)}
                                                        >
                                                            {t('datasource.gov.rebuild', language)}
                                                        </button>
                                                    )}
                                            </span>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    </div>

                    {/* 依赖关系(DAG,按 medallion 层分列) */}
                    <DependencyBoard dag={dag!} language={language} />

                    {/* 执行日志 */}
                    <ExecutionLog runs={runs} language={language} />
                </Fragment>
            )}

            {showTemplates && <TemplatesModal onClose={() => setShowTemplates(false)} onInstalled={load} />}
        </div>
    );
}

// DependencyBoard lays the DAG out as three medallion columns (接入 → 清洗 → 融合),
// each node showing which upstreams feed it — a readable dependency view without a
// full graph layout engine.
function DependencyBoard({ dag, language }: { dag: GovDAG; language: Lang }) {
    const layers: Array<'bronze' | 'silver' | 'gold'> = ['bronze', 'silver', 'gold'];
    const byLayer = (layer: string) => dag.nodes.filter(n => n.layer === layer);
    const upstreamsOf = (table: string) => dag.edges.filter(e => e.to === table).map(e => e.from);
    return (
        <div class="fscard-zone">
            <div class="fscard-zone-title">{t('datasource.gov.dagTitle', language)}</div>
            <div class="gov-dag">
                {layers.map(layer => {
                    const nodes = byLayer(layer);
                    if (nodes.length === 0) return null;
                    return (
                        <div key={layer} class="gov-dag-col">
                            <div class="gov-dag-col-title">{t(`datasource.gov.layer.${layer}`, language)}</div>
                            {nodes.map(n => (
                                <div key={n.table} class={`gov-dag-node gov-dag-node-${n.layer}`}>
                                    <span class="gov-dag-node-name">{n.table.replace(/^bronze:/, '')}</span>
                                    {layer !== 'bronze' && upstreamsOf(n.table).length > 0 && (
                                        <span class="gov-dag-node-up">
                                            ←{' '}
                                            {upstreamsOf(n.table)
                                                .map(u => u.replace(/^bronze:/, ''))
                                                .join(', ')}
                                        </span>
                                    )}
                                </div>
                            ))}
                        </div>
                    );
                })}
            </div>
        </div>
    );
}

// ExecutionLog renders the recent step-run history.
function ExecutionLog({ runs, language }: { runs: GovRun[]; language: Lang }) {
    return (
        <div class="fscard-zone">
            <div class="fscard-zone-title">{t('datasource.gov.logTitle', language)}</div>
            {runs.length === 0 ? (
                <div class="contacts-empty">{t('datasource.gov.neverRun', language)}</div>
            ) : (
                <div class="datasource-runs-table gov-log-table">
                    {runs.map((r, i) => (
                        <div key={`${r.step}-${r.ranAt}-${i}`} class="gov-log-row">
                            <StatusDot run={r} language={language} />
                            <span class="gov-log-step">{r.step}</span>
                            <span class={`gov-lang gov-lang-${r.lang}`}>
                                {t(`datasource.gov.lang.${r.lang}`, language)}
                            </span>
                            <span class="gov-log-rows">{t('datasource.gov.rows', language, { n: r.rows })}</span>
                            <span class="gov-log-dur">{r.durationMs}ms</span>
                            <span class="gov-log-time">{fmtTime(r.ranAt, language)}</span>
                            {r.error && <span class="gov-log-err">{r.error}</span>}
                        </div>
                    ))}
                </div>
            )}
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

// GovernanceTableDetail — one governed output table's 多维表格 (schema-free grid),
// read by physical table name. Preserves the todos → task promote affordance.
export function GovernanceTableDetail({ table, title, domain }: { table: string; title: string; domain?: string }) {
    const language = ui.language.value;
    const [rows, setRows] = useState<SourceRecordRow[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const selected = useSignal<SourceRecordRow | null>(null);
    const [promoting, setPromoting] = useState<{ id: string; title: string } | null>(null);
    const isTodos = domain === 'todos' || table === 'todos';

    useEffect(() => {
        let active = true;
        setLoading(true);
        setError('');
        sourceService
            .governanceTable(table, 1000)
            .then(r => active && setRows(r))
            .catch(e => active && setError((e as Error).message))
            .finally(() => active && setLoading(false));
        return () => {
            active = false;
        };
    }, [table]);

    const columns = useMemo(() => buildSourceColumns(rows, language), [rows, language]);
    const groupOptions = useMemo(() => buildGroupOptions(columns, language), [columns, language]);
    const colSig = columns.map(c => c.key).join('|');

    const q = search.trim().toLowerCase();
    const filtered = q ? rows.filter(r => searchText(r).includes(q)) : rows;

    return (
        <div class="datasource-detail">
            <div class="datasource-subhead">
                <span class="datasource-subhead-title">{title}</span>
                <span class="datasource-subhead-count">{t('datasource.records', language, { n: rows.length })}</span>
                <input
                    class="contacts-search datasource-search"
                    placeholder={t('contacts.searchPlaceholder', language)}
                    value={search}
                    onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                />
            </div>
            {error && <div class="contacts-error">{error}</div>}

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
                                  setPromoting({ id: r.uid, title: (r.preview || r.uid).split('\n')[0].slice(0, 80) });
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
