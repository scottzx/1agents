import { signal } from '@preact/signals';

import type { FsEntry } from '../components/types';
import type { Workspace } from '../components/types';
import { fsService } from '../services/fsService';
import { mergeChildren, setExpanded, mergeFreshEntries } from '../utils/fsTreeUtils';
import { t } from '../i18n';
import * as ui from './uiStore';

/**
 * File-system state (tree browser, flat search, file detail/editor).
 * Previously ~17 fields on App's god-state threaded 4 levels deep into
 * RightPanel/FlatFileBrowser/FileDetailView; now any consumer reads the
 * signals directly.
 */

let initialFavs: string[] = [];
try {
    initialFavs = JSON.parse(localStorage.getItem('fav-files') || '[]');
} catch {
    /* ignore */
}

// ── Tree browser ──
export const fsEntries = signal<FsEntry[]>([]);
export const fsLoading = signal(false);

// ── File detail / editor ──
export const selectedFsEntry = signal<FsEntry | null>(null);
export const fileContent = signal('');
export const editedContent = signal('');
export const fileLoading = signal(false);
export const fileSaving = signal(false);
export const fileSaveMsg = signal('');
export const isImagePreview = signal(false);
export const detailFullscreen = signal(false);
export const isEditingDetail = signal(false);

// ── Flat file browser / search ──
export const flatFiles = signal<FsEntry[]>([]);
export const flatFilesLoading = signal(false);
export const searchQuery = signal('');
export const selectedFilterTag = signal<'all' | 'doc' | 'img' | 'code'>('all');
export const viewMode = signal<'list' | 'detail'>('list');
export const favoriteFiles = signal<string[]>(initialFavs);

// Per-workspace fs tree cache so switching back to a workspace doesn't flash
// an empty tree while it reloads. Keyed by workspace id.
const _treeCache: Record<string, FsEntry[]> = {};
let _treeCacheKey = localStorage.getItem('1agents-active-workspace') || '';

let _crawlCounter = 0;
let _searchTimeout: ReturnType<typeof setTimeout> | null = null;
let _saveMsgTimeout: ReturnType<typeof setTimeout> | null = null;

const cacheTree = (entries: FsEntry[]) => {
    if (_treeCacheKey) {
        _treeCache[_treeCacheKey] = entries;
    }
};

/** Fetch a directory listing and merge it into the tree. */
export const loadDir = async (relPath: string, parent: FsEntry | null) => {
    if (!parent && fsEntries.value.length === 0) {
        fsLoading.value = true;
    }
    try {
        const entries = await fsService.list(relPath);
        let next: FsEntry[];
        if (!parent) {
            next = fsEntries.value.length > 0 ? mergeFreshEntries(fsEntries.value, entries) : entries;
        } else {
            // Merge children into the existing tree
            next = mergeChildren(fsEntries.value, parent.path, entries);
        }
        fsEntries.value = next;
        cacheTree(next);
        if (!parent) fsLoading.value = false;
    } catch (err) {
        console.error('[fs] list error:', err);
        if (!parent) fsLoading.value = false;
    }
};

/** Toggle expand/collapse of a directory entry */
export const toggleFsDir = (entry: FsEntry) => {
    if (!entry.isDir) return;
    const willExpand = !entry.expanded;
    const next = setExpanded(fsEntries.value, entry.path, willExpand);
    fsEntries.value = next;
    cacheTree(next);
    // Lazy-load children only on first expand
    if (willExpand && (!entry.children || entry.children.length === 0)) {
        loadDir(entry.path, entry);
    }
};

/** Check if a filename has an image extension */
export const isImageFile = (name: string): boolean => {
    const ext = name.toLowerCase().split('.').pop() || '';
    return ['gif', 'png', 'jpg', 'jpeg', 'webp', 'bmp', 'svg'].includes(ext);
};

/** Open a file and load its content from /api/fs/read */
export const selectFsFile = async (entry: FsEntry) => {
    if (entry.isDir) {
        toggleFsDir(entry);
        return;
    }
    selectedFsEntry.value = entry;
    fileLoading.value = true;
    fileContent.value = '';
    editedContent.value = '';
    fileSaveMsg.value = '';
    isImagePreview.value = false;

    if (isImageFile(entry.name)) {
        // Image is rendered directly via <img src={fsService.imageUrl(path)}>.
        isImagePreview.value = true;
        fileLoading.value = false;
        return;
    }

    try {
        const text = await fsService.read(entry.path);
        fileContent.value = text;
        editedContent.value = text;
        fileLoading.value = false;
    } catch (err) {
        console.error('[fs] read error:', err);
        fileContent.value = `Error loading file: ${err}`;
        editedContent.value = '';
        fileLoading.value = false;
    }
};

/** Open a file in the detail pane of the flat browser. */
export const openFileDetail = async (entry: FsEntry) => {
    selectedFsEntry.value = entry;
    viewMode.value = 'detail';
    fileLoading.value = true;
    fileContent.value = '';
    editedContent.value = '';
    isEditingDetail.value = false;
    isImagePreview.value = false;

    if (isImageFile(entry.name)) {
        isImagePreview.value = true;
        fileLoading.value = false;
        return;
    }

    try {
        const text = await fsService.read(entry.path);
        fileContent.value = text;
        editedContent.value = text;
        fileLoading.value = false;
    } catch (err) {
        fileContent.value = `Error: ${err}`;
        editedContent.value = '';
        fileLoading.value = false;
    }
};

