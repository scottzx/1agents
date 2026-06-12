import { h, Component, Fragment } from 'preact';
import { effect } from '@preact/signals';

import { WorkspaceHeader } from '../header/WorkspaceHeader';
import { isChat } from '../types';
import { DiscoveryPanel } from '../drawer/DiscoveryPanel';
import { TaskList } from '../drawer/TaskList';
import { WorkbenchCanvas } from '../shared/WorkbenchCanvas';
import { RightPanelHost } from '../shared/RightPanelHost';
import { SystemSettingsHost } from '../shared/SystemSettingsHost';
import { FilePreviewContent } from '../shared/FilePreviewContent';
import { CcProvidersPanel } from '../shared/CcProvidersPanel';
import { t } from '../../i18n';
import type { App, AppState } from '../app';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import * as modal from '../../stores/modalStore';
import { SETTINGS_STATIC_MANIFEST, type SettingsCategory } from '../../modules/settings-manifest';
import './MobileAppLayout.scss';

/**
 * Inline SVG icons for each settings category. The manifest only carries
 * i18n labels, not icons — the host's `ModuleNav` (desktop) shows a dot
 * per link, but the mobile menu shows full icons, so we keep them here.
 */
function renderSettingsCategoryIcon(cat: SettingsCategory) {
    switch (cat) {
        case 'general':
            return (
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
            );
        case 'appearance':
            return (
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
            );
        case 'security':
            return (
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
            );
        case 'feedback':
            return (
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
            );
        case 'about':
            return (
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
            );
        default:
            return null;
    }
}

interface MobileAppLayoutProps {
    app: App;
    state: AppState;
}

interface MobileAppLayoutState {
    activeMobileTab: 'workspaces' | 'providers' | 'skills' | 'more';
    selectedWorkspaceId: string;
    inSessionView: boolean;
    skillsInDetail: boolean;
    /**
     * The path the skills iframe was last mounted with, baked into its URL
     * hash (e.g. `#/skills/use`). Captured at the moment the user clicks
     * a sub-link in the skills list, so the iframe boots directly at the
     * right route — no race with the host's postMessage handshake, no
     * flash of the catch-all `* → /overview` redirect. Reset whenever
     * the iframe is unmounted (going back to the list, switching tabs).
     */
    mountedSkillsPath: string;
    activeMoreSubView: 'menu' | 'settings' | 'discovery';
    activeSettingsCategory: SettingsCategory | 'menu';
    pendingConfirm:
        | { kind: 'session'; name: string; sessionIndex: number; isChat: boolean; sessionId?: string }
        | { kind: 'workspace'; name: string; workspaceId: string }
        | null;
}

export class MobileAppLayout extends Component<MobileAppLayoutProps, MobileAppLayoutState> {
    state: MobileAppLayoutState = {
        activeMobileTab: 'workspaces',
        selectedWorkspaceId: '',
        inSessionView: false,
        skillsInDetail: false,
        mountedSkillsPath: '',
        activeMoreSubView: 'menu',
        activeSettingsCategory: 'menu',
        pendingConfirm: null,
    };

    /**
     * Mirrors workspace switches (auto-select on load, deletes, …) into the
     * local navigation state. Replaces the former componentWillReceiveProps
     * prop comparison now that activeWorkspaceId lives in a signal: the
     * effect fires on every signal write, the previous-value guard keeps the
     * original "only on change" semantics.
     */
    private _prevActiveWsId = wsStore.activeWorkspaceId.value;
    private _disposeWsSync: (() => void) | null = null;

    componentDidMount() {
        this._disposeWsSync = effect(() => {
            const id = wsStore.activeWorkspaceId.value;
            if (id !== this._prevActiveWsId) {
                this._prevActiveWsId = id;
                this.setState({ selectedWorkspaceId: id });
            }
        });
    }

    componentWillUnmount() {
        if (this._disposeWsSync) {
            this._disposeWsSync();
            this._disposeWsSync = null;
        }
    }

    componentWillReceiveProps(nextProps: MobileAppLayoutProps) {
        if (nextProps.state.activeTabId === 'terminal' && this.props.state.activeTabId !== 'terminal') {
            this.setState({ activeMobileTab: 'workspaces', inSessionView: true });
        }
    }

