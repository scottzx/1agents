import { computed, signal } from '@preact/signals';

import { isFullPageTab, type FsEntry, type RightDrawerTab } from '../components/types';
import { t } from '../i18n';
import { getModuleByTab, mergeManifests, type ModuleRegistration } from '../modules/registry';
import type { ModuleManifest } from '../modules/module-types';
import {
    SETTINGS_MODULE_ID,
    SETTINGS_DEFAULT_CATEGORY,
    pathToSettingsCategory,
    settingsCategoryToPath,
    type SettingsCategory,
} from '../modules/settings-manifest';
import {
    DISCOVERY_MODULE_ID,
    DISCOVERY_DEFAULT_CATEGORY,
    pathToDiscoveryCategory,
    discoveryCategoryToPath,
} from '../modules/discovery-manifest';
import * as ui from './uiStore';
import * as fs from './fsStore';
import * as wsStore from './workspaceStore';

/**
 * Tab / drawer / module navigation state. Previously lived on App's
 * god-state; now any consumer reads the signals and calls the navigation
 * functions directly. Module state (activeModulePath, moduleManifests,
 * activeSettingsCategory) lives here rather than in a separate moduleStore
 * because toggleDrawerTab and the module nav are mutually entangled
 * (toggleDrawerTab sets activeModulePath and triggers manifest loads; the
 * module nav reads activeDrawerTab) — one store avoids a circular import.
 */

export interface Tab {
    id: string; // 'tasks', 'terminal', 'preview-[path]', 'browser-[timestamp]'
    title: string;
    // 'tasks' is the project landing / kanban background sentinel. It is
    // fixed at the front of the tab bar, non-closable, and renders no
    // overlay (the kanban lives in DesktopAppLayout's background layer).
    type: 'terminal' | 'preview' | 'browser' | 'tasks';
    path?: string;
    url?: string;
    closable: boolean;
}

const initialLang = ui.language.value;

// Tab order: 'tasks' is the project landing (fixed first, non-closable).
// 'terminal' is the second non-closable default overlay. Dynamic
// preview/browser tabs are appended on demand.
export const tabs = signal<Tab[]>([
    { id: 'tasks', title: t('app.tab.tasks', initialLang), type: 'tasks', closable: false },
    { id: 'terminal', title: t('app.tab.workbench', initialLang), type: 'terminal', closable: false },
]);
// 'tasks' is the "no overlay" sentinel — the kanban background layer
// is always mounted in DesktopAppLayout, so this lands on the
// project's task kanban by default.
export const activeTabId = signal('tasks');
export const activeTab = signal<'terminal' | 'agents' | 'console' | 'folders' | 'new_chat'>('terminal');

/**
 * The two-column right-artifact tabs. Only these are persisted across
 * reloads (so the workbench restores which column was open); full-page
 * modules and `pm` are transient.
 */
const CONTENT_DRAWER_TABS: RightDrawerTab[] = ['tasks', 'channels', 'files', 'browser', 'git', 'terminal'];
const DRAWER_KEY = '1agents-drawer-tab';
/**
 * Persist the artifact column's state across reloads. We store content tabs
 * and the explicit `'none'` (so a user-closed column stays closed); transient
 * tabs (pm / full-page modules) are left out so they don't clobber the
 * remembered content tab.
 */
const persistDrawerTab = (tab: RightDrawerTab) => {
    // Desktop-only: mobile keeps its unchanged full-screen overlay drawer.
    if (ui.isMobile.value) return;
    if (tab === 'none' || CONTENT_DRAWER_TABS.includes(tab)) localStorage.setItem(DRAWER_KEY, tab);
};
const initialDrawerTab = (): RightDrawerTab => {
    // Mobile boots to the workbench (drawer closed) exactly as before.
    if (ui.isMobile.value) return 'none';
    const stored = localStorage.getItem(DRAWER_KEY) as RightDrawerTab | null;
    // Beginner mode hides the task kanban — never auto-open it as the landing.
    if (localStorage.getItem('1agents-ui-mode') === 'beginner') {
        return stored !== null && CONTENT_DRAWER_TABS.includes(stored) && stored !== 'tasks' ? stored : 'none';
    }
    // First-ever desktop load → project-landing default (项目管理 / kanban first).
    if (stored === null) return 'tasks';
    return CONTENT_DRAWER_TABS.includes(stored) ? stored : 'none';
};
export const activeDrawerTab = signal<RightDrawerTab>(initialDrawerTab());

export type SidePanelTabType = 'tasks' | 'files' | 'browser' | 'git' | 'terminal';

export interface SidePanelTab {
    id: string;
    type: SidePanelTabType;
    title: string;
    reusableKey: string;
    createdAt: number;
    lastActiveAt: number;
    mounted: boolean;
    reclaimed?: boolean;
    path?: string;
    line?: number;
    lineEnd?: number;
    selectedTaskId?: string | null;
    url?: string;
    terminalWindowIndex?: number;
}

interface SidePanelState {
    panelOpen: boolean;
    activeTabId: string | null;
    tabs: SidePanelTab[];
}

