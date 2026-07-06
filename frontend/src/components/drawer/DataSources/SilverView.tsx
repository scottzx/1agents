import { h } from 'preact';
import { useState, useEffect, useCallback, useMemo } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { sourceService, type SourceRecordRow, type SilverSummary } from '@1agents/core/services/sourceService';
import { DataGrid } from '../TaskList/DataGrid';
import {
    buildSourceColumns,
    buildGroupOptions,
    compareSource,
    sourceDefaultCompare,
    sourceGroupValue,
    renderSourceCell,
    searchText,
} from './sourceGrid';

// 数据归一 (silver) — the cross-source, conformed four-domain view. Unlike the
// source-centric bronze browser, silver is domain-oriented: pick a domain
// (联系人/消息/日历/待办) and see every source's rows together in one 多维表格,
// filterable by source. Reuses the bronze grid wholesale (silver rows share the
// SourceRecordRow envelope). "重新清洗" re-runs bronze→silver on demand.

const DOMAINS = ['contacts', 'messages', 'events', 'todos'] as const;
type Domain = (typeof DOMAINS)[number];

export function SilverView() {
    const language = ui.language.value;
    const [domain, setDomain] = useState<Domain>('messages');
    const [source, setSource] = useState(''); // '' = 全部来源
    const [rows, setRows] = useState<SourceRecordRow[]>([]);
    const [summary, setSummary] = useState<SilverSummary[]>([]);
    const [loading, setLoading] = useState(false);
    const [rerunning, setRerunning] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');

    const loadSummary = useCallback(() => {
        sourceService
            .silverSummary()
            .then(setSummary)
            .catch(() => setSummary([]));
    }, []);
    useEffect(loadSummary, [loadSummary]);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            setRows(await sourceService.silverRecords(domain, source, 1000));
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [domain, source]);
    useEffect(() => {
        refresh();
    }, [refresh]);

    // A domain switch clears the source filter (sources differ per domain).
    const pickDomain = (d: Domain) => {
        setDomain(d);
        setSource('');
    };

    const rerun = async () => {
        setRerunning(true);
        try {
            await sourceService.runSilver();
            loadSummary();
            await refresh();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setRerunning(false);
        }
    };

    // Sources available for the active domain (for the filter dropdown) + total.
    const domainRollup = summary.filter(s => s.domain === domain);
    const domainSources = [...new Set(domainRollup.map(s => s.source))].sort();
    const domainTotal = domainRollup.reduce((n, s) => n + s.count, 0);
    const countFor = (d: Domain) => summary.filter(s => s.domain === d).reduce((n, s) => n + s.count, 0);

    const columns = useMemo(() => buildSourceColumns(rows, language), [rows, language]);
    const groupOptions = useMemo(() => buildGroupOptions(columns, language), [columns, language]);
    const colSig = columns.map(c => c.key).join('|');

    const q = search.trim().toLowerCase();
    const filtered = q ? rows.filter(r => searchText(r).includes(q)) : rows;

    return (
        <div class="datasource-detail">
            <div class="silver-tabs">
                {DOMAINS.map(d => (
                    <button key={d} class={`silver-tab${d === domain ? ' active' : ''}`} onClick={() => pickDomain(d)}>
                        {t(`datasource.silver.domain.${d}`, language)}
                        <span class="silver-tab-count">{countFor(d)}</span>
                    </button>
                ))}
            </div>

            <div class="datasource-subhead">
                <span class="datasource-subhead-count">{t('datasource.records', language, { n: domainTotal })}</span>
                <select
                    class="silver-source-filter"
                    value={source}
                    onChange={(e: Event) => setSource((e.target as HTMLSelectElement).value)}
                >
                    <option value="">{t('datasource.silver.allSources', language)}</option>
                    {domainSources.map(s => (
                        <option key={s} value={s}>
                            {s}
                        </option>
                    ))}
                </select>
                <input
                    class="contacts-search datasource-search"
                    placeholder={t('contacts.searchPlaceholder', language)}
                    value={search}
                    onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                />
                <button class="silver-rerun-btn" disabled={rerunning} onClick={rerun}>
                    {rerunning ? t('datasource.silver.rerunning', language) : t('datasource.silver.rerun', language)}
                </button>
            </div>
            {error && <div class="contacts-error">{error}</div>}

            <DataGrid<SourceRecordRow>
                key={colSig}
                persistKey={`silver:cols:${domain}`}
                rows={filtered}
                totalCount={rows.length}
                columns={columns}
                groupOptions={groupOptions}
                getRowKey={r => `${r.collection}:${r.uid}`}
                loading={loading}
                emptyAll={t('datasource.silver.empty', language)}
                emptyFiltered={t('contacts.emptyFiltered', language)}
                compare={compareSource}
                defaultCompare={sourceDefaultCompare}
                groupValue={sourceGroupValue}
                rowClass={r => `task-row${r.deleted ? ' ds-row-deleted' : ''}`}
                renderCell={(r, col, helpers) => renderSourceCell(r, col, helpers, language)}
            />
        </div>
    );
}
