import { h, Fragment } from 'preact';
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
}

export function WorkspaceHeader(props: WorkspaceHeaderProps) {
    const { leftSidebarOpen, toggleLeftSidebar, activeDrawerTab, toggleDrawerTab, activeTab, setActiveTab } = props;
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

    // session tab is "active" when terminal is shown and right panel is closed
    const sessionActive = activeTab === 'terminal' && activeDrawerTab === 'none';

    const handleSessionClick = () => {
        setActiveTab('terminal');
        // On mobile: collapse the right panel to reveal the terminal full-screen
        if (activeDrawerTab !== 'none') {
            toggleDrawerTab(activeDrawerTab);
        }
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
                        <span class="title">1agents</span>
                        <div class="status-indicator">
                            <div class="pulse-dot" />
                            <span>运行中</span>
                        </div>
                    </div>
                </div>

                {/* Desktop: right shortcut buttons — theme is inside settings drawer */}
                <div class="header-right">
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
            </header>

            {/* Mobile: fixed bottom navigation bar — 终端 / 文件 / 任务 / 设置 */}
            <nav class="mobile-bottom-nav">
                <button
                    id="mob-btn-session"
                    class={`mob-nav-btn ${sessionActive ? 'active' : ''}`}
                    onClick={handleSessionClick}
                >
                    {IconSession}
                    <span>终端</span>
                </button>
                <button
                    id="mob-btn-files"
                    class={`mob-nav-btn ${activeDrawerTab === 'files' ? 'active' : ''}`}
                    onClick={() => toggleDrawerTab('files')}
                >
                    {IconFiles}
                    <span>文件</span>
                </button>
                <button
                    id="mob-btn-git"
                    class={`mob-nav-btn ${activeDrawerTab === 'git' ? 'active' : ''}`}
                    onClick={() => toggleDrawerTab('git')}
                >
                    {IconGit}
                    <span>Git</span>
                </button>
                <button
                    id="mob-btn-settings"
                    class={`mob-nav-btn ${activeDrawerTab === 'settings' ? 'active' : ''}`}
                    onClick={() => toggleDrawerTab('settings')}
                >
                    {IconSettings}
                    <span>设置</span>
                </button>
            </nav>
        </Fragment>
    );
}
