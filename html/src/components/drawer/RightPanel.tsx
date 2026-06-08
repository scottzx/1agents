import { h } from 'preact';
import { useState, useCallback } from 'preact/hooks';
import { FsEntry, RightDrawerTab } from '../types';
import { FlatFileBrowser } from './FlatFileBrowser';
import { FileDetailView } from './FileDetailView';
import { ThemeSettings } from './ThemeSettings';
import { GitPanel } from './GitPanel';
import { t, type Lang } from '../i18n';

interface RightPanelProps {
    activeDrawerTab: RightDrawerTab;
    activeWorkspaceId: string;
    activeWorkspacePath: string;
    rightPanelWidth: number;
    closeDrawer: () => void;
    ccConnectUrl?: string;

    // Theme settings props
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
    language: Lang;
    toggleLanguage: (lang: Lang) => void;

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
    imageUrl: string;

    // File Handlers
    onSearchQueryChange: (query: string) => void;
    onFilterTagChange: (tag: 'all' | 'doc' | 'img' | 'code') => void;
    onRefreshFlatFiles: () => void;
    onOpenFileDetail: (entry: FsEntry) => void;
    onBackToList: () => void;
    onToggleFavorite: (path: string) => void;
    onCopyContent: () => void;
    onDownloadFile: () => void;
    onRenameFile: () => void;
    onToggleFullscreen: () => void;
    onShareFile: () => void;
    onSaveFile: () => void;
    onToggleEditing: (isEditing: boolean) => void;
    onEditedContentChange: (content: string) => void;
    onOpenPreview?: (path: string, name: string) => void;

    // Access token props
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;

    // Tree system props
    fsEntries: FsEntry[];
    fsLoading: boolean;
    onToggleFsDir: (entry: FsEntry) => void;
}

