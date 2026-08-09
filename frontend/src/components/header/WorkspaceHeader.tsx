import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { RightDrawerTab, isFullPageTab, type AgentType } from '../types';
import { t, type Lang } from '../i18n';
import type { ModuleManifest } from '../../modules/module-types';
import type { ConnectionState } from '@1agents/core/protocol/types';
import { AgentAvatar } from '../chat/AgentAvatar';
import { CrumbTrail, type Crumb } from '../platform/ShellNav';
import * as stage from '../../stores/stageStore';
import * as tabsStore from '../../stores/tabsStore';
import { isBeginnerMode } from '../../stores/uiStore';
import * as taskNav from '../../stores/taskNavStore';
import { activeWorkspaceDeviceId, activeWorkspaceId, remoteDevices, workspaces } from '../../stores/workspaceStore';
import * as shellStore from '../../stores/productShellStore';
import { ShellIcon } from '../settings/ProductShellPanel';

interface WorkspaceHeaderProps {
    leftSidebarOpen: boolean;
    toggleLeftSidebar: () => void;
    activeDrawerTab: RightDrawerTab;
    toggleDrawerTab: (tab: RightDrawerTab) => void;
    activeTab: 'terminal' | 'agents' | 'console' | 'folders' | 'new_chat';
    setActiveTab: (tab: 'terminal' | 'agents' | 'console' | 'folders' | 'new_chat') => void;
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
    keyboardVisible?: boolean;
    workspaceName: string;
    /** Absolute path of the active workspace — shown in the info-icon tooltip. */
    workspacePath?: string;
    sessionName: string;
    tmuxMouseOn?: boolean;
    onTmuxMouseToggle?: () => void;
    /** True when the primary pane is the xterm terminal (gates the tmux mouse toggle). */
    isTerminalView?: boolean;
    language: Lang;
    /**
     * Optional module manifest for the active drawer tab. When set, the
     * mobile hamburger menu gets a section that mirrors the manifest so
     * the user always sees the module's navigation in the host chrome.
     */
    moduleNav?: {
        manifest: ModuleManifest;
        activePath: string;
        onNavigate: (to: string) => void;
    };
    onBack?: () => void;
    /** True when the active workspace has at least one chat session. */
    hasChatSession?: boolean;
    /**
     * Active chat session descriptors. When set (i.e. the active session is a
     * chat), the header renders the agent avatar and the live connection
     * status — the former chat status bar, now merged into the header. Absent
     * for terminal/tasks views, which fall back to just the name group.
     */
    agentType?: AgentType;
    /** Active session run status for the agent avatar indicator. */
    sessionStatus?: string;
    connection?: ConnectionState;
    /**
     * Force-reconnect the active chat session (header refresh button, sits
     * to the right of the project info icon). Absent when no chat session
     * is active.
     */
    onRefreshSession?: () => void;
    /**
     * When set, overrides the default session/entity title with a fixed
     * breadcrumb trail (e.g. for project-overview and project-detail modes
     * where there is no active session to surface).
     */
    customCrumbs?: Crumb[];
    /**
     * When false, hides the right-side column control buttons (chat/tasks/
     * files/git toggles). Pass false for layout modes where the two-column
     * workbench controls are not applicable (project-overview, project-detail).
     */
    showControls?: boolean;
}

// Focus panes that live under the 助手 context (gated to it in the sidebar) —
// they show a "助手 › <module>" breadcrumb. Everything else full-page
// (数据源 / 设置 / 技能 / 发现 / providers) is a first-level module and shows
// just its own name, no parent.
const ASSISTANT_FOCUS_TABS: RightDrawerTab[] = ['contacts', 'inbox', 'reminders'];

const FULLPAGE_TITLE_KEYS: Partial<Record<RightDrawerTab, string>> = {
    providers: 'header.title.providers',
    skills: 'header.title.skills',
    settings: 'header.title.settings',
    discovery: 'header.title.discovery',
    reminders: 'header.title.reminders',
    contacts: 'header.title.contacts',
    datasources: 'header.title.datasources',
    inbox: 'header.title.inbox',
};