export const SIDE_PANEL_IDLE_CLEANUP_MS = 30 * 60 * 1000;
const SIDE_PANEL_STORAGE_PREFIX = '1agents-side-panel-tabs:v1:';
const SIDE_PANEL_TYPES: SidePanelTabType[] = ['tasks', 'files', 'browser', 'git', 'terminal'];
const sidePanelDefaultOwner = (): string =>
    `workspace:${wsStore.activeWorkspaceId.value || localStorage.getItem('1agents-active-workspace') || 'none'}`;

export const sidePanelOwnerKey = signal<string>(sidePanelDefaultOwner());

const sidePanelStorageKey = (ownerKey: string): string => `${SIDE_PANEL_STORAGE_PREFIX}${ownerKey}`;
const isSidePanelType = (tab: RightDrawerTab): tab is SidePanelTabType =>
    SIDE_PANEL_TYPES.includes(tab as SidePanelTabType);

const defaultSidePanelTitle = (type: SidePanelTabType): string => {
    switch (type) {
        case 'tasks':
            return t('sidePanel.tab.tasks', ui.language.value);
        case 'files':
            return t('sidePanel.tab.files', ui.language.value);
        case 'browser':
            return t('sidePanel.tab.browser', ui.language.value);
        case 'git':
            return t('sidePanel.tab.git', ui.language.value);
        case 'terminal':
            return t('sidePanel.tab.terminal', ui.language.value);
    }
};

const defaultReusableKey = (type: SidePanelTabType): string => `${type}:default`;

const loadSidePanelState = (ownerKey: string): SidePanelState => {
    try {
        const raw = localStorage.getItem(sidePanelStorageKey(ownerKey));
        if (!raw) return { panelOpen: false, activeTabId: null, tabs: [] };
        const parsed = JSON.parse(raw) as Partial<SidePanelState>;
        const tabs = Array.isArray(parsed.tabs) ? parsed.tabs.filter(t => t && typeof t.id === 'string') : [];
        return {
            panelOpen: Boolean(parsed.panelOpen),
            activeTabId: typeof parsed.activeTabId === 'string' ? parsed.activeTabId : tabs[0]?.id || null,
            tabs: tabs as SidePanelTab[],
        };
    } catch {
        return { panelOpen: false, activeTabId: null, tabs: [] };
    }
};

const initialSidePanel = loadSidePanelState(sidePanelOwnerKey.value);
if (!initialSidePanel.panelOpen && isSidePanelType(activeDrawerTab.value)) {
    activeDrawerTab.value = 'none';
}
export const sidePanelOpen = signal<boolean>(initialSidePanel.panelOpen);
export const sidePanelTabs = signal<SidePanelTab[]>(initialSidePanel.tabs);
export const activeSidePanelTabId = signal<string | null>(initialSidePanel.activeTabId);
export const activeSidePanelTab = computed<SidePanelTab | null>(
    () => sidePanelTabs.value.find(tab => tab.id === activeSidePanelTabId.value) || null
);

const persistSidePanelState = () => {
    try {
        const state: SidePanelState = {
            panelOpen: sidePanelOpen.value,
            activeTabId: activeSidePanelTabId.value,
            tabs: sidePanelTabs.value,
        };
        localStorage.setItem(sidePanelStorageKey(sidePanelOwnerKey.value), JSON.stringify(state));
    } catch {
        /* private mode / quota */
    }
};

const activateSidePanelDrawer = (type?: SidePanelTabType) => {
    const drawer = (type || activeSidePanelTab.value?.type || 'tasks') as RightDrawerTab;
    sidePanelOpen.value = true;
    activeDrawerTab.value = drawer;
    activeModulePath.value = '';
    persistDrawerTab(drawer);
    persistSidePanelState();
    ui.triggerTerminalFit();
};

export const setSidePanelOwner = (ownerKey: string) => {
    if (!ownerKey || ownerKey === sidePanelOwnerKey.value) return;
    persistSidePanelState();
    sidePanelOwnerKey.value = ownerKey;
    const next = loadSidePanelState(ownerKey);
    sidePanelOpen.value = next.panelOpen;
    sidePanelTabs.value = next.tabs;
    activeSidePanelTabId.value = next.activeTabId;
    if (next.panelOpen) {
        activateSidePanelDrawer(next.tabs.find(t => t.id === next.activeTabId)?.type);
    } else if (isSidePanelType(activeDrawerTab.value)) {
        activeDrawerTab.value = 'none';
    }
};

export const setSidePanelOwnerForWorkspace = (workspaceId: string) => {
    setSidePanelOwner(`workspace:${workspaceId || 'none'}`);
};

export const setSidePanelOwnerForChat = (sessionId: string) => {
    setSidePanelOwner(`chat:${sessionId}`);
};

