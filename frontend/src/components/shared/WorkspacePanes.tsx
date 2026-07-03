import { h } from 'preact';

import type { App } from '../app';
import type { Lang } from '../../i18n';
import { t } from '../../i18n';
import * as fs from '../../stores/fsStore';
import * as wsStore from '../../stores/workspaceStore';
import * as tabsStore from '../../stores/tabsStore';
import { fsService } from '../../services/fsService';
import { extractCcToken, extractCcRedirect } from '../../modules/cc-token';
import { FlatFileBrowser } from '../drawer/FlatFileBrowser';
import { FileDetailView } from '../drawer/FileDetailView';

/**
 * Workspace-scoped panes shared by the primary-pane host (ContentViewHost) and
 * the 助理 detail tabs (AssistantDetail). Extracting them here keeps a single
 * source of truth for the file browser / channels wiring and avoids a circular
 * import between the host and the assistant pages.
 *
 * Both read the fs / workspace signal stores directly, so they always reflect
 * the *active* workspace context — callers that want a specific workspace's
 * files/channels must make it active first (see AssistantDetail).
 */

/** Absolute path of the active workspace (fallback '.'), used as git/file root. */
export const activeWorkspacePath = (): string => {
    const ws = wsStore.workspaces.value.find(w => w.id === wsStore.activeWorkspaceId.value);
    return ws?.path || '.';
};

/** Desktop "fullscreen": promote the selected file to its own preview tab. */
function openSelectedAsPreview() {
    const entry = fs.selectedFsEntry.value;
    if (!entry) return;
    const base = activeWorkspacePath();
    const absolutePath = entry.path.startsWith('/') ? entry.path : `${base}/${entry.path}`;
    if (IS_DESKTOP) {
        tabsStore.openPreviewTab(absolutePath, entry.name);
    } else {
        const shareUrl = `${window.location.origin}${window.location.pathname}?preview=${encodeURIComponent(
            absolutePath
        )}`;
        window.open(shareUrl, '_blank');
    }
}

/**
 * The single-file preview/editor, wired to the fs store. Renders the currently
 * selected file (markdown/code/image) with the full toolbar (favorite / edit /
 * save / copy / download / rename / share / fullscreen). Shows a hint when
 * nothing is selected — used as the right half of a split browser.
 */
export function FilePreviewPane({
    app,
    language,
    hideBack,
}: {
    app: App;
    language: Lang;
    /** Hide the back-to-list button — set in the side-by-side split, where the
     *  file list is always on-screen so "返回" is redundant. */
    hideBack?: boolean;
}) {
    const selectedFsEntry = fs.selectedFsEntry.value;
    if (!selectedFsEntry) {
        return <div class="file-preview-empty">{t('assistant.detail.selectFile', language)}</div>;
    }
    return (
        <FileDetailView
            hideBack={hideBack}
            selectedFsEntry={selectedFsEntry}
            favoriteFiles={fs.favoriteFiles.value}
            detailFullscreen={fs.detailFullscreen.value}
            isEditingDetail={fs.isEditingDetail.value}
            fileContent={fs.fileContent.value}
            editedContent={fs.editedContent.value}
            fileLoading={fs.fileLoading.value}
            fileSaving={fs.fileSaving.value}
            fileSaveMsg={fs.fileSaveMsg.value}
            isImagePreview={fs.isImagePreview.value}
            imageUrl={fsService.imageUrl(selectedFsEntry.path)}
            onBackToList={() => {
                fs.viewMode.value = 'list';
                fs.detailFullscreen.value = false;
            }}
            onToggleFavorite={fs.toggleFavorite}
            onCopyContent={fs.copyFileContent}
            onDownloadFile={fs.downloadFile}
            onRenameFile={fs.renameFile}
            onToggleFullscreen={() => openSelectedAsPreview()}
            onShareFile={app.shareFile}
            onSaveFile={fs.saveFile}
            onToggleEditing={isEditing => (fs.isEditingDetail.value = isEditing)}
            onEditedContentChange={content => (fs.editedContent.value = content)}
            onOpenPreview={IS_DESKTOP ? (path, name) => tabsStore.openPreviewTab(path, name) : undefined}
            targetLine={fs.detailTargetLine.value ?? undefined}
            targetLineEnd={fs.detailTargetLineEnd.value ?? undefined}
            language={language}
        />
    );
}

/** File browser (list) → in-place detail/preview, scoped to the active workspace. */
export function FilesPane({ app, language }: { app: App; language: Lang }) {
    if (fs.viewMode.value === 'list') {
        return (
            <FlatFileBrowser
                flatFiles={fs.flatFiles.value}
                flatFilesLoading={fs.flatFilesLoading.value}
                searchQuery={fs.searchQuery.value}
                selectedFilterTag={fs.selectedFilterTag.value}
                favoriteFiles={fs.favoriteFiles.value}
                onSearchQueryChange={fs.handleSearchChange}
                onFilterTagChange={fs.handleFilterTagChange}
                onOpenFileDetail={fs.openFileDetail}
                fsEntries={fs.fsEntries.value}
                fsLoading={fs.fsLoading.value}
                onToggleFsDir={fs.toggleFsDir}
                language={language}
            />
        );
    }
    return <FilePreviewPane app={app} language={language} />;
}

/**
 * Two-pane workspace file browser: the tree/search list on the left, the live
 * preview on the right. Unlike FilesPane it shows both at once (no list↔detail
 * swap) — used by the 助理 详情 文件 tab.
 */
export function WorkspaceFilesSplit({ app, language }: { app: App; language: Lang }) {
    return (
        <div class="file-split">
            <div class="file-split-list">
                <FlatFileBrowser
                    flatFiles={fs.flatFiles.value}
                    flatFilesLoading={fs.flatFilesLoading.value}
                    searchQuery={fs.searchQuery.value}
                    selectedFilterTag={fs.selectedFilterTag.value}
                    favoriteFiles={fs.favoriteFiles.value}
                    onSearchQueryChange={fs.handleSearchChange}
                    onFilterTagChange={fs.handleFilterTagChange}
                    onOpenFileDetail={fs.openFileDetail}
                    fsEntries={fs.fsEntries.value}
                    fsLoading={fs.fsLoading.value}
                    onToggleFsDir={fs.toggleFsDir}
                    language={language}
                />
            </div>
            <div class="file-split-preview">
                <FilePreviewPane app={app} language={language} hideBack />
            </div>
        </div>
    );
}

/** IM 渠道 web component, scoped to the active workspace's cc-connect URL. */
export function ChannelsPane({ theme, language }: { theme: 'light' | 'dark'; language: Lang }) {
    const ccConnectUrl = wsStore.ccConnectUrl.value;
    if (!ccConnectUrl) return null;
    return (
        <div style="flex: 1; overflow: hidden; display: flex; flex-direction: column; height: 100%;">
            <cc-connect-panel
                id="cc-channels-panel"
                route={extractCcRedirect(ccConnectUrl)}
                theme={theme}
                lang={language}
                auth-token={extractCcToken(ccConnectUrl)}
                style="width: 100%; height: 100%; display: flex; flex-direction: column; min-height: 0; overflow: hidden;"
            />
        </div>
    );
}
