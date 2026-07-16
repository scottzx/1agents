import { signal, effect } from '@preact/signals';

/**
 * Per-workspace view preferences for the 项目详情/任务 tab. Each workspace
 * gets its own slot so switching projects restores the prior view (sort,
 * filters, view mode, group-by, collapsed groups, hierarchy toggle). The
 * top-level view tab (overview/discussion/requirements/tasks/milestone) is
 * also per-workspace so each project remembers which tab you were on.
 *
 * DataGrid state (sort/groupBy/collapsed/showHierarchy) lives in the
 * `grids` sub-slot, keyed by surface (e.g. `tasks`, `sessions`). This keeps
 * the two grids' state from clobbering each other when they share a
 * workspaceId. The flat `sort/groupBy/collapsed/showHierarchy` fields are
 * kept as a legacy fallback for users who set prefs before #133 landed —
 * reads fall through to them when no sub-slot exists.
 *
 * Persisted as one localStorage entry under `1agents-project-view-prefs`
 * containing a `{ [workspaceId]: ViewPrefs }` map. Module-level `effect`
 * mirrors the signal back to localStorage on every change. Storage failures
 * are non-fatal (private mode / quota).
 */

export type TaskListView = 'overview' | 'discussion' | 'requirements' | 'tasks' | 'milestone';
export type TaskSubView = 'list' | 'board' | 'calendar';
export type SortDir = 'asc' | 'desc';

export interface SortState {
    key: string;
    dir: SortDir;
}

/** DataGrid prefs scoped to one surface (e.g. tasks table or sessions table). */
export interface GridPrefs {
    sort: SortState | null;
    groupBy: string;
    collapsed: string[];
    showHierarchy?: boolean; // tasks only; sessions ignore this
}

export interface ViewPrefs {
    // TaskList top-level tab switcher.
    activeView: TaskListView;
    // TasksView (list ↔ board ↔ calendar) — previously module-level, now
    // per-workspace so each project remembers its preferred sub-view.
    taskView: TaskSubView;
    // Filter state in TaskFilterBar.
    search: string;
    statusFilter: string[];
    priorityFilter: string[];
    assigneeFilter: string[];
    // Legacy DataGrid prefs (flat). Kept readable so users with prefs saved
    // before the per-surface split don't lose them on upgrade — reads fall
    // through to these when no `grids` sub-slot exists for a surface. New
    // writes go only into `grids[surface]`.
    sort: SortState | null;
    groupBy: string;
    collapsed: string[];
    showHierarchy: boolean;
    // Per-surface DataGrid prefs. Surface ids are free-form strings; today
    // the two consumers pass 'tasks' and 'sessions'.
    grids?: {
        tasks?: GridPrefs;
        sessions?: GridPrefs;
        [surface: string]: GridPrefs | undefined;
    };
}

export const DEFAULT_PREFS: ViewPrefs = {
    activeView: 'tasks',
    taskView: 'list',
    search: '',
    statusFilter: [],
    priorityFilter: [],
    assigneeFilter: [],
    sort: null,
    groupBy: 'none',
    collapsed: [],
    showHierarchy: true,
};

const STORAGE_KEY = '1agents-project-view-prefs';

const loadAll = (): Record<string, ViewPrefs> => {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return {};
        const parsed = JSON.parse(raw);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {};
        return parsed as Record<string, ViewPrefs>;
    } catch {
        return {};
    }
};

// One signal holds every workspace's prefs. Components subscribe by reading
// `allPrefs.value[workspaceId]`; mutations go through `updatePrefs` so the
// effect below mirrors the whole map back to localStorage atomically.
export const allPrefs = signal<Record<string, ViewPrefs>>(loadAll());

effect(() => {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(allPrefs.value));
    } catch {
        /* storage full / disabled — non-fatal */
    }
});

export const getPrefs = (workspaceId: string): ViewPrefs => {
    const stored = allPrefs.value[workspaceId];
    return stored ? { ...DEFAULT_PREFS, ...stored } : DEFAULT_PREFS;
};

export const updatePrefs = (workspaceId: string, patch: Partial<ViewPrefs>): void => {
    const current = allPrefs.value[workspaceId] || DEFAULT_PREFS;
    const next = { ...DEFAULT_PREFS, ...current, ...patch };
    allPrefs.value = { ...allPrefs.value, [workspaceId]: next };
};

/** Resolve the DataGrid prefs for a given surface. Returns the sub-slot if
 *  populated; otherwise falls back to the legacy flat fields so a user with
 *  prefs saved before #133 still sees their previous sort/groupBy. */
export const getGridPrefs = (workspaceId: string, surface: string): GridPrefs => {
    const stored = allPrefs.value[workspaceId];
    if (!stored) {
        return {
            sort: DEFAULT_PREFS.sort,
            groupBy: DEFAULT_PREFS.groupBy,
            collapsed: [...DEFAULT_PREFS.collapsed],
            showHierarchy: DEFAULT_PREFS.showHierarchy,
        };
    }
    const slot = stored.grids?.[surface];
    if (slot) {
        return {
            sort: slot.sort ?? DEFAULT_PREFS.sort,
            groupBy: slot.groupBy ?? DEFAULT_PREFS.groupBy,
            collapsed: Array.isArray(slot.collapsed) ? slot.collapsed : [...DEFAULT_PREFS.collapsed],
            showHierarchy: slot.showHierarchy ?? DEFAULT_PREFS.showHierarchy,
        };
    }
    // Legacy fallback: read from the flat fields. Whichever surface mounts
    // first after the upgrade claims these values; the first write into a
    // surface slot copies them over (so the other surface still sees the
    // legacy state until it, too, mounts and writes).
    return {
        sort: stored.sort ?? DEFAULT_PREFS.sort,
        groupBy: stored.groupBy ?? DEFAULT_PREFS.groupBy,
        collapsed: Array.isArray(stored.collapsed) ? stored.collapsed : [...DEFAULT_PREFS.collapsed],
        showHierarchy: stored.showHierarchy ?? DEFAULT_PREFS.showHierarchy,
    };
};

/** Write DataGrid prefs for a single surface. Does not touch the legacy flat
 *  fields or other surfaces — each surface keeps its own slot. */
export const updateGridPrefs = (
    workspaceId: string,
    surface: string,
    patch: Partial<GridPrefs>
): void => {
    const current = getGridPrefs(workspaceId, surface);
    const next: GridPrefs = { ...current, ...patch };
    const allCurrent = allPrefs.value[workspaceId] || DEFAULT_PREFS;
    const nextGrids = { ...(allCurrent.grids || {}), [surface]: next };
    allPrefs.value = {
        ...allPrefs.value,
        [workspaceId]: { ...allCurrent, grids: nextGrids },
    };
};