export function WorkspaceHeader(props: WorkspaceHeaderProps) {
    const {
        leftSidebarOpen,
        toggleLeftSidebar,
        activeDrawerTab,
        toggleDrawerTab,
        activeTab,
        workspaceName,
        workspacePath,
        sessionName,
        tmuxMouseOn,
        onTmuxMouseToggle,
        isTerminalView,
        language,
        onBack,
        agentType,
        sessionStatus,
        connection,
        onRefreshSession,
        customCrumbs,
        showControls = true,
    } = props;

    // Mobile hamburger menu open state
    const mobileMenuOpen = useSignal(false);

    const toggleMobileMenu = () => (mobileMenuOpen.value = !mobileMenuOpen.value);
    const closeMobileMenu = () => (mobileMenuOpen.value = false);

    // ── Shared SVG icons ──────────────────────────────────────────────────
    const IconFiles = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
        </svg>
    );
    const IconGit = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="12" cy="18" r="3" />
            <circle cx="6" cy="6" r="3" />
            <circle cx="18" cy="6" r="3" />
            <path d="M18 9v1a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V9" />
            <line x1="12" x2="12" y1="12" y2="15" />
        </svg>
    );
    // Built-in browser (globe) — peer of 文件 in the right column
    const IconBrowser = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="12" cy="12" r="10" />
            <line x1="2" y1="12" x2="22" y2="12" />
            <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
        </svg>
    );
    // AI Agent / chat icon (used as the unified 会话 view icon on mobile)
    const IconAgents = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
            <path d="M9 10h.01M12 10h.01M15 10h.01" />
        </svg>
    );
    // Chat-column collapse toggle icon (panel on the left)
    const IconChatColumn = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
            <line x1="9" y1="3" x2="9" y2="21" />
        </svg>
    );
    // Tasks dashboard icon
    const IconTasks = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
            <line x1="9" y1="9" x2="15" y2="9" />
            <line x1="9" y1="13" x2="15" y2="13" />
            <line x1="9" y1="17" x2="15" y2="17" />
        </svg>
    );

    // Data sources (database cylinder) icon
    const IconDataSources = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <ellipse cx="12" cy="5" rx="9" ry="3" />
            <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3" />
            <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5" />
        </svg>
    );

    // Hamburger / Close icon
    const IconHamburger = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
        </svg>
    );
    const IconClose = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
    );

    // Two-column toggle state (desktop): chat shown unless railed; the chat
    // toggle is disabled when chat is the only column (the ≥1 invariant).
    const collapsed = stage.collapsed.value;
    const hasContent = stage.hasContent.value;
    const chatShown = collapsed !== 'chat';

    // "会话" view is active when no artifact drawer is open — the current
    // session's workbench (chat or terminal) is showing.
    const sessionActive = activeDrawerTab === 'none';
    const sidePanelOpen = tabsStore.sidePanelOpen.value;

    // Product Shells (#328): enabled shells + the active one, for the mobile
    // menu drawer's switch section (desktop uses the header <ShellSwitcher />).
    const shellList = shellStore.enabledShells.value;
    const activeShellId = shellStore.activeShellId.value;

    // Entity context (breadcrumb L3): when the active workspace is an assistant
    // OR a project, the session view's header shows the full 助理/项目 › <name> ›
    // <session> trail instead of a bare session name — clicking a crumb pops
    // back to the overview / that entity's detail (L1 / L2).
    const activeWs = workspaces.value.find(w => w.id === activeWorkspaceId.value);
    const wsKind = activeWs?.kind ?? 'project';
    const isAssistantCtx = wsKind === 'workforce';
    const isTmpCtx = wsKind === 'tmp' || !!activeWs?.id?.startsWith('tmp-');
    const isProjectCtx = !!activeWs && wsKind === 'project' && !activeWs.builtin && activeWs.id !== 'default';
    const isEntityCtx = isAssistantCtx || isProjectCtx || isTmpCtx;
    const openOverview = () => {
        if (isAssistantCtx) {
            tabsStore.assistantDetailId.value = null;
            tabsStore.activeDrawerTab.value = 'assistants';
        } else if (!isTmpCtx) {
            stage.projectOverview();
        }
    };
    const openEntityDetail = () => {
        if (!activeWs || isTmpCtx) return;
        if (isAssistantCtx) {
            tabsStore.assistantDetailId.value = activeWs.id;
            tabsStore.activeDrawerTab.value = 'assistants';
        } else {
            stage.enterProjectDetail(activeWs.id, activeWs.name);
        }
    };
    // L3 crumb: the new-chat landing shows 新建对话 until the session starts,
    // then the session title takes its place. Tmp hides the path; breadcrumb is 单次对话 › session.
    const entityLeaf = activeTab === 'new_chat' ? t('assistant.detail.newChat', language) : sessionName;
    const rootLabel = isTmpCtx
        ? t('newchat.kind.oneshot', language)
        : isAssistantCtx
          ? t('sidebar.assistants', language)
          : t('projectHome.title', language);
    const entityCrumbs = activeWs
        ? isTmpCtx
            ? [{ label: rootLabel }, ...(entityLeaf ? [{ label: entityLeaf }] : [{ label: activeWs.name }])]
            : [
                  { label: rootLabel, onClick: openOverview },
                  ...(entityLeaf
                      ? [{ label: activeWs.name, onClick: openEntityDetail }, { label: entityLeaf }]
                      : [{ label: activeWs.name }]),
              ]
        : [];
    const effectiveBack = taskNav.headerBackAction.value ?? onBack;

    // Show the current session: close any open artifact drawer (keeps the
    // session's own chat/terminal tab as-is).
    const handleShowSession = () => {
        if (activeDrawerTab !== 'none') {
            toggleDrawerTab(activeDrawerTab);
        }
        closeMobileMenu();
    };

    // Helper: switch to a drawer view and close the mobile menu.
    const handleDrawerToggle = (tab: RightDrawerTab) => {
        toggleDrawerTab(tab);
        closeMobileMenu();
    };

    return (
        <Fragment>
            <header class="workspace-header">
                <div class="header-left">
                    {effectiveBack ? (
                        <button
                            type="button"
                            class="header-back-btn"
                            onClick={effectiveBack}
                            title={t('header.back', language)}
                            aria-label={t('header.back', language)}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="15 18 9 12 15 6" />
                            </svg>
                        </button>
                    ) : (
                        !leftSidebarOpen && (
                            <button
                                class="sidebar-toggle-btn"
                                onClick={toggleLeftSidebar}
                                style="margin-right: 4px;"
                                title={t('header.expandSidebar', language)}
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
                        )
                    )}
                    {taskNav.headerCrumbs.value ? (
                        // Drill-in breadcrumb published by a per-tab surface
                        // (SkillsTab, AssistantsPage, DataSources, …) — wins over
                        // the layout-mode customCrumbs so any mode (助理 detail,
                        // 项目 detail, full-page modules) shows the same drill
                        // trail through one mechanism.
                        <div class="header-title-group header-crumb-group">
                            <CrumbTrail crumbs={taskNav.headerCrumbs.value} />
                        </div>
                    ) : customCrumbs ? (
                        // Layout-mode override (project-overview / project-detail):
                        // show a fixed breadcrumb trail instead of session context.
                        <div class="header-title-group header-crumb-group">
                            <CrumbTrail crumbs={customCrumbs} />
                        </div>
                    ) : isFullPageTab(activeDrawerTab) ? (
                        <div class="header-title-group header-crumb-group">
                            <CrumbTrail
                                crumbs={
                                    // A full-page module can publish its own drill breadcrumb
                                    // (e.g. 数据源 › 联系人) — that wins over the default title.
                                    taskNav.headerCrumbs.value
                                        ? taskNav.headerCrumbs.value
                                        : ASSISTANT_FOCUS_TABS.includes(activeDrawerTab)
                                          ? [
                                                {
                                                    label: t('sidebar.mode.assistant', language),
                                                    // Clicking 助手 closes the focus pane, back to the conversation.
                                                    onClick: () => toggleDrawerTab(activeDrawerTab),
                                                },
                                                { label: t(FULLPAGE_TITLE_KEYS[activeDrawerTab] ?? '', language) },
                                            ]
                                          : // First-level module (数据源/设置/…) — its own root, no parent.
                                            [{ label: t(FULLPAGE_TITLE_KEYS[activeDrawerTab] ?? '', language) }]
                                }
                            />
                        </div>
                    ) : (
                        <div class={`header-title-group${isEntityCtx ? ' header-crumb-group' : ''}`}>
                            {agentType && (
                                <AgentAvatar agentType={agentType} status={sessionStatus} class="header-agent-avatar" />
                            )}
                            {isEntityCtx ? (
                                <CrumbTrail crumbs={entityCrumbs} />
                            ) : (
                                <span class="session-name">{sessionName || t('header.noSession', language)}</span>
                            )}
                            {connection && (
                                <span class={`header-conn header-conn-${connection}`}>
                                    {connectionLabel(connection)}
                                </span>
                            )}
                            {(workspaceName || workspacePath) && (
                                <span class="header-project-info" tabIndex={0}>
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <circle cx="12" cy="12" r="10" />
                                        <line x1="12" y1="16" x2="12" y2="12" />
                                        <line x1="12" y1="8" x2="12.01" y2="8" />
                                    </svg>
                                    <span class="header-project-tooltip">
                                        <span class="header-project-tooltip-name">
                                            {workspaceName || t('header.noWorkspace', language)}
                                        </span>
                                        {workspacePath && (
                                            <span class="header-project-tooltip-path">{workspacePath}</span>
                                        )}
                                    </span>
                                </span>
                            )}
                            {onRefreshSession && (
                                <button
                                    type="button"
                                    class="header-session-refresh"
                                    onClick={onRefreshSession}
                                    title={t('header.refreshSession', language)}
                                    aria-label={t('header.refreshSession', language)}
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M23 4v6h-6M1 20v-6h6" />
                                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
                                    </svg>
                                </button>
                            )}
                            {/* 多设备(#114):连接到远程设备时,在标题栏标出设备名。 */}
                            {activeWorkspaceDeviceId.value &&
                                (() => {
                                    const dev = remoteDevices.value.find(d => d.id === activeWorkspaceDeviceId.value);
                                    return (
                                        <span class="header-device-badge" title={dev?.name || ''}>
                                            <span class="device-status-dot online" aria-hidden="true" />
                                            {t('sidebar.device.connectedBanner', language, {
                                                name: dev?.name || activeWorkspaceDeviceId.value,
                                            })}
                                        </span>
                                    );
                                })()}
                        </div>
                    )}
                </div>

                {!isFullPageTab(activeDrawerTab) && showControls && (
                    <div class="header-right">
                        {onTmuxMouseToggle && isTerminalView && (
                            <button
                                class={`tmux-mouse-toggle ${tmuxMouseOn ? 'active' : ''}`}
                                onClick={onTmuxMouseToggle}
                                title={
                                    tmuxMouseOn
                                        ? t('header.modeToggleTitleScroll', language)
                                        : t('header.modeToggleTitleSelect', language)
                                }
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect x="5" y="2" width="14" height="20" rx="7" />
                                    <path d="M12 2v6" />
                                    <path d="M5 10h14" />
                                </svg>
                                <span>
                                    {t(tmuxMouseOn ? 'header.modeLabelScroll' : 'header.modeLabelSelect', language)}
                                </span>
                            </button>
                        )}

                        {onTmuxMouseToggle && isTerminalView && <div class="divider" />}

                        {/* ── Two-column toggle cluster (icon-only, top-right) ──
                            Left: single chat-column toggle. Divider. Right: four
                            single-select artifact tabs (项目管理 · 渠道 · 文件 · Git). */}
                        <button
                            id="hdr-btn-chat"
                            class={`shortcut-btn ${chatShown ? 'active' : ''}`}
                            onClick={stage.toggleChat}
                            disabled={!hasContent}
                            title={t(chatShown ? 'header.col.collapseChat' : 'header.col.expandChat', language)}
                            aria-label={t(chatShown ? 'header.col.collapseChat' : 'header.col.expandChat', language)}
                            aria-pressed={chatShown}
                        >
                            {IconChatColumn}
                        </button>

                        <div class="divider" />

                        <button
                            id="hdr-btn-side-panel"
                            class={`shortcut-btn ${sidePanelOpen ? 'active' : ''}`}
                            onClick={() => {
                                if (sidePanelOpen) {
                                    tabsStore.closeContentTab();
                                } else {
                                    tabsStore.openSidePanel();
                                }
                            }}
                            title={t(sidePanelOpen ? 'sidePanel.close' : 'sidePanel.open', language)}
                            aria-label={t(sidePanelOpen ? 'sidePanel.close' : 'sidePanel.open', language)}
                            aria-pressed={sidePanelOpen}
                        >
                            {IconFiles}
                        </button>
                    </div>
                )}

                {/* Mobile: hamburger button (only visible on mobile via CSS) */}
                <button
                    id="mob-hamburger-btn"
                    class={`mobile-hamburger-btn ${mobileMenuOpen.value ? 'open' : ''}`}
                    onClick={toggleMobileMenu}
                    title={t('header.menu', language)}
                    aria-label={t('header.openMenu', language)}
                    aria-expanded={mobileMenuOpen.value}
                >
                    {mobileMenuOpen.value ? IconClose : IconHamburger}
                </button>
            </header>

            {/* Mobile: slide-down drawer menu */}
            {mobileMenuOpen.value && <div class="mobile-menu-backdrop" onClick={closeMobileMenu} />}
            <div class={`mobile-menu-drawer ${mobileMenuOpen.value ? 'open' : ''}`}>
                <div class="mobile-menu-section-title">{t('header.mobile.switchView', language)}</div>

                {/* Unified in-project views, fixed order: 会话 · 任务 · 文件 · 版本 · 渠道.
                    会话 shows the current session; the rest are scoped to its
                    project (activeWorkspaceId). Tapping one switches the whole pane. */}
                <button
                    id="mob-menu-session"
                    class={`mobile-menu-item ${sessionActive ? 'active' : ''}`}
                    onClick={handleShowSession}
                >
                    <span class="mob-menu-icon">{IconAgents}</span>
                    <span class="mob-menu-label">{t('header.mobile.session', language)}</span>
                    {sessionActive && <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>}
                </button>

                {!isBeginnerMode.value && (
                    <button
                        id="mob-menu-tasks"
                        class={`mobile-menu-item ${activeDrawerTab === 'tasks' ? 'active' : ''}`}
                        onClick={() => handleDrawerToggle('tasks')}
                    >
                        <span class="mob-menu-icon">{IconTasks}</span>
                        <span class="mob-menu-label">{t('header.mobile.tasks', language)}</span>
                        {activeDrawerTab === 'tasks' && (
                            <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>
                        )}
                    </button>
                )}

                <button
                    id="mob-menu-files"
                    class={`mobile-menu-item ${activeDrawerTab === 'files' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('files')}
                >
                    <span class="mob-menu-icon">{IconFiles}</span>
                    <span class="mob-menu-label">{t('header.mobile.files', language)}</span>
                    {activeDrawerTab === 'files' && (
                        <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>
                    )}
                </button>

                <button
                    id="mob-menu-browser"
                    class={`mobile-menu-item ${activeDrawerTab === 'browser' ? 'active' : ''}`}
                    onClick={() => {
                        closeMobileMenu();
                        if (activeDrawerTab === 'browser') {
                            handleDrawerToggle('browser');
                        } else {
                            tabsStore.openBrowserTab('');
                        }
                    }}
                >
                    <span class="mob-menu-icon">{IconBrowser}</span>
                    <span class="mob-menu-label">{t('header.mobile.browser', language)}</span>
                    {activeDrawerTab === 'browser' && (
                        <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>
                    )}
                </button>

                <button
                    id="mob-menu-git"
                    class={`mobile-menu-item ${activeDrawerTab === 'git' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('git')}
                >
                    <span class="mob-menu-icon">{IconGit}</span>
                    <span class="mob-menu-label">{t('header.mobile.git', language)}</span>
                    {activeDrawerTab === 'git' && (
                        <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>
                    )}
                </button>

                <button
                    id="mob-menu-datasources"
                    class={`mobile-menu-item ${activeDrawerTab === 'datasources' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('datasources')}
                >
                    <span class="mob-menu-icon">{IconDataSources}</span>
                    <span class="mob-menu-label">{t('sidebar.datasources', language)}</span>
                    {activeDrawerTab === 'datasources' && (
                        <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>
                    )}
                </button>

                {/* Product Shells (#328): switch the active product shell. */}
                {shellList.length > 0 && (
                    <div class="mobile-menu-section-title">{t('settings.nav.shells', language)}</div>
                )}
                {shellList.map(s => (
                    <button
                        key={s.id}
                        id={`mob-menu-shell-${s.id}`}
                        class={`mobile-menu-item ${activeShellId === s.id ? 'active' : ''}`}
                        onClick={() => {
                            closeMobileMenu();
                            if (activeShellId !== s.id) stage.switchShell(s.id);
                        }}
                    >
                        <span class="mob-menu-icon">
                            <ShellIcon id={s.id} />
                        </span>
                        <span class="mob-menu-label">{s.name}</span>
                        {activeShellId === s.id && (
                            <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>
                        )}
                    </button>
                ))}
            </div>
        </Fragment>
    );
}

/** Connection-state label, ported verbatim from the former SessionStatusBar. */
function connectionLabel(state: ConnectionState): string {
    switch (state) {
        case 'idle':
            return '未连接';
        case 'connecting':
            return '连接中…';
        case 'connected':
            return '已连接';
        case 'reconnecting':
            return '连接已断开，正在重连…';
        case 'closed':
            return '已关闭';
        case 'error':
            return '会话不可用';
    }
}
