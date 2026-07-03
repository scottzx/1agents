import { h } from 'preact';
import { useMemo } from 'preact/hooks';
import { FsEntry, getFileTag, formatBytes } from '../types';
import { t, type Lang } from '../i18n';
import { uploadFileAction, openFolderAction } from '../../stores/fsStore';
import { openFsRenameModal, openFsDeleteModal } from '../../stores/modalStore';

interface FlatFileBrowserProps {
    flatFiles: FsEntry[];
    flatFilesLoading: boolean;
    searchQuery: string;
    selectedFilterTag: 'all' | 'doc' | 'img' | 'code' | 'fav';
    favoriteFiles: string[];
    onSearchQueryChange: (query: string) => void;
    onFilterTagChange: (tag: 'all' | 'doc' | 'img' | 'code' | 'fav') => void;
    onOpenFileDetail: (entry: FsEntry) => void;

    // Tree system props
    fsEntries: FsEntry[];
    fsLoading: boolean;
    onToggleFsDir: (entry: FsEntry) => void;

    language: Lang;
}

const TAG_KEYS: Record<'all' | 'doc' | 'img' | 'code' | 'fav', string> = {
    all: 'fileBrowser.tagAll',
    doc: 'fileBrowser.tagDoc',
    img: 'fileBrowser.tagImg',
    code: 'fileBrowser.tagCode',
    fav: 'fileBrowser.tagFav',
};

