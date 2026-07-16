import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import type { Signal } from '@preact/signals';

import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';

export interface ColState {
    key: string;
    visible: boolean;
    /** Persisted column width in px. Absent means use GridColumn.width. */
    width?: number;
}

/** Minimal column metadata the toolbar needs for labels + lock state. */
export interface ToolbarColumn {
    key: string;
    label: string;
    locked?: boolean;
}

interface GridToolbarProps {
    columnDefs: ToolbarColumn[];
    groupOptions: Array<[string, string]>;
    groupBy: Signal<string>;
    columns: Signal<ColState[]>;
    showHierarchy?: Signal<boolean>;
    hierarchyLabel?: string;
    hierarchyHint?: string;
    /** Live column key order for "restore defaults". */
    allColumnKeys?: string[];
}

// Generic list-view controls shared by every DataGrid: group-by, optional
// hierarchy toggle, column show/hide + reorder, restore defaults.
export function GridToolbar({
    columnDefs,
    groupOptions,
    groupBy,
    columns,
    showHierarchy,
    hierarchyLabel,
    hierarchyHint,
    allColumnKeys,
}: GridToolbarProps) {
    const showColumns = useSignal(false);
    const hostRef = useRef<HTMLDivElement>(null);
    const lang = ui.language.value;
    const hLabel = hierarchyLabel || t('grid.toolbar.hierarchy', lang);

    const colLabel = (key: string) => columnDefs.find(c => c.key === key)?.label || key;
    const isLocked = (key: string) => !!columnDefs.find(c => c.key === key)?.locked;

    useEffect(() => {
        if (!showColumns.value) return;
        const onDown = (e: MouseEvent) => {
            const el = hostRef.current;
            if (el && !el.contains(e.target as Node)) showColumns.value = false;
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') showColumns.value = false;
        };
        document.addEventListener('mousedown', onDown);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onDown);
            document.removeEventListener('keydown', onKey);
        };
    }, [showColumns.value]);

    const moveColumn = (idx: number, dir: -1 | 1) => {
        const next = [...columns.value];
        const swap = idx + dir;
        if (swap < 0 || swap >= next.length) return;
        if (isLocked(next[idx].key) || isLocked(next[swap].key)) return;
        [next[idx], next[swap]] = [next[swap], next[idx]];
        columns.value = next;
    };

    const restoreDefaults = () => {
        const keys = allColumnKeys || columnDefs.map(c => c.key);
        columns.value = keys.map(key => ({ key, visible: true }));
    };

    return (
        <div class="data-grid-toolbar">
            <label class="grid-toolbar-group">
                {t('grid.toolbar.group', lang)}
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
                    {hLabel}
                </label>
            )}

            <div class="grid-toolbar-popover-host" ref={hostRef}>
                <button
                    class={`grid-toolbar-btn${showColumns.value ? ' active' : ''}`}
                    onClick={() => (showColumns.value = !showColumns.value)}
                >
                    {t('grid.toolbar.columns', lang)}
                </button>
                {showColumns.value && (
                    <div class="grid-toolbar-popover">
                        <div class="grid-filter-label">{t('grid.toolbar.columnPanel', lang)}</div>
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
                                            {locked && (
                                                <span class="grid-col-pinned">
                                                    {' · '}
                                                    {t('grid.toolbar.pinned', lang)}
                                                </span>
                                            )}
                                        </span>
                                    </label>
                                    <span class="grid-col-move">
                                        <button
                                            disabled={upDisabled}
                                            title={t('grid.toolbar.moveUp', lang)}
                                            onClick={() => moveColumn(idx, -1)}
                                        >
                                            ↑
                                        </button>
                                        <button
                                            disabled={downDisabled}
                                            title={t('grid.toolbar.moveDown', lang)}
                                            onClick={() => moveColumn(idx, 1)}
                                        >
                                            ↓
                                        </button>
                                    </span>
                                </div>
                            );
                        })}
                        <button type="button" class="grid-filter-clear" onClick={restoreDefaults}>
                            {t('grid.toolbar.restoreDefaults', lang)}
                        </button>
                    </div>
                )}
            </div>
        </div>
    );
}
