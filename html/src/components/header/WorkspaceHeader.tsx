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
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    setActiveTab: (tab: 'terminal' | 'agents' | 'console' | 'folders') => void;
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
        moduleNav,
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

    // Settings (gear) icon
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
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
        </svg>
    );
    // Skills / puzzle icon
    const IconSkills = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M19.439 7.85c-.049.322.059.648.289.878l1.568 1.568c.47.47.706 1.087.706 1.704s-.235 1.233-.706 1.704l-1.611 1.611a.98.98 0 0 1-.837.276c-.47-.07-.802-.48-.968-.925a2.501 2.501 0 1 0-3.214 3.214c.446.166.855.497.925.968a.979.979 0 0 1-.276.837l-1.611 1.611c-.47.47-1.087.706-1.704.706s-1.233-.235-1.704-.706l-1.568-1.568a1.026 1.026 0 0 0-.878-.289c-.493.074-.84.348-1.08.649L6.36 21.95a2 2 0 0 1-2.828 0l-.482-.482a2 2 0 0 1 0-2.828L4.5 17.19c.302-.24.575-.587.649-1.08a1.026 1.026 0 0 0-.289-.878L3.292 13.66c-.47-.47-.706-1.087-.706-1.704s.235-1.233.706-1.704L4.903 8.64a.98.98 0 0 1 .837-.276c.47.07.802.48.968.925a2.501 2.501 0 1 0 3.214-3.214c-.446-.166-.855-.497-.925-.968a.979.979 0 0 1 .276-.837L10.884 3.66c.47-.47 1.087-.706 1.704-.706s1.233.235 1.704.706l1.568 1.568c.23.23.556.338.878.289.493-.074.84-.348 1.08-.649L19.36 3.42a2 2 0 0 1 2.828 0l.482.482a2 2 0 0 1 0 2.828L21.22 7.18c-.302.24-.575.587-.649 1.08z" />
            <path d="M12 8v8M8 12h8" />
        </svg>
    );
    // Providers / cloud icon
    const IconProviders = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 0 1 0 9z" />
            <circle cx="12" cy="10" r="3" />
        </svg>
    );
    // Discovery / compass icon
    const IconDiscovery = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="12" cy="12" r="10" />
            <polygon points="16.24 7.76 14.12 14.12 7.76 16.24 9.88 9.88 16.24 7.76" />
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

                <div class="mobile-menu-section-title">{t('header.mobile.manage', language)}</div>

                <button
                    id="mob-menu-settings"
                    class={`mobile-menu-item ${activeDrawerTab === 'settings' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('settings')}
                >
                    <span class="mob-menu-icon">{IconSettings}</span>
                    <span class="mob-menu-label">{t('header.mobile.settings', language)}</span>
                    {activeDrawerTab === 'settings' && (
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
                    )}
                </button>

                <button
                    id="mob-menu-skills"
                    class={`mobile-menu-item ${activeDrawerTab === 'skills' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('skills')}
                >
                    <span class="mob-menu-icon">{IconSkills}</span>
                    <span class="mob-menu-label">{t('header.mobile.skills', language)}</span>
                    {activeDrawerTab === 'skills' && (
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
                    )}
                </button>

                <button
                    id="mob-menu-providers"
                    class={`mobile-menu-item ${activeDrawerTab === 'providers' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('providers')}
                >
                    <span class="mob-menu-icon">{IconProviders}</span>
                    <span class="mob-menu-label">{t('header.mobile.providers', language)}</span>
                    {activeDrawerTab === 'providers' && (
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
                    )}
                </button>

                <button
                    id="mob-menu-discovery"
                    class={`mobile-menu-item ${activeDrawerTab === 'discovery' ? 'active' : ''}`}
                    onClick={() => handleDrawerToggle('discovery')}
                >
                    <span class="mob-menu-icon">{IconDiscovery}</span>
                    <span class="mob-menu-label">{t('header.mobile.discovery', language)}</span>
                    {activeDrawerTab === 'discovery' && (
                        <span class="mob-menu-badge">{t('header.mobile.opening', language)}</span>
                    )}
                </button>

                {moduleNav && (
                    <Fragment>
                        <div class="mobile-menu-section-title">{t('header.mobile.moduleNav', language)}</div>
                        {moduleNav.manifest.topLinks?.map(link => (
                            <button
                                key={`mnav-top-${link.key}`}
                                class={`mobile-menu-item ${moduleNav.activePath === link.to ? 'active' : ''}`}
                                onClick={() => {
                                    moduleNav.onNavigate(link.to);
                                    closeMobileMenu();
                                }}
                            >
                                <span class="mob-menu-label">{link.label}</span>
                                {moduleNav.activePath === link.to && (
                                    <span class="mob-menu-badge">{t('header.mobile.current', language)}</span>
                                )}
                            </button>
                        ))}
                        {moduleNav.manifest.groups.map(group => (
                            <Fragment key={`mnav-group-${group.key}`}>
                                <div class="mobile-menu-subtitle">{group.label}</div>
                                {group.links.map(link => (
                                    <button
                                        key={`mnav-link-${link.key}`}
                                        class={`mobile-menu-item mobile-menu-item--indent ${
                                            moduleNav.activePath === link.to ? 'active' : ''
                                        }`}
                                        onClick={() => {
                                            moduleNav.onNavigate(link.to);
                                            closeMobileMenu();
                                        }}
                                    >
                                        <span class="mob-menu-label">{link.label}</span>
                                        {link.count !== null && link.count !== undefined && (
                                            <span class="mob-menu-count">{link.count}</span>
                                        )}
                                    </button>
                                ))}
                            </Fragment>
                        ))}
                    </Fragment>
                )}
            </div>
        </Fragment>
    );
}
