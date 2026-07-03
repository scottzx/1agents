import { signal, computed } from '@preact/signals';

import { isFullPageTab, type RightDrawerTab } from '../components/types';
import * as tabsStore from './tabsStore';
import * as ui from './uiStore';
import * as appStore from './appManifestStore';

/**
 * Stage layout model — the unified two-column workbench shell.
 *
 * The desktop content area is one fixed shape (never flipped by role):
 * a CHAT column on the left and a WORK-ARTIFACT column on the right.
 *
 *   - PRIMARY pane (`panes[0]`, always present): the left CHAT column —
 *     chat / new-chat / terminal. Driven by the left sidebar (session
 *     selection) and derived here from `tabsStore.activeTab`.
 *   - SECONDARY pane (`panes[1]`, optional): the right ARTIFACT column —
 *     one of the four content tabs (项目管理 / 渠道 / 文件 / Git) plus the
 *     PM副屏. Driven by `tabsStore.activeDrawerTab` (single-select; the
 *     header toolbar re-uses `toggleDrawerTab`'s "click again = close").
 *
 * Full-page modules (providers / skills / discovery / settings) are not a
 * two-column state: they take over as a single full-width pane.
 *
 * This store owns the *layout* state (which column is collapsed, the split
 * ratio); *what content* each column shows still lives in `tabsStore`.
 * `panes` is a computed view over those signals so `DesktopAppLayout`
 * renders from one place.
 */

/**
 * A unit of content that can live in a pane. The `kind` selects the leaf
 * component rendered by `<ContentViewHost>`.
 */
export type ContentView =
    | { kind: 'chat'; sessionId?: string }
    | { kind: 'newChat' }
    | { kind: 'terminal' }
    | { kind: 'preview'; tabId: string }
    | { kind: 'browser'; tabId: string }
    | { kind: 'files' }
    | { kind: 'git' }
    | { kind: 'tasks' }
    | { kind: 'reminders' }
    | { kind: 'assistants' }
    | { kind: 'contacts' }
    | { kind: 'datasources' }
    | { kind: 'inbox' }
    | { kind: 'personal' }
    | { kind: 'retro' }
    | { kind: 'channels' }
    | { kind: 'providers' }
    | { kind: 'skills' }
    | { kind: 'discovery' }
    | { kind: 'settings' }
    /** L1 app page rendered via MountPointRenderer (#332). */
    | { kind: 'l1-app'; mountId: string };

export type ContentViewKind = ContentView['kind'];

export interface Pane {
    id: string;
    view: ContentView;
}

/**
 * The four desktop layout modes (#redesign). Derived from the top-level
 * 对话/项目 context (`ui.sidebarMode`), the active full-page/focus tab, and
 * the project drill stack — see `layoutMode` below.
 *
 *   - `focus`            single full-width pane (联系人 / 消息 / 待办 / 日历 /
 *                        Inbox / 复盘 / 设置 / 发现 / 技能 / L1 app). The pane
 *                        owns its own master-detail (inline drawer / modal /
 *                        popover) via <ListDetailShell>.
 *   - `split`            the two-column conversation workbench (chat + artifact).
 *   - `project-overview` the 项目总览 card wall (ProjectHome).
 *   - `project`          a single project's detail page (ProjectShell) reached
 *                        by drilling into a card; `projectStack` drives the
 *                        breadcrumb.
 */
export type LayoutMode = 'focus' | 'split' | 'project-overview' | 'project';

/** One breadcrumb level of the 项目 drill-down (overview → detail → …). */
export interface ProjectStackEntry {
    workspaceId: string;
    name: string;
}

/**
 * The 项目-mode drill stack. Empty ⇒ 项目总览 (card wall); one entry ⇒ that
 * project's detail page. Persisted so a reload (e.g. the 大屏 → main-app drill,
 * which navigates via `window.location`) restores the drilled-in project.
 */
const PROJECT_STACK_KEY = '1agents-project-stack';
const loadProjectStack = (): ProjectStackEntry[] => {
    try {
        const raw = localStorage.getItem(PROJECT_STACK_KEY);
        const parsed = raw ? JSON.parse(raw) : [];
        return Array.isArray(parsed) ? parsed : [];
    } catch {
        return [];
    }
};
export const projectStack = signal<ProjectStackEntry[]>(loadProjectStack());
const setProjectStack = (next: ProjectStackEntry[]): void => {
    projectStack.value = next;
    try {
        localStorage.setItem(PROJECT_STACK_KEY, JSON.stringify(next));
    } catch {
        /* non-fatal — drill state just won't survive reload */
    }
};

/** Drop any drill entries whose workspace no longer exists (called on boot). */
export const pruneProjectStack = (validIds: Set<string>): void => {
    const cur = projectStack.value;
    const pruned = cur.filter(e => validIds.has(e.workspaceId));
    if (pruned.length !== cur.length) setProjectStack(pruned);
};