/** Local access (desktop/localhost) has direct filesystem reach, so the upload shortcut is only useful for remote access. */
const IS_LOCALHOST =
    typeof window !== 'undefined' &&
    !!window.location &&
    (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1');

/**
 * Sort children at each level: directories first, then files, alphabetically.
 * Implemented as a free function so it can be used inside useMemo without
 * closing over component scope (which would invalidate the cache every render).
 */
function sortTree(nodes: FsEntry[]): FsEntry[] {
    return [...nodes]
        .sort((a, b) => {
            if (a.isDir && !b.isDir) return -1;
            if (!a.isDir && b.isDir) return 1;
            return a.name.localeCompare(b.name);
        })
        .map(n => (n.children ? { ...n, children: sortTree(n.children) } : n));
}

export function FlatFileBrowser({
    flatFiles,
    flatFilesLoading,
    searchQuery,
    selectedFilterTag,
    favoriteFiles,
    onSearchQueryChange,
    onFilterTagChange,
    onOpenFileDetail,
    fsEntries,
    fsLoading,
    onToggleFsDir,
    language,
}: FlatFileBrowserProps) {
    const isSearching = searchQuery !== '' || selectedFilterTag !== 'all';

    // 1. Filter flat list for search/tag results. Memoized so typing in the
    //    search input doesn't recompute on every unrelated parent re-render.
    //    Lowercases the query once instead of per-row.
    const filtered = useMemo(() => {
        const q = searchQuery.toLowerCase();
        return flatFiles.filter(f => {
            if (q && !f.name.toLowerCase().includes(q) && !f.path.toLowerCase().includes(q)) {
                return false;
            }
            if (selectedFilterTag === 'fav') {
                return favoriteFiles.includes(f.path);
            }
            if (selectedFilterTag !== 'all' && getFileTag(f.name) !== selectedFilterTag) {
                return false;
            }
            return true;
        });
    }, [flatFiles, searchQuery, selectedFilterTag, favoriteFiles]);

    // 2. Sort the tree once per fsEntries change. Without this, every render
    //    re-spreads and re-sorts the full tree (O(N log N) per render) even
    //    when nothing about the tree itself changed.
    const sortedTree = useMemo(() => sortTree(fsEntries), [fsEntries]);

    // 3. Recursive Tree Renderer
    const renderTreeNodes = (nodes: FsEntry[], depth: number = 0) => {
        // nodes is pre-sorted by sortTree() at the top level (recursively, for
        // children), so we just map directly here.
        return nodes.map(node => {
            const isDir = node.isDir;
            const expanded = !!node.expanded;
            const ext = node.name.includes('.') ? node.name.split('.').pop()! : '?';
            const tag = getFileTag(node.name);
            const isFav = favoriteFiles.includes(node.path);

            if (isDir) {
                return (
                    <div key={node.path} class="fb-tree-node-wrap">
                        <div
                            class={`fb-file-row fb-row-dir ${expanded ? 'expanded' : ''}`}
                            style={`padding-left: ${depth * 14 + 8}px`}
                            onClick={() => onToggleFsDir(node)}
                        >
                            <svg
                                class={`fb-chevron-icon ${expanded ? 'expanded' : ''}`}
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="3"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="9 18 15 12 9 6" />
                            </svg>
                            <svg
                                class="fb-folder-icon"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                {expanded ? (
                                    <path d="M5 19h14a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2H9L7 3H3a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2z" />
                                ) : (
                                    <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                )}
                            </svg>
                            <div class="fb-file-info">
                                <span class="fb-file-name">{node.name}</span>
                            </div>
                            <div class="fb-row-actions">
                                <button
                                    class="fb-row-action-btn fb-action-rename"
                                    title={t('fileBrowser.rename', language)}
                                    onClick={(e: MouseEvent) => {
                                        e.stopPropagation();
                                        openFsRenameModal(node);
                                    }}
                                >
                                    <svg
                                        width="12"
                                        height="12"
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                                        <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                                    </svg>
                                </button>
                                <button
                                    class="fb-row-action-btn fb-action-delete"
                                    title={t('fileBrowser.delete', language)}
                                    onClick={(e: MouseEvent) => {
                                        e.stopPropagation();
                                        openFsDeleteModal(node);
                                    }}
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <polyline points="3 6 5 6 21 6" />
                                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                    </svg>
                                </button>
                            </div>
                        </div>
                        {expanded && node.children && (
                            <div class="fb-tree-children">
                                {node.children.length === 0 ? (
                                    <div class="fb-tree-empty-dir" style={`padding-left: ${(depth + 1) * 14 + 32}px`}>
                                        {t('fileBrowser.emptyDir', language)}
                                    </div>
                                ) : (
                                    renderTreeNodes(node.children, depth + 1)
                                )}
                            </div>
                        )}
                    </div>
                );
            } else {
                return (
                    <div
                        key={node.path}
                        class="fb-file-row fb-row-file"
                        style={`padding-left: ${depth * 14 + 26}px`}
                        onClick={() => onOpenFileDetail(node)}
                    >
                        <div class={`fb-ext-badge fb-ext-${tag}`}>{ext.slice(0, 3)}</div>
                        <div class="fb-file-info">
                            <span class="fb-file-name">{node.name}</span>
                            <span class="fb-file-meta">{formatBytes(node.size)}</span>
                        </div>
                        {isFav && (
                            <svg class="fb-star-indicator" viewBox="0 0 24 24" fill="currentColor">
                                <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                            </svg>
                        )}
                        <div class="fb-row-actions">
                            <button
                                class="fb-row-action-btn fb-action-rename"
                                title={t('fileBrowser.rename', language)}
                                onClick={(e: MouseEvent) => {
                                    e.stopPropagation();
                                    openFsRenameModal(node);
                                }}
                            >
                                <svg
                                    width="12"
                                    height="12"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                                    <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                                </svg>
                            </button>
                            <button
                                class="fb-row-action-btn fb-action-delete"
                                title={t('fileBrowser.delete', language)}
                                onClick={(e: MouseEvent) => {
                                    e.stopPropagation();
                                    openFsDeleteModal(node);
                                }}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="3 6 5 6 21 6" />
                                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                </svg>
                            </button>
                        </div>
                    </div>
                );
            }
        });
    };

    return (
        <div class="flat-file-browser">
            {/* Search Input */}
            <div class="fb-search-wrap">
                <input
                    id="fb-search-input"
                    class="fb-search-input"
                    type="text"
                    placeholder={t('fileBrowser.searchPlaceholder', language)}
                    value={searchQuery}
                    onInput={e => onSearchQueryChange((e.target as HTMLInputElement).value)}
                />
            </div>
            {/* Filter Tags */}
            <div class="fb-filter-tags">
                {(['all', 'doc', 'img', 'code', 'fav'] as const).map(tag => (
                    <button
                        key={tag}
                        class={`fb-tag ${tag === 'fav' ? 'fb-tag-fav' : ''} ${selectedFilterTag === tag ? 'active' : ''}`}
                        onClick={() => onFilterTagChange(tag)}
                    >
                        {tag === 'fav' && (
                            <svg
                                class="fb-tag-fav-icon"
                                viewBox="0 0 24 24"
                                fill="currentColor"
                                aria-hidden="true"
                            >
                                <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                            </svg>
                        )}
                        {t(TAG_KEYS[tag], language)}
                    </button>
                ))}

                {/* Upload Button (icon only, hidden on localhost access) */}
                {!IS_LOCALHOST && (
                    <button
                        class="fb-tag fb-upload-btn"
                        style="margin-left: auto; display: flex; align-items: center; justify-content: center; border-color: var(--accent-color); color: var(--accent-color);"
                        title={t('fileBrowser.upload', language)}
                        onClick={uploadFileAction}
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            style="width: 14px; height: 14px;"
                        >
                            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                            <polyline points="17 8 12 3 7 8" />
                            <line x1="12" y1="3" x2="12" y2="15" />
                        </svg>
                    </button>
                )}

                {/* Open in Finder / Explorer Button (Desktop mode only) */}
                {IS_DESKTOP && (
                    <button
                        class="fb-tag fb-open-folder-btn"
                        style={`display: flex; align-items: center; gap: 4px; border-color: var(--text-secondary); color: var(--text-secondary);${IS_LOCALHOST ? ' margin-left: auto;' : ''}`}
                        onClick={openFolderAction}
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            style="width: 14px; height: 14px;"
                        >
                            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                            <polyline points="15 3 21 3 21 9" />
                            <line x1="10" y1="14" x2="21" y2="3" />
                        </svg>
                        <span>
                            {navigator.userAgent.toLowerCase().includes('mac')
                                ? t('fileBrowser.openInFinder', language)
                                : t('fileBrowser.openInExplorer', language)}
                        </span>
                    </button>
                )}
            </div>
            {/* Main Content Area */}
            {isSearching ? (
                // ── SEARCH RESULTS / FLAT FILTER MODE ──
                flatFilesLoading ? (
                    <div class="fb-loading">
                        <div class="fb-loading-spinner" />
                        <span>{t('fileBrowser.searching', language)}</span>
                    </div>
                ) : filtered.length === 0 ? (
                    <div class="fb-empty">{t('fileBrowser.noMatch', language)}</div>
                ) : (
                    <div class="fb-file-list">
                        {filtered.map(f => {
                            const tag = getFileTag(f.name);
                            const ext = f.name.includes('.') ? f.name.split('.').pop()! : '?';
                            const isFav = favoriteFiles.includes(f.path);
                            return (
                                <div
                                    key={f.path}
                                    class="fb-file-row fb-row-file fb-search-row"
                                    onClick={() => onOpenFileDetail(f)}
                                >
                                    <div class={`fb-ext-badge fb-ext-${tag}`}>{ext.slice(0, 3)}</div>
                                    <div class="fb-file-info">
                                        <span class="fb-file-name" title={f.name}>
                                            {f.name}
                                        </span>
                                        <span class="fb-file-meta">{formatBytes(f.size)}</span>
                                    </div>
                                    {isFav && (
                                        <svg class="fb-star-indicator" viewBox="0 0 24 24" fill="currentColor">
                                            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                                        </svg>
                                    )}
                                    <div class="fb-row-actions" style="margin-left: 4px; margin-right: 4px;">
                                        <button
                                            class="fb-row-action-btn fb-action-rename"
                                            title={t('fileBrowser.rename', language)}
                                            onClick={(e: MouseEvent) => {
                                                e.stopPropagation();
                                                openFsRenameModal(f);
                                            }}
                                        >
                                            <svg
                                                width="12"
                                                height="12"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                                                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                                            </svg>
                                        </button>
                                        <button
                                            class="fb-row-action-btn fb-action-delete"
                                            title={t('fileBrowser.delete', language)}
                                            onClick={(e: MouseEvent) => {
                                                e.stopPropagation();
                                                openFsDeleteModal(f);
                                            }}
                                        >
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <polyline points="3 6 5 6 21 6" />
                                                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                            </svg>
                                        </button>
                                    </div>
                                    <div class="fb-info-icon" tabIndex={0}>
                                        <svg
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <circle cx="12" cy="12" r="10" />
                                            <line x1="12" y1="16" x2="12" y2="12" />
                                            <line x1="12" y1="8" x2="12.01" y2="8" />
                                        </svg>
                                        <div class="fb-info-tooltip">{f.path}</div>
                                    </div>
                                </div>
                            );
                        })}
                        <div class="fb-list-footer">
                            {t('fileBrowser.resultCount', language, { count: filtered.length })}
                            {filtered.length >= 1000 && (
                                <span class="fb-truncated-hint" style="color: var(--text-muted); margin-left: 8px;">
                                    {t('fileBrowser.truncated', language)}
                                </span>
                            )}
                        </div>
                    </div>
                )
            ) : // ── REGULAR FILE TREE MODE ──
            fsLoading && fsEntries.length === 0 ? (
                <div class="fb-loading">
                    <div class="fb-loading-spinner" />
                    <span>{t('fileBrowser.loading', language)}</span>
                </div>
            ) : fsEntries.length === 0 ? (
                <div class="fb-empty">{t('fileBrowser.empty', language)}</div>
            ) : (
                <div class="fb-file-list fb-tree-list">
                    {renderTreeNodes(sortedTree)}
                    <div class="fb-list-footer">{t('fileBrowser.loaded', language)}</div>
                </div>
            )}
        </div>
    );
}
