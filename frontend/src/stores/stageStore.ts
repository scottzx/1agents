import { signal, computed } from '@preact/signals';

import { isFullPageTab, type RightDrawerTab } from '../components/types';
import * as tabsStore from './tabsStore';
import * as ui from './uiStore';

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
    | { kind: 'contacts' }
    | { kind: 'inbox' }
    | { kind: 'personal' }
    | { kind: 'retro' }
    | { kind: 'channels' }
    | { kind: 'providers' }
    | { kind: 'skills' }
    | { kind: 'discovery' }
    | { kind: 'settings' };

export type ContentViewKind = ContentView['kind'];

export interface Pane {
    id: string;
    view: ContentView;
}

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
    tabsStore.openContentTab('tasks');
    setChatRailed(true);
};

/**
 * Entry default — opening a conversation (history / new): chat column leads,
 * artifact column closed. Desktop only. Used when the user explicitly starts a
 * NEW conversation (the artifact column resets away).
 */
export const enterConversation = (): void => {
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
    if (projectChanged) tabsStore.closeContentTab();
    setChatRailed(false);
};
