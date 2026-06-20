import { h } from 'preact';
import { useState } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { RightDrawerTab, Session } from '../types';
import { FlatFileBrowser } from './FlatFileBrowser';
import { FileDetailView } from './FileDetailView';
import { ThemeSettings } from './ThemeSettings';
import { GitPanel } from './GitPanel';
import { TaskList } from './TaskList';
import { t } from '../../i18n';
import { fsService } from '../../services/fsService';
import { extractCcToken, extractCcRedirect } from '../../modules/cc-token';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';

interface RightPanelProps {
    activeDrawerTab: RightDrawerTab;
    activeWorkspaceId: string;
    activeWorkspacePath: string;
    rightPanelWidth: number;
    closeDrawer: () => void;
    ccConnectUrl?: string;
    onSelectSession?: (session: Session) => void;
    /**
     * Overrides the aside's inline sizing. Desktop two-column passes a
     * flex/split style; mobile/legacy leaves it unset to keep the fixed
     * `rightPanelWidth` px behavior.
     */
    paneStyle?: string;

    // Context-dependent file actions (need app/workspace knowledge)
    onRefreshFlatFiles: () => void;
    onToggleFullscreen: () => void;
    onShareFile: () => void;
    onOpenPreview?: (path: string, name: string) => void;

    // Access token props
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;
}

export function RightPanel({
    activeDrawerTab,
    activeWorkspaceId,
    activeWorkspacePath,
    rightPanelWidth,
    closeDrawer,
    ccConnectUrl,
    onSelectSession,
    onRefreshFlatFiles,
    onToggleFullscreen,
    onShareFile,
    onOpenPreview,
    accessTokenExists,
    onGenerateAccessToken,
    onRevokeAccessToken,
    paneStyle,
}: RightPanelProps) {
    const [gitLoading, setGitLoading] = useState(false);
    const [gitRefreshFn, setGitRefreshFn] = useState<(() => void) | null>(null);
    const taskSelectedId = useSignal<string | null>(null);
    // Back stack for in-detail task→task navigation (e.g. clicking a #N
    // reference). The header back arrow pops this so it returns to the task you
    // came from (GitHub-style), falling back to the list when empty.
    const taskNavStack = useSignal<string[]>([]);
    const selectTask = (id: string | null) => {
        const cur = taskSelectedId.value;
        if (id === null) {
            taskNavStack.value = [];
        } else if (cur && cur !== id) {
            taskNavStack.value = [...taskNavStack.value, cur];
        }
        taskSelectedId.value = id;
    };
    const taskBack = () => {
        const stack = taskNavStack.value;
        if (stack.length > 0) {
            taskSelectedId.value = stack[stack.length - 1];
            taskNavStack.value = stack.slice(0, -1);
        } else {
            taskSelectedId.value = null;
        }
    };

    const language = ui.language.value;
    const theme = ui.theme.value;
    const viewMode = fs.viewMode.value;
    const selectedFsEntry = fs.selectedFsEntry.value;

    let isSpinning = false;
    if (activeDrawerTab === 'files') {
        isSpinning = fs.fsLoading.value || fs.flatFilesLoading.value;
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
            case 'tasks':
                return t('header.col.tasks', language);
            default:
                return '';
        }
    };

    // Desktop two-column passes an explicit flex/split style; otherwise fall
    // back to the legacy fixed px width (mobile full-width overlay).
    const asideStyle =
        activeDrawerTab === 'none' ? '' : paneStyle !== undefined ? paneStyle : `width: ${rightPanelWidth}px`;

    return (
        <aside class={`right-panel ${activeDrawerTab === 'none' ? 'collapsed' : ''}`} style={asideStyle}>
            <div class="panel-tabs-header">
                <span class="panel-tab-title">{getDrawerTitle(activeDrawerTab)}</span>
                <div class="panel-header-actions">
                    {activeDrawerTab === 'tasks' && taskSelectedId.value !== null && (
                        <div
                            class="panel-back-btn"
                            onClick={taskBack}
                            title={taskNavStack.value.length > 0 ? '返回上一个任务' : '返回列表'}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <line x1="19" y1="12" x2="5" y2="12" />
                                <polyline points="12 19 5 12 12 5" />
                            </svg>
                        </div>
                    )}
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
                    <div
                        class="panel-close-btn"
                        onClick={() => {
                            taskSelectedId.value = null;
                            taskNavStack.value = [];
                            closeDrawer();
                        }}
                        title={t('drawer.collapse', language)}
                    >
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

            {/* cc-connect channels panel (custom element, kept alive to avoid remount latency) */}
            <div
                class="panel-body-iframe"
                style={`flex: 1; overflow: hidden; display: ${
                    activeDrawerTab === 'channels' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%;`}
            >
                {ccConnectUrl && (
                    <cc-connect-panel
                        id="cc-channels-panel"
                        route={extractCcRedirect(ccConnectUrl)}
                        theme={theme}
                        lang={language}
                        auth-token={extractCcToken(ccConnectUrl)}
                        style="width: 100%; height: 100%; display: flex; flex-direction: column; min-height: 0; overflow: hidden;"
                    />
                )}
            </div>

            {/* Tasks panel (side-by-side with terminal/chat) */}
            <div
                class="panel-body-tasks"
                style={`flex: 1; overflow: hidden; display: ${
                    activeDrawerTab === 'tasks' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%; min-height: 0;`}
            >
                {activeDrawerTab === 'tasks' && (
                    <TaskList
                        workspaceId={activeWorkspaceId}
                        selectedTaskId={taskSelectedId.value}
                        onTaskSelect={selectTask}
                        onSelectSession={onSelectSession}
                    />
                )}
            </div>

            {/* Other drawer tab contents (files, git, settings) */}
            <div
                class="panel-body-scroll"
                style={`display: ${activeDrawerTab !== 'channels' && activeDrawerTab !== 'tasks' && activeDrawerTab !== 'none' ? 'flex' : 'none'};`}
            >
                {activeDrawerTab === 'files' &&
                    (viewMode === 'list' ? (
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
                    ) : (
                        selectedFsEntry && (
                            <FileDetailView
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
                                onToggleFullscreen={onToggleFullscreen}
                                onShareFile={onShareFile}
                                onSaveFile={fs.saveFile}
                                onToggleEditing={isEditing => (fs.isEditingDetail.value = isEditing)}
                                onEditedContentChange={content => (fs.editedContent.value = content)}
                                onOpenPreview={onOpenPreview}
                                targetLine={fs.detailTargetLine.value ?? undefined}
                                targetLineEnd={fs.detailTargetLineEnd.value ?? undefined}
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

                {activeDrawerTab === 'tasks' && (
                    <TaskList workspaceId={activeWorkspaceId} onSelectSession={onSelectSession} />
                )}

                {activeDrawerTab === 'settings' && (
                    <ThemeSettings
                        theme={theme}
                        toggleTheme={ui.toggleTheme}
                        language={language}
                        toggleLanguage={ui.toggleLanguage}
                        accessTokenExists={accessTokenExists}
                        onGenerateAccessToken={onGenerateAccessToken}
                        onRevokeAccessToken={onRevokeAccessToken}
                    />
                )}
            </div>
        </aside>
    );
}
