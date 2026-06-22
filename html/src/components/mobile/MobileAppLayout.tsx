import { h, Component, Fragment } from 'preact';
import { effect } from '@preact/signals';

import { WorkspaceHeader } from '../header/WorkspaceHeader';
import { isChat, AGENT_TYPE_LABELS, type Session } from '../types';
import { NewChatHome } from '../chat/NewChatHome';
import { AgentAvatar } from '../chat/AgentAvatar';
import { DiscoveryPanel } from '../drawer/DiscoveryPanel';
import { WorkbenchCanvas } from '../shared/WorkbenchCanvas';
import { RightPanelHost } from '../shared/RightPanelHost';
import { SystemSettingsHost } from '../shared/SystemSettingsHost';
import { FilePreviewContent } from '../shared/FilePreviewContent';
import { CcProvidersPanel } from '../shared/CcProvidersPanel';
import { BuiltinBrowser } from '../browser/BuiltinBrowser';
import { t } from '../../i18n';
import type { App, AppState } from '../app';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import * as modal from '../../stores/modalStore';
import * as tabsStore from '../../stores/tabsStore';
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
        case 'agents':
            return (
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
                    <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
                    <line x1="12" y1="22.08" x2="12" y2="12" />
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
        case 'relay':
            return (
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="12" cy="12" r="2" />
                    <path d="M4.93 4.93a10 10 0 0 0 0 14.14" />
                    <path d="M7.76 7.76a6 6 0 0 0 0 8.49" />
                    <path d="M16.24 7.76a6 6 0 0 1 0 8.49" />
                    <path d="M19.07 4.93a10 10 0 0 1 0 14.14" />
                </svg>
            );
        default:
            return null;
    }
}

/**
 * Short timestamp for a conversation row on the session-first home: HH:MM when
 * it's today, otherwise MM-DD. Chats carry `lastEventAt`/`createdAt`; terminals
 * have none, so they render blank.
 */
function formatSessionTime(iso?: string): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    const pad = (n: number) => n.toString().padStart(2, '0');
    const now = new Date();
    if (d.toDateString() === now.toDateString()) {
        return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
    }
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/**
 * Terminal `agent` values come from backend detection ('claude', …); map the
 * ones whose name differs from the AgentAvatar logo key (others pass through to
 * AgentAvatar's two-letter fallback). Mirrors SessionRow.
 */
const TERM_AGENT_LOGO_KEY: Record<string, string> = {
    claude: 'claudecode',
};

interface MobileAppLayoutProps {
    app: App;
    state: AppState;
}