const makeSidePanelTabId = (type: SidePanelTabType): string =>
    `${type}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

const buildSidePanelTab = (
    type: SidePanelTabType,
    payload: Partial<SidePanelTab> = {},
    reusableKey = defaultReusableKey(type)
): SidePanelTab => {
    const now = Date.now();
    return {
        id: makeSidePanelTabId(type),
        type,
        title: payload.title || defaultSidePanelTitle(type),
        reusableKey,
        createdAt: now,
        lastActiveAt: now,
        mounted: true,
        ...payload,
    };
};

export const updateSidePanelTab = (tabId: string, patch: Partial<SidePanelTab>) => {
    sidePanelTabs.value = sidePanelTabs.value.map(tab =>
        tab.id === tabId ? { ...tab, ...patch, lastActiveAt: Date.now() } : tab
    );
    persistSidePanelState();
};

export const touchSidePanelTab = (tabId = activeSidePanelTabId.value) => {
    if (!tabId) return;
    updateSidePanelTab(tabId, { mounted: true });
};

export const openSidePanel = () => {
    activateSidePanelDrawer(activeSidePanelTab.value?.type);
};

export const openOrReuseSidePanelTab = (type: SidePanelTabType, payload: Partial<SidePanelTab> = {}) => {
    const reusableKey = payload.reusableKey || defaultReusableKey(type);
    const existing = sidePanelTabs.value.find(tab => tab.reusableKey === reusableKey);
    if (existing) {
        updateSidePanelTab(existing.id, {
            ...payload,
            type,
            title: payload.title || existing.title || defaultSidePanelTitle(type),
            mounted: true,
            reclaimed:
                type === 'terminal' && typeof payload.terminalWindowIndex !== 'number' ? existing.reclaimed : false,
        });
        activeSidePanelTabId.value = existing.id;
    } else {
        const tab = buildSidePanelTab(type, payload, reusableKey);
        sidePanelTabs.value = [...sidePanelTabs.value, tab];
        activeSidePanelTabId.value = tab.id;
    }
    activateSidePanelDrawer(type);
};

export const addSidePanelTab = (type: SidePanelTabType, payload: Partial<SidePanelTab> = {}) => {
    const tab = buildSidePanelTab(type, payload, `${type}:${makeSidePanelTabId(type)}`);
    sidePanelTabs.value = [...sidePanelTabs.value, tab];
    activeSidePanelTabId.value = tab.id;
    activateSidePanelDrawer(type);
};

export const selectSidePanelTab = (tabId: string) => {
    const tab = sidePanelTabs.value.find(t => t.id === tabId);
    if (!tab) return;
    activeSidePanelTabId.value = tabId;
    updateSidePanelTab(tabId, { mounted: true });
    activateSidePanelDrawer(tab.type);
};

export const updateSidePanelBrowserUrl = (tabId: string, url: string) => {
    updateSidePanelTab(tabId, { url, title: browserTitleFromUrl(url) });
};

export const bindTerminalToSidePanelTab = (tabId: string, terminalWindowIndex: number) => {
    updateSidePanelTab(tabId, {
        terminalWindowIndex,
        reclaimed: false,
        mounted: true,
        title: `${t('sidePanel.tab.terminal', ui.language.value)} #${terminalWindowIndex}`,
    });
};

const killTerminalWindow = (windowIndex: number) => {
    void import('./sessionStore').then(sess => sess.killTerminal(windowIndex));
};

const reclaimedTerminalPatch = (): Partial<SidePanelTab> => ({
    mounted: false,
    reclaimed: true,
    terminalWindowIndex: undefined,
    title: t('sidePanel.terminal.reclaimedTitle', ui.language.value),
});

const reclaimTerminalTab = (tab: SidePanelTab) => {
    if (typeof tab.terminalWindowIndex === 'number') {
        killTerminalWindow(tab.terminalWindowIndex);
    }
    updateSidePanelTab(tab.id, reclaimedTerminalPatch());
};

export const closeSidePanelTab = (tabId: string) => {
    const tab = sidePanelTabs.value.find(t => t.id === tabId);
    if (!tab) return;
    if (tab.type === 'terminal' && !tab.reclaimed) reclaimTerminalTab(tab);
    const current = sidePanelTabs.value;
    const index = current.findIndex(t => t.id === tabId);
    const nextTabs = current.filter(t => t.id !== tabId);
    sidePanelTabs.value = nextTabs;
    if (activeSidePanelTabId.value === tabId) {
        const nextActive = nextTabs[index - 1] || nextTabs[index] || null;
        activeSidePanelTabId.value = nextActive?.id || null;
        if (nextActive) activateSidePanelDrawer(nextActive.type);
    }
    if (nextTabs.length === 0) {
        activeSidePanelTabId.value = null;
        sidePanelOpen.value = true;
    }
    persistSidePanelState();
};