    setMobileTab = (tab: 'workspaces' | 'providers' | 'skills' | 'more') => {
        this.setState({ activeMobileTab: tab, mountedSkillsPath: '' });
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
        const { tabs, activeTabId, ccProvidersUrl, activeDrawerTab } = state;
        const workspaces = wsStore.workspaces.value;
        const activeWorkspaceId = wsStore.activeWorkspaceId.value;
        const folders = wsStore.folders.value;
        const workspacesLoading = wsStore.workspacesLoading.value;
        const activeSession = sess.activeSession.value;
        const tmuxMouseOn = sess.tmuxMouseOn.value;
        const selectedFsEntry = fs.selectedFsEntry.value;
        const language = ui.language.value;
        const theme = ui.theme.value;
        const keyboardVisible = ui.keyboardVisible.value;
        const viewportHeight = ui.viewportHeight.value;

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
                                                                    onClick={() => modal.openRenameWorkspaceModal(ws)}
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
                                                                    onClick={() =>
                                                                        this.setState({
                                                                            pendingConfirm: {
                                                                                kind: 'workspace',
                                                                                name: ws.name,
                                                                                workspaceId: ws.id,
                                                                            },
                                                                        })
                                                                    }
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
                                                    onClick={modal.openCreateWorkspacePicker}
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
                                                                        const sessionAgent = isChat(s)
                                                                            ? s.agentType
                                                                            : s.agent;
                                                                        const sessionCwd = isChat(s)
                                                                            ? undefined
                                                                            : s.cwd;
                                                                        const sessionIndex = isChat(s) ? -1 : s.index;
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
                                                                                        {isChat(s) ? (
                                                                                            <span style="font-size: 16px;">
                                                                                                💬
                                                                                            </span>
                                                                                        ) : (
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
                                                                                        )}
                                                                                    </div>
                                                                                    <div class="session-card-info">
                                                                                        <div class="session-card-name-row">
                                                                                            <span class="session-card-name">
                                                                                                {s.name}
                                                                                            </span>
                                                                                            {sessionAgent ? (
                                                                                                <span class="session-card-agent">
                                                                                                    {sessionAgent ===
                                                                                                    'antigravity'
                                                                                                        ? 'agy'
                                                                                                        : sessionAgent
                                                                                                              .charAt(0)
                                                                                                              .toUpperCase() +
                                                                                                          sessionAgent.slice(
                                                                                                              1
                                                                                                          )}
                                                                                                </span>
                                                                                            ) : null}
                                                                                        </div>
                                                                                        <span class="session-card-cwd">
                                                                                            {sessionCwd ||
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
                                                                                            modal.openRenameSessionModal(
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
                                                                                            this.setState({
                                                                                                pendingConfirm: {
                                                                                                    kind: 'session',
                                                                                                    name: s.name,
                                                                                                    sessionIndex:
                                                                                                        sessionIndex,
                                                                                                    isChat: isChat(s),
                                                                                                    sessionId: s.id,
                                                                                                },
                                                                                            });
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
                                        toggleTheme={ui.toggleTheme}
                                        keyboardVisible={keyboardVisible}
                                        workspaceName={activeWorkspace?.name || ''}
                                        sessionName={activeSession?.name || ''}
                                        tmuxMouseOn={tmuxMouseOn}
                                        onTmuxMouseToggle={app.toggleTmuxMouse}
                                        language={language}
                                        hasChatSession={folders.some(
                                            f => f.id === selectedWorkspaceId && f.sessions.some(isChat)
                                        )}
                                    />
                                    <div class="workspace-body-container" style="flex: 1; min-height: 0;">
                                        {activeDrawerTab === 'none' && (
                                            <WorkbenchCanvas app={app} state={state} fontSize={12} />
                                        )}

                                        {activeDrawerTab !== 'none' && (
                                            <div class="mobile-drawer-flat-container">
                                                <RightPanelHost
                                                    app={app}
                                                    state={state}
                                                    activeWorkspaceId={selectedWorkspaceId}
                                                    activeWorkspacePath={activeWorkspacePath}
                                                    rightPanelWidth={window.innerWidth}
                                                    onToggleFullscreen={() => {
                                                        if (selectedFsEntry) {
                                                            const encodedPath = selectedFsEntry.path
                                                                .split('/')
                                                                .map(encodeURIComponent)
                                                                .join('/');
                                                            window.open(`/api/fs/view/${encodedPath}`, '_blank');
                                                        }
                                                    }}
                                                    onSelectSession={s => app.selectSession(s)}
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
                                <div class="mobile-iframe-container" style="flex: 1; min-height: 0; overflow: hidden;">
                                    <CcProvidersPanel
                                        ccProvidersUrl={ccProvidersUrl}
                                        panelStyle="width: 100%; height: 100%; display: flex; flex-direction: column;"
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
                                                                // Capture the path BEFORE mounting the iframe so
                                                                // the iframe's URL hash can boot the iframe at
                                                                // the right route on first paint.
                                                                this.setState({
                                                                    mountedSkillsPath: link.to,
                                                                    skillsInDetail: true,
                                                                });
                                                                moduleNav.onNavigate(link.to);
                                                            }}
                                                        >
                                                            <span class="row-label">{t(link.label, language)}</span>
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
                                                    <div class="group-title">{t(group.label, language)}</div>
                                                    <div class="mobile-menu-group">
                                                        {group.links.map(link => (
                                                            <button
                                                                key={link.key}
                                                                class="mobile-menu-row"
                                                                onClick={() => {
                                                                    this.setState({
                                                                        mountedSkillsPath: link.to,
                                                                        skillsInDetail: true,
                                                                    });
                                                                    moduleNav.onNavigate(link.to);
                                                                }}
                                                            >
                                                                <span class="row-label">{t(link.label, language)}</span>
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
                                            onClick={() =>
                                                this.setState({ skillsInDetail: false, mountedSkillsPath: '' })
                                            }
                                        >
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                <polyline points="15 18 9 12 15 6" />
                                            </svg>
                                        </button>
                                        <div class="mobile-subview-title">{t('sidebar.skills', language)}</div>
                                    </div>
                                    <div class="mobile-subview-content" style="overflow: hidden;">
                                        <skills-panel
                                            id="skills-panel"
                                            route={
                                                (activeMobileTab === 'skills' && state.activeModulePath) ||
                                                this.state.mountedSkillsPath ||
                                                '/overview'
                                            }
                                            theme={theme}
                                            lang={language}
                                            style="width: 100%; height: 100%; display: flex; flex-direction: column;"
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
                                                {(() => {
                                                    const link = SETTINGS_STATIC_MANIFEST.topLinks?.find(
                                                        l => l.to === `/${activeSettingsCategory}`
                                                    );
                                                    return link ? t(link.label, language) : '';
                                                })()}
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
                                                        {(SETTINGS_STATIC_MANIFEST.topLinks ?? []).map(link => {
                                                            const cat = link.to.replace('/', '') as SettingsCategory;
                                                            return (
                                                                <button
                                                                    key={link.key}
                                                                    class="mobile-menu-row"
                                                                    onClick={() =>
                                                                        this.setState({
                                                                            activeSettingsCategory: cat,
                                                                        })
                                                                    }
                                                                >
                                                                    <div class="row-icon-wrapper settings-category-icon">
                                                                        {renderSettingsCategoryIcon(cat)}
                                                                    </div>
                                                                    <span class="row-label">
                                                                        {t(link.label, language)}
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
                                                            );
                                                        })}
                                                    </div>
                                                </div>
                                            ) : (
                                                <SystemSettingsHost
                                                    app={app}
                                                    state={state}
                                                    activeCategory={activeSettingsCategory}
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
                                <FilePreviewContent app={app} activeTabId={activeTabId} onOpenPreview={undefined} />
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

                    {activeTabObj?.type === 'tasks' && (
                        <div class="mobile-subview-layout">
                            <div class="mobile-subview-header">
                                <button class="mobile-subview-back-btn" onClick={() => app.selectTab('terminal')}>
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                        <polyline points="15 18 9 12 15 6" />
                                    </svg>
                                </button>
                                <div class="mobile-subview-title">项目任务</div>
                            </div>
                            <div
                                class="mobile-subview-content scrollable"
                                style="background-color: var(--bg-panel); padding: 12px 16px;"
                            >
                                <TaskList
                                    workspaceId={selectedWorkspaceId || activeWorkspaceId}
                                    onSelectSession={s => {
                                        app.selectSession(s);
                                        app.selectTab('terminal');
                                        this.setState({ inSessionView: true });
                                    }}
                                />
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

                {/* Delete Confirmation Modal (session or workspace) */}
                {this.state.pendingConfirm &&
                    (() => {
                        const confirm = this.state.pendingConfirm!;
                        const titleKey =
                            confirm.kind === 'session'
                                ? 'mobile.confirmDeleteSession.title'
                                : 'mobile.confirmDeleteWorkspace.title';
                        const messageKey =
                            confirm.kind === 'session'
                                ? 'mobile.confirmDeleteSession.message'
                                : 'mobile.confirmDeleteWorkspace.message';
                        const fallbackTitle = confirm.kind === 'session' ? '删除会话' : '删除工作空间';
                        const fallbackMessage = `确定要删除 ${
                            confirm.kind === 'session' ? '会话' : '工作空间'
                        } “${confirm.name}” 吗?此操作无法撤销。`;
                        return (
                            <div class="mobile-confirm-modal" role="dialog" aria-modal="true">
                                <div
                                    class="mobile-confirm-backdrop"
                                    onClick={() => this.setState({ pendingConfirm: null })}
                                />
                                <div class="mobile-confirm-box">
                                    <div class="mobile-confirm-icon">
                                        <svg
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <polyline points="3 6 5 6 21 6" />
                                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                        </svg>
                                    </div>
                                    <h3 class="mobile-confirm-title">{t(titleKey, language) || fallbackTitle}</h3>
                                    <p class="mobile-confirm-message">
                                        {t(messageKey, language, { name: confirm.name }) || fallbackMessage}
                                    </p>
                                    <div class="mobile-confirm-actions">
                                        <button
                                            class="mobile-confirm-btn cancel"
                                            onClick={() => this.setState({ pendingConfirm: null })}
                                        >
                                            {t('common.cancel', language) || '取消'}
                                        </button>
                                        <button
                                            class="mobile-confirm-btn danger"
                                            onClick={async () => {
                                                const target = this.state.pendingConfirm;
                                                this.setState({ pendingConfirm: null });
                                                if (!target) return;
                                                if (target.kind === 'session') {
                                                    if (target.isChat && target.sessionId) {
                                                        await app.killChatSession(target.sessionId);
                                                    } else {
                                                        await app.killTerminal(target.sessionIndex);
                                                    }
                                                } else {
                                                    await app.deleteWorkspace(target.workspaceId);
                                                }
                                            }}
                                        >
                                            {t('common.delete', language) || '删除'}
                                        </button>
                                    </div>
                                </div>
                            </div>
                        );
                    })()}
            </div>
        );
    }
}
