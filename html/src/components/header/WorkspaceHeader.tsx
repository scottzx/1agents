import { h, Fragment } from 'preact';
import { useState } from 'preact/hooks';
import { RightDrawerTab } from '../types';

interface WorkspaceHeaderProps {
    leftSidebarOpen: boolean;
    toggleLeftSidebar: () => void;
    activeDrawerTab: RightDrawerTab;
    toggleDrawerTab: (tab: RightDrawerTab) => void;
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    setActiveTab: (tab: 'terminal' | 'agents' | 'console' | 'folders') => void;
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
    keyboardVisible?: boolean;
    sessionId: string;
    sessionPath: string;
}

export function WorkspaceHeader(props: WorkspaceHeaderProps) {
    const { leftSidebarOpen, toggleLeftSidebar, activeDrawerTab, toggleDrawerTab, activeTab, setActiveTab, sessionId, sessionPath } = props;

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
    const IconSettings = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
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
                    {!leftSidebarOpen && (
                        <button
                            class="sidebar-toggle-btn"
                            onClick={toggleLeftSidebar}
                            style="margin-right: 4px;"
                            title="展开左侧栏"
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
                    <div class="header-title-group">
                        <span class="title">{sessionId || '无会话'}</span>
                    </div>
                </div>

                {/* Desktop: right shortcut buttons — channels, files, git, settings */}
                <div class="header-right">
                    <button
                        id="hdr-btn-channels"
                        class={`shortcut-btn ${activeDrawerTab === 'channels' ? 'active' : ''}`}
                        onClick={() => toggleDrawerTab('channels')}
                        title="AI 渠道连接"
                    >
                        {IconChannels}
                    </button>
                    <button
                        id="hdr-btn-files"
                        class={`shortcut-btn ${activeDrawerTab === 'files' ? 'active' : ''}`}
                        onClick={() => toggleDrawerTab('files')}
                        title="文件浏览器"
                    >
                        {IconFiles}
                    </button>
                    <button
                        id="hdr-btn-git"
                        class={`shortcut-btn ${activeDrawerTab === 'git' ? 'active' : ''}`}
                        onClick={() => toggleDrawerTab('git')}
                        title="版本控制 (Git)"
                    >
                        {IconGit}
                    </button>
                    <button
                        id="hdr-btn-settings"
                        class={`shortcut-btn ${activeDrawerTab === 'settings' ? 'active' : ''}`}
                        onClick={() => toggleDrawerTab('settings')}
                        title="系统设置（含主题）"
                    >
                        {IconSettings}
                    </button>
                </div>

                {/* Mobile: hamburger button (only visible on mobile via CSS) */}
                <button
                    id="mob-hamburger-btn"
                    class={`mobile-hamburger-btn ${mobileMenuOpen ? 'open' : ''}`}
                    onClick={toggleMobileMenu}
                    title="菜单"
                    aria-label="打开功能菜单"
                    aria-expanded={mobileMenuOpen}
                >
                    {mobileMenuOpen ? IconClose : IconHamburger}
                </button>
            </header>

            {/* Mobile: slide-down drawer menu */}
            {mobileMenuOpen && (
                <div class="mobile-menu-backdrop" onClick={closeMobileMenu} />
            )}
            <div class={`mobile-menu-drawer ${mobileMenuOpen ? 'open' : ''}`}>
                <div class="mobile-menu-section-title">切换视图</div>

                <button
                    id="mob-menu-terminal"
                    class={`mobile-menu-item ${sessionActive ? 'active' : ''}`}
                    onClick={handleSessionClick}
                >
                    <span class="mob-menu-icon">{IconSession}</span>
                    <span class="mob-menu-label">终端</span>
                    {sessionActive && <span class="mob-menu-badge">当前</span>}
                </button>

                <button
                    id="mob-menu-channels"
                    class={`mobile-menu-item ${activeDrawerTab === 'channels' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('channels')}
                >
                    <span class="mob-menu-icon">{IconChannels}</span>
                    <span class="mob-menu-label">AI 渠道</span>
                    {activeDrawerTab === 'channels' && <span class="mob-menu-badge">打开中</span>}
                </button>

                <button
                    id="mob-menu-files"
                    class={`mobile-menu-item ${activeDrawerTab === 'files' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('files')}
                >
                    <span class="mob-menu-icon">{IconFiles}</span>
                    <span class="mob-menu-label">文件浏览器</span>
                    {activeDrawerTab === 'files' && <span class="mob-menu-badge">打开中</span>}
                </button>

                <button
                    id="mob-menu-git"
                    class={`mobile-menu-item ${activeDrawerTab === 'git' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('git')}
                >
                    <span class="mob-menu-icon">{IconGit}</span>
                    <span class="mob-menu-label">版本控制 (Git)</span>
                    {activeDrawerTab === 'git' && <span class="mob-menu-badge">打开中</span>}
                </button>

                <button
                    id="mob-menu-settings"
                    class={`mobile-menu-item ${activeDrawerTab === 'settings' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('settings')}
                >
                    <span class="mob-menu-icon">{IconSettings}</span>
                    <span class="mob-menu-label">设置</span>
                    {activeDrawerTab === 'settings' && <span class="mob-menu-badge">打开中</span>}
                </button>
            </div>
        </Fragment>
    );
}
