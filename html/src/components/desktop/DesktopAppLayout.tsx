import { h, Component, Fragment } from 'preact';
import type { ITerminalOptions } from '@xterm/xterm';
import { isFullPageTab, isChat, AGENT_TYPE_LABELS } from '../types';
import { LeftSidebar } from '../sidebar/LeftSidebar';
import { NewChatHome } from '../chat/NewChatHome';
import { WorkspaceHeader } from '../header/WorkspaceHeader';
import { MiddleCanvas } from '../canvas/MiddleCanvas';
import { RightPanel } from '../drawer/RightPanel';
import { DiscoveryPanel } from '../drawer/DiscoveryPanel';
import { SystemSettings } from '../settings/SystemSettings';
import { FileDetailView } from '../drawer/FileDetailView';
import { TaskList } from '../drawer/TaskList';
import { fsService } from '../../services/fsService';
import { t } from '../../i18n';
import type { App, AppState } from '../app';
import * as ui from '../../stores/uiStore';
import {
    lightTermTheme,
    darkTermTheme,
    baseTermOptions,
    wsUrl,
    tokenUrl,
    clientOptions,
    flowControl,
} from '../terminal/terminalConfig';
import { getModuleByTab } from '../../modules/registry';
import { extractCcToken, extractCcRedirect } from '../../modules/cc-token';

interface DesktopAppLayoutProps {
    app: App;
    state: AppState;
}

export class DesktopAppLayout extends Component<DesktopAppLayoutProps> {
    render() {
        const { app, state } = this.props;
        const {
            workspaces,
            activeWorkspaceId,
            tabs,
            activeTabId,
            folders,
            workspacesLoading,
            activeDrawerTab,
            activeSession,
            tmuxMouseOn,
            ccProvidersUrl,
            ccConnectUrl,
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
            accessAuthRequired,
            sidebarCollapsedGroups,
        } = state;
        const language = ui.language.value;
        const theme = ui.theme.value;
        const leftSidebarOpen = ui.leftSidebarOpen.value;
        const leftSidebarWidth = ui.leftSidebarWidth.value;
        const keyboardVisible = ui.keyboardVisible.value;
        const isMobile = ui.isMobile.value;

        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
            fontSize: 13, // Desktop standard
        } as ITerminalOptions;

        const activeWorkspace = workspaces.find(w => w.id === activeWorkspaceId);
        const activeWorkspacePath = activeWorkspace?.path || '.';
        const activeTabObj = tabs.find(t => t.id === activeTabId);

        // Shell layout (LeftSidebar + WorkspaceHeader) is shared by the
        // project landing ('tasks') and the workbench ('terminal'). Dynamic
        // tabs (preview/browser) cover the whole content area without the shell.
        const isShell = activeTabId === 'tasks' || activeTabId === 'terminal';
        const isTerminal = activeTabId === 'terminal';
        const isDynamicTab = activeTabObj?.type === 'preview' || activeTabObj?.type === 'browser';

