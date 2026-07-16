import { h, Fragment } from 'preact';
import type { VNode } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal, useSignalEffect } from '@preact/signals';

import { GridToolbar, type ColState, type ToolbarColumn } from './GridToolbar';
import * as viewPrefs from '../../../stores/projectViewPrefs';
import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';

/** Parse a persisted JSON blob into a sanitized `ColState[]`. Tolerant of the
 *  pre-width format (`{key, visible}` only) and of any shape mismatch — the
 *  goal is "read should never throw, even on a corrupt or legacy entry".
 *  Returns `[]` for null / empty / non-array / unparseable input; the caller
 *  passes the result through `reconcileColState` which fills in defaults. */
export function loadColState(raw: string | null): ColState[] {
    if (!raw) return [];
    let parsed: unknown;
    try {
        parsed = JSON.parse(raw);
    } catch {
        return [];
    }
    if (!Array.isArray(parsed)) return [];
    const out: ColState[] = [];
    for (const item of parsed) {
        if (!item || typeof item !== 'object') continue;
        const k = (item as { key?: unknown }).key;
        const v = (item as { visible?: unknown }).visible;
        if (typeof k !== 'string' || !k) continue;
        const w = (item as { width?: unknown }).width;
        const next: ColState = {
            key: k,
            visible: typeof v === 'boolean' ? v : true,
        };
        if (typeof w === 'number' && Number.isFinite(w) && w > 0) {
            next.width = w;
        }
        out.push(next);
    }
    return out;
}

/** Reconcile a previously-saved column state against the live column defs.
 *  Pure: same inputs → same output, no DOM/storage reads.
 *  - Keys still in `allColumns` keep their `prev` order, visibility, and width.
 *  - Keys absent from `allColumns` are dropped (column was removed).
 *  - Keys new in `allColumns` are appended as visible (no width yet — the
 *    renderer falls back to `GridColumn.width` until the user resizes).
 *  - Empty `prev` yields the all-visible baseline in `allColumns` order. */
export function reconcileColState(prev: ColState[], allColumns: GridColumn[]): ColState[] {
    const live = new Set(allColumns.map(c => c.key));
    const savedKeys = new Set(prev.map(s => s.key));
    const ordered: ColState[] = [];
    for (const s of prev) if (live.has(s.key)) ordered.push({ ...s });
    for (const c of allColumns) if (!savedKeys.has(c.key)) ordered.push({ key: c.key, visible: true });
    return ordered.length ? ordered : allColumns.map(c => ({ key: c.key, visible: true }));
}

/** Initial load: read persisted JSON (if a `persistKey` is set) and reconcile
 *  it with the live columns. Kept as a thin wrapper so the call site reads
 *  the same way it always has — the two pure helpers above carry the logic. */
function initColState(persistKey: string | undefined, allColumns: GridColumn[]): ColState[] {
    const base = allColumns.map(c => ({ key: c.key, visible: true }));
    if (!persistKey) return base;
    return reconcileColState(loadColState(localStorage.getItem(persistKey)), allColumns);
}

/** Column metadata for a DataGrid. The matching cell content is produced by the
 *  consumer's `renderCell`; this only drives layout, sorting and grouping. */
export interface GridColumn {
    key: string;
    label: string;
    width: number;
    /** Pinned: always first, cannot be reordered or hidden. */
    locked?: boolean;
    /** Selectable as a group-by key. */
    groupable?: boolean;
    /** Header-click sorting (default true). */
    sortable?: boolean;
}

/** Per-cell helpers handed to renderCell — covers read-only and inline-edit cells. */
export interface CellHelpers {
    isChild: boolean;
    editing: boolean;
    /** 1-based display position of the row in the current view (for a 序号 column). */
    index: number;
    startEdit: () => void;
    commit: (patch: Record<string, unknown>) => void;
    cancel: () => void;
    openDetail: () => void;
}

