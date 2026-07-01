import { h, Fragment } from 'preact';
import type { VNode } from 'preact';
import { useSignal, useSignalEffect } from '@preact/signals';

import { GridToolbar, type ColState, type ToolbarColumn } from './GridToolbar';

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

// initColState builds the initial column visibility/order, restoring a persisted
// selection when persistKey is set. Reconciles with the live columns: keeps the
// saved order/visibility for keys still present, appends new keys (visible),
// drops keys that no longer exist — so dynamic schemas stay stable across loads.
function initColState(persistKey: string | undefined, allColumns: GridColumn[]): ColState[] {
    const base = allColumns.map(c => ({ key: c.key, visible: true }));
    if (!persistKey) return base;
    try {
        const saved = JSON.parse(localStorage.getItem(persistKey) || 'null') as ColState[] | null;
        if (!Array.isArray(saved)) return base;
        const live = new Set(allColumns.map(c => c.key));
        const savedKeys = new Set(saved.map(s => s.key));
        const ordered: ColState[] = [];
        for (const s of saved) if (live.has(s.key)) ordered.push({ key: s.key, visible: s.visible });
        for (const c of allColumns) if (!savedKeys.has(c.key)) ordered.push({ key: c.key, visible: true });
        return ordered.length ? ordered : base;
    } catch {
        return base;
    }
}

interface DataGridProps<T> {
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
    const groupBy = useSignal<string>('none');
    const collapsed = useSignal<string[]>([]);
    const sort = useSignal<{ key: string; dir: 'asc' | 'desc' } | null>(null);
    const showHierarchy = useSignal(true);
    const columns = useSignal<ColState[]>(initColState(persistKey, allColumns));

    // Persist the user's column choice (visibility + order) across sessions.
    useSignalEffect(() => {
        if (!persistKey) return;
        try {
            localStorage.setItem(persistKey, JSON.stringify(columns.value));
        } catch {
            /* storage full / disabled — non-fatal */
        }
    });

    if (loading && totalCount === 0) {
        return <div class="task-loading">正在载入...</div>;
    }

    const colDefs = new Map(allColumns.map(c => [c.key, c]));
    const visibleCols = columns.value
        .filter(c => c.visible)
        .map(c => colDefs.get(c.key))
        .filter((c): c is GridColumn => !!c);
    const colSpan = visibleCols.length + (renderActions ? 1 : 0);

    const toolbarCols: ToolbarColumn[] = allColumns.map(c => ({ key: c.key, label: c.label, locked: c.locked }));

    const commit = async (rowId: string, patch: Record<string, unknown>) => {
        editingCell.value = null;
        if (!onPatchRow) return;
        try {
            await onPatchRow(rowId, patch);
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const renderCells = (row: T, isChild: boolean, index: number) => {
        const rowKey = getRowKey(row);
        return visibleCols.map(col =>
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
            {renderActions && <td class="col-actions">{renderActions(row)}</td>}
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

    return (
        <div class="task-grid">
            <GridToolbar
                columnDefs={toolbarCols}
                groupOptions={groupOptions}
                groupBy={groupBy}
                columns={columns}
                showHierarchy={hierarchy ? showHierarchy : undefined}
                hierarchyLabel={hierarchy?.label}
                hierarchyHint={hierarchy?.hint}
            />

            <div class="task-table-scroller">
                <table class="task-table">
                    <thead>
                        <tr>
                            {visibleCols.map(col => {
                                const sortable = col.sortable !== false;
                                const active = sort.value?.key === col.key;
                                return (
                                    <th
                                        key={col.key}
                                        class={`col-${col.key}${sortable ? ' grid-sortable' : ''}${
                                            active ? ' sorted' : ''
                                        }`}
                                        style={{ minWidth: `${col.width}px` }}
                                        onClick={sortable ? () => cycleSort(col.key) : undefined}
                                        title={sortable ? '点击按此列排序' : undefined}
                                    >
                                        {col.label}
                                        {active && (
                                            <span class="sort-ind">{sort.value!.dir === 'asc' ? '▲' : '▼'}</span>
                                        )}
                                    </th>
                                );
                            })}
                            {renderActions && <th class="col-actions" />}
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
                                        <tr class="task-group-header" onClick={() => toggleGroup(g)}>
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
