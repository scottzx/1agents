import { h, Fragment } from 'preact';
import { useState } from 'preact/hooks';
import { RightDrawerTab, isFullPageTab } from '../types';
import { t, type Lang } from '../i18n';
import type { ModuleManifest } from '../../modules/module-types';

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
    sessionName: string;
    tmuxMouseOn?: boolean;
    onTmuxMouseToggle?: () => void;
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
}

const FULLPAGE_TITLE_KEYS: Partial<Record<RightDrawerTab, string>> = {
    providers: 'header.title.providers',
    skills: 'header.title.skills',
    settings: 'header.title.settings',
    discovery: 'header.title.discovery',
};

export function WorkspaceHeader(props: WorkspaceHeaderProps) {
    const {
        leftSidebarOpen,
        toggleLeftSidebar,
        activeDrawerTab,
        toggleDrawerTab,
        activeTab,
        setActiveTab,
        workspaceName,
        sessionName,
        tmuxMouseOn,
        onTmuxMouseToggle,
        language,
        onBack,
        hasChatSession,
    } = props;

    // Mobile hamburger menu open state
    const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

    const toggleMobileMenu = () => setMobileMenuOpen(v => !v);
    const closeMobileMenu = () => setMobileMenuOpen(false);

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
    // Terminal / session icon
    const IconSession = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" x2="20" y1="19" y2="19" />
        </svg>
    );
    // AI Agent / chat icon
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
    // AI Chat channels icon
    const IconChannels = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
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

    // session tab is "active" when terminal is shown and right panel is closed
    const sessionActive = activeTab === 'terminal' && activeDrawerTab === 'none';

    const handleSessionClick = () => {
        setActiveTab('terminal');
        // On mobile: collapse the right panel to reveal the terminal full-screen
        if (activeDrawerTab !== 'none') {
            toggleDrawerTab(activeDrawerTab);
        }
        closeMobileMenu();
    };

    // Helper: toggle a drawer tab and close the mobile menu
    const handleDrawerToggle = (tab: RightDrawerTab) => {
        toggleDrawerTab(tab);
        closeMobileMenu();
    };

    return (
        <Fragment>
            <header class="workspace-header">
                <div class="header-left">
                    {onBack ? (
                        <button
                            class="header-back-btn"
                            onClick={onBack}
                            style="margin-right: 8px; display: flex; align-items: center; justify-content: center; background: none; border: none; color: var(--text-main); cursor: pointer; padding: 4px;"
                            title="Back"
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                style="width: 20px; height: 20px;"
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
                    {isFullPageTab(activeDrawerTab) ? (
                        <div class="header-title-group">
                            <span class="ws-name" style="font-weight: 600;">
                                {t(FULLPAGE_TITLE_KEYS[activeDrawerTab] ?? '', language)}
                            </span>
                        </div>
                    ) : (
                        <div class="header-title-group">
                            <span class="ws-name">{workspaceName || t('header.noWorkspace', language)}</span>
                            <span class="session-name">{sessionName || t('header.noSession', language)}</span>
                        </div>
                    )}
                </div>

                {!isFullPageTab(activeDrawerTab) && (
                    <div class="header-right">
                        {onTmuxMouseToggle && (
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

                        {onTmuxMouseToggle && <div class="divider" />}

                        {hasChatSession && (
                            <button
                                id="hdr-btn-agents"
                                class={`shortcut-btn ${activeTab === 'agents' ? 'active' : ''}`}
                                onClick={() => setActiveTab('agents')}
                                title="智能体聊天"
                            >
                                {IconAgents}
                            </button>
                        )}
                        <button
                            id="hdr-btn-channels"
                            class={`shortcut-btn ${activeDrawerTab === 'channels' ? 'active' : ''}`}
                            onClick={() => toggleDrawerTab('channels')}
                            title={t('header.channels', language)}
                        >
                            {IconChannels}
                        </button>
                        <button
                            id="hdr-btn-files"
                            class={`shortcut-btn ${activeDrawerTab === 'files' ? 'active' : ''}`}
                            onClick={() => toggleDrawerTab('files')}
                            title={t('header.files', language)}
                        >
                            {IconFiles}
                        </button>
                        <button
                            id="hdr-btn-git"
                            class={`shortcut-btn ${activeDrawerTab === 'git' ? 'active' : ''}`}
                            onClick={() => toggleDrawerTab('git')}
                            title={t('header.git', language)}
                        >
                            {IconGit}
                        </button>
                    </div>
                )}

                {/* Mobile: hamburger button (only visible on mobile via CSS) */}
                <button
                    id="mob-hamburger-btn"
                    class={`mobile-hamburger-btn ${mobileMenuOpen ? 'open' : ''}`}
                    onClick={toggleMobileMenu}
                    title={t('header.menu', language)}
                    aria-label={t('header.openMenu', language)}
                    aria-expanded={mobileMenuOpen}
                >
                    {mobileMenuOpen ? IconClose : IconHamburger}
                </button>
            </header>

            {/* Mobile: slide-down drawer menu */}
            {mobileMenuOpen && <div class="mobile-menu-backdrop" onClick={closeMobileMenu} />}
            <div class={`mobile-menu-drawer ${mobileMenuOpen ? 'open' : ''}`}>
                <div class="mobile-menu-section-title">{t('header.mobile.switchView', language)}</div>

                <button
                    id="mob-menu-terminal"
                    class={`mobile-menu-item ${sessionActive ? 'active' : ''}`}
                    onClick={handleSessionClick}
                >
                    <span class="mob-menu-icon">{IconSession}</span>
                    <span class="mob-menu-label">{t('header.mobile.workbench', language)}</span>
                    {sessionActive && <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>}
                </button>

                {hasChatSession && (
                    <button
                        id="mob-menu-agents"
                        class={`mobile-menu-item ${activeTab === 'agents' ? 'active' : ''}`}
                        onClick={() => {
                            setActiveTab('agents');
                            closeMobileMenu();
                        }}
                    >
                        <span class="mob-menu-icon">{IconAgents}</span>
                        <span class="mob-menu-label">智能体</span>
                        {activeTab === 'agents' && <span class="mob-menu-badge">当前</span>}
                    </button>
                )}

                <button
                    id="mob-menu-channels"
                    class={`mobile-menu-item ${activeDrawerTab === 'channels' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('channels')}
                >
                    <span class="mob-menu-icon">{IconChannels}</span>
                    <span class="mob-menu-label">{t('header.mobile.channels', language)}</span>
                    {activeDrawerTab === 'channels' && (
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
                    )}
                </button>
                <button
                    id="mob-menu-tasks"
                    class={`mobile-menu-item ${activeDrawerTab === 'tasks' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('tasks')}
                >
                    <span class="mob-menu-icon">{IconTasks}</span>
                    <span class="mob-menu-label">任务仪表盘</span>
                    {activeDrawerTab === 'tasks' && (
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
                    )}
                </button>

                <button
                    id="mob-menu-files"
                    class={`mobile-menu-item ${activeDrawerTab === 'files' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('files')}
                >
                    <span class="mob-menu-icon">{IconFiles}</span>
                    <span class="mob-menu-label">{t('header.mobile.files', language)}</span>
                    {activeDrawerTab === 'files' && (
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
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
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
                    )}
                </button>
            </div>
        </Fragment>
    );
}
