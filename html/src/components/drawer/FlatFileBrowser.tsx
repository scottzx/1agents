import { h } from 'preact';
import { FsEntry, getFileTag, formatBytes } from '../types';

interface FlatFileBrowserProps {
    flatFiles: FsEntry[];
    flatFilesLoading: boolean;
    searchQuery: string;
    selectedFilterTag: 'all' | 'doc' | 'img' | 'code';
    favoriteFiles: string[];
    onSearchQueryChange: (query: string) => void;
    onFilterTagChange: (tag: 'all' | 'doc' | 'img' | 'code') => void;
    onRefresh: () => void;
    onOpenFileDetail: (entry: FsEntry) => void;
}

export function FlatFileBrowser({
    flatFiles,
    flatFilesLoading,
    searchQuery,
    selectedFilterTag,
    favoriteFiles,
    onSearchQueryChange,
    onFilterTagChange,
    onRefresh,
    onOpenFileDetail,
}: FlatFileBrowserProps) {
    const filtered = flatFiles.filter(f => {
        const matchSearch =
            !searchQuery ||
            f.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
            f.path.toLowerCase().includes(searchQuery.toLowerCase());
        const tag = getFileTag(f.name);
        const matchTag = selectedFilterTag === 'all' || tag === selectedFilterTag;
        return matchSearch && matchTag;
    });

    return (
        <div class="flat-file-browser">
            {/* Search Input */}
            <div class="fb-search-wrap">
                <input
                    id="fb-search-input"
                    class="fb-search-input"
                    type="text"
                    placeholder="搜索文件名或路径..."
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
                        {tag === 'all' ? '全部' : tag === 'doc' ? '文档' : tag === 'img' ? '图片' : '代码'}
                    </button>
                ))}
                <button class="fb-tag fb-tag-refresh" onClick={onRefresh} title="刷新文件列表">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        style="width:12px;height:12px"
                    >
                        <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
                        <path d="M3 3v5h5" />
                    </svg>
                </button>
            </div>
            {/* File List */}
            {flatFilesLoading ? (
                <div class="fb-loading">
                    <div class="fb-loading-spinner" />
                    <span>扫描文件中…</span>
                </div>
            ) : filtered.length === 0 ? (
                <div class="fb-empty">没有匹配的文件</div>
            ) : (
                <div class="fb-file-list">
                    {filtered.map(f => {
                        const tag = getFileTag(f.name);
                        const ext = f.name.includes('.') ? f.name.split('.').pop()! : '?';
                        const isFav = favoriteFiles.includes(f.path);
                        return (
                            <div key={f.path} class="fb-file-row" onClick={() => onOpenFileDetail(f)}>
                                <div class={`fb-ext-badge fb-ext-${tag}`}>{ext.slice(0, 4)}</div>
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
                    <div class="fb-list-footer">共 {filtered.length} 个文件</div>
                </div>
            )}
        </div>
    );
}
