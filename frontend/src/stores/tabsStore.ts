import { signal } from '@preact/signals';

import type { FsEntry, RightDrawerTab } from '../components/types';
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
import { DISCOVERY_MODULE_ID, pathToDiscoveryCategory, discoveryCategoryToPath } from '../modules/discovery-manifest';
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
const CONTENT_DRAWER_TABS: RightDrawerTab[] = ['tasks', 'channels', 'files', 'browser', 'git'];
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
/** Selected discovery category, drives the sidebar second-level menu. */
export const discoveryCategory = signal('featured');
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

/** Ensure a single browser session tab exists; return its id. */
const ensureBrowserSessionTab = (url: string): string => {
    const existing = tabs.value.find(tb => tb.type === 'browser');
    if (existing) {
        if (url && url !== existing.url) {
            let title = t('app.browser.title', ui.language.value);
            try {
                if (url && url !== 'about:blank') title = new URL(url).hostname || title;
            } catch {
                /* keep default */
            }
            tabs.value = tabs.value.map(tb => (tb.id === existing.id ? { ...tb, url, title } : tb));
        }
        return existing.id;
    }
    const tabId = `browser-${Date.now()}`;
    const newTab: Tab = {
        id: tabId,
        title: t('app.browser.title', ui.language.value),
        type: 'browser',
        url: url || '',
        closable: true,
    };
    tabs.value = [...tabs.value, newTab];
    return tabId;
};

/**
 * Open a URL in the built-in lightweight browser as the right-column
 * content (peer of 文件 / Git). Does not use the top workspace tab bar.
 */
export const openBrowserTab = (url = '') => {
    const normalized = normalizeBrowserUrl(url);
    ensureBrowserSessionTab(normalized);
    openContentTab('browser');
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
    let title = t('app.browser.title', ui.language.value);
    try {
        if (url && url !== 'about:blank') {
            title = new URL(url).hostname || title;
        }
    } catch {
        /* keep default title */
    }
    tabs.value = tabs.value.map(tb => {
        if (tb.id === tabId) {
            return { ...tb, url, title };
        }
        return tb;
    });
};

/**
 * Open a two-column content tab directly (no toggle) — used by the stage
 * entry defaults. `tasks` carries no module/cc URL; `channels` needs its
 * embed URL loaded.
 */
export const openContentTab = (tab: RightDrawerTab) => {
    if (activeDrawerTab.value === tab) return;
    activeDrawerTab.value = tab;
    activeModulePath.value = '';
    persistDrawerTab(tab);
    if (tab === 'channels') wsStore.loadCcConnectUrl();
    ui.triggerTerminalFit();
};

/** Close the right content column. */
export const closeContentTab = () => {
    activeExternalApp.value = null;
    if (activeDrawerTab.value === 'none') return;
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
    // 任务 is a normal right-column / drawer content tab on both platforms now
    // (the old mobile full-screen 任务 overlay was removed in favor of the
    // unified in-project view switcher).
    if (activeDrawerTab.value === tab) {
        // Collapse the drawer
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
        ui.rightPanelWidth.value = smartWidth;
        activeDrawerTab.value = tab;
        persistDrawerTab(tab);
        activeModulePath.value = mod ? mod.entryPath : '';
        if (tab === 'channels') {
            wsStore.loadCcConnectUrl();
        } else if (tab === 'providers') {
            wsStore.loadCcProvidersUrl();
        } else if (mod) {
            loadModuleManifest(mod);
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
