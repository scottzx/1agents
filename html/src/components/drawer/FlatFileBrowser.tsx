import { h } from 'preact';
import { FsEntry, getFileTag, formatBytes } from '../types';
import { t, type Lang } from '../i18n';

interface FlatFileBrowserProps {
    flatFiles: FsEntry[];
    flatFilesLoading: boolean;
    searchQuery: string;
    selectedFilterTag: 'all' | 'doc' | 'img' | 'code';
    favoriteFiles: string[];
    onSearchQueryChange: (query: string) => void;
    onFilterTagChange: (tag: 'all' | 'doc' | 'img' | 'code') => void;
    onOpenFileDetail: (entry: FsEntry) => void;

    // Tree system props
    fsEntries: FsEntry[];
    fsLoading: boolean;
    onToggleFsDir: (entry: FsEntry) => void;

    language: Lang;
}

const TAG_KEYS: Record<'all' | 'doc' | 'img' | 'code', string> = {
    all: 'fileBrowser.tagAll',
    doc: 'fileBrowser.tagDoc',
    img: 'fileBrowser.tagImg',
    code: 'fileBrowser.tagCode',
};

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

    // 1. Filter flat list for search/tag results
    const filtered = flatFiles.filter(f => {
        const matchSearch =
            !searchQuery ||
            f.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
            f.path.toLowerCase().includes(searchQuery.toLowerCase());
        const tag = getFileTag(f.name);
        const matchTag = selectedFilterTag === 'all' || tag === selectedFilterTag;
        return matchSearch && matchTag;
    });

    // 2. Recursive Tree Renderer
    const renderTreeNodes = (nodes: FsEntry[], depth: number = 0) => {
        // Sort folders first, then files alphabetically
        const sortedNodes = [...nodes].sort((a, b) => {
            if (a.isDir && !b.isDir) return -1;
            if (!a.isDir && b.isDir) return 1;
            return a.name.localeCompare(b.name);
        });

        return sortedNodes.map(node => {
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
                {(['all', 'doc', 'img', 'code'] as const).map(tag => (
                    <button
                        key={tag}
                        class={`fb-tag ${selectedFilterTag === tag ? 'active' : ''}`}
                        onClick={() => onFilterTagChange(tag)}
                    >
                        {t(TAG_KEYS[tag], language)}
                    </button>
                ))}
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
                                        <span class="fb-file-name">{f.name}</span>
                                        <span class="fb-file-meta">
                                            {formatBytes(f.size)} · {f.path}
                                        </span>
                                    </div>
                                    {isFav && (
                                        <svg class="fb-star-indicator" viewBox="0 0 24 24" fill="currentColor">
                                            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                                        </svg>
                                    )}
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
                    {renderTreeNodes(fsEntries)}
                    <div class="fb-list-footer">{t('fileBrowser.loaded', language)}</div>
                </div>
            )}
        </div>
    );
}