export function RightPanel({
    activeDrawerTab,
    activeWorkspaceId,
    activeWorkspacePath,
    rightPanelWidth,
    closeDrawer,
    ccConnectUrl,

    theme,
    toggleTheme,
    language,
    toggleLanguage,

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
    imageUrl,

    onSearchQueryChange,
    onFilterTagChange,
    onRefreshFlatFiles,
    onOpenFileDetail,
    onBackToList,
    onToggleFavorite,
    onCopyContent,
    onDownloadFile,
    onRenameFile,
    onToggleFullscreen,
    onShareFile,
    onSaveFile,
    onToggleEditing,
    onEditedContentChange,
    onOpenPreview,

    // Access token props
    accessTokenExists,
    onGenerateAccessToken,
    onRevokeAccessToken,

    // Tree props
    fsEntries,
    fsLoading,
    onToggleFsDir,
}: RightPanelProps) {
    const [gitLoading, setGitLoading] = useState(false);
    const [gitRefreshFn, setGitRefreshFn] = useState<(() => void) | null>(null);

    // Stable callback identity so FlatFileBrowser's referential-equality
    // short-circuits (and the parent toggleFsDir's stable reference downstream)
    // don't churn on every RightPanel re-render (e.g. when activeDrawerTab or
    // gitLoading toggles). Without useCallback, every RightPanel render would
    // hand FlatFileBrowser a new onToggleFsDir prop reference and force a
    // re-render of the entire tree.
    const handleToggleFsDir = useCallback(
        (entry: FsEntry) => onToggleFsDir(entry),
        [onToggleFsDir]
    );

    let isSpinning = false;
    if (activeDrawerTab === 'files') {
        isSpinning = fsLoading || flatFilesLoading;
    } else if (activeDrawerTab === 'git') {
        isSpinning = gitLoading;
    }
    const getDrawerTitle = (tab: RightDrawerTab) => {
        switch (tab) {
            case 'files':
                return t('drawer.title.files', language);
            case 'git':
                return t('drawer.title.git', language);
            case 'channels':
                return t('drawer.title.channels', language);
            case 'providers':
                return t('drawer.title.providers', language);
            case 'settings':
                return t('drawer.title.settings', language);
            case 'skills':
                return t('drawer.title.skills', language);
            case 'discovery':
                return t('drawer.title.discovery', language);
            default:
                return '';
        }
    };

    const getCcConnectIframeUrl = (url?: string) => {
        if (!url) return '';
        if (url.startsWith('/')) {
            return url;
        }
        try {
            const parsed = new URL(url);
            if (parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1') {
                parsed.hostname = window.location.hostname;
            }
            return parsed.toString();
        } catch (e) {
            return url;
        }
    };

    return (
        <aside
            class={`right-panel ${activeDrawerTab === 'none' ? 'collapsed' : ''}`}
            style={activeDrawerTab !== 'none' ? `width: ${rightPanelWidth}px` : ''}
        >
            <div class="panel-tabs-header">
                <span class="panel-tab-title">{getDrawerTitle(activeDrawerTab)}</span>
                <div class="panel-header-actions">
                    {(activeDrawerTab === 'files' || activeDrawerTab === 'git') && (
                        <div
                            class={`panel-refresh-btn ${isSpinning ? 'spinning' : ''}`}
                            onClick={activeDrawerTab === 'files' ? onRefreshFlatFiles : () => gitRefreshFn?.()}
                            title={
                                activeDrawerTab === 'files'
                                    ? t('drawer.refresh.files', language)
                                    : t('drawer.refresh.git', language)
                            }
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.72 2.78L21 8" />
                                <polyline points="21 3 21 8 16 8" />
                            </svg>
                        </div>
                    )}
                    <div class="panel-close-btn" onClick={closeDrawer} title={t('drawer.collapse', language)}>
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
            </div>

            {/* cc-connect channels iframe container is kept alive to avoid 1-2s load latency */}
            <div
                class="panel-body-iframe"
                style={`flex: 1; overflow: hidden; display: ${
                    activeDrawerTab === 'channels' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%;`}
            >
                {ccConnectUrl && (
                    <iframe
                        id="cc-connect-iframe"
                        src={getCcConnectIframeUrl(ccConnectUrl)}
                        onLoad={e => {
                            const iframe = e.target as HTMLIFrameElement;
                            if (iframe && iframe.contentWindow) {
                                iframe.contentWindow.postMessage({ type: 'THEME_CHANGE', theme }, '*');
                                iframe.contentWindow.postMessage({ type: 'LANG_CHANGE', lang: language }, '*');
                            }
                        }}
                        style={{ width: '100%', height: '100%', border: 'none', background: 'transparent' }}
                    />
                )}
            </div>

            {/* Other drawer tab contents (files, git, settings) */}
            <div
                class="panel-body-scroll"
                style={`display: ${activeDrawerTab !== 'channels' && activeDrawerTab !== 'none' ? 'flex' : 'none'};`}
            >
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
                            onOpenFileDetail={onOpenFileDetail}
                            fsEntries={fsEntries}
                            fsLoading={fsLoading}
                            onToggleFsDir={handleToggleFsDir}
                            language={language}
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
                                imageUrl={imageUrl}
                                onBackToList={onBackToList}
                                onToggleFavorite={onToggleFavorite}
                                onCopyContent={onCopyContent}
                                onDownloadFile={onDownloadFile}
                                onRenameFile={onRenameFile}
                                onToggleFullscreen={onToggleFullscreen}
                                onShareFile={onShareFile}
                                onSaveFile={onSaveFile}
                                onToggleEditing={onToggleEditing}
                                onEditedContentChange={onEditedContentChange}
                                onOpenPreview={onOpenPreview}
                                language={language}
                            />
                        )
                    ))}

                {activeDrawerTab === 'git' && (
                    <GitPanel
                        workdir={activeWorkspacePath}
                        activeWorkspaceId={activeWorkspaceId}
                        onLoadingChange={setGitLoading}
                        onRegisterRefresh={fn => setGitRefreshFn(() => fn)}
                        language={language}
                    />
                )}

                {activeDrawerTab === 'settings' && (
                    <ThemeSettings
                        theme={theme}
                        toggleTheme={toggleTheme}
                        language={language}
                        toggleLanguage={toggleLanguage}
                        accessTokenExists={accessTokenExists}
                        onGenerateAccessToken={onGenerateAccessToken}
                        onRevokeAccessToken={onRevokeAccessToken}
                    />
                )}
            </div>
        </aside>
    );
}
