/**
 * Chrome surface mode: 工作台 vs 聊天.
 *
 * Independent of beginner/advanced `uiMode` and of Product Shell id.
 * Pure helpers so tests can drive persist/switch/restore without DOM.
 */

export const CHROME_MODES = ['workbench', 'chat'] as const;
export type ChromeMode = (typeof CHROME_MODES)[number];

/** Verbatim labels required by the chrome dropdown (do not i18n-away). */
export const CHROME_MODE_LABELS: Record<ChromeMode, '工作台' | '聊天'> = {
    workbench: '工作台',
    chat: '聊天',
};

/** Ordered dropdown entries: exactly 工作台 and 聊天. */
export const CHROME_MODE_OPTIONS: Array<{ id: ChromeMode; name: '工作台' | '聊天' }> = CHROME_MODES.map(id => ({
    id,
    name: CHROME_MODE_LABELS[id],
}));

/** Synthetic id for the 聊天 surface in mobile menus that also list product shells. */
export const CHAT_SURFACE_ID = 'chat';

/** Shown when no product shells are registered so the menu still has 工作台. */
export const WORKBENCH_FALLBACK_ID = 'workbench';

export interface WorkbenchMenuItem {
    id: string;
    kind: 'shell' | 'chat';
    name: string;
}

/**
 * One chrome menu: enabled product shells (or a 工作台 fallback) plus 聊天.
 * Desktop keeps ModeSwitcher (工作台/聊天) separate from ShellSwitcher;
 * mobile hamburger uses this combined list.
 */
export function buildWorkbenchMenu(shells: Array<{ id: string; name: string }>): WorkbenchMenuItem[] {
    const shellItems: WorkbenchMenuItem[] =
        shells.length > 0
            ? shells.map(s => ({ id: s.id, kind: 'shell' as const, name: s.name }))
            : [{ id: WORKBENCH_FALLBACK_ID, kind: 'shell', name: CHROME_MODE_LABELS.workbench }];
    return [...shellItems, { id: CHAT_SURFACE_ID, kind: 'chat', name: CHROME_MODE_LABELS.chat }];
}

/** Active item in the combined chrome menu. */
export function resolveActiveMenuId(mode: ChromeMode, activeShellId: string, items: WorkbenchMenuItem[]): string {
    if (mode === 'chat') return CHAT_SURFACE_ID;
    if (activeShellId && items.some(i => i.kind === 'shell' && i.id === activeShellId)) {
        return activeShellId;
    }
    return items.find(i => i.kind === 'shell')?.id ?? WORKBENCH_FALLBACK_ID;
}

export const CHROME_MODE_STORAGE_KEY = '1agents-chrome-mode-state';

export interface WorkbenchSurface {
    sidebarMode: 'assistant' | 'project';
    stageView: 'conversation' | 'project';
    activeDrawerTab: string;
    activeTab: string;
    activeSessionId: string | null;
}

export interface ChromeModeState {
    mode: ChromeMode;
    lastWorkbench: WorkbenchSurface | null;
    lastChatRoomId: string | null;
}

export interface ChromeModeSnapshot {
    workbench?: WorkbenchSurface | null;
    chatRoomId?: string | null;
}

export interface StringStorage {
    getItem(key: string): string | null;
    setItem(key: string, value: string): void;
}

export function parseChromeMode(raw: unknown): ChromeMode {
    return raw === 'chat' ? 'chat' : 'workbench';
}

export function defaultChromeModeState(): ChromeModeState {
    return { mode: 'workbench', lastWorkbench: null, lastChatRoomId: null };
}

function parseWorkbenchSurface(raw: unknown): WorkbenchSurface | null {
    if (!raw || typeof raw !== 'object') return null;
    const s = raw as Record<string, unknown>;
    const sidebarMode = s.sidebarMode === 'project' ? 'project' : 'assistant';
    const stageView = s.stageView === 'project' ? 'project' : 'conversation';
    return {
        sidebarMode,
        stageView,
        activeDrawerTab: typeof s.activeDrawerTab === 'string' ? s.activeDrawerTab : 'none',
        activeTab: typeof s.activeTab === 'string' ? s.activeTab : 'new_chat',
        activeSessionId: typeof s.activeSessionId === 'string' ? s.activeSessionId : null,
    };
}

export function hydrateChromeModeState(raw: unknown): ChromeModeState {
    const base = defaultChromeModeState();
    if (!raw || typeof raw !== 'object') return base;
    const s = raw as Record<string, unknown>;
    return {
        mode: parseChromeMode(s.mode),
        lastWorkbench: parseWorkbenchSurface(s.lastWorkbench),
        lastChatRoomId: typeof s.lastChatRoomId === 'string' && s.lastChatRoomId ? s.lastChatRoomId : null,
    };
}

export function readPersistedChromeMode(storage: StringStorage): ChromeModeState {
    try {
        const raw = storage.getItem(CHROME_MODE_STORAGE_KEY);
        if (!raw) return defaultChromeModeState();
        return hydrateChromeModeState(JSON.parse(raw));
    } catch {
        return defaultChromeModeState();
    }
}

export function writePersistedChromeMode(storage: StringStorage, state: ChromeModeState): void {
    storage.setItem(CHROME_MODE_STORAGE_KEY, JSON.stringify(state));
}

/**
 * Switch 工作台 ↔ 聊天. Leaving a mode snapshots that mode's last surface so
 * switching back restores it instead of wiping it.
 */
export function switchChromeMode(
    state: ChromeModeState,
    next: ChromeMode,
    snapshot: ChromeModeSnapshot = {}
): ChromeModeState {
    if (next === state.mode) {
        return {
            ...state,
            lastWorkbench: snapshot.workbench !== undefined ? snapshot.workbench : state.lastWorkbench,
            lastChatRoomId: snapshot.chatRoomId !== undefined ? snapshot.chatRoomId : state.lastChatRoomId,
        };
    }
    if (state.mode === 'workbench') {
        return {
            mode: next,
            lastWorkbench: snapshot.workbench !== undefined ? snapshot.workbench : state.lastWorkbench,
            lastChatRoomId: state.lastChatRoomId,
        };
    }
    return {
        mode: next,
        lastWorkbench: state.lastWorkbench,
        lastChatRoomId: snapshot.chatRoomId !== undefined ? snapshot.chatRoomId : state.lastChatRoomId,
    };
}

export function restoreTarget(state: ChromeModeState): {
    mode: ChromeMode;
    workbench: WorkbenchSurface | null;
    chatRoomId: string | null;
} {
    return {
        mode: state.mode,
        workbench: state.lastWorkbench,
        chatRoomId: state.lastChatRoomId,
    };
}