interface MobileAppLayoutState {
    activeMobileTab: 'workspaces' | 'providers' | 'skills' | 'more';
    selectedWorkspaceId: string;
    /**
     * Home search query — filters the flat conversation list in place by
     * conversation name OR project (workspace) name, so the user finds a
     * conversation/project without leaving the home.
     */
    homeSearch: string;
    /**
     * The cross-project New Conversation landing (desktop's NewChatHome). It is
     * the unified entry for starting any conversation — the user picks the
     * project / agent / first message inside it — so it lives at the top level,
     * reachable from the home screen regardless of the selected workspace.
     */
    showNewChat: boolean;
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
        homeSearch: '',
        showNewChat: false,
        skillsInDetail: false,
        mountedSkillsPath: '',
        activeMoreSubView: 'menu',
        activeSettingsCategory: 'menu',
        pendingConfirm: null,
    };

    /**
     * Mirrors "the workbench tab became active" into the local navigation
     * state. Replaces the former componentWillReceiveProps prop comparison
     * now that activeTabId lives in a signal. (The home is session-first, so
     * entering a session page is driven explicitly by `openSession` / the
     * new-chat flow — we no longer auto-sync `selectedWorkspaceId` from the
     * active workspace, which would jump into a session page on load.)
     */
    private _prevActiveTabId = tabsStore.activeTabId.value;
    private _disposeTabSync: (() => void) | null = null;
    private _disposeDeepLink: (() => void) | null = null;

    // Swipe-to-archive state (class-level, no re-renders during active drag)
    private _swipeEl: HTMLElement | null = null;
    private _swipeBg: HTMLElement | null = null;
    private _swipeId = '';
    private _swipeIsChat = false;
    private _swipeSessionIndex = 0;
    private _swipeStartX = 0;
    private _swipeStartY = 0;
    private _swipeLocked: 'h' | 'v' | null = null;
    private _didSwipe = false;

    componentDidMount() {
        this._disposeTabSync = effect(() => {
            const id = tabsStore.activeTabId.value;
            if (id !== this._prevActiveTabId) {
                this._prevActiveTabId = id;
                if (id === 'terminal') {
                    this.setState({ activeMobileTab: 'workspaces' });
                }
            }
        });

        // Consume a `?ws=&view=` deep link (set by app.tsx after workspaces
        // load — which is AFTER this mount, hence an effect rather than a
        // one-time read). Maps the unified `view` onto the mobile navigation:
        // discovery/settings → 更多 subview; providers/skills → their tabs;
        // tasks/channels/files/git → the session page's drawer for that project.
        this._disposeDeepLink = effect(() => {
            const dl = tabsStore.mobileDeepLink.value;
            if (!dl) return;
            tabsStore.mobileDeepLink.value = null;
            if (dl.view === 'discovery' || dl.view === 'settings') {
                this.setState({ activeMobileTab: 'more', activeMoreSubView: dl.view });
            } else if (dl.view === 'providers') {
                this.setState({ activeMobileTab: 'providers' });
            } else if (dl.view === 'skills') {
                this.setState({ activeMobileTab: 'skills', skillsInDetail: false });
            } else {
                // tasks / channels / files / git → enter the project's session page.
                tabsStore.activeDrawerTab.value = dl.view;
                this.setState({ activeMobileTab: 'workspaces', selectedWorkspaceId: dl.workspaceId });
            }
        });
    }

    componentWillUnmount() {
        if (this._disposeTabSync) {
            this._disposeTabSync();
            this._disposeTabSync = null;
        }
        if (this._disposeDeepLink) {
            this._disposeDeepLink();
            this._disposeDeepLink = null;
        }
    }

    setMobileTab = (tab: 'workspaces' | 'providers' | 'skills' | 'more') => {
        this.setState({ activeMobileTab: tab, mountedSkillsPath: '' });
        if (tab === 'skills') {
            this.setState({ skillsInDetail: false });
            if (tabsStore.activeDrawerTab.value !== 'skills') {
                tabsStore.activeDrawerTab.value = 'skills';
            }
        } else if (tab === 'providers') {
            tabsStore.activeDrawerTab.value = 'none';
            // ccProvidersUrl is loaded asynchronously; if it's not ready yet, trigger a reload
            if (!wsStore.ccProvidersUrl.value) {
                wsStore.loadCcProvidersUrl();
            }
        } else if (tab === 'more') {
            this.setState({ activeMoreSubView: 'menu', activeSettingsCategory: 'menu' });
            tabsStore.activeDrawerTab.value = 'none';
        } else {
            tabsStore.activeDrawerTab.value = 'none';
        }
    };

    /**
     * Open (resume) a conversation straight from the session-first home — one
     * tap into the session page, no workspace drill-down. selectSession sets
     * the right tab (terminal / agents); we clear any artifact drawer so the
     * workbench leads, and enter the project shell for that session's workspace.
     */
    openSession = (s: Session) => {
        sess.selectSession(s);
        tabsStore.activeDrawerTab.value = 'none';
        this.setState({ selectedWorkspaceId: s.workspaceId });
    };

    /** Open the unified New Conversation landing (cross-project). */
    openNewChat = () => {
        sess.onStartNewChat();
        // NewChatHome only shows the project picker (and lets the chosen project
        // take effect) in 'project' mode; 'assistant' mode locks every chat to
        // the 'default' workspace. Mobile has no sidebar toggle for this, so
        // derive it from the UI mode: advanced → project (pick a project),
        // beginner → assistant (stay simple on the default workspace).
        ui.sidebarMode.value = ui.isBeginnerMode.value ? 'assistant' : 'project';
        this.setState({ showNewChat: true });
    };

    private onSwipeDown = (e: PointerEvent, sessionId: string, isChatSession: boolean, sessionIndex: number) => {
        if (e.button !== 0 && e.pointerType !== 'touch') return;
        this._swipeEl = e.currentTarget as HTMLElement;
        this._swipeBg = (e.currentTarget as HTMLElement).previousElementSibling as HTMLElement;
        this._swipeId = sessionId;
        this._swipeIsChat = isChatSession;
        this._swipeSessionIndex = sessionIndex;
        this._swipeStartX = e.clientX;
        this._swipeStartY = e.clientY;
        this._swipeLocked = null;
        this._didSwipe = false;
        this._swipeEl.setPointerCapture(e.pointerId);
    };

    private onSwipeMove = (e: PointerEvent) => {
        if (!this._swipeEl) return;
        const dx = e.clientX - this._swipeStartX;
        const dy = e.clientY - this._swipeStartY;
        if (!this._swipeLocked) {
            if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return;
            this._swipeLocked = Math.abs(dx) > Math.abs(dy) ? 'h' : 'v';
        }
        if (this._swipeLocked !== 'h') return;
        e.preventDefault();
        const clamped = Math.max(0, Math.min(dx, 90));
        this._swipeEl.style.transform = `translateX(${clamped}px)`;
        this._swipeEl.style.transition = 'none';
        if (this._swipeBg) {
            this._swipeBg.style.opacity = String(Math.min(clamped / 80, 1));
        }
        if (clamped > 4) this._didSwipe = true;
    };

    private onSwipeUp = (e: PointerEvent) => {
        if (!this._swipeEl) return;
        const el = this._swipeEl;
        const bg = this._swipeBg;
        const dx = e.clientX - this._swipeStartX;
        this._swipeEl = null;
        this._swipeBg = null;
        if (this._swipeLocked === 'h' && dx > 80 && this._swipeIsChat) {
            // Archive confirmed (chat sessions only)
            sess.killChatSession(this._swipeId);
            // slide off
            el.style.transition = 'transform 0.2s ease';
            el.style.transform = 'translateX(100%)';
        } else {
            // Snap back
            el.style.transition = 'transform 0.25s ease';
            el.style.transform = 'translateX(0)';
            if (bg) {
                bg.style.opacity = '0';
                bg.style.transition = 'opacity 0.25s ease';
            }
        }
    };

    render() {
        const { app, state } = this.props;
        const {
            activeMobileTab,
            selectedWorkspaceId,
            homeSearch,
            showNewChat,
            skillsInDetail,
            activeMoreSubView,
            activeSettingsCategory,
        } = this.state;
        const tabs = tabsStore.tabs.value;
        const activeTabId = tabsStore.activeTabId.value;
        const ccProvidersUrl = wsStore.ccProvidersUrl.value;
        const activeDrawerTab = tabsStore.activeDrawerTab.value;
        const workspaces = wsStore.workspaces.value;
        const activeWorkspaceId = wsStore.activeWorkspaceId.value;
        const folders = wsStore.folders.value;
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
            !showNewChat &&
            !skillsInDetail &&
            activeMoreSubView === 'menu' &&
            activeTabObj?.type !== 'preview' &&
            activeTabObj?.type !== 'browser';

        const moduleNav = tabsStore.buildModuleNav();

        return (
            <div class="mobile-app-layout" style={viewportStyle}>
                <div class="mobile-viewport">
                    {/* ── Tab 1: Workspaces ── */}
                    {activeMobileTab === 'workspaces' && (
                        <Fragment>
                            {/* 1.0 New Conversation — unified, cross-project landing.
                                Overlays everything; the project is picked inside. */}
                            {showNewChat && (
                                <div class="mobile-subview-layout">
                                    <div class="mobile-subview-header">
                                        <button
                                            class="mobile-subview-back-btn"
                                            onClick={() => this.setState({ showNewChat: false })}
                                        >
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                <polyline points="15 18 9 12 15 6" />
                                            </svg>
                                        </button>
                                        <div class="mobile-subview-title">{t('sidebar.newChat', language)}</div>
                                    </div>

                                    {/* 会话 / 项目 scope toggle — pinned below the header.
                                        会话 = default workspace (no picker); 项目 = pick a
                                        project. Drives NewChatHome via ui.sidebarMode. */}
                                    <div
                                        class="new-chat-scope-switch"
                                        role="group"
                                        aria-label={t('newchat.scope.aria', language)}
                                    >
                                        <button
                                            type="button"
                                            class={`scope-btn ${ui.sidebarMode.value === 'assistant' ? 'active' : ''}`}
                                            aria-pressed={ui.sidebarMode.value === 'assistant'}
                                            onClick={() => (ui.sidebarMode.value = 'assistant')}
                                        >
                                            {t('newchat.scope.chat', language)}
                                        </button>
                                        <button
                                            type="button"
                                            class={`scope-btn ${ui.sidebarMode.value === 'project' ? 'active' : ''}`}
                                            aria-pressed={ui.sidebarMode.value === 'project'}
                                            onClick={() => (ui.sidebarMode.value = 'project')}
                                        >
                                            {t('newchat.scope.project', language)}
                                        </button>
                                    </div>

                                    <div
                                        class="mobile-subview-content"
                                        style="overflow: hidden; display: flex; flex-direction: column;"
                                    >
                                        <NewChatHome
                                            workspaces={workspaces}
                                            activeWorkspaceId={activeWorkspaceId}
                                            onSubmitChat={async (wsId, agentType, prompt, role, permissionMode) => {
                                                const name = `${AGENT_TYPE_LABELS[agentType] ?? agentType} 会话`;
                                                await sess.createChatSession(
                                                    wsId,
                                                    name,
                                                    agentType,
                                                    prompt,
                                                    role === 'pm' ? 'pm' : undefined,
                                                    permissionMode
                                                );
                                                tabsStore.activeDrawerTab.value = 'none';
                                                this.setState({
                                                    showNewChat: false,
                                                    selectedWorkspaceId: wsId,
                                                });
                                            }}
                                            onSubmitTerminal={(wsId, cwd, initialCommand) => {
                                                sess.createTerminal(wsId, cwd, initialCommand);
                                                tabsStore.activeDrawerTab.value = 'none';
                                                this.setState({
                                                    showNewChat: false,
                                                    selectedWorkspaceId: wsId,
                                                });
                                            }}
                                            onOpenFolder={modal.openCreateWorkspacePicker}
                                            language={language}
                                        />
                                    </div>
                                </div>
                            )}

                            {/* 1.1 Home — session-first. All conversations across every
                                workspace live here: an inline search box (filters by
                                conversation OR project name) and a flat list (tap to
                                resume directly). New Conversation is the floating button
                                bottom-right. Workspaces are a per-row 所属项目 chip. */}
                            {!selectedWorkspaceId && (
                                <div class="mobile-tab-content">
                                    <div class="mobile-menu-view scrollable">
                                        <div class="mobile-menu-header">
                                            <h2>{t('mobile.home.title', language) || '对话'}</h2>
                                            <p>{t('mobile.home.desc', language) || '继续已有会话，或新建一个'}</p>
                                        </div>

                                        {folders.some(f => f.sessions.length > 0) && (
                                            <div class="mobile-home-search">
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                    <circle cx="11" cy="11" r="8" />
                                                    <line x1="21" y1="21" x2="16.65" y2="16.65" />
                                                </svg>
                                                <input
                                                    type="text"
                                                    placeholder={
                                                        t('mobile.home.searchPlaceholder', language) ||
                                                        '搜索会话或项目…'
                                                    }
                                                    value={homeSearch}
                                                    onInput={e =>
                                                        this.setState({
                                                            homeSearch: (e.target as HTMLInputElement).value,
                                                        })
                                                    }
                                                />
                                                {homeSearch && (
                                                    <button
                                                        class="search-clear"
                                                        title={t('common.clear', language) || '清除'}
                                                        onClick={() => this.setState({ homeSearch: '' })}
                                                    >
                                                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                            <line x1="18" y1="6" x2="6" y2="18" />
                                                            <line x1="6" y1="6" x2="18" y2="18" />
                                                        </svg>
                                                    </button>
                                                )}
                                            </div>
                                        )}

                                        {(() => {
                                            const q = homeSearch.trim().toLowerCase();
                                            const all = folders.flatMap(f =>
                                                f.sessions.map(s => ({ s, wsName: f.name }))
                                            );
                                            all.sort((a, b) => {
                                                if (!!a.s.active !== !!b.s.active) return a.s.active ? -1 : 1;
                                                const ta = isChat(a.s) ? a.s.lastEventAt || a.s.createdAt || '' : '';
                                                const tb = isChat(b.s) ? b.s.lastEventAt || b.s.createdAt || '' : '';
                                                return tb.localeCompare(ta);
                                            });
                                            const filtered = q
                                                ? all.filter(
                                                      ({ s, wsName }) =>
                                                          s.name.toLowerCase().includes(q) ||
                                                          wsName.toLowerCase().includes(q)
                                                  )
                                                : all;
                                            if (all.length === 0) {
                                                return (
                                                    <div class="mobile-home-empty">
                                                        {t('mobile.home.empty', language) ||
                                                            '还没有会话，点上方新建聊天开始'}
                                                    </div>
                                                );
                                            }
                                            if (filtered.length === 0) {
                                                return (
                                                    <div class="mobile-home-empty">
                                                        {t('mobile.home.noMatch', language) || '没有匹配的会话或项目'}
                                                    </div>
                                                );
                                            }
                                            return (
                                                <div class="mobile-session-cards-grid">
                                                    <div class="mobile-menu-group">
                                                        {filtered.map(({ s, wsName }, idx) => {
                                                            const sessionAgent = isChat(s) ? s.agentType : s.agent;
                                                            const timeLabel = formatSessionTime(
                                                                isChat(s) ? s.lastEventAt || s.createdAt : undefined
                                                            );
                                                            return (
                                                                <div key={s.id} class="swipe-row-wrapper">
                                                                    <div class="swipe-bg-archive" aria-hidden="true">
                                                                        <svg
                                                                            viewBox="0 0 24 24"
                                                                            fill="none"
                                                                            stroke="currentColor"
                                                                            stroke-width="2"
                                                                            stroke-linecap="round"
                                                                            stroke-linejoin="round"
                                                                        >
                                                                            <polyline points="21 8 21 21 3 21 3 8" />
                                                                            <rect x="1" y="3" width="22" height="5" />
                                                                            <line x1="10" y1="12" x2="14" y2="12" />
                                                                        </svg>
                                                                        <span>归档</span>
                                                                    </div>
                                                                    <div
                                                                        class={`mobile-session-item-row ${s.active ? 'active' : ''}`}
                                                                        onPointerDown={(e: PointerEvent) =>
                                                                            this.onSwipeDown(
                                                                                e,
                                                                                isChat(s) ? s.id : '',
                                                                                isChat(s),
                                                                                idx
                                                                            )
                                                                        }
                                                                        onPointerMove={this.onSwipeMove}
                                                                        onPointerUp={this.onSwipeUp}
                                                                        onPointerCancel={this.onSwipeUp}
                                                                        onClick={(e: MouseEvent) => {
                                                                            if (this._didSwipe) {
                                                                                e.stopPropagation();
                                                                                return;
                                                                            }
                                                                            this.openSession(s);
                                                                        }}
                                                                    >
                                                                        <div class="card-left">
                                                                            {sessionAgent ? (
                                                                                <AgentAvatar
                                                                                    agentType={
                                                                                        TERM_AGENT_LOGO_KEY[
                                                                                            sessionAgent
                                                                                        ] || sessionAgent
                                                                                    }
                                                                                    role={
                                                                                        isChat(s) ? s.role : undefined
                                                                                    }
                                                                                    class="session-card-avatar"
                                                                                />
                                                                            ) : (
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
                                                                            )}
                                                                            <div class="session-card-info">
                                                                                <div class="session-card-name-row">
                                                                                    <span class="session-card-name">
                                                                                        {s.name}
                                                                                    </span>
                                                                                </div>
                                                                                <div class="session-card-meta-row">
                                                                                    <span
                                                                                        class="session-card-project"
                                                                                        title={wsName}
                                                                                    >
                                                                                        <svg
                                                                                            viewBox="0 0 24 24"
                                                                                            fill="none"
                                                                                            stroke="currentColor"
                                                                                        >
                                                                                            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                                                                        </svg>
                                                                                        <span class="proj-name">
                                                                                            {wsName}
                                                                                        </span>
                                                                                    </span>
                                                                                </div>
                                                                            </div>
                                                                        </div>
                                                                        <div class="card-right">
                                                                            {timeLabel && (
                                                                                <span class="session-card-date">
                                                                                    {timeLabel}
                                                                                </span>
                                                                            )}
                                                                        </div>
                                                                    </div>
                                                                </div>
                                                            );
                                                        })}
                                                    </div>
                                                </div>
                                            );
                                        })()}
                                    </div>

                                    {/* New Conversation — floating action button (bottom-right).
                                        Hidden while the New Conversation overlay is up (it sits
                                        above the home, which stays mounted underneath). */}
                                    {!showNewChat && (
                                        <button
                                            class="mobile-fab"
                                            onClick={this.openNewChat}
                                            title={t('sidebar.newChat', language)}
                                            aria-label={t('sidebar.newChat', language)}
                                        >
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                                                <line x1="12" y1="8" x2="12" y2="14" />
                                                <line x1="9" y1="11" x2="15" y2="11" />
                                            </svg>
                                        </button>
                                    )}
                                </div>
                            )}
                            {/* 1.2 Session page — entered by tapping a conversation on the
                                home. The hamburger switches the in-project views (工作台 /
                                智能体 / 任务看板 / 渠道 / 文件 / Git). Back returns to the
                                session-first home. */}
                            {selectedWorkspaceId && (
                                <div class="mobile-tab-content">
                                    <WorkspaceHeader
                                        leftSidebarOpen={false}
                                        toggleLeftSidebar={() => {}}
                                        onBack={() => this.setState({ selectedWorkspaceId: '' })}
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
                                        hasChatSession={folders.some(
                                            f => f.id === selectedWorkspaceId && f.sessions.some(isChat)
                                        )}
                                    />
                                    <div class="workspace-body-container" style="flex: 1; min-height: 0;">
                                        {activeDrawerTab === 'none' && <WorkbenchCanvas app={app} fontSize={12} />}

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
                                                    onSelectSession={s => {
                                                        sess.selectSession(s);
                                                        tabsStore.activeDrawerTab.value = 'none';
                                                    }}
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
                                                (activeMobileTab === 'skills' && tabsStore.activeModulePath.value) ||
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
                                <button class="mobile-subview-back-btn" onClick={() => tabsStore.closeTab(activeTabId)}>
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
                                <button class="mobile-subview-back-btn" onClick={() => tabsStore.closeTab(activeTabId)}>
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                        <polyline points="15 18 9 12 15 6" />
                                    </svg>
                                </button>
                                <div class="mobile-subview-title">{t('mobile.browser', language) || 'Browser'}</div>
                            </div>
                            <div class="mobile-subview-content">
                                {tabs
                                    .filter(t => t.id === activeTabId)
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

                {/* ── Bottom Navigation Bar ── */}
                {showBottomBar && (
                    <div class="mobile-bottom-nav">
                        <button
                            class={`mobile-tab-btn ${activeMobileTab === 'workspaces' ? 'active' : ''}`}
                            onClick={() => this.setMobileTab('workspaces')}
                        >
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                            </svg>
                            {t('mobile.nav.chats', language) || '对话'}
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
                                                        await sess.killChatSession(target.sessionId);
                                                    } else {
                                                        await sess.killTerminal(target.sessionIndex);
                                                    }
                                                } else {
                                                    await wsStore.deleteWorkspace(target.workspaceId);
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