const cleanupStoredSidePanelTabs = (now: number) => {
    const currentKey = sidePanelStorageKey(sidePanelOwnerKey.value);
    for (let i = 0; i < localStorage.length; i += 1) {
        const key = localStorage.key(i);
        if (!key || key === currentKey || !key.startsWith(SIDE_PANEL_STORAGE_PREFIX)) continue;
        try {
            const raw = localStorage.getItem(key);
            if (!raw) continue;
            const state = JSON.parse(raw) as Partial<SidePanelState>;
            if (!Array.isArray(state.tabs)) continue;
            let changed = false;
            const tabs = state.tabs.map(tab => {
                if (!tab || typeof tab.id !== 'string') return tab;
                if (now - (tab.lastActiveAt || 0) < SIDE_PANEL_IDLE_CLEANUP_MS) return tab;
                if (tab.type === 'terminal' && !tab.reclaimed) {
                    if (typeof tab.terminalWindowIndex === 'number') {
                        killTerminalWindow(tab.terminalWindowIndex);
                    }
                    changed = true;
                    return { ...tab, ...reclaimedTerminalPatch() };
                }
                if (tab.mounted) {
                    changed = true;
                    return { ...tab, mounted: false };
                }
                return tab;
            });
            if (changed) {
                localStorage.setItem(key, JSON.stringify({ ...state, tabs }));
            }
        } catch {
            /* ignore malformed side-panel state */
        }
    }
};

const cleanupIdleSidePanelTabs = () => {
    const now = Date.now();
    for (const tab of sidePanelTabs.value) {
        if (tab.id === activeSidePanelTabId.value && sidePanelOpen.value) continue;
        if (now - (tab.lastActiveAt || 0) < SIDE_PANEL_IDLE_CLEANUP_MS) continue;
        if (tab.type === 'terminal' && !tab.reclaimed) {
            reclaimTerminalTab(tab);
        } else if (tab.mounted) {
            updateSidePanelTab(tab.id, { mounted: false });
        }
    }
    cleanupStoredSidePanelTabs(now);
};

declare global {
    interface Window {
        __sidePanelIdleCleanupInterval?: number;
    }
}

if (typeof window !== 'undefined' && !window.__sidePanelIdleCleanupInterval) {
    window.__sidePanelIdleCleanupInterval = window.setInterval(cleanupIdleSidePanelTabs, 60 * 1000);
}
/** Selected discovery category — default「应用」so 更多 → 发现中心 lands on apps (design §6.3). */
export const discoveryCategory = signal<string>(DISCOVERY_DEFAULT_CATEGORY);
export const activeExternalApp = signal<string | null>(null);

/**
 * Assistant currently opened in the 助理 detail view (workspace id), or null for
 * the card grid. Set when a card on AssistantsPage is clicked; cleared by the
 * detail's back button. Kept here alongside the other L1 sub-view state.
 */
export const assistantDetailId = signal<string | null>(null);

/**
 * Deep-link intent parsed from `?ws=<id>&view=<tab>` on boot (app.tsx
 * `checkUrlDeepLink`). The 小程序 native shell loads main-H5 modules
 * (tasks/files/git/discovery/settings) in a web-view with these params so the
 * mobile layout can jump straight into the right project + view. Set once after
 * workspaces load; MobileAppLayout consumes it (via effect) then clears it to
 * null. Desktop ignores it beyond the activeDrawerTab side-effect already set.
 */
export const mobileDeepLink = signal<{ workspaceId: string; view: RightDrawerTab } | null>(null);

// ── Module slot state ──
/** Active sub-path inside the active module, e.g. "/skills/use". */
export const activeModulePath = signal('');
/** Live manifest per module id (overlays the static fallback). */
export const moduleManifests = signal<Record<string, ModuleManifest>>({});
/**
 * Active sub-category inside the system settings page. The settings
 * module is host-rendered (no iframe) and lives in the same chrome as
 * 1skills, so we keep a separate piece of state for it rather than
 * overloading `activeModulePath`.
 */
export const activeSettingsCategory = signal<SettingsCategory>(SETTINGS_DEFAULT_CATEGORY);
export const setActiveTab = (tab: 'terminal' | 'agents' | 'console' | 'folders' | 'new_chat') => {
    activeTab.value = tab;
    ui.triggerTerminalFit();
};

export const selectTab = async (tabId: string) => {
    const tab = tabs.value.find(t => t.id === tabId);
    if (!tab) return;

    activeTabId.value = tabId;

    if (tab.type === 'preview' && tab.path) {
        const entry: FsEntry = {
            name: tab.title.replace(t('app.preview.prefix', ui.language.value), ''),
            path: tab.path,
            isDir: false,
            size: 0,
            modTime: 0,
        };
        await fs.openFileDetail(entry);
    } else if (tab.type === 'terminal') {
        ui.triggerTerminalFit();
    }
};

export const openPreviewTab = async (path: string, fileName: string) => {
    if (!ui.isMobile.value) {
        openOrReuseSidePanelTab('files', { path, title: fileName || defaultSidePanelTitle('files') });
        await fs.openFileDetail({
            name: fileName || path.split('/').pop() || path,
            path,
            isDir: false,
            size: 0,
            modTime: 0,
        });
        return;
    }

    const tabId = `preview-${path}`;
    const exists = tabs.value.some(t => t.id === tabId);

    if (!exists) {
        const newTab: Tab = {
            id: tabId,
            title: `${t('app.preview.prefix', ui.language.value)}${fileName}`,
            type: 'preview',
            path: path,
            closable: true,
        };
        tabs.value = [...tabs.value, newTab];
    }
    selectTab(tabId);
};

