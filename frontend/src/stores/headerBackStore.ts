import { signal } from '@preact/signals';

/**
 * The one global back action displayed as the icon before WorkspaceHeader's
 * breadcrumb. Publishers register by owner and priority so nested details can
 * temporarily override a parent action; removing the detail automatically
 * restores the parent action instead of losing it.
 */
export const headerBackAction = signal<(() => void) | null>(null);

export const HEADER_BACK_PRIORITY = {
    surface: 10,
    detail: 100,
} as const;

interface HeaderBackEntry {
    action: () => void;
    priority: number;
    order: number;
}

const headerBackEntries = new Map<string, HeaderBackEntry>();
let headerBackOrder = 0;

const syncHeaderBackAction = (): void => {
    let winner: HeaderBackEntry | undefined;
    for (const entry of headerBackEntries.values()) {
        if (
            !winner ||
            entry.priority > winner.priority ||
            (entry.priority === winner.priority && entry.order > winner.order)
        ) {
            winner = entry;
        }
    }
    headerBackAction.value = winner?.action ?? null;
};

/** Publish one global-header back layer. Returns an ownership-safe disposer. */
export const registerHeaderBackAction = (owner: string, action: () => void, priority = 0): (() => void) => {
    const entry: HeaderBackEntry = { action, priority, order: ++headerBackOrder };
    headerBackEntries.set(owner, entry);
    syncHeaderBackAction();
    return () => {
        if (headerBackEntries.get(owner) !== entry) return;
        headerBackEntries.delete(owner);
        syncHeaderBackAction();
    };
};

/** Remove a named back layer without disturbing newer or unrelated layers. */
export const clearHeaderBackAction = (owner: string): void => {
    if (!headerBackEntries.delete(owner)) return;
    syncHeaderBackAction();
};

/** Reset navigation ownership when switching to an unrelated primary surface. */
export const clearHeaderBackActions = (): void => {
    headerBackEntries.clear();
    headerBackAction.value = null;
};
