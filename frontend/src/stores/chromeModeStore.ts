/**
 * Live chrome-mode signals. Persistence and switch semantics live in
 * `chromeMode.ts`; this module is the UI-facing wrapper.
 */
import { signal } from '@preact/signals';

import {
    readPersistedChromeMode,
    switchChromeMode,
    writePersistedChromeMode,
    type ChromeMode,
    type ChromeModeState,
    type WorkbenchSurface,
} from './chromeMode';

function browserStorage(): Storage | null {
    try {
        return typeof localStorage === 'undefined' ? null : localStorage;
    } catch {
        return null;
    }
}

function load(): ChromeModeState {
    const storage = browserStorage();
    if (!storage) {
        return { mode: 'workbench', lastWorkbench: null, lastChatRoomId: null };
    }
    return readPersistedChromeMode(storage);
}

const initial = load();

export const chromeMode = signal<ChromeMode>(initial.mode);
export const lastWorkbenchSurface = signal<WorkbenchSurface | null>(initial.lastWorkbench);
export const lastChatRoomId = signal<string | null>(initial.lastChatRoomId);

function persist(state: ChromeModeState): void {
    chromeMode.value = state.mode;
    lastWorkbenchSurface.value = state.lastWorkbench;
    lastChatRoomId.value = state.lastChatRoomId;
    const storage = browserStorage();
    if (storage) writePersistedChromeMode(storage, state);
}

function currentState(): ChromeModeState {
    return {
        mode: chromeMode.value,
        lastWorkbench: lastWorkbenchSurface.value,
        lastChatRoomId: lastChatRoomId.value,
    };
}

export function setChromeMode(
    next: ChromeMode,
    snapshot: { workbench?: WorkbenchSurface | null; chatRoomId?: string | null } = {}
): ChromeModeState {
    const state = switchChromeMode(currentState(), next, snapshot);
    persist(state);
    return state;
}

export function rememberChatRoom(roomId: string | null): void {
    persist({ ...currentState(), lastChatRoomId: roomId });
}

export { CHROME_MODE_LABELS, CHROME_MODES } from './chromeMode';
export type { ChromeMode, WorkbenchSurface } from './chromeMode';