interface DataGridProps<T> {
    /** Workspace that owns the persisted DataGrid prefs. Combined with
     *  `prefsSurface` so the tasks grid and the sessions grid (both pass
     *  the same workspaceId) keep independent sort/groupBy/collapsed state.
     *  Omit both to keep that state in-memory only. */
    workspaceId?: string;
    /** Which DataGrid view is calling (`tasks`, `sessions`, ...). Required
     *  for persistence: it scopes the slot under `grids[surface]` so two
     *  consumers sharing a workspace don't clobber each other. */
    prefsSurface?: string;
    /** Rows to render (already filtered by the view's own filter bar). */
    rows: T[];
    /** Total row count before filtering — drives the "empty" vs "no match" copy. */
    totalCount: number;
    columns: GridColumn[];
    /** Group-by options: [key, label]; first entry = "no grouping". */
    groupOptions: Array<[string, string]>;
    getRowKey: (row: T) => string;
    renderCell: (row: T, col: GridColumn, helpers: CellHelpers) => VNode;
    renderActions?: (row: T) => VNode;
    onPatchRow?: (rowId: string, patch: Record<string, unknown>) => Promise<void>;
    onOpenRow?: (row: T) => void;
    /** Ascending comparator for the active sort column. */
    compare: (a: T, b: T, key: string) => number;
    /** Default order when no column sort is active. */
    defaultCompare: (a: T, b: T) => number;
    groupValue: (row: T, key: string) => string;
    /** Full `<tr>` class (receives isChild for hierarchy styling). */
    rowClass: (row: T, isChild: boolean) => string;
    /** Optional parent/child hierarchy (task-only). */
    hierarchy?: { parentId: (row: T) => string | undefined; label?: string; hint?: string };
    loading?: boolean;
    emptyAll: string;
    emptyFiltered: string;
    /** When set, the column visibility/order chosen via the toolbar persists to
     *  localStorage under this key and is restored on mount. */
    persistKey?: string;
}

const cellKey = (rowKey: string, colKey: string) => `${rowKey}:${colKey}`;

