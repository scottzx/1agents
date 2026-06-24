import { h } from 'preact';
import { useSignal } from '@preact/signals';
import type { Signal } from '@preact/signals';

export interface ColState {
    key: string;
    visible: boolean;
}

/** Minimal column metadata the toolbar needs for labels + lock state. */
export interface ToolbarColumn {
    key: string;
    label: string;
    locked?: boolean;
}

interface GridToolbarProps {
    /** Column metadata (label + locked) keyed by the same keys as `columns`. */
    columnDefs: ToolbarColumn[];
    /** Group-by options: [key, label]. First entry is the "no grouping" option. */
    groupOptions: Array<[string, string]>;
    groupBy: Signal<string>;
    columns: Signal<ColState[]>;
    /** Optional parent/child hierarchy toggle — omit to hide the checkbox. */
    showHierarchy?: Signal<boolean>;
    hierarchyLabel?: string;
    hierarchyHint?: string;
}

// Generic list-view controls shared by every 多维表格 (DataGrid): group-by,
// optional parent/child hierarchy display, and column show/hide + reorder.
// Search & filters live in each view's own filter bar.
export function GridToolbar({
    columnDefs,
    groupOptions,
    groupBy,
    columns,
    showHierarchy,
    hierarchyLabel = '显示层级',
    hierarchyHint,
}: GridToolbarProps) {
    const showColumns = useSignal(false);

    const colLabel = (key: string) => columnDefs.find(c => c.key === key)?.label || key;
    const isLocked = (key: string) => !!columnDefs.find(c => c.key === key)?.locked;

    const moveColumn = (idx: number, dir: -1 | 1) => {
        const next = [...columns.value];
        const swap = idx + dir;
        if (swap < 0 || swap >= next.length) return;
        // Never move the locked column, nor displace it from the first slot.
        if (isLocked(next[idx].key) || isLocked(next[swap].key)) return;
        [next[idx], next[swap]] = [next[swap], next[idx]];
        columns.value = next;
    };

    return (
        <div class="task-grid-toolbar">
            <label class="grid-toolbar-group">
                分组
                <select
                    value={groupBy.value}
                    onChange={(e: Event) => (groupBy.value = (e.target as HTMLSelectElement).value)}
                >
                    {groupOptions.map(([key, label]) => (
                        <option key={key} value={key}>
                            {label}
                        </option>
                    ))}
                </select>
            </label>

            {showHierarchy && (
                <label class="grid-toolbar-check" title={hierarchyHint}>
                    <input
                        type="checkbox"
                        checked={showHierarchy.value}
                        onChange={() => (showHierarchy.value = !showHierarchy.value)}
                    />
                    {hierarchyLabel}
                </label>
            )}

            <div class="grid-toolbar-popover-host">
                <button class="grid-toolbar-btn" onClick={() => (showColumns.value = !showColumns.value)}>
                    列
                </button>
                {showColumns.value && (
                    <div class="grid-toolbar-popover">
                        <div class="grid-filter-label">显示 / 排序列</div>
                        {columns.value.map((c, idx) => {
                            const locked = isLocked(c.key);
                            const upDisabled = idx === 0 || locked || isLocked(columns.value[idx - 1]?.key);
                            const downDisabled =
                                idx === columns.value.length - 1 || locked || isLocked(columns.value[idx + 1]?.key);
                            return (
                                <div key={c.key} class="grid-col-row">
                                    <label class="grid-col-toggle">
                                        <input
                                            type="checkbox"
                                            checked={c.visible}
                                            disabled={locked}
                                            onChange={() => {
                                                const next = [...columns.value];
                                                next[idx] = { ...c, visible: !c.visible };
                                                columns.value = next;
                                            }}
                                        />
                                        <span>
                                            {colLabel(c.key)}
                                            {locked && <span class="grid-col-pinned"> · 固定</span>}
                                        </span>
                                    </label>
                                    <span class="grid-col-move">
                                        <button disabled={upDisabled} title="上移" onClick={() => moveColumn(idx, -1)}>
                                            ↑
                                        </button>
                                        <button disabled={downDisabled} title="下移" onClick={() => moveColumn(idx, 1)}>
                                            ↓
                                        </button>
                                    </span>
                                </div>
                            );
                        })}
                    </div>
                )}
            </div>
        </div>
    );
}
