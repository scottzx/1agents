import { h, Component, Fragment } from 'preact';
import { isFullPageTab, isChat } from '../types';
import { LeftSidebar } from '../sidebar/LeftSidebar';
import { WorkspaceHeader } from '../header/WorkspaceHeader';
import { RightPanelHost } from '../shared/RightPanelHost';
import { ContentViewHost } from '../stage/ContentViewHost';
import { ProjectHome } from '../platform/ProjectHome';
import { ProjectDetailShell } from '../platform/ProjectDetailShell';
import { GlobalSearch } from '../search/GlobalSearch';
import { t } from '../../i18n';
import type { App, AppState } from '../app';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import * as modal from '../../stores/modalStore';
import * as tabsStore from '../../stores/tabsStore';
import * as stage from '../../stores/stageStore';
import { globalBridgeManager } from '../chat/hooks';

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

        // File preview may still use workspace tabs; browser is a right-drawer tab.
        const isSidePreview = activeTabObj?.type === 'preview';
        // Unified two-column shell, read from the stage store: pane[0] is the
        // left CHAT column, pane[1] (optional) the right ARTIFACT column.
        const panes = stage.panes.value;
        const collapsed = stage.collapsed.value;
        const splitRatio = stage.splitRatio.value;
        // The desktop layout mode (#redesign): project-overview / project drive
        // their own full-width pages; focus / split share the header + body below.
        const mode = stage.layoutMode.value;
        const isFocusOrSplit = mode === 'focus' || mode === 'split';
        // 设置 / 发现中心 render their category nav as a top tab bar (ShellNav) in
        // the content pane now, so the sidebar shows the normal tree instead of
        // a module nav. Mobile keeps its own menu (buildModuleNav still serves it).
        const sidebarModuleNav =
            activeDrawerTab === 'settings' || activeDrawerTab === 'discovery' ? undefined : tabsStore.buildModuleNav();
        // The primary (left) pane's content kind. The tmux mouse toggle only
        // makes sense when the xterm terminal is the one showing.
        const primaryView = panes[0].view;
        const hasContent = panes.length > 1;
        // Built-in browser / file preview force a two-column workbench so they
        // land in the secondary (right) pane next to chat.
        const sidePaneActive = isSidePreview;
        const showRightPane = hasContent || sidePaneActive;
        // Browser/preview can open from project-overview; still show workbench split.
        const workbenchVisible = isFocusOrSplit || sidePaneActive || activeDrawerTab === 'browser';
        // Left column flex: hidden when railed, split-share otherwise.
        const chatPaneStyle =
            collapsed === 'chat'
                ? 'flex: 0 1 0; min-width: 0; overflow: hidden;'
                : showRightPane
                  ? `flex: ${splitRatio} 1 0; min-width: 0;`
                  : 'flex: 1 1 0; min-width: 0;';
        // Right column flex: fills when chat railed, split-share otherwise.
        const contentPaneStyle =
            collapsed === 'chat'
                ? 'flex: 1 1 0; width: auto; min-width: 0;'
                : `flex: ${1 - splitRatio} 1 0; width: auto; min-width: 0;`;

        // Shell stays up for browser/preview so they render as the right pane.
        const isShell = activeTabId === 'tasks' || activeTabId === 'terminal' || isSidePreview;
        // The general new-chat landing is a focused full-bleed page, so hide the
        // workspace header (it has no active session/workspace context yet). But
        // when launched from an assistant (locked), keep the header so its
        // breadcrumb shows 助理 › <name> › 新建对话.
        const isNewChat =
            tabsStore.activeTab.value === 'new_chat' &&
            !isFullPageTab(activeDrawerTab) &&
            !sess.lockedNewChatWorkspaceId.value;

        return (
            <Fragment>
                {/* workspace-tabs-bar hidden: browser lives in right drawer */}

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
                                onCreateWorkspace={modal.openCreateWorkspacePicker}
                                onRenameWorkspace={ws => modal.openRenameWorkspaceModal(ws)}
                                onSelectWorkspace={ws => {
                                    wsStore.selectWorkspace(ws);
                                    // 点击文件夹 → 统一下钻到对应详情页:
                                    // 助理模式进助理详情, 项目模式(非默认工作区)进项目详情。
                                    if (ui.sidebarMode.value === 'assistant') {
                                        tabsStore.assistantDetailId.value = ws.id;
                                        tabsStore.activeDrawerTab.value = 'assistants';
                                    } else if (ui.sidebarMode.value === 'project' && ws.id !== 'default') {
                                        stage.enterProjectDetail(ws.id, ws.name);
                                    } else {
                                        stage.enterProject();
                                    }
                                }}
                                onSelectSession={s => {
                                    // 同项目内打开/恢复/切换会话 → chat 领, 右栏保留;
                                    // 仅当切到别的项目时才关闭右栏。
                                    const projectChanged = s.workspaceId !== wsStore.activeWorkspaceId.value;
                                    sess.selectSession(s);
                                    stage.openConversation(projectChanged);
                                }}
                                onTerminalCreate={(wsId, cwd) => sess.createTerminal(wsId, cwd)}
                                onTerminalKill={idx => sess.killTerminal(idx)}
                                onRenameSession={s => modal.openRenameSessionModal(s)}
                                onReorderFolders={wsStore.reorderFolders}
                                language={language}
                                moduleNav={sidebarModuleNav}
                                onChatCreate={modal.openChatCreate}
                                onChatKill={sess.killChatSession}
                                onStartNewChat={() => {
                                    // 入口默认态: 新建对话 → chat 领, 右栏关闭。
                                    sess.onStartNewChat();
                                    stage.enterConversation();
                                }}
                                activeTab={tabsStore.activeTab.value}
                                activeSession={activeSession}
                                activeTabId={activeTabId}
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
                          [SHELL HEADER]: always shown when the shell is active
                          (all four layout modes), so the top bar stays stable
                          across mode switches — no 48px jump when navigating
                          between split/focus/project-overview/project-detail.
                          In project-overview and project modes, the header shows
                          a fixed breadcrumb instead of session context and hides
                          the workbench column controls.
                        */}
                        {isShell && !isNewChat && (
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
                                workspacePath={activeWorkspace?.path || ''}
                                sessionName={activeSession?.name || ''}
                                agentType={activeSession && isChat(activeSession) ? activeSession.agentType : undefined}
                                sessionStatus={
                                    activeSession && isChat(activeSession)
                                        ? sess.liveSessionStatus.value[activeSession.id] ?? activeSession.status
                                        : undefined
                                }
                                connection={
                                    activeSession && isChat(activeSession)
                                        ? sess.liveSessionConnection.value[activeSession.id] ?? 'idle'
                                        : undefined
                                }
                                onRefreshSession={
                                    activeSession && isChat(activeSession)
                                        ? () => {
                                              globalBridgeManager.dismissError(activeSession.id);
                                              globalBridgeManager.retry(activeSession);
                                          }
                                        : undefined
                                }
                                tmuxMouseOn={tmuxMouseOn}
                                onTmuxMouseToggle={sess.toggleTmuxMouse}
                                isTerminalView={activeTabId === 'terminal' && primaryView.kind === 'terminal'}
                                language={language}
                                moduleNav={sidebarModuleNav}
                                hasChatSession={folders.some(
                                    f => f.id === activeWorkspaceId && f.sessions.some(isChat)
                                )}
                                customCrumbs={
                                    mode === 'project-overview'
                                        ? [{ label: t('projectHome.title', language) }]
                                        : mode === 'project' && stage.projectStack.value.length > 0
                                          ? [
                                                {
                                                    label: t('projectHome.title', language),
                                                    onClick: stage.projectOverview,
                                                },
                                                {
                                                    label: stage.projectStack.value[stage.projectStack.value.length - 1]
                                                        .name,
                                                },
                                            ]
                                          : undefined
                                }
                                showControls={isFocusOrSplit}
                            />
                        )}

                        {/* [项目总览]: the 项目 card wall (empty drill stack). */}
                        {isShell && mode === 'project-overview' && !sidePaneActive && activeDrawerTab !== 'browser' && (
                            <ProjectHome />
                        )}

                        {/* [项目详情]: a drilled-in project's detail page. */}
                        {isShell && mode === 'project' && !sidePaneActive && activeDrawerTab !== 'browser' && (
                            <ProjectDetailShell app={app} />
                        )}

                        {/*
                          [WORKBENCH BODY]: the unified two-column shell —
                          left CHAT column (terminal/chat/new-chat) + right
                          ARTIFACT column (项目管理/渠道/文件/Git/PM). Either
                          column can collapse, but never both (stageStore
                          `collapsed`). Full-page modules (providers/skills/
                          discovery/settings) take over as a single full-width
                          pane instead.
                        */}
                        {isShell && workbenchVisible && (
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
                                        <ContentViewHost view={primaryView} app={app} state={state} />
                                    </div>
                                ) : (
                                    <Fragment>
                                        {/* [LEFT / CHAT COLUMN]: terminal / chat / new-chat.
                                            Railed to width 0 when collapsed==='chat'; the rail
                                            below then offers to bring it back. */}
                                        <div class="stage-pane stage-pane-chat" style={chatPaneStyle}>
                                            <ContentViewHost view={primaryView} app={app} state={state} fontSize={13} />
                                        </div>

                                        {/* Collapsed chat rail + chevron (reuses the slide motion). */}
                                        {collapsed === 'chat' && (
                                            <button
                                                class="stage-chat-rail"
                                                onClick={stage.toggleChat}
                                                title={t('header.col.expandChat', language)}
                                                aria-label={t('header.col.expandChat', language)}
                                            >
                                                <svg
                                                    viewBox="0 0 24 24"
                                                    fill="none"
                                                    stroke="currentColor"
                                                    stroke-width="2.5"
                                                    stroke-linecap="round"
                                                    stroke-linejoin="round"
                                                >
                                                    <polyline points="9 18 15 12 9 6" />
                                                </svg>
                                            </button>
                                        )}

                                        {/* Resizer: between the two columns (only when both shown). */}
                                        {showRightPane && collapsed === 'none' && (
                                            <div
                                                class="resizer resizer-split"
                                                onMouseDown={(e: MouseEvent) => app.handleResizerDown('split', e)}
                                                title={t('app.resizer.rightTitle', language)}
                                            />
                                        )}

                                        {/* [RIGHT / ARTIFACT COLUMN]: tasks / channels / files /
                                            git / pm. Always mounted so closing slides out via the
                                            `.right-panel.collapsed` animation. */}
                                        <RightPanelHost
                                            app={app}
                                            state={state}
                                            activeWorkspaceId={activeWorkspaceId}
                                            activeWorkspacePath={activeWorkspacePath}
                                            rightPanelWidth={ui.rightPanelWidth.value}
                                            paneStyle={contentPaneStyle}
                                            onSelectSession={s => {
                                                const projectChanged =
                                                    s.workspaceId !== wsStore.activeWorkspaceId.value;
                                                sess.selectSession(s);
                                                stage.openConversation(projectChanged);
                                            }}
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
                                                    tabsStore.openPreviewTab(absolutePath, selectedFsEntry.name);
                                                }
                                            }}
                                            onOpenPreview={(path, name) => tabsStore.openPreviewTab(path, name)}
                                        />
                                    </Fragment>
                                )}
                            </div>
                        )}
                    </div>
                </div>
                <GlobalSearch />
            </Fragment>
        );
    }
}