// Config-driven multi-dimensional table (多维表格). Owns column visibility /
// order, header-click sort, grouping, optional parent/child hierarchy, inline
// cell editing state, and the actions column. Both the task and session views
// drive it with their own column config + cell renderers.
export function DataGrid<T>({
    workspaceId,
    prefsSurface,
    rows,
    totalCount,
    columns: allColumns,
    groupOptions,
    getRowKey,
    renderCell,
    renderActions,
    onPatchRow,
    onOpenRow,
    compare,
    defaultCompare,
    groupValue,
    rowClass,
    hierarchy,
    loading,
    emptyAll,
    emptyFiltered,
    persistKey,
}: DataGridProps<T>) {
    const editingCell = useSignal<string | null>(null);
    // When workspaceId + prefsSurface are both set, hydrate from the
    // surface-scoped persisted slot so each grid (tasks, sessions, ...)
    // remembers its own sort/groupBy/collapsed/hierarchy choice independently
    // of the others. Without prefsSurface we fall back to plain in-memory
    // state — this keeps other DataGrid users (contacts, governance, ...)
    // working without any per-surface wiring.
    const initial =
        workspaceId && prefsSurface
            ? viewPrefs.getGridPrefs(workspaceId, prefsSurface)
            : {
                  sort: null as { key: string; dir: 'asc' | 'desc' } | null,
                  groupBy: 'none',
                  collapsed: [] as string[],
                  showHierarchy: true,
              };
    const groupBy = useSignal<string>(initial.groupBy);
    const collapsed = useSignal<string[]>(initial.collapsed);
    const sort = useSignal<{ key: string; dir: 'asc' | 'desc' } | null>(initial.sort);
    const showHierarchy = useSignal<boolean>(initial.showHierarchy ?? true);
    const columns = useSignal<ColState[]>(initColState(persistKey, allColumns));

    // Reconcile `columns` whenever the live column set changes — language
    // switch (label-only changes don't affect keys, but new keys from a future
    // localization can), or a data source adding/dropping a column. Gated on
    // the key signature so unrelated parent re-renders don't churn the signal
    // (TaskTable builds a fresh `taskColumns` array on every render, so a
    // reference-equality dep would fire constantly).
    const liveKeySig = allColumns.map(c => c.key).join('|');
    useEffect(() => {
        columns.value = reconcileColState(columns.value, allColumns);
        // allColumns keyed via liveKeySig to avoid parent re-render churn.
        // restart the loop on every parent render.
    }, [liveKeySig]);

    // Re-init sort / groupBy / collapsed / showHierarchy when the owning
    // workspace or surface changes so we don't leak one project's view into
    // another, or one surface's view into the other.
    useEffect(() => {
        if (!workspaceId || !prefsSurface) return;
        const p = viewPrefs.getGridPrefs(workspaceId, prefsSurface);
        sort.value = p.sort;
        groupBy.value = p.groupBy;
        collapsed.value = p.collapsed;
        showHierarchy.value = p.showHierarchy ?? true;
    }, [workspaceId, prefsSurface]);

    // Persist the column choice (visibility + order) across sessions.
    useSignalEffect(() => {
        if (!persistKey) return;
        try {
            localStorage.setItem(persistKey, JSON.stringify(columns.value));
        } catch {
            /* storage full / disabled — non-fatal */
        }
    });

    // Persist sort / groupBy / collapsed / showHierarchy per (workspace, surface)
    // so each grid keeps its own slot. Reads happen at mount via the
    // useSignal seed above and on workspace/surface change via the effect
    // above; this effect mirrors any in-memory change back into storage.
    useEffect(() => {
        if (!workspaceId || !prefsSurface) return;
        viewPrefs.updateGridPrefs(workspaceId, prefsSurface, {
            sort: sort.value,
            groupBy: groupBy.value,
            collapsed: collapsed.value,
            showHierarchy: showHierarchy.value,
        });
    }, [workspaceId, prefsSurface, sort.value, groupBy.value, collapsed.value, showHierarchy.value]);

    if (loading && totalCount === 0) {
        return <div class="task-loading">正在载入...</div>;
    }

    const colDefs = new Map(allColumns.map(c => [c.key, c]));
    // Join ColState with its GridColumn so the renderer can read either
    // persisted state (width) or live state (label, sortable). Missing width
    // falls back to the column default — that's how legacy / never-resized
    // columns still get a sensible size.
    const visibleCols = columns.value
        .filter(c => c.visible)
        .map(c => {
            const def = colDefs.get(c.key);
            if (!def) return null;
            return { def, width: c.width ?? def.width };
        })
        .filter((c): c is { def: GridColumn; width: number } => !!c);
    const colSpan = visibleCols.length + (renderActions ? 1 : 0);

    const toolbarCols: ToolbarColumn[] = allColumns.map(c => ({ key: c.key, label: c.label, locked: c.locked }));

    const commit = async (rowId: string, patch: Record<string, unknown>) => {
        editingCell.value = null;
        if (!onPatchRow) return;
        try {
            await onPatchRow(rowId, patch);
        } catch (err) {
            ui.showToast((err as Error).message || String(err));
        }
    };

    const renderCells = (row: T, isChild: boolean, index: number) => {
        const rowKey = getRowKey(row);
        return visibleCols.map(({ def: col }) =>
            renderCell(row, col, {
                isChild,
                index,
                editing: editingCell.value === cellKey(rowKey, col.key),
                startEdit: () => (editingCell.value = cellKey(rowKey, col.key)),
                commit: patch => commit(rowKey, patch),
                cancel: () => (editingCell.value = null),
                openDetail: () => onOpenRow?.(row),
            })
        );
    };

    const renderRow = (row: T, isChild: boolean, index: number) => (
        <tr key={getRowKey(row)} class={rowClass(row, isChild)}>
            {renderCells(row, isChild, index)}
            {renderActions && <td class="col-actions col-sticky-right">{renderActions(row)}</td>}
        </tr>
    );

    // Header-click sort: cycle off → asc → desc → off on the active column.
    const cycleSort = (key: string) => {
        const s = sort.value;
        if (!s || s.key !== key) sort.value = { key, dir: 'asc' };
        else if (s.dir === 'asc') sort.value = { key, dir: 'desc' };
        else sort.value = null;
    };

    const cmp = (a: T, b: T): number => {
        const s = sort.value;
        if (!s) return defaultCompare(a, b);
        return s.dir === 'asc' ? compare(a, b, s.key) : -compare(a, b, s.key);
    };

    // Hierarchy-aware order: top-level rows sorted by `cmp`, each parent
    // immediately followed by its children sorted by `cmp` within the parent.
    const orderHierarchical = (list: T[]): Array<{ row: T; isChild: boolean }> => {
        const parentId = hierarchy!.parentId;
        const byParent = new Map<string, T[]>();
        const tops: T[] = [];
        for (const r of list) {
            const pid = parentId(r);
            if (pid && list.some(p => getRowKey(p) === pid)) {
                (byParent.get(pid) || byParent.set(pid, []).get(pid)!).push(r);
            } else {
                tops.push(r);
            }
        }
        tops.sort(cmp);
        const out: Array<{ row: T; isChild: boolean }> = [];
        for (const r of tops) {
            out.push({ row: r, isChild: false });
            for (const c of (byParent.get(getRowKey(r)) || []).slice().sort(cmp)) out.push({ row: c, isChild: true });
        }
        return out;
    };

    const buildGroups = (): Array<[string, T[]]> => {
        const ordered = [...rows].sort(cmp);
        const map = new Map<string, T[]>();
        for (const r of ordered) {
            const g = groupValue(r, groupBy.value);
            (map.get(g) || map.set(g, []).get(g)!).push(r);
        }
        return Array.from(map.entries());
    };

    const toggleGroup = (g: string) => {
        collapsed.value = collapsed.value.includes(g) ? collapsed.value.filter(x => x !== g) : [...collapsed.value, g];
    };

    // Drag the header right edge to set ColState.width (min 48px). Clicks on
    // the handle stopPropagation so they don't cycle sort.
    const startResize = (e: MouseEvent, key: string, startW: number) => {
        e.preventDefault();
        e.stopPropagation();
        const startX = e.clientX;
        const onMove = (ev: MouseEvent) => {
            const next = Math.max(48, Math.round(startW + (ev.clientX - startX)));
            columns.value = columns.value.map(c => (c.key === key ? { ...c, width: next } : c));
        };
        const onUp = () => {
            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onUp);
        };
        document.addEventListener('mousemove', onMove);
        document.addEventListener('mouseup', onUp);
    };

    return (
        <div class="data-grid">
            <GridToolbar
                columnDefs={toolbarCols}
                groupOptions={groupOptions}
                groupBy={groupBy}
                columns={columns}
                showHierarchy={hierarchy ? showHierarchy : undefined}
                hierarchyLabel={hierarchy?.label}
                hierarchyHint={hierarchy?.hint}
                allColumnKeys={allColumns.map(c => c.key)}
            />

            <div class="data-grid-scroller">
                <table class="data-grid-table">
                    <thead>
                        <tr>
                            {visibleCols.map(({ def: col, width }) => {
                                const sortable = col.sortable !== false;
                                const active = sort.value?.key === col.key;
                                const stickyL = col.locked ? ' col-sticky-left' : '';
                                return (
                                    <th
                                        key={col.key}
                                        class={`col-${col.key} grid-col-resizable${sortable ? ' grid-sortable' : ''}${
                                            active ? ' sorted' : ''
                                        }${stickyL}`}
                                        style={{ minWidth: `${width}px`, width: `${width}px` }}
                                        onClick={sortable ? () => cycleSort(col.key) : undefined}
                                        title={sortable ? t('grid.toolbar.sortHint', ui.language.value) : undefined}
                                    >
                                        {col.label}
                                        {active && (
                                            <span class="sort-ind">{sort.value!.dir === 'asc' ? '▲' : '▼'}</span>
                                        )}
                                        <span
                                            class="grid-col-resize-handle"
                                            onMouseDown={(e: MouseEvent) => startResize(e, col.key, width)}
                                        />
                                    </th>
                                );
                            })}
                            {renderActions && <th class="col-actions col-sticky-right" />}
                        </tr>
                    </thead>
                    <tbody>
                        {rows.length === 0 && (
                            <tr class="task-empty-row">
                                <td colSpan={colSpan}>{totalCount === 0 ? emptyAll : emptyFiltered}</td>
                            </tr>
                        )}

                        {groupBy.value === 'none' &&
                            (hierarchy && showHierarchy.value
                                ? orderHierarchical(rows).map(({ row, isChild }, i) => renderRow(row, isChild, i))
                                : [...rows].sort(cmp).map((row, i) => renderRow(row, false, i)))}

                        {groupBy.value !== 'none' &&
                            buildGroups().map(([g, members]) => {
                                const isCollapsed = collapsed.value.includes(g);
                                return (
                                    <Fragment key={g}>
                                        <tr class="data-grid-group-header" onClick={() => toggleGroup(g)}>
                                            <td colSpan={colSpan}>
                                                <span class="group-caret">{isCollapsed ? '▶' : '▼'}</span>
                                                <span class="group-name">{g}</span>
                                                <span class="group-count">{members.length}</span>
                                            </td>
                                        </tr>
                                        {!isCollapsed && members.map((row, i) => renderRow(row, false, i))}
                                    </Fragment>
                                );
                            })}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