/** Normalize a user/agent-facing URL for the built-in browser. */
const normalizeBrowserUrl = (raw: string): string => {
    const s = raw.trim();
    if (!s) return '';
    if (/^https?:\/\//i.test(s) || s.startsWith('about:')) return s;
    return 'http://' + s;
};

// ── Per-project (workspace) browser sessions ─────────────────────────────
// Each workspace keeps its own URL/title so switching projects restores the
// prior page. Only the active workspace's browser tab is kept in `tabs`
// (and thus only one iframe mounts) to avoid Remotion-level memory growth.

export interface BrowserSession {
    url: string;
    title: string;
    lastActiveAt: number;
}

const BROWSER_SESSIONS_KEY = '1agents-browser-sessions';
/** Cap persisted sessions (URL metadata only — no iframe). */
const MAX_BROWSER_SESSIONS = 24;
/**
 * Auto-close the browser content column after this idle period while it is
 * open. Unmounts the iframe and frees heavy pages (e.g. Remotion Studio).
 * Activity = open / navigate / address-bar commit / iframe load message.
 */
export const BROWSER_IDLE_CLOSE_MS = 15 * 60 * 1000;

const browserTitleFromUrl = (url: string): string => {
    let title = t('app.browser.title', ui.language.value);
    try {
        if (url && url !== 'about:blank') title = new URL(url).hostname || title;
    } catch {
        /* keep default */
    }
    return title;
};

const loadBrowserSessions = (): Record<string, BrowserSession> => {
    try {
        const raw = localStorage.getItem(BROWSER_SESSIONS_KEY);
        if (!raw) return {};
        const parsed = JSON.parse(raw) as Record<string, BrowserSession>;
        return parsed && typeof parsed === 'object' ? parsed : {};
    } catch {
        return {};
    }
};

export const browserSessions = signal<Record<string, BrowserSession>>(loadBrowserSessions());

const persistBrowserSessions = (map: Record<string, BrowserSession>) => {
    try {
        localStorage.setItem(BROWSER_SESSIONS_KEY, JSON.stringify(map));
    } catch {
        /* private mode / quota */
    }
};

const pruneBrowserSessions = (map: Record<string, BrowserSession>): Record<string, BrowserSession> => {
    const entries = Object.entries(map);
    if (entries.length <= MAX_BROWSER_SESSIONS) return map;
    entries.sort((a, b) => (b[1].lastActiveAt || 0) - (a[1].lastActiveAt || 0));
    const next: Record<string, BrowserSession> = {};
    for (const [k, v] of entries.slice(0, MAX_BROWSER_SESSIONS)) next[k] = v;
    return next;
};

const activeBrowserWorkspaceKey = (): string => wsStore.activeWorkspaceId.value || 'none';

export const browserTabIdForWorkspace = (wsId: string): string => `browser-${wsId || 'none'}`;

const writeBrowserSession = (wsId: string, url: string, title?: string) => {
    const key = wsId || 'none';
    const session: BrowserSession = {
        url: url || '',
        title: title || browserTitleFromUrl(url || ''),
        lastActiveAt: Date.now(),
    };
    const next = pruneBrowserSessions({ ...browserSessions.value, [key]: session });
    browserSessions.value = next;
    persistBrowserSessions(next);
};

let browserIdleTimer: ReturnType<typeof setTimeout> | null = null;

const clearBrowserIdleTimer = () => {
    if (browserIdleTimer !== null) {
        clearTimeout(browserIdleTimer);
        browserIdleTimer = null;
    }
};

/** Reset idle auto-close while the browser column is open. */
export const touchBrowserActivity = () => {
    const key = activeBrowserWorkspaceKey();
    const cur = browserSessions.value[key];
    if (cur) {
        const next = {
            ...browserSessions.value,
            [key]: { ...cur, lastActiveAt: Date.now() },
        };
        browserSessions.value = next;
        persistBrowserSessions(next);
    }
    scheduleBrowserIdleClose();
};

const scheduleBrowserIdleClose = () => {
    clearBrowserIdleTimer();
    if (typeof window === 'undefined') return;
    if (activeDrawerTab.value !== 'browser') return;
    browserIdleTimer = setTimeout(() => {
        browserIdleTimer = null;
        if (activeDrawerTab.value === 'browser') {
            // Unmount iframe — free Remotion / SPA memory; session URL is kept.
            closeContentTab();
        }
    }, BROWSER_IDLE_CLOSE_MS);
};

/**
 * Sync the visible `tabs` browser entry to the given workspace's session.
 * Drops other workspaces' browser tabs so at most one browser iframe mounts.
 */
const syncBrowserTabForWorkspace = (wsId: string, urlOverride?: string): string => {
    const key = wsId || 'none';
    const tabId = browserTabIdForWorkspace(key);
    const session = browserSessions.value[key];
    const url =
        urlOverride !== undefined ? urlOverride : session?.url && session.url !== 'about:blank' ? session.url : '';
    const title = urlOverride !== undefined ? browserTitleFromUrl(url) : session?.title || browserTitleFromUrl(url);

    writeBrowserSession(key, url, title);

    const nextTab: Tab = {
        id: tabId,
        title: browserSessions.value[key]?.title || title,
        type: 'browser',
        url: browserSessions.value[key]?.url ?? url,
        closable: true,
    };
    const withoutBrowsers = tabs.value.filter(tb => tb.type !== 'browser');
    tabs.value = [...withoutBrowsers, nextTab];
    return tabId;
};

/** Ensure a browser tab exists for the *active* workspace; return its id. */
const ensureBrowserSessionTab = (url: string): string => {
    const wsId = activeBrowserWorkspaceKey();
    if (url) {
        return syncBrowserTabForWorkspace(wsId, url);
    }
    // Open existing session URL for this project (or empty home).
    return syncBrowserTabForWorkspace(wsId);
};

/**
 * Called when the active workspace changes. Restores that project's browser
 * URL into the single browser tab slot (previous project's iframe unmounts
 * when the tab id/url changes). Session metadata for other projects is kept.
 */
export const onWorkspaceBrowserSwitch = (_prevWsId: string, nextWsId: string) => {
    clearBrowserIdleTimer();
    const key = nextWsId || 'none';
    // Drop other workspaces' browser tabs from the tab bar — only one live iframe.
    const withoutBrowsers = tabs.value.filter(tb => tb.type !== 'browser');
    const session = browserSessions.value[key];
    if (session && session.url && session.url !== 'about:blank') {
        tabs.value = [
            ...withoutBrowsers,
            {
                id: browserTabIdForWorkspace(key),
                title: session.title || browserTitleFromUrl(session.url),
                type: 'browser',
                url: session.url,
                closable: true,
            },
        ];
    } else {
        tabs.value = withoutBrowsers;
    }
    // If browser column stays open, re-arm idle close for the new page.
    if (activeDrawerTab.value === 'browser') {
        if (!tabs.value.some(tb => tb.type === 'browser')) {
            // Ensure a home tab so the column is not empty.
            syncBrowserTabForWorkspace(key, '');
        }
        scheduleBrowserIdleClose();
    }
};

/** Tab id for the active workspace browser session (for stage panes). */
export const getActiveBrowserTabId = (): string | null => {
    const id = browserTabIdForWorkspace(activeBrowserWorkspaceKey());
    return tabs.value.some(tb => tb.id === id) ? id : tabs.value.find(tb => tb.type === 'browser')?.id || null;
};

/**
 * Open a URL in the built-in lightweight browser as the right-column
 * content (peer of 文件 / Git). State is scoped to the active workspace.
 */
export const openBrowserTab = (url = '') => {
    const normalized = normalizeBrowserUrl(url);
    if (!ui.isMobile.value) {
        openOrReuseSidePanelTab('browser', { url: normalized, title: browserTitleFromUrl(normalized) });
        return;
    }
    ensureBrowserSessionTab(normalized);
    openContentTab('browser');
    touchBrowserActivity();
};

export const closeTab = (tabId: string) => {
    const currentTabs = tabs.value;
    if (currentTabs.length <= 1) return;

    const index = currentTabs.findIndex(t => t.id === tabId);
    if (index === -1) return;

    const nextTabs = currentTabs.filter(t => t.id !== tabId);
    let nextActiveId = activeTabId.value;

    if (activeTabId.value === tabId) {
        // When the active overlay tab is closed, fall back to the project
        // landing ('tasks') — the kanban is always mounted underneath, so
        // this shows the background instead of an empty pane.
        const nextActiveTab = nextTabs[index - 1] || nextTabs[index] || nextTabs[0];
        nextActiveId = nextActiveTab ? nextActiveTab.id : 'tasks';
    }

    tabs.value = nextTabs;
    selectTab(nextActiveId);
};

export const updateBrowserUrl = (tabId: string, url: string) => {
    const title = browserTitleFromUrl(url);
    tabs.value = tabs.value.map(tb => {
        if (tb.id === tabId) {
            return { ...tb, url, title };
        }
        return tb;
    });
    // Prefer workspace key encoded in tab id (`browser-<wsId>`).
    const wsKey = tabId.startsWith('browser-') ? tabId.slice('browser-'.length) : activeBrowserWorkspaceKey();
    writeBrowserSession(wsKey, url, title);
    touchBrowserActivity();
};

/**
 * Open a two-column content tab directly (no toggle) — used by the stage
 * entry defaults. `tasks` carries no module/cc URL; `channels` needs its
 * embed URL loaded.
 */
export const openContentTab = (tab: RightDrawerTab) => {
    if (!ui.isMobile.value && isSidePanelType(tab)) {
        openOrReuseSidePanelTab(tab);
        return;
    }
    if (activeDrawerTab.value === tab) return;
    if (activeDrawerTab.value === 'browser' && tab !== 'browser') clearBrowserIdleTimer();
    activeDrawerTab.value = tab;
    activeModulePath.value = '';
    persistDrawerTab(tab);
    if (tab === 'channels') wsStore.loadCcConnectUrl();
    if (tab === 'browser') scheduleBrowserIdleClose();
    ui.triggerTerminalFit();
};

/** Close the right content column. */
export const closeContentTab = () => {
    activeExternalApp.value = null;
    const activeSideTab = activeSidePanelTab.value;
    if (activeSideTab?.type === 'terminal' && !activeSideTab.reclaimed) {
        updateSidePanelTab(activeSideTab.id, { mounted: false });
    }
    sidePanelOpen.value = false;
    persistSidePanelState();
    if (activeDrawerTab.value === 'none') return;
    if (activeDrawerTab.value === 'browser') clearBrowserIdleTimer();
    activeDrawerTab.value = 'none';
    activeModulePath.value = '';
    persistDrawerTab('none');
    ui.triggerTerminalFit();
};

// Open external app helper
export const openExternalApp = (appId: string) => {
    activeDrawerTab.value = 'discovery';
    activeExternalApp.value = appId;
    ui.triggerTerminalFit();
};

export const closeExternalApp = () => {
    activeExternalApp.value = null;
    ui.triggerTerminalFit();
};

// Coze click shortcut toggle dynamic drawer logic
export const toggleDrawerTab = (tab: RightDrawerTab) => {
    activeExternalApp.value = null;
    if (!ui.isMobile.value && isSidePanelType(tab)) {
        if (sidePanelOpen.value && activeSidePanelTab.value?.type === tab) {
            closeContentTab();
        } else {
            openOrReuseSidePanelTab(tab);
        }
        return;
    }
    // Full-page modules (discovery / settings / …) and L1 apps are same-level
    // primary surfaces — leave any active L1 app so it cannot cover the module.
    // Lazy import avoids store cycle (stageStore → tabsStore).
    if (isFullPageTab(tab) || (activeDrawerTab.value === tab && isFullPageTab(activeDrawerTab.value))) {
        void import('./stageStore').then(s => s.exitL1App());
    }
    // 任务 is a normal right-column / drawer content tab on both platforms now
    // (the old mobile full-screen 任务 overlay was removed in favor of the
    // unified in-project view switcher).
    if (activeDrawerTab.value === tab) {
        // Collapse the drawer
        if (tab === 'browser') clearBrowserIdleTimer();
        activeDrawerTab.value = 'none';
        activeModulePath.value = '';
        persistDrawerTab('none');
    } else {
        // Expand drawer with smart width: widest for tasks, wide for
        // channels/git/files, narrow otherwise.
        const smartWidth =
            tab === 'tasks'
                ? Math.max(ui.rightPanelWidth.value, 500)
                : tab === 'channels' || tab === 'providers' || tab === 'git' || tab === 'files'
                  ? Math.max(ui.rightPanelWidth.value, 450)
                  : 320;

        // Module-backed tabs get their entry path; non-module tabs clear it.
        const mod = getModuleByTab(tab);
        if (activeDrawerTab.value === 'browser' && tab !== 'browser') clearBrowserIdleTimer();
        ui.rightPanelWidth.value = smartWidth;
        activeDrawerTab.value = tab;
        persistDrawerTab(tab);
        activeModulePath.value = mod ? mod.entryPath : '';
        // 更多 → 发现中心: land on「应用」category (Agents 圆桌 entry, design §6.3).
        if (tab === 'discovery') {
            discoveryCategory.value = DISCOVERY_DEFAULT_CATEGORY;
        }
        if (tab === 'channels') {
            wsStore.loadCcConnectUrl();
        } else if (tab === 'providers') {
            wsStore.loadCcProvidersUrl();
        } else if (mod) {
            loadModuleManifest(mod);
        }
        if (tab === 'browser') {
            // Ensure a session tab for this workspace exists when opening the column.
            if (!tabs.value.some(tb => tb.type === 'browser')) {
                ensureBrowserSessionTab('');
            }
            scheduleBrowserIdleClose();
        }
    }
    ui.triggerTerminalFit();
};

// Open the discovery panel (if needed) and scroll to a given category.
export const selectDiscoveryCategory = (category: string) => {
    activeDrawerTab.value = 'discovery';
    discoveryCategory.value = category;
    ui.triggerTerminalFit();
};

/**
 * Handles `CustomEvent('navigate', { detail: { path } })` bubbling up
 * from a module custom element. Mirrors the path into host state and
 * the main app URL. Registered on `document` by App.componentDidMount.
 */
export const handleModuleNavigate = (e: Event) => {
    const target = e.target as HTMLElement | null;
    if (!target) return;
    const tag = target.tagName ? target.tagName.toLowerCase() : '';
    if (tag !== 'skills-panel' && tag !== 'cc-connect-panel') return;
    const detail = (e as CustomEvent<{ path: string }>).detail;
    if (!detail || typeof detail.path !== 'string' || !detail.path) return;
    const path = detail.path;
    if (path === activeModulePath.value) return;
    activeModulePath.value = path;
    syncModuleUrl(path);
};

/**
 * Map an active drawer tab to the id of its module-side custom element.
 * All three module-backed tabs (channels, providers, skills) now use
 * custom elements instead of iframes.
 */
const getActiveModulePanelId = (): string | null => {
    const tab = activeDrawerTab.value;
    if (tab === 'channels') return 'cc-channels-panel';
    if (tab === 'providers') return 'cc-providers-panel';
    if (tab === 'skills') return 'skills-panel';
    return null;
};

/**
 * Pushes a route change to the active module panel. Called by
 * `<ModuleNav />` when the user clicks a manifest link.
 *
 * Since all modules now use custom elements (no more iframes), we
 * update host state and set the `route` attribute on the panel
 * element directly. The element's `attributeChangedCallback`
 * forwards this to its internal MemoryRouter via `EmbedBridge`.
 */
export const navigateInModule = (to: string) => {
    if (!to) return;
    if (to === activeModulePath.value) return;
    activeModulePath.value = to;
    syncModuleUrl(to);
    const panelId = getActiveModulePanelId();
    if (panelId) {
        const panel = document.getElementById(panelId);
        if (panel) panel.setAttribute('route', to);
    }
};

/**
 * Mirrors the active module path into the main app URL as
 * `/m/<moduleId>/<subPath>`. Uses `replaceState` so the iframe's
 * internal back/forward doesn't get clobbered.
 */
const syncModuleUrl = (subPath: string) => {
    const mod = getModuleByTab(activeDrawerTab.value);
    if (!mod) return;
    const url = new URL(window.location.href);
    const cleanPath = subPath.startsWith('/') ? subPath : '/' + subPath;
    url.search = '';
    url.hash = `/m/${mod.moduleId}${cleanPath}`;
    try {
        window.history.replaceState({}, '', url.toString());
    } catch {
        /* ignore */
    }
};

/**
 * Fetches the live manifest for a module and merges it over the static
 * one. Failures are silent — the static manifest keeps the sidebar
 * functional even when the module is offline.
 */
export const loadModuleManifest = async (mod: ModuleRegistration) => {
    if (!mod.manifestUrl) return;
    try {
        const res = await fetch(mod.manifestUrl, { credentials: 'same-origin' });
        if (!res.ok) return;
        const live = (await res.json()) as ModuleManifest;
        moduleManifests.value = {
            ...moduleManifests.value,
            [mod.moduleId]: mergeManifests(mod.staticManifest, live),
        };
    } catch {
        /* static manifest is the fallback — nothing to do */
    }
};

/**
 * Switches the active sub-category in the system settings page. Called
 * by the host's `LeftSidebar` `ModuleNav` (desktop) and by the mobile
 * "more" menu when the user picks a settings category.
 */
export const setSettingsCategory = (category: SettingsCategory) => {
    if (activeSettingsCategory.value === category) return;
    activeSettingsCategory.value = category;
};

/**
 * Returns the module nav data to pass to `LeftSidebar`, or undefined if
 * the active drawer tab isn't module-backed. The live manifest is used
 * when available; the static manifest is the fallback.
 *
 * Settings is a special case: it's a host-rendered page (no iframe),
 * so its `onNavigate` updates `activeSettingsCategory` and we use that
 * state (not `activeModulePath`) to derive the active link.
 */
export const buildModuleNav = ():
    | { manifest: ModuleManifest; activePath: string; onNavigate: (to: string) => void }
    | undefined => {
    const mod = getModuleByTab(activeDrawerTab.value);
    if (!mod) return undefined;
    // Skills renders its own top-tab navigation inside the panel (project-page
    // style), so the host suppresses the module sub-nav in the left sidebar —
    // the sidebar falls back to the workspace tree, matching project pages.
    if (mod.moduleId === 'skills') return undefined;
    const live = moduleManifests.value[mod.moduleId];
    const manifest = live ?? mod.staticManifest;
    if (mod.moduleId === SETTINGS_MODULE_ID) {
        return {
            manifest,
            activePath: settingsCategoryToPath(activeSettingsCategory.value),
            onNavigate: (to: string) => setSettingsCategory(pathToSettingsCategory(to)),
        };
    }
    if (mod.moduleId === DISCOVERY_MODULE_ID) {
        // Discovery, like settings, is host-rendered: its `onNavigate`
        // updates `discoveryCategory` (which drives the panel's scroll) and
        // we derive the active link from that state, not `activeModulePath`.
        return {
            manifest,
            activePath: discoveryCategoryToPath(discoveryCategory.value),
            onNavigate: (to: string) => selectDiscoveryCategory(pathToDiscoveryCategory(to)),
        };
    }
    return {
        manifest,
        activePath: activeModulePath.value || mod.entryPath,
        onNavigate: navigateInModule,
    };
};