/**
 * What the MAIN content area is currently showing, independent of which list
 * the left sidebar renders (`ui.sidebarMode`). This is the key decoupling:
 * flipping the 助手/项目 sidebar tabs only re-renders the sidebar list — it must
 * NOT drag the main stage along. Only explicit navigation (opening a
 * conversation, drilling into a project, hitting 项目总览) moves the stage.
 *
 *   - 'conversation' — the split chat/terminal workbench (or a focus full-page
 *     tab, which wins in `layoutMode` regardless).
 *   - 'project'      — the 项目总览 card wall (empty stack) or a drilled-in
 *     project detail page (non-empty stack).
 */
const STAGE_VIEW_KEY = '1agents-stage-view';
const loadStageView = (): 'conversation' | 'project' => {
    const v = localStorage.getItem(STAGE_VIEW_KEY);
    if (v === 'conversation' || v === 'project') return v;
    // Legacy / first run: mirror the persisted sidebar context so a reload
    // still lands on the project pages when the user last left 项目 context.
    return localStorage.getItem('1agents-sidebar-mode') === 'project' ? 'project' : 'conversation';
};
export const stageView = signal<'conversation' | 'project'>(loadStageView());
const setStageView = (v: 'conversation' | 'project'): void => {
    stageView.value = v;
    try {
        localStorage.setItem(STAGE_VIEW_KEY, v);
    } catch {
        /* non-fatal */
    }
};

/**
 * The four single-select content tabs surfaced by the header toolbar, in
 * display order (项目管理 first, before 渠道). These are also the only
 * secondary (right-column) views.
 */
export const HEADER_CONTENT_TABS: RightDrawerTab[] = ['tasks', 'channels', 'files', 'git'];
const SECONDARY_TABS: RightDrawerTab[] = HEADER_CONTENT_TABS;

// ── Persisted layout state ──────────────────────────────────────────────
const SPLIT_KEY = '1agents-split-ratio';
const RAIL_KEY = '1agents-chat-railed';

const loadSplitRatio = (): number => {
    const n = parseFloat(localStorage.getItem(SPLIT_KEY) || '');
    return Number.isFinite(n) && n >= 0.2 && n <= 0.8 ? n : 0.6;
};

/** Fraction of the content area given to the chat (left) column when split. */
export const splitRatio = signal(loadSplitRatio());

export const setSplitRatio = (ratio: number): void => {
    const clamped = Math.max(0.2, Math.min(0.8, ratio));
    splitRatio.value = clamped;
    localStorage.setItem(SPLIT_KEY, String(clamped));
};

/**
 * Whether the left chat column is collapsed to a rail. First-ever load
 * defaults to railed so the project landing (kanban) leads; afterwards the
 * persisted value wins.
 */
const railPref = localStorage.getItem(RAIL_KEY);
// Beginner mode leads with the conversation, so the chat column is never railed
// on first load; afterwards the persisted value wins (same as advanced).
const beginnerFirstLoad = railPref === null && localStorage.getItem('1agents-ui-mode') === 'beginner';
export const chatRailed = signal(beginnerFirstLoad ? false : railPref === null ? true : railPref === 'true');

const setChatRailed = (v: boolean): void => {
    chatRailed.value = v;
    localStorage.setItem(RAIL_KEY, String(v));
};

// ── Derived content → panes ─────────────────────────────────────────────
const primaryView = (): ContentView => {
    // L1 app page takes over the primary pane when active (#332).
    const l1PageId = appStore.activeL1PageId.value;
    if (l1PageId) return { kind: 'l1-app', mountId: l1PageId };

    const drawer = tabsStore.activeDrawerTab.value;
    if (isFullPageTab(drawer)) return { kind: drawer } as ContentView;
    const tab = tabsStore.activeTab.value;
    if (tab === 'new_chat') return { kind: 'newChat' };
    if (tab === 'agents') return { kind: 'chat' };
    return { kind: 'terminal' };
};

/**
 * `[primary]` or `[primary, secondary]`, computed from the nav signals.
 * Full-page modules collapse to a single pane (the secondary is suppressed).
 */
export const panes = computed<Pane[]>(() => {
    const drawer = tabsStore.activeDrawerTab.value;
    const primary: Pane = { id: 'primary', view: primaryView() };
    if (isFullPageTab(drawer)) return [primary];
    if (SECONDARY_TABS.includes(drawer)) {
        return [primary, { id: 'content', view: { kind: drawer } as ContentView }];
    }
    return [primary];
});

/** True when the right artifact column has content (a two-column state). */
export const hasContent = computed<boolean>(() => panes.value.length > 1);

/**
 * The active desktop layout mode (see `LayoutMode`). Full-page / focus panes
 * (incl. the L1 app page) win first — they are single-pane regardless of
 * context. Otherwise the *stage* view (`stageView`, NOT the sidebar list)
 * decides: 项目 splits into overview (empty stack) vs a drilled-in detail
 * page; a conversation is the split workbench. Toggling the 助手/项目 sidebar
 * tabs does not touch `stageView`, so it never moves the main content.
 */
