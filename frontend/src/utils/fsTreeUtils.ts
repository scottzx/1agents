import type { FsEntry } from '../components/types';

/**
 * Walk the tree and toggle `expanded` on the node whose path matches `targetPath`.
 * Returns a new array (immutable update).
 *
 * Children are retained on collapse because the backend returns and caches the
 * complete recursive tree. Re-expanding a directory must not trigger I/O.
 */
export function setExpanded(entries: FsEntry[], targetPath: string, expanded: boolean): FsEntry[] {
    return entries.map(e => {
        if (e.path === targetPath) {
            return { ...e, expanded };
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
                children: f.children && ext.children ? mergeFreshEntries(ext.children, f.children) : f.children,
            };
        }
        return f;
    });
}
