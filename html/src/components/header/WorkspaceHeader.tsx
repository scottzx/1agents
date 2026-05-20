import { h } from 'preact';
import { RightDrawerTab } from '../types';

interface WorkspaceHeaderProps {
    leftSidebarOpen: boolean;
    toggleLeftSidebar: () => void;
    activeDrawerTab: RightDrawerTab;
    toggleDrawerTab: (tab: RightDrawerTab) => void;
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
}

export function WorkspaceHeader({
    leftSidebarOpen,
    toggleLeftSidebar,
    activeDrawerTab,
    toggleDrawerTab,
    theme,
    toggleTheme,
}: WorkspaceHeaderProps) {
    return (
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

            {/* Coze right shortcut buttons in red box */}
            <div class="header-right">
                <button
                    class={`shortcut-btn ${activeDrawerTab === 'files' ? 'active' : ''}`}
                    onClick={() => toggleDrawerTab('files')}
                    title="文件浏览器"
                >
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
                </button>
                <button
                    class={`shortcut-btn ${activeDrawerTab === 'tasks' ? 'active' : ''}`}
                    onClick={() => toggleDrawerTab('tasks')}
                    title="任务追踪与调试"
                >
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z" />
                        <path d="m9 12 2 2 4-4" />
                    </svg>
                </button>
                <button
                    class={`shortcut-btn ${activeDrawerTab === 'settings' ? 'active' : ''}`}
                    onClick={() => toggleDrawerTab('settings')}
                    title="系统设置"
                >
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
                </button>
                <div class="divider" />
                <button
                    class="shortcut-btn"
                    onClick={() => toggleTheme()}
                    title={theme === 'light' ? '深色主题' : '浅色主题'}
                >
                    {theme === 'light' ? (
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                        </svg>
                    ) : (
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
                    )}
                </button>
            </div>
        </header>
    );
}
