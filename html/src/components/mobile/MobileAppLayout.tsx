import { h, Component } from 'preact';
import type { ITerminalOptions } from '@xterm/xterm';

import { LeftSidebar } from '../sidebar/LeftSidebar';
import { WorkspaceHeader } from '../header/WorkspaceHeader';
import { MiddleCanvas } from '../canvas/MiddleCanvas';
import { RightPanel } from '../drawer/RightPanel';
import { DiscoveryPanel } from '../drawer/DiscoveryPanel';
import { SystemSettings } from '../settings/SystemSettings';
import { FileDetailView } from '../drawer/FileDetailView';
import { fsService } from '../../services/fsService';
import { t } from '../../i18n';
import type { App, AppState } from '../app';
import {
    lightTermTheme,
    darkTermTheme,
    baseTermOptions,
    wsUrl,
    tokenUrl,
    clientOptions,
    flowControl,
} from '../terminal/terminalConfig';
import { getModuleByTab, buildModuleIframeSrc } from '../../modules/registry';
import { postToModule } from '../../modules/post-message';
import './MobileAppLayout.scss';

interface MobileAppLayoutProps {
    app: App;
    state: AppState;
}

interface MobileAppLayoutState {
    activeMobileTab: 'nodes' | 'terminal' | 'files' | 'more';
    subView: 'menu' | 'settings' | 'skills' | 'discovery' | 'providers';
}

export class MobileAppLayout extends Component<MobileAppLayoutProps, MobileAppLayoutState> {
    state: MobileAppLayoutState = {
        activeMobileTab: 'terminal',
        subView: 'menu',
    };

    componentWillReceiveProps(nextProps: MobileAppLayoutProps) {
        // Auto-switch to terminal tab when active session changes or terminal becomes active
        if (nextProps.state.activeTabId === 'terminal' && this.props.state.activeTabId !== 'terminal') {
            this.setState({ activeMobileTab: 'terminal' });
        }
    }

    setMobileTab = (tab: 'nodes' | 'terminal' | 'files' | 'more') => {
        this.setState({ activeMobileTab: tab });
        // If switching to terminal tab, make sure app knows it is in terminal view mode
        if (tab === 'terminal') {
            this.props.app.selectTab('terminal');
        }
    };

    render() {
        const { app, state } = this.props;
        const { activeMobileTab, subView } = this.state;
        const {
            workspaces,
            activeWorkspaceId,
            language,
            tabs,
            activeTabId,
            folders,
            workspacesLoading,
            keyboardVisible,
            viewportHeight,
            activeSession,
            tmuxMouseOn,
            ccProvidersUrl,
            ccConnectUrl,
            theme,
            activeDrawerTab,
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
        } = state;

        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
            fontSize: 12, // Mobile standard
        } as ITerminalOptions;

        const activeWorkspace = workspaces.find(w => w.id === activeWorkspaceId);
        const activeWorkspacePath = activeWorkspace?.path || '.';
        const activeTabObj = tabs.find(t => t.id === activeTabId);

        // Dynamic inline styles based on keyboard state and visual viewport height
        const viewportStyle = keyboardVisible ? { height: `${viewportHeight}px`, flex: 'none' } : undefined;