/** Write editedContent back to the server via /api/fs/write */
export const saveFile = async () => {
    const entry = selectedFsEntry.value;
    if (!entry || entry.isDir || fileSaving.value) return;
    fileSaving.value = true;
    fileSaveMsg.value = '';
    try {
        await fsService.write(entry.path, editedContent.value);
        fileContent.value = editedContent.value;
        fileSaving.value = false;
        fileSaveMsg.value = t('app.toast.fileSaved', ui.language.value);
        if (_saveMsgTimeout) clearTimeout(_saveMsgTimeout);
        _saveMsgTimeout = setTimeout(() => {
            fileSaveMsg.value = '';
        }, 2000);
    } catch (err) {
        console.error('[fs] write error:', err);
        fileSaving.value = false;
        fileSaveMsg.value = t('app.toast.fileSaveFailed', ui.language.value, { err: String(err) });
    }
};

export const loadFlatFiles = async () => {
    const isSearching = searchQuery.value !== '' || selectedFilterTag.value !== 'all';
    if (!isSearching) {
        flatFiles.value = [];
        flatFilesLoading.value = false;
        return;
    }

    _crawlCounter++;
    const currentCrawl = _crawlCounter;
    flatFilesLoading.value = true;
    try {
        const files = await fsService.search(searchQuery.value, selectedFilterTag.value);
        if (currentCrawl === _crawlCounter) {
            flatFiles.value = files;
            flatFilesLoading.value = false;
        }
    } catch (err) {
        if (currentCrawl === _crawlCounter) {
            console.error('[search] error:', err);
            flatFilesLoading.value = false;
        }
    }
};

export const handleSearchChange = (query: string) => {
    searchQuery.value = query;
    if (_searchTimeout) {
        clearTimeout(_searchTimeout);
        _searchTimeout = null;
    }
    if (query === '' && selectedFilterTag.value === 'all') {
        flatFiles.value = [];
        flatFilesLoading.value = false;
        return;
    }
    _searchTimeout = setTimeout(() => {
        loadFlatFiles();
    }, 300);
};

export const handleFilterTagChange = (tag: 'all' | 'doc' | 'img' | 'code') => {
    selectedFilterTag.value = tag;
    if (_searchTimeout) {
        clearTimeout(_searchTimeout);
        _searchTimeout = null;
    }
    loadFlatFiles();
};

export const toggleFavorite = (path: string) => {
    const favs = favoriteFiles.value.includes(path)
        ? favoriteFiles.value.filter(p => p !== path)
        : [...favoriteFiles.value, path];
    favoriteFiles.value = favs;
    try {
        localStorage.setItem('fav-files', JSON.stringify(favs));
    } catch {
        /* ignore */
    }
};

export const copyFileContent = async () => {
    try {
        await navigator.clipboard.writeText(fileContent.value);
        ui.showToast(t('app.toast.copySuccess', ui.language.value));
    } catch (_) {
        ui.showToast(t('app.toast.copyFailed', ui.language.value));
    }
};

export const duplicateFile = async () => {
    const entry = selectedFsEntry.value;
    if (!entry) return;
    const dot = entry.name.lastIndexOf('.');
    const base = dot > 0 ? entry.name.slice(0, dot) : entry.name;
    const ext = dot > 0 ? entry.name.slice(dot) : '';
    const dir = entry.path.includes('/') ? entry.path.slice(0, entry.path.lastIndexOf('/') + 1) : '';
    const newPath = `${dir}${base}_copy${ext}`;
    try {
        await fsService.write(newPath, fileContent.value);
        ui.showToast(t('app.toast.fileDuplicated', ui.language.value));
        loadDir('', null);
    } catch (err) {
        ui.showToast(t('app.toast.fileDuplicateFailed', ui.language.value, { err: String(err) }));
    }
};

export const downloadFile = () => {
    const entry = selectedFsEntry.value;
    if (!entry) return;
    const blob = new Blob([fileContent.value], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = entry.name;
    a.click();
    URL.revokeObjectURL(url);
};

export const renameFile = async () => {
    const entry = selectedFsEntry.value;
    if (!entry) return;
    const newName = window.prompt(t('app.prompt.rename', ui.language.value), entry.name);
    if (!newName || newName === entry.name) return;
    const dir = entry.path.includes('/') ? entry.path.slice(0, entry.path.lastIndexOf('/') + 1) : '';
    const newPath = `${dir}${newName}`;
    try {
        // Write content to new path
        await fsService.write(newPath, fileContent.value);
        ui.showToast(t('app.toast.renameSuccess', ui.language.value));
        selectedFsEntry.value = { ...entry, name: newName, path: newPath };
        viewMode.value = 'list';
        loadDir('', null);
    } catch (err) {
        ui.showToast(t('app.toast.renameFailed', ui.language.value, { err: String(err) }));
    }
};

/**
 * Point the backend fs/git roots at a workspace and reload the tree,
 * restoring the cached tree (if any) to avoid UI flashing.
 */
export const switchFsContext = async (ws: Workspace) => {
    try {
        await fsService.setContext(ws.path);
    } catch (err) {
        console.error('[context] set error:', err);
    }

    _treeCacheKey = ws.id;
    const cached = _treeCache[ws.id] || [];
    fsEntries.value = cached;
    selectedFsEntry.value = null;
    fileContent.value = '';
    editedContent.value = '';
    fsLoading.value = cached.length === 0;
    loadDir('', null);
};