export const layoutMode = computed<LayoutMode>(() => {
    if (appStore.activeL1PageId.value) return 'focus';
    if (isFullPageTab(tabsStore.activeDrawerTab.value)) return 'focus';
    if (stageView.value === 'project') {
        // Drilling from a project's detail page into a conversation (chat /
        // new-chat) flips to the split workbench; the project detail page and
        // the 项目总览 card wall share the rest of the 项目 stage.
        const tab = tabsStore.activeTab.value;
        if (tab === 'agents' || tab === 'new_chat') return 'split';
        return projectStack.value.length > 0 ? 'project' : 'project-overview';
    }
    return 'split';
});

/**
 * Tri-state column collapse, honoring the "≥1 column on screen" invariant:
 * with no right content there is nothing to collapse *into*, so the state is
 * always `'content'` (only the chat column shows).
 */
export const collapsed = computed<'none' | 'chat' | 'content'>(() => {
    if (!hasContent.value) return 'content';
    return chatRailed.value ? 'chat' : 'none';
});

// ── Actions ─────────────────────────────────────────────────────────────
/**
 * Header chat toggle: rail the chat column / bring it back. No-op when chat
 * is the only visible column (can't collapse the last one).
 */
export const toggleChat = (): void => {
    if (!hasContent.value) return;
    setChatRailed(!chatRailed.value);
    ui.triggerTerminalFit();
};

/** Close the right artifact column (e.g. the panel close button). */
export const closeContent = (): void => {
    tabsStore.closeContentTab();
};

/**
 * Entry default — opening a project / kanban: artifact column shows 项目管理,
 * the chat column collapses to a rail. Desktop only (mobile keeps its
 * full-screen overlay drawer).
 */
export const enterProject = (): void => {
    setStageView('conversation');
    tabsStore.openContentTab('tasks');
    setChatRailed(true);
};

/**
 * Entry default — opening a conversation (history / new): chat column leads,
 * artifact column closed. Desktop only. Used when the user explicitly starts a
 * NEW conversation (the artifact column resets away).
 */
export const enterConversation = (): void => {
    setStageView('conversation');
    tabsStore.closeContentTab();
    setChatRailed(false);
};

/**
 * Open / resume / switch a session while staying inside a project. The artifact
 * (right) column is preserved — only switching to a different project closes it
 * (that's a project change, handled like `enterConversation`). Either way the
 * chat column un-rails so the conversation leads. Desktop only.
 */
export const openConversation = (projectChanged: boolean): void => {
    setStageView('conversation');
    if (projectChanged) tabsStore.closeContentTab();
    setChatRailed(false);
};

/**
 * Top-level 助手/项目 sidebar-tab switch. This ONLY changes which list the left
 * sidebar renders — it deliberately does nothing to the main stage
 * (`stageView` / drill stack / active session / drawer). Flipping the tabs
 * back and forth must leave the content area exactly as it was; only explicit
 * navigation (opening a session, drilling into a project, 项目总览) moves the
 * stage. See `stageView` / `layoutMode`.
 */
export const showProjectContext = (): void => {
    ui.sidebarMode.value = 'project';
    localStorage.setItem('1agents-sidebar-mode', 'project');
    ui.triggerTerminalFit();
};

export const showAssistantContext = (): void => {
    ui.sidebarMode.value = 'assistant';
    localStorage.setItem('1agents-sidebar-mode', 'assistant');
    ui.triggerTerminalFit();
};

/**
 * 项目总览 — land on the card wall (empty drill stack). Switches the top-level
 * context to 项目 and clears any drilled-in detail. Desktop only.
 */
export const projectOverview = (): void => {
    ui.sidebarMode.value = 'project';
    localStorage.setItem('1agents-sidebar-mode', 'project');
    setStageView('project');
    setProjectStack([]);
    tabsStore.closeContentTab();
    // Drop any lingering conversation focus so the card wall (not a stale chat)
    // shows — see the `agents`/`new_chat` check in `layoutMode`.
    tabsStore.activeTab.value = 'terminal';
    ui.triggerTerminalFit();
};

/**
 * Drill into a project's detail page (ProjectShell) from the overview card wall
 * or the sidebar project tree. Switches to 项目 context, pushes one breadcrumb
 * level, and rails the chat column (the detail page owns the full width).
 */
export const enterProjectDetail = (workspaceId: string, name: string): void => {
    ui.sidebarMode.value = 'project';
    localStorage.setItem('1agents-sidebar-mode', 'project');
    setStageView('project');
    setProjectStack([{ workspaceId, name }]);
    tabsStore.closeContentTab();
    // Reset conversation focus so the detail page (not a stale chat) shows.
    tabsStore.activeTab.value = 'terminal';
    setChatRailed(true);
    ui.triggerTerminalFit();
};

/**
 * Enter an L1 app page (#332). The app page takes over the primary pane;
 * the right artifact column closes so the app gets full width.
 */
export const enterL1App = (mountId: string): void => {
    appStore.setActiveL1Page(mountId);
    tabsStore.closeContentTab();
    setChatRailed(true);
};

/**
 * Exit the active L1 app page and return to the normal shell.
 * Restores the chat column and clears L1 page state.
 */
export const exitL1App = (): void => {
    appStore.setActiveL1Page(null);
};
