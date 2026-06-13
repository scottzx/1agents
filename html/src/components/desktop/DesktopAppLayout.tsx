import { h, Component, Fragment } from 'preact';
import { isFullPageTab, isChat, type RightDrawerTab } from '../types';
import { LeftSidebar } from '../sidebar/LeftSidebar';
import { WorkspaceHeader } from '../header/WorkspaceHeader';
import { RightPanelHost } from '../shared/RightPanelHost';
import { FilePreviewContent } from '../shared/FilePreviewContent';
import { BuiltinBrowser } from '../browser/BuiltinBrowser';
import { ContentViewHost } from '../stage/ContentViewHost';
import type { ContentView } from '../../stores/stageStore';
import { t } from '../../i18n';
import type { App, AppState } from '../app';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import * as modal from '../../stores/modalStore';
import * as tabsStore from '../../stores/tabsStore';

/**
 * Maps the active tab state to the primary pane's `ContentView`. Tasks is
 * now a peer primary view (the project landing), not a z-index background
 * layer. Full-page modules (providers/skills/discovery/settings) take the
 * primary pane full-width (secondary closed); otherwise it's the
 * tasks/terminal/chat/new-chat content. Bridge from the legacy signals
 * onto the unified pane renderer.
 */
function primaryViewFor(activeTabId: string, activeTab: string, drawerTab: RightDrawerTab): ContentView {
    if (isFullPageTab(drawerTab)) {
        // isFullPageTab guarantees one of providers/skills/discovery/settings.
        return { kind: drawerTab } as ContentView;
    }
    if (activeTabId === 'tasks') return { kind: 'tasks' };
    if (activeTab === 'new_chat') return { kind: 'newChat' };
    if (activeTab === 'agents') return { kind: 'chat' };
    return { kind: 'terminal' };
}

interface DesktopAppLayoutProps {
    app: App;
    state: AppState;
}