        return (
            <div class="mobile-app-layout" style={viewportStyle}>
                <div class="mobile-viewport">
                    {/* ── Tab 1: Nodes / Workspaces ── */}
                    {activeMobileTab === 'nodes' && (
                        <div class="mobile-tab-content">
                            <div class="mobile-sidebar-flat-container">
                                <LeftSidebar
                                    folders={folders}
                                    workspaces={workspaces}
                                    workspacesLoading={workspacesLoading}
                                    leftSidebarOpen={true}
                                    leftSidebarWidth={window.innerWidth}
                                    activeWorkspaceId={activeWorkspaceId}
                                    toggleLeftSidebar={() => {}}
                                    toggleFolder={app.toggleFolder}
                                    toggleDrawerTab={app.toggleDrawerTab}
                                    activeDrawerTab={activeDrawerTab}
                                    onCreateWorkspace={app.openCreateWorkspacePicker}
                                    onRenameWorkspace={ws => app.openRenameWorkspaceModal(ws)}
                                    onDeleteWorkspace={app.deleteWorkspace}
                                    onSelectWorkspace={ws => app.selectWorkspace(ws)}
                                    onSelectSession={s => {
                                        app.selectSession(s);
                                        this.setMobileTab('terminal');
                                    }}
                                    onTerminalCreate={(wsId, cwd) => {
                                        app.createTerminal(wsId, cwd);
                                        this.setMobileTab('terminal');
                                    }}
                                    onTerminalKill={idx => app.killTerminal(idx)}
                                    onRenameSession={s => app.openRenameSessionModal(s)}
                                    onReorderFolders={app.reorderFolders}
                                    language={language}
                                    moduleNav={app.buildModuleNav()}
                                />
                            </div>
                        </div>
                    )}

                    {/* ── Tab 2: Terminal Workspace ── */}
                    {activeMobileTab === 'terminal' && (
                        <div class="mobile-tab-content">
                            <WorkspaceHeader
                                leftSidebarOpen={false}
                                toggleLeftSidebar={() => this.setMobileTab('nodes')}
                                activeDrawerTab="none"
                                toggleDrawerTab={() => {}}
                                activeTab={state.activeTab}
                                setActiveTab={app.setActiveTab}
                                theme={theme}
                                toggleTheme={app.toggleTheme}
                                keyboardVisible={keyboardVisible}
                                workspaceName={activeWorkspace?.name || ''}
                                sessionName={activeSession?.name || ''}
                                tmuxMouseOn={tmuxMouseOn}
                                onTmuxMouseToggle={app.toggleTmuxMouse}
                                language={language}
                                moduleNav={app.buildModuleNav()}
                            />
                            <div class="workspace-body-container" style="flex: 1; height: auto;">
                                <MiddleCanvas
                                    activeTab={state.activeTab as 'terminal' | 'agents' | 'console' | 'folders'}
                                    wsUrl={wsUrl}
                                    tokenUrl={tokenUrl}
                                    clientOptions={clientOptions}
                                    termOptions={termOptions}
                                    flowControl={flowControl}
                                    onMobileDetect={isMobile => app.setState({ isMobile })}
                                    onKeyboardStateChange={app.handleKeyboardStateChange}
                                    tmuxMouseOn={tmuxMouseOn}
                                    onTmuxMouseToggle={app.toggleTmuxMouse}
                                    language={language}
                                />
                            </div>
                        </div>
                    )}

                    {/* ── Tab 3: File Manager ── */}
                    {activeMobileTab === 'files' && (
                        <div class="mobile-tab-content">
                            <div class="mobile-drawer-flat-container">
                                <RightPanel
                                    activeDrawerTab="files"
                                    activeWorkspaceId={activeWorkspaceId}
                                    activeWorkspacePath={activeWorkspacePath}
                                    rightPanelWidth={window.innerWidth}
                                    closeDrawer={() => {}}
                                    ccConnectUrl={ccConnectUrl}
                                    theme={theme}
                                    toggleTheme={app.toggleTheme}
                                    language={language}
                                    toggleLanguage={app.toggleLanguage}
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
                                    }}
                                    onOpenFileDetail={app.openFileDetail}
                                    onBackToList={() => app.setState({ viewMode: 'list', detailFullscreen: false })}
                                    onToggleFavorite={app.toggleFavorite}
                                    onCopyContent={app.copyFileContent}
                                    onDownloadFile={app.downloadFile}
                                    onRenameFile={app.renameFile}
                                    onToggleFullscreen={() => {}}
                                    onShareFile={app.shareFile}
                                    onSaveFile={app.saveFile}
                                    onToggleEditing={isEditing => app.setState({ isEditingDetail: isEditing })}
                                    onEditedContentChange={content => app.setState({ editedContent: content })}
                                    fsEntries={state.fsEntries}
                                    fsLoading={state.fsLoading}
                                    onToggleFsDir={app.toggleFsDir}
                                    accessTokenExists={state.accessAuthRequired}
                                    onGenerateAccessToken={app.generateAccessToken}
                                    onRevokeAccessToken={app.revokeAccessToken}
                                />
                            </div>
                        </div>
                    )}

                    {/* ── Tab 4: More / System Subviews ── */}
                    {activeMobileTab === 'more' && (
                        <div class="mobile-tab-content">
                            {subView === 'menu' && (
                                <div class="mobile-menu-view">
                                    <div class="mobile-menu-header">
                                        <h2>{t('app.workbench', language) || '1agents'}</h2>
                                        <p>
                                            {t('terminal.mouse.select', language)
                                                ? '分布式智能协同'
                                                : 'Distributed Multi-Agent Network'}
                                        </p>
                                    </div>

                                    <div class="mobile-menu-group">
                                        <button
                                            class="mobile-menu-row"
                                            onClick={() => this.setState({ subView: 'skills' })}
                                        >
                                            <div class="row-icon-wrapper">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
                                                </svg>
                                            </div>
                                            <span class="row-label">{t('sidebar.skills', language)}</span>
                                            <div class="row-chevron">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <polyline points="9 18 15 12 9 6" />
                                                </svg>
                                            </div>
                                        </button>
                                        <button
                                            class="mobile-menu-row"
                                            onClick={() => this.setState({ subView: 'discovery' })}
                                        >
                                            <div class="row-icon-wrapper">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <circle cx="12" cy="12" r="10" />
                                                    <polygon points="16.24 7.76 14.12 14.12 7.76 16.24 9.88 9.88 16.24 7.76" />
                                                </svg>
                                            </div>
                                            <span class="row-label">{t('sidebar.discovery', language)}</span>
                                            <div class="row-chevron">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <polyline points="9 18 15 12 9 6" />
                                                </svg>
                                            </div>
                                        </button>
                                    </div>

                                    <div class="mobile-menu-group">
                                        <button
                                            class="mobile-menu-row"
                                            onClick={() => this.setState({ subView: 'providers' })}
                                        >
                                            <div class="row-icon-wrapper">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
                                                    <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
                                                    <line x1="6" y1="6" x2="6.01" y2="6" />
                                                    <line x1="6" y1="18" x2="6.01" y2="18" />
                                                </svg>
                                            </div>
                                            <span class="row-label">{t('sidebar.providers', language)}</span>
                                            <div class="row-chevron">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <polyline points="9 18 15 12 9 6" />
                                                </svg>
                                            </div>
                                        </button>
                                        <button
                                            class="mobile-menu-row"
                                            onClick={() => this.setState({ subView: 'settings' })}
                                        >
                                            <div class="row-icon-wrapper">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <circle cx="12" cy="12" r="3" />
                                                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                                                </svg>
                                            </div>
                                            <span class="row-label">Settings</span>
                                            <div class="row-chevron">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <polyline points="9 18 15 12 9 6" />
                                                </svg>
                                            </div>
                                        </button>
                                    </div>
                                </div>
                            )}

                            {subView !== 'menu' && (
                                <div class="mobile-subview-layout">
                                    <div class="mobile-subview-header">
                                        <button
                                            class="mobile-subview-back-btn"
                                            onClick={() => this.setState({ subView: 'menu' })}
                                        >
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                <polyline points="15 18 9 12 15 6" />
                                            </svg>
                                            Back
                                        </button>
                                        <div class="mobile-subview-title">
                                            {subView === 'settings' && 'Settings'}
                                            {subView === 'discovery' && t('sidebar.discovery', language)}
                                            {subView === 'skills' && t('sidebar.skills', language)}
                                            {subView === 'providers' && t('sidebar.providers', language)}
                                        </div>
                                    </div>
                                    <div class="mobile-subview-content">
                                        {subView === 'settings' && (
                                            <SystemSettings
                                                theme={theme}
                                                toggleTheme={app.toggleTheme}
                                                language={language}
                                                toggleLanguage={app.toggleLanguage}
                                                tmuxMouseOn={tmuxMouseOn}
                                                onTmuxMouseToggle={app.toggleTmuxMouse}
                                                accessTokenExists={accessAuthRequired}
                                                onGenerateAccessToken={app.generateAccessToken}
                                                onRevokeAccessToken={app.revokeAccessToken}
                                            />
                                        )}
                                        {subView === 'discovery' && (
                                            <div style={{ padding: '16px' }}>
                                                <DiscoveryPanel onOpenBrowserTab={undefined} language={language} />
                                            </div>
                                        )}
                                        {subView === 'skills' &&
                                            (() => {
                                                const skillsMod = getModuleByTab('skills');
                                                const skillsSrc = skillsMod
                                                    ? buildModuleIframeSrc(skillsMod)
                                                    : '/1skills/';
                                                return (
                                                    <iframe
                                                        id="skills-iframe"
                                                        src={skillsSrc}
                                                        onLoad={e => {
                                                            const iframe = e.target as HTMLIFrameElement;
                                                            postToModule(iframe, {
                                                                type: 'THEME_CHANGE',
                                                                theme,
                                                            });
                                                            postToModule(iframe, {
                                                                type: 'LANG_CHANGE',
                                                                lang: language,
                                                            });
                                                        }}
                                                        style={{
                                                            width: '100%',
                                                            height: '100%',
                                                            border: 'none',
                                                            background: 'transparent',
                                                        }}
                                                    />
                                                );
                                            })()}
                                        {subView === 'providers' && ccProvidersUrl && (
                                            <iframe
                                                id="cc-providers-iframe"
                                                src={app.getCcConnectIframeUrl(ccProvidersUrl)}
                                                onLoad={e => {
                                                    const iframe = e.target as HTMLIFrameElement;
                                                    if (iframe && iframe.contentWindow) {
                                                        iframe.contentWindow.postMessage(
                                                            { type: 'THEME_CHANGE', theme },
                                                            '*'
                                                        );
                                                        iframe.contentWindow.postMessage(
                                                            { type: 'LANG_CHANGE', lang: language },
                                                            '*'
                                                        );
                                                    }
                                                }}
                                                style={{
                                                    width: '100%',
                                                    height: '100%',
                                                    border: 'none',
                                                    background: 'transparent',
                                                }}
                                            />
                                        )}
                                    </div>
                                </div>
                            )}
                        </div>
                    )}

                    {/* ── Subview Layer: Standalone Preview / Browser tabs on mobile ── */}
                    {activeTabObj?.type === 'preview' && (
                        <div class="mobile-subview-layout">
                            <div class="mobile-subview-header">
                                <button class="mobile-subview-back-btn" onClick={() => app.closeTab(activeTabId)}>
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                        <polyline points="15 18 9 12 15 6" />
                                    </svg>
                                    Close
                                </button>
                                <div class="mobile-subview-title">Preview</div>
                            </div>
                            <div
                                class="mobile-subview-content"
                                style="background-color: var(--bg-panel); padding: 12px 16px;"
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
                                        onToggleEditing={isEditing => app.setState({ isEditingDetail: isEditing })}
                                        onEditedContentChange={content => app.setState({ editedContent: content })}
                                        onOpenPreview={undefined}
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
                        </div>
                    )}

                    {activeTabObj?.type === 'browser' && (
                        <div class="mobile-subview-layout">
                            <div class="mobile-subview-header">
                                <button class="mobile-subview-back-btn" onClick={() => app.closeTab(activeTabId)}>
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                        <polyline points="15 18 9 12 15 6" />
                                    </svg>
                                    Close
                                </button>
                                <div class="mobile-subview-title">Browser</div>
                            </div>
                            <div class="mobile-subview-content">
                                {tabs.filter(t => t.id === activeTabId).map(t => app.renderBuiltinBrowser(t))}
                            </div>
                        </div>
                    )}
                </div>

                {/* ── Bottom Navigation Bar ── */}
                <div class="mobile-bottom-nav">
                    <button
                        class={`mobile-tab-btn ${activeMobileTab === 'nodes' ? 'active' : ''}`}
                        onClick={() => this.setMobileTab('nodes')}
                    >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                        </svg>
                        {t('sidebar.workspaces', language) || 'Nodes'}
                    </button>
                    <button
                        class={`mobile-tab-btn ${activeMobileTab === 'terminal' ? 'active' : ''}`}
                        onClick={() => this.setMobileTab('terminal')}
                    >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                            <rect width="20" height="16" x="2" y="4" rx="2" />
                            <path d="m7 8 3 2-3 2" />
                            <path d="M12 12h4" />
                        </svg>
                        Terminal
                    </button>
                    <button
                        class={`mobile-tab-btn ${activeMobileTab === 'files' ? 'active' : ''}`}
                        onClick={() => this.setMobileTab('files')}
                    >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                            <polyline points="14 2 14 8 20 8" />
                        </svg>
                        Files
                    </button>
                    <button
                        class={`mobile-tab-btn ${activeMobileTab === 'more' ? 'active' : ''}`}
                        onClick={() => this.setMobileTab('more')}
                    >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                            <circle cx="12" cy="12" r="3" />
                            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                        </svg>
                        More
                    </button>
                </div>
            </div>
        );
    }
}