        return (
            <Fragment>
                {IS_DESKTOP && (
                    <div class="workspace-tabs-bar">
                        <div class="workspace-tabs-list">
                            {tabs.map(tab => {
                                const isActive = tab.id === activeTabId;
                                return (
                                    <div
                                        key={tab.id}
                                        class={`workspace-tab-item ${isActive ? 'active' : ''}`}
                                        onClick={() => app.selectTab(tab.id)}
                                    >
                                        <span class="tab-title">{tab.title}</span>
                                        {tab.closable && (
                                            <span
                                                class="workspace-tab-close"
                                                onClick={(e: MouseEvent) => {
                                                    e.stopPropagation();
                                                    app.closeTab(tab.id);
                                                }}
                                                title={t('common.closeTab', language)}
                                            >
                                                <svg
                                                    viewBox="0 0 24 24"
                                                    fill="none"
                                                    stroke="currentColor"
                                                    stroke-width="2.5"
                                                    stroke-linecap="round"
                                                    stroke-linejoin="round"
                                                >
                                                    <line x1="18" y1="6" x2="6" y2="18" />
                                                    <line x1="6" y1="6" x2="18" y2="18" />
                                                </svg>
                                            </span>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                        <button
                            class="workspace-tab-add-btn"
                            onClick={() => app.openBrowserTab('')}
                            title={t('common.openBrowserTab', language)}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <line x1="12" y1="5" x2="12" y2="19" />
                                <line x1="5" y1="12" x2="19" y2="12" />
                            </svg>
                        </button>
                    </div>
                )}

                <div
                    class="app-main-layout"
                    style="display: flex; flex: 1; flex-direction: row; overflow: hidden; width: 100%;"
                >
                    {/* [COLUMN 1]: LEFT Workspaces Tree Sidebar */}
                    {isShell && (
                        <Fragment>
                            <LeftSidebar
                                folders={folders}
                                workspaces={workspaces}
                                workspacesLoading={workspacesLoading}
                                leftSidebarOpen={leftSidebarOpen}
                                leftSidebarWidth={leftSidebarWidth}
                                activeWorkspaceId={activeWorkspaceId}
                                toggleLeftSidebar={ui.toggleLeftSidebar}
                                toggleFolder={app.toggleFolder}
                                toggleDrawerTab={app.toggleDrawerTab}
                                activeDrawerTab={activeDrawerTab}
                                activeDiscoveryCategory={state.discoveryCategory}
                                onSelectDiscoveryCategory={app.selectDiscoveryCategory}
                                onCreateWorkspace={app.openCreateWorkspacePicker}
                                onRenameWorkspace={ws => app.openRenameWorkspaceModal(ws)}
                                onDeleteWorkspace={app.deleteWorkspace}
                                onSelectWorkspace={ws => app.selectWorkspace(ws)}
                                onSelectSession={s => app.selectSession(s)}
                                onTerminalCreate={(wsId, cwd) => app.createTerminal(wsId, cwd)}
                                onTerminalKill={idx => app.killTerminal(idx)}
                                onRenameSession={s => app.openRenameSessionModal(s)}
                                onReorderFolders={app.reorderFolders}
                                language={language}
                                moduleNav={app.buildModuleNav()}
                                onChatCreate={app.openChatCreate}
                                onChatKill={app.killChatSession}
                                collapsedGroups={sidebarCollapsedGroups}
                                onToggleGroup={app.toggleSidebarGroup}
                                onStartNewChat={app.onStartNewChat}
                                activeTab={state.activeTab}
                            />

                            {/* Resizer: between LEFT sidebar and MIDDLE canvas */}
                            {leftSidebarOpen && (
                                <div
                                    class="resizer resizer-left"
                                    onMouseDown={(e: MouseEvent) => app.handleResizerDown('left', e)}
                                    title={t('app.resizer.leftTitle', language)}
                                />
                            )}
                        </Fragment>
                    )}

                    {/* [WORKSPACE MAIN CONTENT]: Occupies rest of screen */}
                    <div class="workspace-main-content">
                        {/*
                          [BACKGROUND LAYER]: the project task kanban is always
                          mounted underneath, so it persists across tab switches
                          and prevents the white-flash that occurred when the
                          'terminal' overlay unmounted/remounted. The 'tasks'
                          activeTabId sentinel means "no overlay; show the
                          background"; other activeTabIds (terminal /
                          preview-* / browser-*) render an overlay on top.
                        */}
                        <div class="kanban-background-layer">
                            <TaskList workspaceId={activeWorkspaceId} onSelectSession={s => app.selectSession(s)} />
                        </div>

                        {/*
                          [SHELL HEADER]: shown for both 'tasks' (project
                          landing) and 'terminal' (workbench), so the user
                          always has access to theme / language / drawer tabs
                          regardless of which view is on top.
                        */}
                        {isShell && (
                            <WorkspaceHeader
                                leftSidebarOpen={leftSidebarOpen}
                                toggleLeftSidebar={ui.toggleLeftSidebar}
                                activeDrawerTab={activeDrawerTab}
                                toggleDrawerTab={app.toggleDrawerTab}
                                activeTab={state.activeTab}
                                setActiveTab={app.setActiveTab}
                                theme={theme}
                                toggleTheme={ui.toggleTheme}
                                keyboardVisible={keyboardVisible}
                                workspaceName={activeWorkspace?.name || ''}
                                sessionName={activeSession?.name || ''}
                                tmuxMouseOn={tmuxMouseOn}
                                onTmuxMouseToggle={app.toggleTmuxMouse}
                                language={language}
                                moduleNav={app.buildModuleNav()}
                                hasChatSession={folders.some(
                                    f => f.id === activeWorkspaceId && f.sessions.some(isChat)
                                )}
                            />
                        )}

                        {/*
                          [TERMINAL BODY]: rendered on top of the kanban. For
                          'tasks' this is omitted, so the kanban background is
                          the visible body. Full-page drawer tabs (providers /
                          skills / discovery / settings) replace the canvas +
                          right panel; otherwise the standard MiddleCanvas +
                          Resizer + RightPanel is shown.
                        */}
                        {isTerminal && (
                            <div
                                class={`workspace-body-container ${activeDrawerTab !== 'none' && !isFullPageTab(activeDrawerTab) ? 'drawer-open' : ''}`}
                            >
                                {isFullPageTab(activeDrawerTab) ? (
                                    <div
                                        style={{
                                            flex: 1,
                                            display: 'flex',
                                            flexDirection: 'column',
                                            height: '100%',
                                            width: '100%',
                                            overflow: 'hidden',
                                        }}
                                    >
                                        {activeDrawerTab === 'providers' && ccProvidersUrl && (
                                            <cc-connect-panel
                                                id="cc-providers-panel"
                                                route={extractCcRedirect(ccProvidersUrl, '/providers')}
                                                theme={theme}
                                                lang={language}
                                                auth-token={extractCcToken(ccProvidersUrl)}
                                                style={{
                                                    width: '100%',
                                                    height: '100%',
                                                    display: 'flex',
                                                    flexDirection: 'column',
                                                    minHeight: 0,
                                                    overflow: 'hidden',
                                                }}
                                            />
                                        )}
                                        {activeDrawerTab === 'skills' &&
                                            (() => {
                                                const skillsMod = getModuleByTab('skills');
                                                const initialRoute =
                                                    skillsMod && state.activeModulePath && activeDrawerTab === 'skills'
                                                        ? state.activeModulePath
                                                        : skillsMod
                                                          ? skillsMod.entryPath
                                                          : '/overview';
                                                return (
                                                    <skills-panel
                                                        id="skills-panel"
                                                        route={initialRoute}
                                                        theme={theme}
                                                        lang={language}
                                                        style={{
                                                            width: '100%',
                                                            height: '100%',
                                                            display: 'flex',
                                                            flexDirection: 'column',
                                                            minHeight: 0,
                                                            overflow: 'hidden',
                                                        }}
                                                    />
                                                );
                                            })()}
                                        {activeDrawerTab === 'discovery' && (
                                            <div style={{ flex: 1, overflowY: 'auto', padding: '24px' }}>
                                                <DiscoveryPanel
                                                    onOpenBrowserTab={IS_DESKTOP ? app.openBrowserTab : undefined}
                                                    language={language}
                                                    scrollToCategory={state.discoveryCategory}
                                                />
                                            </div>
                                        )}
                                        {activeDrawerTab === 'settings' && (
                                            <div
                                                style={{
                                                    flex: 1,
                                                    overflow: 'hidden',
                                                    display: 'flex',
                                                    flexDirection: 'column',
                                                    height: '100%',
                                                }}
                                            >
                                                <SystemSettings
                                                    theme={theme}
                                                    toggleTheme={ui.toggleTheme}
                                                    language={language}
                                                    toggleLanguage={ui.toggleLanguage}
                                                    tmuxMouseOn={tmuxMouseOn}
                                                    onTmuxMouseToggle={app.toggleTmuxMouse}
                                                    accessTokenExists={accessAuthRequired}
                                                    onGenerateAccessToken={app.generateAccessToken}
                                                    onRevokeAccessToken={app.revokeAccessToken}
                                                    activeCategory={state.activeSettingsCategory}
                                                />
                                            </div>
                                        )}
                                    </div>
                                ) : (
                                    <Fragment>
                                        {/* [COLUMN 2]: MIDDLE main workspace Terminal container */}
                                        {state.activeTab === 'new_chat' ? (
                                            <NewChatHome
                                                workspaces={workspaces}
                                                activeWorkspaceId={activeWorkspaceId}
                                                onSelectWorkspace={ws => app.selectWorkspace(ws)}
                                                onSubmitChat={(wsId, agentType, prompt) => {
                                                    const name = `${AGENT_TYPE_LABELS[agentType] ?? agentType} 会话`;
                                                    app.createChatSession(wsId, name, agentType, prompt);
                                                }}
                                                language={language}
                                            />
                                        ) : (
                                            <MiddleCanvas
                                                activeTab={
                                                    state.activeTab as 'terminal' | 'agents' | 'console' | 'folders'
                                                }
                                                wsUrl={wsUrl}
                                                tokenUrl={tokenUrl}
                                                clientOptions={clientOptions}
                                                termOptions={termOptions}
                                                flowControl={flowControl}
                                                isMobile={isMobile}
                                                onMobileDetect={isMobile => (ui.isMobile.value = isMobile)}
                                                onKeyboardStateChange={app.handleKeyboardStateChange}
                                                tmuxMouseOn={tmuxMouseOn}
                                                onTmuxMouseToggle={app.toggleTmuxMouse}
                                                language={language}
                                                activeChatSession={
                                                    activeSession && isChat(activeSession) ? activeSession : null
                                                }
                                                pendingInitialMessage={state.pendingInitialMessage}
                                                onClearPendingInitialMessage={app.clearPendingInitialMessage}
                                            />
                                        )}

                                        {/* Resizer: between MIDDLE canvas and RIGHT panel */}
                                        {activeDrawerTab !== 'none' && (
                                            <div
                                                class="resizer resizer-right"
                                                onMouseDown={(e: MouseEvent) => app.handleResizerDown('right', e)}
                                                title={t('app.resizer.rightTitle', language)}
                                            />
                                        )}

                                        {/* [COLUMN 3]: RIGHT side dynamic sliding drawer panel */}
                                        <RightPanel
                                            activeDrawerTab={activeDrawerTab}
                                            activeWorkspaceId={activeWorkspaceId}
                                            activeWorkspacePath={activeWorkspacePath}
                                            rightPanelWidth={ui.rightPanelWidth.value}
                                            closeDrawer={() => app.setState({ activeDrawerTab: 'none' })}
                                            ccConnectUrl={ccConnectUrl}
                                            theme={theme}
                                            toggleTheme={ui.toggleTheme}
                                            language={language}
                                            toggleLanguage={ui.toggleLanguage}
                                            flatFiles={flatFiles}
                                            flatFilesLoading={flatFilesLoading}
                                            searchQuery={searchQuery}
                                            selectedFilterTag={selectedFilterTag}
                                            viewMode={viewMode}
                                            favoriteFiles={favoriteFiles}
                                            detailFullscreen={detailFullscreen}
                                            isEditingDetail={isEditingDetail}
                                            selectedFsEntry={selectedFsEntry}
                                            fileContent={fileContent}
                                            editedContent={editedContent}
                                            fileLoading={fileLoading}
                                            fileSaving={fileSaving}
                                            fileSaveMsg={fileSaveMsg}
                                            isImagePreview={isImagePreview}
                                            imageUrl={fsService.imageUrl(selectedFsEntry?.path ?? '')}
                                            onSearchQueryChange={app.handleSearchChange}
                                            onFilterTagChange={app.handleFilterTagChange}
                                            onRefreshFlatFiles={async () => {
                                                app.loadDir('', null);
                                                const isSearching = searchQuery !== '' || selectedFilterTag !== 'all';
                                                if (isSearching) {
                                                    app.loadFlatFiles();
                                                }
                                                try {
                                                    await app.checkAccessStatus();
                                                    await Promise.all([app.loadWorkspaces(true), app.loadTerminals()]);

                                                    const { workspaces, activeWorkspaceId } = app.state;
                                                    if (!activeWorkspaceId && workspaces.length > 0) {
                                                        await app.selectWorkspace(workspaces[0]);
                                                    } else if (activeWorkspaceId) {
                                                        await Promise.all([
                                                            app.loadCcConnectUrl(),
                                                            app.loadCcProvidersUrl(),
                                                        ]);
                                                    }
                                                } catch (e) {
                                                    console.error('Failed to reconnect/refresh:', e);
                                                }
                                            }}
                                            onOpenFileDetail={app.openFileDetail}
                                            onBackToList={() =>
                                                app.setState({ viewMode: 'list', detailFullscreen: false })
                                            }
                                            onToggleFavorite={app.toggleFavorite}
                                            onCopyContent={app.copyFileContent}
                                            onDownloadFile={app.downloadFile}
                                            onRenameFile={app.renameFile}
                                            onToggleFullscreen={() => {
                                                const { selectedFsEntry, workspaces, activeWorkspaceId } = app.state;
                                                if (selectedFsEntry) {
                                                    const activeWorkspace = workspaces.find(
                                                        w => w.id === activeWorkspaceId
                                                    );
                                                    const activeWorkspacePath = activeWorkspace?.path || '.';
                                                    const absolutePath = selectedFsEntry.path.startsWith('/')
                                                        ? selectedFsEntry.path
                                                        : `${activeWorkspacePath}/${selectedFsEntry.path}`;
                                                    if (IS_DESKTOP) {
                                                        app.openPreviewTab(absolutePath, selectedFsEntry.name);
                                                    } else {
                                                        const shareUrl = `${window.location.origin}${
                                                            window.location.pathname
                                                        }?preview=${encodeURIComponent(absolutePath)}`;
                                                        window.open(shareUrl, '_blank');
                                                    }
                                                }
                                            }}
                                            onShareFile={app.shareFile}
                                            onSaveFile={app.saveFile}
                                            onToggleEditing={isEditing => app.setState({ isEditingDetail: isEditing })}
                                            onEditedContentChange={content => app.setState({ editedContent: content })}
                                            onOpenPreview={
                                                IS_DESKTOP ? (path, name) => app.openPreviewTab(path, name) : undefined
                                            }
                                            fsEntries={state.fsEntries}
                                            fsLoading={state.fsLoading}
                                            onToggleFsDir={app.toggleFsDir}
                                            accessTokenExists={state.accessAuthRequired}
                                            onGenerateAccessToken={app.generateAccessToken}
                                            onRevokeAccessToken={app.revokeAccessToken}
                                        />
                                    </Fragment>
                                )}
                            </div>
                        )}

                        {/*
                          [DYNAMIC TAB OVERLAY]: preview / browser tabs cover
                          the whole main content (no shell chrome), sitting on
                          top of the kanban background.
                        */}
                        {!isShell && isDynamicTab && (
                            <div class="workspace-body-container dynamic-tab-view">
                                {activeTabObj?.type === 'preview' && (
                                    <div
                                        class="fb-detail-view-tab-container"
                                        style="flex: 1; height: 100%; display: flex; flex-direction: column; overflow: hidden; background-color: var(--bg-panel); padding: 12px 16px;"
                                    >
                                        {selectedFsEntry ? (
                                            <FileDetailView
                                                selectedFsEntry={selectedFsEntry}
                                                favoriteFiles={favoriteFiles}
                                                detailFullscreen={false}
                                                isEditingDetail={isEditingDetail}
                                                fileContent={fileContent}
                                                editedContent={editedContent}
                                                fileLoading={fileLoading}
                                                fileSaving={fileSaving}
                                                fileSaveMsg={fileSaveMsg}
                                                isImagePreview={isImagePreview}
                                                imageUrl={fsService.imageUrl(selectedFsEntry.path)}
                                                onBackToList={() => app.closeTab(activeTabId)}
                                                onToggleFavorite={app.toggleFavorite}
                                                onCopyContent={app.copyFileContent}
                                                onDownloadFile={app.downloadFile}
                                                onRenameFile={app.renameFile}
                                                onToggleFullscreen={() => {}}
                                                onShareFile={app.shareFile}
                                                onSaveFile={app.saveFile}
                                                onToggleEditing={isEditing =>
                                                    app.setState({ isEditingDetail: isEditing })
                                                }
                                                onEditedContentChange={content =>
                                                    app.setState({ editedContent: content })
                                                }
                                                onOpenPreview={
                                                    IS_DESKTOP
                                                        ? (path, name) => app.openPreviewTab(path, name)
                                                        : undefined
                                                }
                                                isStandalone={true}
                                                language={language}
                                            />
                                        ) : (
                                            <div class="fb-loading">
                                                <div class="fb-loading-spinner" />
                                                <span>{t('app.loading.preview', language)}</span>
                                            </div>
                                        )}
                                    </div>
                                )}
                                <div
                                    class="builtin-browser-container"
                                    style={{
                                        flex: 1,
                                        height: '100%',
                                        display: activeTabObj?.type === 'browser' ? 'flex' : 'none',
                                        flexDirection: 'column',
                                        overflow: 'hidden',
                                    }}
                                >
                                    {tabs.filter(t => t.type === 'browser').map(t => app.renderBuiltinBrowser(t))}
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            </Fragment>
        );
    }
}
