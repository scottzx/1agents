import { h, Component, Fragment } from 'preact';
import type { ITerminalOptions } from '@xterm/xterm';

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
import './MobileAppLayout.scss';

interface MobileAppLayoutProps {
    app: App;
    state: AppState;
}

interface MobileAppLayoutState {
    activeMobileTab: 'workspaces' | 'providers' | 'skills' | 'more';
    selectedWorkspaceId: string;
    inSessionView: boolean;
    skillsInDetail: boolean;
    activeMoreSubView: 'menu' | 'settings' | 'discovery';
    activeSettingsCategory: 'menu' | 'general' | 'appearance' | 'security' | 'feedback' | 'about';
}

export class MobileAppLayout extends Component<MobileAppLayoutProps, MobileAppLayoutState> {
    state: MobileAppLayoutState = {
        activeMobileTab: 'workspaces',
        selectedWorkspaceId: '',
        inSessionView: false,
        skillsInDetail: false,
        activeMoreSubView: 'menu',
        activeSettingsCategory: 'menu',
    };

    componentWillReceiveProps(nextProps: MobileAppLayoutProps) {
        if (nextProps.state.activeWorkspaceId !== this.props.state.activeWorkspaceId) {
            this.setState({ selectedWorkspaceId: nextProps.state.activeWorkspaceId });
        }
        if (nextProps.state.activeTabId === 'terminal' && this.props.state.activeTabId !== 'terminal') {
            this.setState({ activeMobileTab: 'workspaces', inSessionView: true });
        }
    }

    setMobileTab = (tab: 'workspaces' | 'providers' | 'skills' | 'more') => {
        this.setState({ activeMobileTab: tab });
        if (tab === 'skills') {
            this.setState({ skillsInDetail: false });
            if (this.props.state.activeDrawerTab !== 'skills') {
                this.props.app.setState({ activeDrawerTab: 'skills' });
            }
        } else if (tab === 'providers') {
            this.props.app.setState({ activeDrawerTab: 'none' });
            // ccProvidersUrl is loaded asynchronously; if it's not ready yet, trigger a reload
            if (!this.props.state.ccProvidersUrl) {
                this.props.app.loadCcProvidersUrl();
            }
        } else if (tab === 'more') {
            this.setState({ activeMoreSubView: 'menu', activeSettingsCategory: 'menu' });
            this.props.app.setState({ activeDrawerTab: 'none' });
        } else {
            this.props.app.setState({ activeDrawerTab: 'none' });
        }
    };

