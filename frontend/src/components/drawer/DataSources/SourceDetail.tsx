import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback, useMemo } from 'preact/hooks';
import { useSignal } from '@preact/signals';

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

// 数据详情 (data detail) — a schema-free 多维表格 over one (source, kind)'s raw
// bronze records. Columns are built from whatever native fields the records
// carry; the toolbar (show/hide/reorder columns, group, sort) + search are the
// generic bitable controls, and the column choice persists per (source, kind).
// The leftmost 序号 opens a per-record popup listing every native field.
export function SourceDetail({ source, kind, title }: { source: string; kind: string; title: string }) {
    const language = ui.language.value;
    const [rows, setRows] = useState<SourceRecordRow[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const selected = useSignal<SourceRecordRow | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            setRows(await sourceService.records(source, kind));
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [source, kind]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const columns = useMemo(() => buildSourceColumns(rows, language), [rows, language]);
    const groupOptions = useMemo(() => buildGroupOptions(columns, language), [columns, language]);
    // Remount the grid only when the column SET changes (data first loads / schema
    // shifts) so persisted visibility re-applies; same columns → no remount.
    const colSig = columns.map(c => c.key).join('|');

    const q = search.trim().toLowerCase();
    const filtered = q ? rows.filter(r => searchText(r).includes(q)) : rows;

    return (
        <div class="datasource-detail">
            <div class="datasource-subhead">
                {/* Breadcrumb 数据源 › 飞书 › <title> handles back-navigation — no button here. */}
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
                persistKey={`ds:cols:${source}:${kind}`}
                rows={filtered}
                totalCount={rows.length}
                columns={columns}
                groupOptions={groupOptions}
                getRowKey={r => r.uid}
                loading={loading}
                emptyAll={t('datasource.detailEmpty', language)}
                emptyFiltered={t('contacts.emptyFiltered', language)}
                compare={compareSource}
                defaultCompare={sourceDefaultCompare}
                groupValue={sourceGroupValue}
                rowClass={r => `task-row${r.deleted ? ' ds-row-deleted' : ''}`}
                onOpenRow={r => (selected.value = r)}
                renderCell={(r, col, helpers) => renderSourceCell(r, col, helpers, language)}
            />

            {selected.value && <RecordModal record={selected.value} onClose={() => (selected.value = null)} />}
        </div>
    );
}

// RecordModal lists every native field of one record, in source order (repeats
// included), plus the record envelope + raw payload preview — "逐个展示数据".
function RecordModal({ record, onClose }: { record: SourceRecordRow; onClose: () => void }) {
    const language = ui.language.value;

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [onClose]);

    const meta: Array<[string, string]> = [
        ['UID', record.uid],
        ['ETag', record.etag],
        [t('datasource.col.contentType', language), record.contentType],
        [t('datasource.col.collection', language), record.collection],
        [t('datasource.col.fetchedAt', language), new Date(record.fetchedAt).toLocaleString(language)],
    ];

    return (
        <div class="ds-record-overlay" onClick={onClose}>
            <div class="ds-record-modal" role="dialog" aria-modal="true" onClick={(e: Event) => e.stopPropagation()}>
                <button class="ds-record-close" aria-label={t('contacts.close', language)} onClick={onClose}>
                    ×
                </button>
                <h3 class="ds-record-title">
                    {t('datasource.recordDetail', language)}
                    {record.deleted && <span class="ds-tombstone">{t('datasource.deleted', language)}</span>}
                </h3>

                <div class="ds-record-section">{t('datasource.nativeFields', language)}</div>
                <div class="ds-record-fields">
                    {record.fields.length === 0 && <div class="contacts-empty">—</div>}
                    {record.fields.map((f, i) => (
                        <Fragment key={i}>
                            <div class="ds-record-key">{f.key}</div>
                            <div class="ds-record-val">{f.value}</div>
                        </Fragment>
                    ))}
                </div>

                <div class="ds-record-section">{t('datasource.recordMeta', language)}</div>
                <div class="ds-record-fields">
                    {meta.map(([k, v]) => (
                        <Fragment key={k}>
                            <div class="ds-record-key">{k}</div>
                            <div class="ds-record-val">{v || '—'}</div>
                        </Fragment>
                    ))}
                </div>

                <div class="ds-record-section">{t('datasource.rawPayload', language)}</div>
                <pre class="ds-record-raw">{record.preview}</pre>
            </div>
        </div>
    );
}
