import type { FsEntry } from '../components/types';

/**
 * Walk the tree and set `children` on the node whose path matches `targetPath`.
 * Returns a new array (immutable update).
 */
export function mergeChildren(entries: FsEntry[], targetPath: string, children: FsEntry[]): FsEntry[] {
    return entries.map(e => {
        if (e.path === targetPath) {
            return { ...e, children };
        }
        if (e.children) {
            return { ...e, children: mergeChildren(e.children, targetPath, children) };
        }
        return e;
    });
}

/**
 * Walk the tree and toggle `expanded` on the node whose path matches `targetPath`.
 * Returns a new array (immutable update).
 *
 * On collapse, the previously-loaded `children` array is dropped so it can be
 * garbage-collected. The next time the directory is expanded, `loadDir` will
 * re-fetch its children. This prevents the tree from holding onto every
 * expanded directory's contents for the lifetime of the App instance.
 */
export function setExpanded(entries: FsEntry[], targetPath: string, expanded: boolean): FsEntry[] {
    return entries.map(e => {
        if (e.path === targetPath) {
            return { ...e, expanded, children: expanded ? e.children : undefined };
        }
        if (e.children) {
            return { ...e, children: setExpanded(e.children, targetPath, expanded) };
        }
        return e;
    });
}

/**
 * Merges a fresh list of directory entries into the existing tree structure,
 * preserving already loaded children and expansion states of matching paths.
 */
export function mergeFreshEntries(existing: FsEntry[], fresh: FsEntry[]): FsEntry[] {
    const existingMap = new Map<string, FsEntry>();
    existing.forEach(e => {
        existingMap.set(e.path, e);
    });

    return fresh.map(f => {
        const ext = existingMap.get(f.path);
        if (ext) {
            return {
                ...f,
                expanded: ext.expanded,
                children: ext.children,
            };
        }
        return f;
    });
}
