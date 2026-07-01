import { apiFetch } from './apiClient';

// 数据源 (data sources) — the read-only bronze layer (backend/internal/sources).
// Overview cards read the per-(source,kind) rollup; the 多维表格 detail view reads
// the raw records for one (source, kind). Mirrors sources.SourceSummary /
// sources.SourceRecordRow (json-tagged → camelCase).

export interface SourceSummary {
    source: string;
    kind: string;
    /** Non-deleted record count. */
    count: number;
    /** Distinct collections (address books / calendars / chats). */
    collections: number;
    /** Epoch ms of the most recent pull; 0 when empty. */
    lastFetchedAt: number;
}

// One native property of a record (vCard FN/TEL/…). No fixed schema — the grid
// builds columns from whatever keys appear, so the same viewer serves contacts
// now and todos/calendars later.
export interface SourceField {
    key: string;
    value: string;
}

// One bronze record for the grid: the generic envelope + its native fields in
// source order (repeats preserved, e.g. two TELs).
export interface SourceRecordRow {
    uid: string;
    collection: string;
    etag: string;
    contentType: string;
    deleted: boolean;
    fetchedAt: number;
    fields: SourceField[];
    preview: string;
}

export const sourceService = {
    /** GET /api/sources/summary — per-(source,kind) rollup for the overview cards. */
    async summary(): Promise<SourceSummary[]> {
        const res = await apiFetch('/sources/summary');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SourceSummary[];
    },

    /** GET /api/sources/records?source=&kind=&limit= — raw records as grid rows. */
    async records(source: string, kind: string, limit = 1000): Promise<SourceRecordRow[]> {
        const qs = new URLSearchParams({ source, kind, limit: String(limit) });
        const res = await apiFetch(`/sources/records?${qs.toString()}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SourceRecordRow[];
    },
};
