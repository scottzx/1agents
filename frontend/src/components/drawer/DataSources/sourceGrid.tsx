import { h } from 'preact';
import type { VNode } from 'preact';

import { t, type Lang } from '../../../i18n';
import type { SourceRecordRow } from '@1agents/core/services/sourceService';
import type { CellHelpers, GridColumn } from '../TaskList/DataGrid';

// 数据源多维表格 (data-source DataGrid) config — schema-FREE. Columns are derived
// from the native field keys that actually appear in the records (vCard FN/TEL/…
// today, iCal/JSON later), so nothing here is hardcoded per data type. Two fixed
// columns bracket them: a leftmost 序号 that opens the per-record popup, and a
// trailing 同步时间. Governance (elsewhere) is where fields get normalized; this
// viewer stays faithful to the raw record.

const SEQ = '__seq';
const FETCHED = '__fetchedAt';
const FKEY = 'f:'; // dynamic native-field column prefix

function fmtTime(ms: number, lang: Lang): string {
    if (!ms) return '—';
    const d = new Date(ms);
    return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(lang);
}

// buildSourceColumns derives the column set from the union of native field keys
// across the loaded rows, in first-seen order — headers are the source's own
// field names.
export function buildSourceColumns(rows: SourceRecordRow[], lang: Lang): GridColumn[] {
    const seen = new Set<string>();
    const keys: string[] = [];
    for (const r of rows) {
        for (const f of r.fields) {
            if (!seen.has(f.key)) {
                seen.add(f.key);
                keys.push(f.key);
            }
        }
    }
    const cols: GridColumn[] = [{ key: SEQ, label: '#', width: 54, locked: true, sortable: false }];
    for (const k of keys) cols.push({ key: FKEY + k, label: k, width: 170, groupable: true });
    cols.push({ key: FETCHED, label: t('datasource.col.fetchedAt', lang), width: 170 });
    return cols;
}

// Group-by options: none + every native field column.
export function buildGroupOptions(columns: GridColumn[], lang: Lang): Array<[string, string]> {
    const opts: Array<[string, string]> = [['none', t('contacts.group.none', lang)]];
    for (const c of columns) if (c.key.startsWith(FKEY)) opts.push([c.key, c.label]);
    return opts;
}

// fieldValue joins all values a record carries for a native key (repeats like
// two TELs → "a / b").
function fieldValue(r: SourceRecordRow, nativeKey: string): string {
    return r.fields
        .filter(f => f.key === nativeKey)
        .map(f => f.value)
        .join(' / ');
}

function cellValue(r: SourceRecordRow, colKey: string): string {
    if (colKey.startsWith(FKEY)) return fieldValue(r, colKey.slice(FKEY.length));
    if (colKey === FETCHED) return String(r.fetchedAt);
    return '';
}

export function compareSource(a: SourceRecordRow, b: SourceRecordRow, key: string): number {
    if (key === FETCHED) return a.fetchedAt - b.fetchedAt;
    return cellValue(a, key).localeCompare(cellValue(b, key));
}

// Default order: most recently fetched first.
export function sourceDefaultCompare(a: SourceRecordRow, b: SourceRecordRow): number {
    return b.fetchedAt - a.fetchedAt;
}

export function sourceGroupValue(r: SourceRecordRow, key: string): string {
    return cellValue(r, key) || '—';
}

// searchText flattens a record's native values for the search box.
export function searchText(r: SourceRecordRow): string {
    return r.fields
        .map(f => `${f.key} ${f.value}`)
        .join(' ')
        .toLowerCase();
}

export function renderSourceCell(r: SourceRecordRow, col: GridColumn, helpers: CellHelpers, lang: Lang): VNode {
    if (col.key === SEQ) {
        return (
            <td class="col-ds-seq">
                <button
                    class="ds-seq-btn"
                    title={t('datasource.viewRecord', lang)}
                    onClick={(e: Event) => {
                        e.stopPropagation();
                        helpers.openDetail();
                    }}
                >
                    {helpers.index + 1}
                </button>
            </td>
        );
    }
    if (col.key === FETCHED) {
        return <td class="col-ds-date">{fmtTime(r.fetchedAt, lang)}</td>;
    }
    const v = fieldValue(r, col.key.slice(FKEY.length));
    return (
        <td class={`col-ds-field${r.deleted ? ' ds-deleted' : ''}`} title={v}>
            {v || '—'}
        </td>
    );
}
