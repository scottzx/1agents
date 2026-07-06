import { h } from 'preact';
import { useState, useEffect, useCallback, useMemo } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { sourceService, type SourceRecordRow } from '@1agents/core/services/sourceService';
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

// 已治理数据详情 (silver detail) — the cleaned 多维表格 for ONE (source, domain).
// Silver is single-table governance: one bronze table → one cleaning scheme,
// re-run incrementally after that source's scheduled sync. Rows share the bronze
// SourceRecordRow envelope, so the whole grid (sourceGrid + DataGrid) is reused.
// "重新治理" re-runs bronze→silver on demand (global, cursor-gated = cheap).
export function SilverDetail({ domain, source, title }: { domain: string; source: string; title: string }) {
    const language = ui.language.value;
    const [rows, setRows] = useState<SourceRecordRow[]>([]);
    const [loading, setLoading] = useState(false);
    const [rerunning, setRerunning] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');

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

    const rerun = async () => {
        setRerunning(true);
        try {
            await sourceService.runSilver();
            await refresh();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setRerunning(false);
        }
    };

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
                <button class="silver-rerun-btn" disabled={rerunning} onClick={rerun}>
                    {rerunning ? t('datasource.silver.rerunning', language) : t('datasource.silver.rerun', language)}
                </button>
            </div>
            {error && <div class="contacts-error">{error}</div>}

            <DataGrid<SourceRecordRow>
                key={colSig}
                persistKey={`silver:cols:${source}:${domain}`}
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