export class DesktopAppLayout extends Component<DesktopAppLayoutProps> {
    render() {
        const { app, state } = this.props;
        const tabs = tabsStore.tabs.value;
        const activeTabId = tabsStore.activeTabId.value;
        const activeDrawerTab = tabsStore.activeDrawerTab.value;
        const workspaces = wsStore.workspaces.value;
        const activeWorkspaceId = wsStore.activeWorkspaceId.value;
        const folders = wsStore.folders.value;
        const workspacesLoading = wsStore.workspacesLoading.value;
        const sidebarCollapsedGroups = wsStore.sidebarCollapsedGroups.value;
        const activeSession = sess.activeSession.value;
        const tmuxMouseOn = sess.tmuxMouseOn.value;
        const language = ui.language.value;
        const theme = ui.theme.value;
        const leftSidebarOpen = ui.leftSidebarOpen.value;
        const leftSidebarWidth = ui.leftSidebarWidth.value;
        const keyboardVisible = ui.keyboardVisible.value;

        const activeWorkspace = workspaces.find(w => w.id === activeWorkspaceId);
        const activeWorkspacePath = activeWorkspace?.path || '.';
        const activeTabObj = tabs.find(t => t.id === activeTabId);

        // Shell layout (LeftSidebar + WorkspaceHeader) is shared by the
        // project landing ('tasks') and the workbench ('terminal'). Dynamic
        // tabs (preview/browser) cover the whole content area without the shell.
        // The shell (LeftSidebar + WorkspaceHeader + workbench body) hosts
        // both the project landing ('tasks') and the workbench ('terminal').
        // Tasks now renders as a primary-pane ContentView, not a background
        // layer, so the body is shown for both.
        const isShell = activeTabId === 'tasks' || activeTabId === 'terminal';
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
                                        onClick={() => tabsStore.selectTab(tab.id)}
                                    >
                                        <span class="tab-title">{tab.title}</span>
                                        {tab.closable && (
                                            <span
                                                class="workspace-tab-close"
                                                onClick={(e: MouseEvent) => {
                                                    e.stopPropagation();
                                                    tabsStore.closeTab(tab.id);
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
                            onClick={() => tabsStore.openBrowserTab('')}
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
                                toggleFolder={wsStore.toggleFolder}
                                toggleDrawerTab={tabsStore.toggleDrawerTab}
                                activeDrawerTab={activeDrawerTab}
                                activeDiscoveryCategory={tabsStore.discoveryCategory.value}
                                onSelectDiscoveryCategory={tabsStore.selectDiscoveryCategory}
                                onCreateWorkspace={modal.openCreateWorkspacePicker}
                                onRenameWorkspace={ws => modal.openRenameWorkspaceModal(ws)}
                                onDeleteWorkspace={wsStore.deleteWorkspace}
                                onSelectWorkspace={ws => wsStore.selectWorkspace(ws)}
                                onSelectSession={s => sess.selectSession(s)}
                                onTerminalCreate={(wsId, cwd) => sess.createTerminal(wsId, cwd)}
                                onTerminalKill={idx => sess.killTerminal(idx)}
                                onRenameSession={s => modal.openRenameSessionModal(s)}
                                onReorderFolders={wsStore.reorderFolders}
                                language={language}
                                moduleNav={tabsStore.buildModuleNav()}
                                onChatCreate={modal.openChatCreate}
                                onChatKill={sess.killChatSession}
                                collapsedGroups={sidebarCollapsedGroups}
                                onToggleGroup={wsStore.toggleSidebarGroup}
                                onStartNewChat={sess.onStartNewChat}
                                activeTab={tabsStore.activeTab.value}
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
                                toggleDrawerTab={tabsStore.toggleDrawerTab}
                                activeTab={tabsStore.activeTab.value}
                                setActiveTab={tabsStore.setActiveTab}
                                theme={theme}
                                toggleTheme={ui.toggleTheme}
                                keyboardVisible={keyboardVisible}
                                workspaceName={activeWorkspace?.name || ''}
                                sessionName={activeSession?.name || ''}
                                tmuxMouseOn={tmuxMouseOn}
                                onTmuxMouseToggle={sess.toggleTmuxMouse}
                                language={language}
                                moduleNav={tabsStore.buildModuleNav()}
                                hasChatSession={folders.some(
                                    f => f.id === activeWorkspaceId && f.sessions.some(isChat)
                                )}
                            />
                        )}

                        {/*
                          [WORKBENCH BODY]: the content area = primary pane
                          (+ optional secondary drawer pane). Renders for both
                          'tasks' (primary = kanban) and 'terminal' (primary =
                          terminal/chat/new-chat). Full-page drawer tabs
                          (providers/skills/discovery/settings) fill the
                          primary pane; otherwise primary + Resizer + drawer.
                        */}
                        {isShell && (
                            <div
                                class={`workspace-body-container ${activeDrawerTab !== 'none' && !isFullPageTab(activeDrawerTab) ? 'drawer-open' : ''}`}
                            >
                                {isFullPageTab(activeDrawerTab) ? (
                                    // [SINGLE PANE]: full-page module fills the primary pane;
                                    // the secondary drawer is closed (single column).
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
                                        <ContentViewHost
                                            view={primaryViewFor(
                                                activeTabId,
                                                tabsStore.activeTab.value,
                                                activeDrawerTab
                                            )}
                                            app={app}
                                            state={state}
                                        />
                                    </div>
                                ) : (
                                    <Fragment>
                                        {/* [PRIMARY PANE]: terminal / chat / new-chat workbench */}
                                        <ContentViewHost
                                            view={primaryViewFor(
                                                activeTabId,
                                                tabsStore.activeTab.value,
                                                activeDrawerTab
                                            )}
                                            app={app}
                                            state={state}
                                            fontSize={13}
                                        />

                                        {/* Resizer: between PRIMARY pane and SECONDARY drawer pane */}
                                        {activeDrawerTab !== 'none' && (
                                            <div
                                                class="resizer resizer-right"
                                                onMouseDown={(e: MouseEvent) => app.handleResizerDown('right', e)}
                                                title={t('app.resizer.rightTitle', language)}
                                            />
                                        )}

                                        {/* [COLUMN 3]: RIGHT side dynamic sliding drawer panel */}
                                        <RightPanelHost
                                            app={app}
                                            state={state}
                                            activeWorkspaceId={activeWorkspaceId}
                                            activeWorkspacePath={activeWorkspacePath}
                                            rightPanelWidth={ui.rightPanelWidth.value}
                                            onExtraRefresh={async () => {
                                                try {
                                                    await app.checkAccessStatus();
                                                    await Promise.all([
                                                        wsStore.loadWorkspaces(true),
                                                        sess.loadTerminals(),
                                                    ]);

                                                    const workspaces = wsStore.workspaces.value;
                                                    const activeWorkspaceId = wsStore.activeWorkspaceId.value;
                                                    if (!activeWorkspaceId && workspaces.length > 0) {
                                                        await wsStore.selectWorkspace(workspaces[0]);
                                                    } else if (activeWorkspaceId) {
                                                        await Promise.all([
                                                            wsStore.loadCcConnectUrl(),
                                                            wsStore.loadCcProvidersUrl(),
                                                        ]);
                                                    }
                                                } catch (e) {
                                                    console.error('Failed to reconnect/refresh:', e);
                                                }
                                            }}
                                            onToggleFullscreen={() => {
                                                const selectedFsEntry = fs.selectedFsEntry.value;
                                                if (selectedFsEntry) {
                                                    const activeWorkspace = wsStore.workspaces.value.find(
                                                        w => w.id === wsStore.activeWorkspaceId.value
                                                    );
                                                    const activeWorkspacePath = activeWorkspace?.path || '.';
                                                    const absolutePath = selectedFsEntry.path.startsWith('/')
                                                        ? selectedFsEntry.path
                                                        : `${activeWorkspacePath}/${selectedFsEntry.path}`;
                                                    if (IS_DESKTOP) {
                                                        tabsStore.openPreviewTab(absolutePath, selectedFsEntry.name);
                                                    } else {
                                                        const shareUrl = `${window.location.origin}${
                                                            window.location.pathname
                                                        }?preview=${encodeURIComponent(absolutePath)}`;
                                                        window.open(shareUrl, '_blank');
                                                    }
                                                }
                                            }}
                                            onOpenPreview={
                                                IS_DESKTOP
                                                    ? (path, name) => tabsStore.openPreviewTab(path, name)
                                                    : undefined
                                            }
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
                                        <FilePreviewContent
                                            app={app}
                                            activeTabId={activeTabId}
                                            onOpenPreview={
                                                IS_DESKTOP
                                                    ? (path, name) => tabsStore.openPreviewTab(path, name)
                                                    : undefined
                                            }
                                        />
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
                                    {tabs
                                        .filter(t => t.type === 'browser')
                                        .map(t => (
                                            <BuiltinBrowser
                                                tab={t}
                                                active={activeTabId === t.id}
                                                onUrlChange={tabsStore.updateBrowserUrl}
                                                language={language}
                                            />
                                        ))}
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            </Fragment>
        );
    }
}