    render() {
        const { app, state } = this.props;
        const {
            activeMobileTab,
            selectedWorkspaceId,
            inSessionView,
            skillsInDetail,
            activeMoreSubView,
            activeSettingsCategory,
        } = this.state;
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
            isMobile,
        } = state;

        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
            fontSize: 12,
        } as ITerminalOptions;

        const activeWorkspace = workspaces.find(w => w.id === selectedWorkspaceId || w.id === activeWorkspaceId);
        const activeWorkspacePath = activeWorkspace?.path || '.';
        const activeTabObj = tabs.find(t => t.id === activeTabId);

        // Dynamic inline styles based on keyboard state and visual viewport height
        const viewportStyle = keyboardVisible ? { height: `${viewportHeight}px`, flex: 'none' } : undefined;

        // Bottom bar is visible only on level-1 screens
        const showBottomBar =
            !selectedWorkspaceId &&
            !skillsInDetail &&
            activeMoreSubView === 'menu' &&
            activeTabObj?.type !== 'preview' &&
            activeTabObj?.type !== 'browser';

        // Skills iframe src
        const skillsMod = getModuleByTab('skills');
        const skillsSrc = skillsMod ? buildModuleIframeSrc(skillsMod) : '/1skills/';
        const moduleNav = app.buildModuleNav();

        return (
            <div class="mobile-app-layout" style={viewportStyle}>
                <div class="mobile-viewport">
                    {/* ── Tab 1: Workspaces ── */}
                    {activeMobileTab === 'workspaces' && (
                        <Fragment>
                            {/* 1.1 Workspaces Level-1 List View */}
                            {!selectedWorkspaceId && (
                                <div class="mobile-tab-content">
                                    <div class="mobile-menu-view scrollable">
                                        <div class="mobile-menu-header">
                                            <h2>{t('sidebar.workspaces', language) || '工作空间'}</h2>
                                            <p>
                                                {t('mobile.workspaces.desc', language) ||
                                                    '管理并协同您的分布式设备节点'}
                                            </p>
                                        </div>
                                        {workspacesLoading && workspaces.length === 0 ? (
                                            <div class="fb-loading">
                                                <div class="fb-loading-spinner" />
                                                <span>{t('app.loading.workspaces', language)}</span>
                                            </div>
                                        ) : (
                                            <div class="mobile-workspace-list">
                                                <div class="mobile-menu-group">
                                                    {workspaces.map(ws => (
                                                        <div key={ws.id} class="mobile-workspace-item-row">
                                                            <div
                                                                class="item-main"
                                                                onClick={() => {
                                                                    this.setState({ selectedWorkspaceId: ws.id });
                                                                    app.selectWorkspace(ws);
                                                                }}
                                                            >
                                                                <div class="ws-icon-circle">
                                                                    <svg
                                                                        viewBox="0 0 24 24"
                                                                        fill="none"
                                                                        stroke="currentColor"
                                                                    >
                                                                        <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                                                    </svg>
                                                                </div>
                                                                <div class="ws-details">
                                                                    <span class="ws-title">{ws.name}</span>
                                                                    <span class="ws-path">{ws.path}</span>
                                                                </div>
                                                            </div>
                                                            <div class="item-actions">
                                                                <button
                                                                    onClick={() => app.openRenameWorkspaceModal(ws)}
                                                                    class="action-btn"
                                                                    title="Edit"
                                                                >
                                                                    <svg
                                                                        viewBox="0 0 24 24"
                                                                        fill="none"
                                                                        stroke="currentColor"
                                                                    >
                                                                        <path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
                                                                    </svg>
                                                                </button>
                                                                <button
                                                                    onClick={() => app.deleteWorkspace(ws.id)}
                                                                    class="action-btn delete"
                                                                    title="Delete"
                                                                >
                                                                    <svg
                                                                        viewBox="0 0 24 24"
                                                                        fill="none"
                                                                        stroke="currentColor"
                                                                    >
                                                                        <polyline points="3 6 5 6 21 6" />
                                                                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                                                    </svg>
                                                                </button>
                                                            </div>
                                                        </div>
                                                    ))}
                                                </div>
                                                <button
                                                    class="mobile-add-workspace-btn"
                                                    onClick={app.openCreateWorkspacePicker}
                                                >
                                                    + {t('app.workspace.create', language) || '新建工作空间'}
                                                </button>
                                            </div>
                                        )}
                                    </div>
                                </div>
                            )}

                            {/* 1.2 Workspace Detail View (Session Selection) */}
                            {selectedWorkspaceId && !inSessionView && (
                                <div class="mobile-tab-content scrollable">
                                    <div class="mobile-subview-header">
                                        <button
                                            class="mobile-subview-back-btn"
                                            onClick={() => this.setState({ selectedWorkspaceId: '' })}
                                        >
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                <polyline points="15 18 9 12 15 6" />
                                            </svg>
                                        </button>
                                        <div class="mobile-subview-title">
                                            {t('mobile.selectSession', language) || '会话选择'}
                                        </div>
                                    </div>

                                    <div class="mobile-workspace-detail-body">
                                        <div class="workspace-banner">
                                            <div class="banner-icon">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                                </svg>
                                            </div>
                                            <div class="banner-text-info">
                                                <h3>{activeWorkspace?.name}</h3>
                                                <p>{activeWorkspacePath}</p>
                                            </div>
                                        </div>

                                        {(() => {
                                            const folder = folders.find(f => f.id === selectedWorkspaceId);
                                            const sessions = folder?.sessions || [];

                                            return (
                                                <Fragment>
                                                    <div class="mobile-session-section-title">
                                                        <span>
                                                            {t('mobile.sessionList', language, {
                                                                count: sessions.length,
                                                            })}
                                                        </span>
                                                        <button
                                                            class="mobile-new-session-inline-btn"
                                                            onClick={async () => {
                                                                await app.createTerminal(
                                                                    selectedWorkspaceId,
                                                                    activeWorkspacePath
                                                                );
                                                                this.setState({ inSessionView: true });
                                                            }}
                                                        >
                                                            {t('mobile.newSession', language) || '+ 新建会话'}
                                                        </button>
                                                    </div>

                                                    {sessions.length === 0 ? (
                                                        <div class="mobile-no-sessions">
                                                            <div class="no-session-icon">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                >
                                                                    <polyline points="4 17 10 11 4 5" />
                                                                    <line x1="12" x2="20" y1="19" y2="19" />
                                                                </svg>
                                                            </div>
                                                            <p>
                                                                {t('mobile.noSessionsActive', language) ||
                                                                    '当前空间下暂无活动终端会话'}
                                                            </p>
                                                            <button
                                                                class="mobile-primary-btn"
                                                                onClick={async () => {
                                                                    await app.createTerminal(
                                                                        selectedWorkspaceId,
                                                                        activeWorkspacePath
                                                                    );
                                                                    this.setState({ inSessionView: true });
                                                                }}
                                                            >
                                                                {t('mobile.createFirstSession', language) ||
                                                                    '创建并进入第一个会话'}
                                                            </button>
                                                        </div>
                                                    ) : (
                                                        <div class="mobile-session-container">
                                                            <div class="mobile-session-cards-grid">
                                                                <div class="mobile-menu-group">
                                                                    {sessions.map(s => {
                                                                        const isActive = activeSession?.id === s.id;
                                                                        return (
                                                                            <div
                                                                                key={s.id}
                                                                                class={`mobile-session-item-row ${isActive ? 'active' : ''}`}
                                                                                onClick={() => {
                                                                                    app.selectSession(s);
                                                                                    this.setState({
                                                                                        inSessionView: true,
                                                                                    });
                                                                                }}
                                                                            >
                                                                                <div class="card-left">
                                                                                    <div class="session-card-icon">
                                                                                        <svg
                                                                                            viewBox="0 0 24 24"
                                                                                            fill="none"
                                                                                            stroke="currentColor"
                                                                                        >
                                                                                            <polyline points="4 17 10 11 4 5" />
                                                                                            <line
                                                                                                x1="12"
                                                                                                x2="20"
                                                                                                y1="19"
                                                                                                y2="19"
                                                                                            />
                                                                                        </svg>
                                                                                    </div>
                                                                                    <div class="session-card-info">
                                                                                        <span class="session-card-name">
                                                                                            {s.name}
                                                                                        </span>
                                                                                        <span class="session-card-cwd">
                                                                                            {s.cwd ||
                                                                                                activeWorkspacePath}
                                                                                        </span>
                                                                                    </div>
                                                                                </div>
                                                                                <div class="card-right">
                                                                                    <span
                                                                                        class={`status-badge ${s.status || 'idle'}`}
                                                                                    >
                                                                                        {s.status || 'idle'}
                                                                                    </span>
                                                                                    <button
                                                                                        class="action-btn"
                                                                                        title={t(
                                                                                            'sidebar.renameSession',
                                                                                            language
                                                                                        )}
                                                                                        onClick={e => {
                                                                                            e.stopPropagation();
                                                                                            app.openRenameSessionModal(
                                                                                                s
                                                                                            );
                                                                                        }}
                                                                                    >
                                                                                        <svg
                                                                                            viewBox="0 0 24 24"
                                                                                            fill="none"
                                                                                            stroke="currentColor"
                                                                                        >
                                                                                            <path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
                                                                                        </svg>
                                                                                    </button>
                                                                                    <button
                                                                                        class="action-btn delete"
                                                                                        title={t(
                                                                                            'sidebar.closeSession',
                                                                                            language
                                                                                        )}
                                                                                        onClick={e => {
                                                                                            e.stopPropagation();
                                                                                            app.killTerminal(s.index);
                                                                                        }}
                                                                                    >
                                                                                        <svg
                                                                                            viewBox="0 0 24 24"
                                                                                            fill="none"
                                                                                            stroke="currentColor"
                                                                                        >
                                                                                            <polyline points="3 6 5 6 21 6" />
                                                                                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                                                                        </svg>
                                                                                    </button>
                                                                                </div>
                                                                            </div>
                                                                        );
                                                                    })}
                                                                </div>
                                                            </div>
                                                        </div>
                                                    )}
                                                </Fragment>
                                            );
                                        })()}
                                    </div>
                                </div>
                            )}

                            {/* 1.3 Session Workbench/Terminal Detail View */}
                            {selectedWorkspaceId && inSessionView && (
                                <div class="mobile-tab-content">
                                    <WorkspaceHeader
                                        leftSidebarOpen={false}
                                        toggleLeftSidebar={() => {}}
                                        onBack={() => this.setState({ inSessionView: false })}
                                        activeDrawerTab={activeDrawerTab}
                                        toggleDrawerTab={app.toggleDrawerTab}
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
                                    />
                                    <div class="workspace-body-container" style="flex: 1; min-height: 0;">
                                        {activeDrawerTab === 'none' && (
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
                                                onMobileDetect={isMobile => app.setState({ isMobile })}
                                                onKeyboardStateChange={app.handleKeyboardStateChange}
                                                tmuxMouseOn={tmuxMouseOn}
                                                onTmuxMouseToggle={app.toggleTmuxMouse}
                                                language={language}
                                            />
                                        )}

                                        {activeDrawerTab !== 'none' && (
                                            <div class="mobile-drawer-flat-container">
                                                <RightPanel
                                                    activeDrawerTab={activeDrawerTab}
                                                    activeWorkspaceId={selectedWorkspaceId}
                                                    activeWorkspacePath={activeWorkspacePath}
                                                    rightPanelWidth={window.innerWidth}
                                                    closeDrawer={() => app.setState({ activeDrawerTab: 'none' })}
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
                                                        const isSearching =
                                                            searchQuery !== '' || selectedFilterTag !== 'all';
                                                        if (isSearching) {
                                                            app.loadFlatFiles();
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
                                                        if (selectedFsEntry) {
                                                            const encodedPath = selectedFsEntry.path
                                                                .split('/')
                                                                .map(encodeURIComponent)
                                                                .join('/');
                                                            window.open(`/api/fs/view/${encodedPath}`, '_blank');
                                                        }
                                                    }}
                                                    onShareFile={app.shareFile}
                                                    onSaveFile={app.saveFile}
                                                    onToggleEditing={isEditing =>
                                                        app.setState({ isEditingDetail: isEditing })
                                                    }
                                                    onEditedContentChange={content =>
                                                        app.setState({ editedContent: content })
                                                    }
                                                    fsEntries={state.fsEntries}
                                                    fsLoading={state.fsLoading}
                                                    onToggleFsDir={app.toggleFsDir}
                                                    accessTokenExists={state.accessAuthRequired}
                                                    onGenerateAccessToken={app.generateAccessToken}
                                                    onRevokeAccessToken={app.revokeAccessToken}
                                                />
                                            </div>
                                        )}
                                    </div>
                                </div>
                            )}
                        </Fragment>
                    )}

                    {/* ── Tab 2: Providers (Model Management) ── */}
                    {activeMobileTab === 'providers' && (
                        <div class="mobile-tab-content">
                            {ccProvidersUrl ? (
                                <div class="mobile-iframe-container" style="width: 100%; height: 100%;">
                                    <iframe
                                        id="cc-providers-iframe"
                                        src={app.getCcConnectIframeUrl(ccProvidersUrl)}
                                        onLoad={e => {
                                            const iframe = e.target as HTMLIFrameElement;
                                            if (iframe && iframe.contentWindow) {
                                                iframe.contentWindow.postMessage({ type: 'THEME_CHANGE', theme }, '*');
                                                iframe.contentWindow.postMessage(
                                                    { type: 'LANG_CHANGE', lang: language },
                                                    '*'
                                                );
                                            }
                                        }}
                                        style="width: 100%; height: 100%; border: none; background: transparent;"
                                    />
                                </div>
                            ) : (
                                <div class="fb-loading">
                                    <div class="fb-loading-spinner" />
                                    <span>正在加载模型管理...</span>
                                </div>
                            )}
                        </div>
                    )}

                    {/* ── Tab 3: Skills (Skill Management) ── */}
                    {activeMobileTab === 'skills' && (
                        <Fragment>
                            {/* 3.1 Skill Links List View */}
                            {!skillsInDetail && (
                                <div class="mobile-tab-content scrollable">
                                    {moduleNav ? (
                                        <div class="mobile-skills-list-view">
                                            <div class="mobile-menu-header">
                                                <h2>{t('sidebar.skills', language) || '技能中心'}</h2>
                                                <p>为您的协作终端扩展并配置 AI Agent 技能</p>
                                            </div>

                                            {moduleNav.manifest.topLinks && moduleNav.manifest.topLinks.length > 0 && (
                                                <div class="mobile-menu-group">
                                                    {moduleNav.manifest.topLinks.map(link => (
                                                        <button
                                                            key={link.key}
                                                            class="mobile-menu-row"
                                                            onClick={() => {
                                                                moduleNav.onNavigate(link.to);
                                                                this.setState({ skillsInDetail: true });
                                                            }}
                                                        >
                                                            <span class="row-label">{link.label}</span>
                                                            <div class="row-chevron">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                >
                                                                    <polyline points="9 18 15 12 9 6" />
                                                                </svg>
                                                            </div>
                                                        </button>
                                                    ))}
                                                </div>
                                            )}

                                            {moduleNav.manifest.groups.map(group => (
                                                <div key={group.key} class="mobile-skills-group-section">
                                                    <div class="group-title">{group.label}</div>
                                                    <div class="mobile-menu-group">
                                                        {group.links.map(link => (
                                                            <button
                                                                key={link.key}
                                                                class="mobile-menu-row"
                                                                onClick={() => {
                                                                    moduleNav.onNavigate(link.to);
                                                                    this.setState({ skillsInDetail: true });
                                                                }}
                                                            >
                                                                <span class="row-label">{link.label}</span>
                                                                {link.count !== null && link.count !== undefined && (
                                                                    <span class="row-count-badge">{link.count}</span>
                                                                )}
                                                                <div class="row-chevron">
                                                                    <svg
                                                                        viewBox="0 0 24 24"
                                                                        fill="none"
                                                                        stroke="currentColor"
                                                                    >
                                                                        <polyline points="9 18 15 12 9 6" />
                                                                    </svg>
                                                                </div>
                                                            </button>
                                                        ))}
                                                    </div>
                                                </div>
                                            ))}
                                        </div>
                                    ) : (
                                        <div class="fb-loading">
                                            <div class="fb-loading-spinner" />
                                            <span>正在加载技能中心模块...</span>
                                        </div>
                                    )}
                                </div>
                            )}

                            {/* 3.2 Skill Detail Iframe View */}
                            {skillsInDetail && (
                                <div class="mobile-subview-layout">
                                    <div class="mobile-subview-header">
                                        <button
                                            class="mobile-subview-back-btn"
                                            onClick={() => this.setState({ skillsInDetail: false })}
                                        >
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                <polyline points="15 18 9 12 15 6" />
                                            </svg>
                                        </button>
                                        <div class="mobile-subview-title">{t('sidebar.skills', language)}</div>
                                    </div>
                                    <div class="mobile-subview-content">
                                        <iframe
                                            id="skills-iframe"
                                            src={skillsSrc}
                                            style="width: 100%; height: 100%; border: none; background: transparent;"
                                        />
                                    </div>
                                </div>
                            )}
                        </Fragment>
                    )}

                    {/* ── Tab 4: More / Menu ── */}
                    {activeMobileTab === 'more' && (
                        <div class="mobile-tab-content">
                            {activeMoreSubView === 'menu' && (
                                <div class="mobile-menu-view scrollable">
                                    <div class="mobile-menu-header">
                                        <h2>{t('app.workbench', language) || '更多应用'}</h2>
                                        <p>{t('mobile.more.desc', language) || '分布式协同与高级系统管理'}</p>
                                    </div>

                                    <div class="mobile-menu-group">
                                        <button
                                            class="mobile-menu-row"
                                            onClick={() => this.setState({ activeMoreSubView: 'discovery' })}
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
                                        <button
                                            class="mobile-menu-row"
                                            onClick={() => this.setState({ activeMoreSubView: 'settings' })}
                                        >
                                            <div class="row-icon-wrapper">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <circle cx="12" cy="12" r="3" />
                                                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                                                </svg>
                                            </div>
                                            <span class="row-label">
                                                {t('sidebar.settings', language) || '系统设置'}
                                            </span>
                                            <div class="row-chevron">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <polyline points="9 18 15 12 9 6" />
                                                </svg>
                                            </div>
                                        </button>
                                    </div>
                                </div>
                            )}

                            {activeMoreSubView !== 'menu' && (
                                <div class="mobile-subview-layout">
                                    {activeMoreSubView === 'settings' && activeSettingsCategory !== 'menu' ? (
                                        <div class="mobile-subview-header">
                                            <button
                                                class="mobile-subview-back-btn"
                                                onClick={() => this.setState({ activeSettingsCategory: 'menu' })}
                                            >
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <polyline points="15 18 9 12 15 6" />
                                                </svg>
                                            </button>
                                            <div class="mobile-subview-title">
                                                {activeSettingsCategory === 'general' &&
                                                    (language === 'zh-CN' ? '通用设置' : 'General')}
                                                {activeSettingsCategory === 'appearance' &&
                                                    (language === 'zh-CN' ? '外观与终端' : 'Appearance & Terminal')}
                                                {activeSettingsCategory === 'security' &&
                                                    (language === 'zh-CN' ? '安全设置' : 'Security')}
                                                {activeSettingsCategory === 'feedback' &&
                                                    (language === 'zh-CN' ? '反馈与联系' : 'Feedback & Contact')}
                                                {activeSettingsCategory === 'about' &&
                                                    (language === 'zh-CN' ? '关于与维护' : 'About & Maintenance')}
                                            </div>
                                        </div>
                                    ) : (
                                        <div class="mobile-subview-header">
                                            <button
                                                class="mobile-subview-back-btn"
                                                onClick={() => this.setState({ activeMoreSubView: 'menu' })}
                                            >
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <polyline points="15 18 9 12 15 6" />
                                                </svg>
                                            </button>
                                            <div class="mobile-subview-title">
                                                {activeMoreSubView === 'settings' &&
                                                    (t('sidebar.settings', language) || '系统设置')}
                                                {activeMoreSubView === 'discovery' && t('sidebar.discovery', language)}
                                            </div>
                                        </div>
                                    )}
                                    <div class="mobile-subview-content">
                                        {activeMoreSubView === 'settings' &&
                                            (activeSettingsCategory === 'menu' ? (
                                                <div class="mobile-menu-view scrollable">
                                                    <div class="mobile-menu-group">
                                                        <button
                                                            class="mobile-menu-row"
                                                            onClick={() =>
                                                                this.setState({ activeSettingsCategory: 'general' })
                                                            }
                                                        >
                                                            <div class="row-icon-wrapper settings-category-icon">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                    stroke-width="2"
                                                                    stroke-linecap="round"
                                                                    stroke-linejoin="round"
                                                                >
                                                                    <circle cx="12" cy="12" r="3" />
                                                                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                                                                </svg>
                                                            </div>
                                                            <span class="row-label">
                                                                {language === 'zh-CN' ? '通用设置' : 'General'}
                                                            </span>
                                                            <div class="row-chevron">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                >
                                                                    <polyline points="9 18 15 12 9 6" />
                                                                </svg>
                                                            </div>
                                                        </button>
                                                        <button
                                                            class="mobile-menu-row"
                                                            onClick={() =>
                                                                this.setState({ activeSettingsCategory: 'appearance' })
                                                            }
                                                        >
                                                            <div class="row-icon-wrapper settings-category-icon">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                    stroke-width="2"
                                                                    stroke-linecap="round"
                                                                    stroke-linejoin="round"
                                                                >
                                                                    <circle cx="12" cy="12" r="4" />
                                                                    <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                                                                </svg>
                                                            </div>
                                                            <span class="row-label">
                                                                {language === 'zh-CN'
                                                                    ? '外观与终端'
                                                                    : 'Appearance & Terminal'}
                                                            </span>
                                                            <div class="row-chevron">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                >
                                                                    <polyline points="9 18 15 12 9 6" />
                                                                </svg>
                                                            </div>
                                                        </button>
                                                        <button
                                                            class="mobile-menu-row"
                                                            onClick={() =>
                                                                this.setState({ activeSettingsCategory: 'security' })
                                                            }
                                                        >
                                                            <div class="row-icon-wrapper settings-category-icon">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                    stroke-width="2"
                                                                    stroke-linecap="round"
                                                                    stroke-linejoin="round"
                                                                >
                                                                    <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
                                                                </svg>
                                                            </div>
                                                            <span class="row-label">
                                                                {language === 'zh-CN' ? '安全设置' : 'Security'}
                                                            </span>
                                                            <div class="row-chevron">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                >
                                                                    <polyline points="9 18 15 12 9 6" />
                                                                </svg>
                                                            </div>
                                                        </button>
                                                        <button
                                                            class="mobile-menu-row"
                                                            onClick={() =>
                                                                this.setState({ activeSettingsCategory: 'feedback' })
                                                            }
                                                        >
                                                            <div class="row-icon-wrapper settings-category-icon">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                    stroke-width="2"
                                                                    stroke-linecap="round"
                                                                    stroke-linejoin="round"
                                                                >
                                                                    <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
                                                                    <polyline points="22,6 12,13 2,6" />
                                                                </svg>
                                                            </div>
                                                            <span class="row-label">
                                                                {language === 'zh-CN'
                                                                    ? '反馈与联系'
                                                                    : 'Feedback & Contact'}
                                                            </span>
                                                            <div class="row-chevron">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                >
                                                                    <polyline points="9 18 15 12 9 6" />
                                                                </svg>
                                                            </div>
                                                        </button>
                                                        <button
                                                            class="mobile-menu-row"
                                                            onClick={() =>
                                                                this.setState({ activeSettingsCategory: 'about' })
                                                            }
                                                        >
                                                            <div class="row-icon-wrapper settings-category-icon">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                    stroke-width="2"
                                                                    stroke-linecap="round"
                                                                    stroke-linejoin="round"
                                                                >
                                                                    <circle cx="12" cy="12" r="10" />
                                                                    <line x1="12" y1="8" x2="12" y2="12" />
                                                                    <line x1="12" y1="16" x2="12.01" y2="16" />
                                                                </svg>
                                                            </div>
                                                            <span class="row-label">
                                                                {language === 'zh-CN'
                                                                    ? '关于与维护'
                                                                    : 'About & Maintenance'}
                                                            </span>
                                                            <div class="row-chevron">
                                                                <svg
                                                                    viewBox="0 0 24 24"
                                                                    fill="none"
                                                                    stroke="currentColor"
                                                                >
                                                                    <polyline points="9 18 15 12 9 6" />
                                                                </svg>
                                                            </div>
                                                        </button>
                                                    </div>
                                                </div>
                                            ) : (
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
                                                    activeCategory={activeSettingsCategory}
                                                    hideSidebar={true}
                                                />
                                            ))}
                                        {activeMoreSubView === 'discovery' && (
                                            <div style={{ padding: '16px' }}>
                                                <DiscoveryPanel onOpenBrowserTab={undefined} language={language} />
                                            </div>
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
                                </button>
                                <div class="mobile-subview-title">{t('mobile.preview', language) || 'Preview'}</div>
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
                                </button>
                                <div class="mobile-subview-title">{t('mobile.browser', language) || 'Browser'}</div>
                            </div>
                            <div class="mobile-subview-content">
                                {tabs.filter(t => t.id === activeTabId).map(t => app.renderBuiltinBrowser(t))}
                            </div>
                        </div>
                    )}
                </div>

                {/* ── Bottom Navigation Bar ── */}
                {showBottomBar && (
                    <div class="mobile-bottom-nav">
                        <button
                            class={`mobile-tab-btn ${activeMobileTab === 'workspaces' ? 'active' : ''}`}
                            onClick={() => this.setMobileTab('workspaces')}
                        >
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                            </svg>
                            {t('sidebar.workspaces', language) || '工作空间'}
                        </button>
                        <button
                            class={`mobile-tab-btn ${activeMobileTab === 'providers' ? 'active' : ''}`}
                            onClick={() => this.setMobileTab('providers')}
                        >
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                <path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 0 1 0 9z" />
                                <circle cx="12" cy="10" r="3" />
                            </svg>
                            {t('sidebar.providers', language) || '模型管理'}
                        </button>
                        <button
                            class={`mobile-tab-btn ${activeMobileTab === 'skills' ? 'active' : ''}`}
                            onClick={() => this.setMobileTab('skills')}
                        >
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
                            </svg>
                            {t('sidebar.skills', language) || '技能管理'}
                        </button>
                        <button
                            class={`mobile-tab-btn ${activeMobileTab === 'more' ? 'active' : ''}`}
                            onClick={() => this.setMobileTab('more')}
                        >
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                <circle cx="12" cy="12" r="3" />
                                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                            </svg>
                            {t('sidebar.more', language) || '更多'}
                        </button>
                    </div>
                )}
            </div>
        );
    }
}
