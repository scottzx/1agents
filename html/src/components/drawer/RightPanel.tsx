import { h } from 'preact';
import { FsEntry, RightDrawerTab } from '../types';
import { FlatFileBrowser } from './FlatFileBrowser';
import { FileDetailView } from './FileDetailView';
import { ThemeSettings } from './ThemeSettings';
import { GitPanel } from './GitPanel';

interface RightPanelProps {
    activeDrawerTab: RightDrawerTab;
    rightPanelWidth: number;
    closeDrawer: () => void;

    // Theme settings props
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;

    // File Browser / Detail State
    flatFiles: FsEntry[];
    flatFilesLoading: boolean;
    searchQuery: string;
    selectedFilterTag: 'all' | 'doc' | 'img' | 'code';
    viewMode: 'list' | 'detail';
    favoriteFiles: string[];
    detailFullscreen: boolean;
    isEditingDetail: boolean;
    selectedFsEntry: FsEntry | null;
    fileContent: string;
    editedContent: string;
    fileLoading: boolean;
    fileSaving: boolean;
    fileSaveMsg: string;
    isImagePreview: boolean;
    imageDataUrl: string;

    // File Handlers
    onSearchQueryChange: (query: string) => void;
    onFilterTagChange: (tag: 'all' | 'doc' | 'img' | 'code') => void;
    onRefreshFlatFiles: () => void;
    onOpenFileDetail: (entry: FsEntry) => void;
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

    // Tree system props
    fsEntries: FsEntry[];
    fsLoading: boolean;
    onToggleFsDir: (entry: FsEntry) => void;
}

export function RightPanel({
    activeDrawerTab,
    rightPanelWidth,
    closeDrawer,

    theme,
    toggleTheme,

    flatFiles,
    flatFilesLoading,
    searchQuery,
    selectedFilterTag,
    viewMode,
    favoriteFiles,
    detailFullscreen,
    isEditingDetail,
    selectedFsEntry,
    fileContent,
    editedContent,
    fileLoading,
    fileSaving,
    fileSaveMsg,
    isImagePreview,
    imageDataUrl,

    onSearchQueryChange,
    onFilterTagChange,
    onRefreshFlatFiles,
    onOpenFileDetail,
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

    // Tree props
    fsEntries,
    fsLoading,
    onToggleFsDir,
}: RightPanelProps) {
    const getDrawerTitle = (tab: RightDrawerTab) => {
        switch (tab) {
            case 'files':
                return '文件浏览器 (Files)';
            case 'git':
                return '版本控制 (Git)';
            case 'settings':
                return '系统终端设置 (Settings)';
            default:
                return '';
        }
    };

    return (
        <aside
            class={`right-panel ${activeDrawerTab === 'none' ? 'collapsed' : ''}`}
            style={activeDrawerTab !== 'none' ? `width: ${rightPanelWidth}px` : ''}
        >
            <div class="panel-tabs-header">
                <span class="panel-tab-title">{getDrawerTitle(activeDrawerTab)}</span>
                <div class="panel-close-btn" onClick={closeDrawer} title="收起面板">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <line x1="18" x2="6" y1="6" y2="18" />
                        <line x1="6" x2="18" y1="6" y2="18" />
                    </svg>
                </div>
            </div>

            <div class="panel-body-scroll">
                {activeDrawerTab === 'files' &&
                    (viewMode === 'list' ? (
                        <FlatFileBrowser
                            flatFiles={flatFiles}
                            flatFilesLoading={flatFilesLoading}
                            searchQuery={searchQuery}
                            selectedFilterTag={selectedFilterTag}
                            favoriteFiles={favoriteFiles}
                            onSearchQueryChange={onSearchQueryChange}
                            onFilterTagChange={onFilterTagChange}
                            onRefresh={onRefreshFlatFiles}
                            onOpenFileDetail={onOpenFileDetail}
                            fsEntries={fsEntries}
                            fsLoading={fsLoading}
                            onToggleFsDir={onToggleFsDir}
                        />
                    ) : (
                        selectedFsEntry && (
                            <FileDetailView
                                selectedFsEntry={selectedFsEntry}
                                favoriteFiles={favoriteFiles}
                                detailFullscreen={detailFullscreen}
                                isEditingDetail={isEditingDetail}
                                fileContent={fileContent}
                                editedContent={editedContent}
                                fileLoading={fileLoading}
                                fileSaving={fileSaving}
                                fileSaveMsg={fileSaveMsg}
                                isImagePreview={isImagePreview}
                                imageDataUrl={imageDataUrl}
                                onBackToList={onBackToList}
                                onToggleFavorite={onToggleFavorite}
                                onCopyContent={onCopyContent}
                                onDuplicateFile={onDuplicateFile}
                                onDownloadFile={onDownloadFile}
                                onRenameFile={onRenameFile}
                                onToggleFullscreen={onToggleFullscreen}
                                onSaveFile={onSaveFile}
                                onToggleEditing={onToggleEditing}
                                onEditedContentChange={onEditedContentChange}
                            />
                        )
                    ))}

                {activeDrawerTab === 'git' && <GitPanel workdir="." />}

                {activeDrawerTab === 'settings' && <ThemeSettings theme={theme} toggleTheme={toggleTheme} />}
            </div>
        </aside>
    );
}
