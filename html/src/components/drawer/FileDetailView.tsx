import { h } from 'preact';
import { marked } from 'marked';
import { FsEntry, getFileTag } from '../types';

interface FileDetailViewProps {
    selectedFsEntry: FsEntry;
    favoriteFiles: string[];
    detailFullscreen: boolean;
    isEditingDetail: boolean;
    fileContent: string;
    editedContent: string;
    fileLoading: boolean;
    fileSaving: boolean;
    fileSaveMsg: string;
    isImagePreview: boolean;
    imageDataUrl: string;

    onBackToList: () => void;
    onToggleFavorite: (path: string) => void;
    onCopyContent: () => void;
    onDuplicateFile: () => void;
    onDownloadFile: () => void;
    onRenameFile: () => void;
    onToggleFullscreen: () => void;
    onSaveFile: () => void;
    onToggleEditing: (isEditing: boolean) => void;
    onEditedContentChange: (content: string) => void;
}

export function FileDetailView({
    selectedFsEntry,
    favoriteFiles,
    detailFullscreen,
    isEditingDetail,
    fileContent,
    editedContent,
    fileLoading,
    fileSaving,
    fileSaveMsg,
    isImagePreview,
    imageDataUrl,

    onBackToList,
    onToggleFavorite,
    onCopyContent,
    onDuplicateFile,
    onDownloadFile,
    onRenameFile,
    onToggleFullscreen,
    onSaveFile,
    onToggleEditing,
    onEditedContentChange,
}: FileDetailViewProps) {
    const isFav = favoriteFiles.includes(selectedFsEntry.path);
    const tag = getFileTag(selectedFsEntry.name);
    const isImg = tag === 'img';
    const isMd = selectedFsEntry.name.endsWith('.md');

    return (
        <div class={`fb-detail-view ${detailFullscreen ? 'fullscreen' : ''}`}>
            {/* Detail Header */}
            <div class="fb-detail-header">
                <button class="fb-detail-back" onClick={onBackToList} title="返回列表">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <polyline points="15 18 9 12 15 6" />
                    </svg>
                </button>
                <div class="fb-detail-title-wrap">
                    <span class="fb-detail-filename">{selectedFsEntry.name}</span>
                    <span class="fb-detail-path">{selectedFsEntry.path}</span>
                </div>
                <div class="fb-detail-actions">
                    <button
                        class={`fb-icon-btn ${isFav ? 'active-fav' : ''}`}
                        onClick={() => onToggleFavorite(selectedFsEntry.path)}
                        title={isFav ? '取消收藏' : '收藏'}
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill={isFav ? 'currentColor' : 'none'}
                            stroke="currentColor"
                            stroke-width="2"
                        >
                            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                        </svg>
                    </button>
                    <button class="fb-icon-btn" onClick={onCopyContent} title="复制内容">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <rect width="14" height="14" x="8" y="8" rx="2" />
                            <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                        </svg>
                    </button>
                    <button class="fb-icon-btn" onClick={onDuplicateFile} title="复制文件">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
                            <rect width="8" height="4" x="8" y="2" rx="1" />
                        </svg>
                    </button>
                    <button class="fb-icon-btn" onClick={onDownloadFile} title="下载">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                            <polyline points="7 10 12 15 17 10" />
                            <line x1="12" x2="12" y1="15" y2="3" />
                        </svg>
                    </button>
                    <button class="fb-icon-btn" onClick={onRenameFile} title="重命名">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z" />
                        </svg>
                    </button>
                    <button class="fb-icon-btn" onClick={onToggleFullscreen} title="全屏">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3" />
                        </svg>
                    </button>
                </div>
            </div>
            {/* Save bar */}
            {isEditingDetail && (
                <div class="fb-detail-savebar">
                    {fileSaveMsg && <span class="fb-save-msg">{fileSaveMsg}</span>}
                    <button class="fb-save-btn" onClick={onSaveFile} disabled={fileSaving}>
                        {fileSaving ? '保存中…' : '保存 (Ctrl+S)'}
                    </button>
                    <button class="fb-icon-btn" onClick={() => onToggleEditing(false)} title="预览">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
                            <circle cx="12" cy="12" r="3" />
                        </svg>
                    </button>
                </div>
            )}
            {/* Content */}
            <div class="fb-detail-content">
                {fileLoading ? (
                    <div class="fb-loading">
                        <div class="fb-loading-spinner" />
                        <span>读取文件中…</span>
                    </div>
                ) : isImagePreview ? (
                    <div class="image-preview-container">
                        <img src={imageDataUrl} alt={selectedFsEntry.name} class="image-preview" />
                    </div>
                ) : isImg ? (
                    <div class="fb-img-preview">
                        <span class="fb-img-placeholder">🖼 {selectedFsEntry.name}</span>
                    </div>
                ) : isEditingDetail ? (
                    <textarea
                        class="fb-editor"
                        spellcheck={false}
                        value={editedContent}
                        onInput={e => onEditedContentChange((e.target as HTMLTextAreaElement).value)}
                    />
                ) : isMd ? (
                    <div
                        class="fb-md-render"
                        dangerouslySetInnerHTML={{ __html: marked(fileContent) as string }}
                        onClick={() => onToggleEditing(true)}
                    />
                ) : (
                    <pre class="fb-code-preview" onClick={() => onToggleEditing(true)}>
                        {fileContent}
                    </pre>
                )}
            </div>
        </div>
    );
}
