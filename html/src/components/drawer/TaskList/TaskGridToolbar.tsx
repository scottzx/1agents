import { h } from 'preact';
import { useSignal } from '@preact/signals';
import type { Signal } from '@preact/signals';

import { ALL_COLUMNS, GROUP_OPTIONS } from './gridConfig';
import type { GroupKey } from './gridConfig';

export interface ColState {
    key: string;
    visible: boolean;
}

interface ToolbarProps {
    groupBy: Signal<GroupKey>;
    showHierarchy: Signal<boolean>;
    columns: Signal<ColState[]>;
}

const colLabel = (key: string) => ALL_COLUMNS.find(c => c.key === key)?.label || key;
const isLocked = (key: string) => !!ALL_COLUMNS.find(c => c.key === key)?.locked;

// List-only controls: group-by, parent/child hierarchy display, and column
// show/hide + reorder. Search & filters live in the shared TaskFilterBar.
export function TaskGridToolbar({ groupBy, showHierarchy, columns }: ToolbarProps) {
    const showColumns = useSignal(false);

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
                    onChange={(e: Event) => (groupBy.value = (e.target as HTMLSelectElement).value as GroupKey)}
                >
                    {GROUP_OPTIONS.map(([key, label]) => (
                        <option key={key} value={key}>
                            {label}
                        </option>
                    ))}
                </select>
            </label>

            <label class="grid-toolbar-check" title="排序时子任务只在父任务内排序">
                <input
                    type="checkbox"
                    checked={showHierarchy.value}
                    onChange={() => (showHierarchy.value = !showHierarchy.value)}
                />
                显示父子任务层级
            </label>

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
