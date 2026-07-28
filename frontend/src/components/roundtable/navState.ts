/**
 * Persist roundtable navigation for reloads and explicit room/session returns.
 * - view=list  → topic card grid
 * - view=room  → last open room page (needs roomId)
 * User-facing app entries call requestRoundtableListView() and always open list.
 */

export type RoundtableView = 'list' | 'room';

const LS_VIEW = 'oneagents.roundtable.view';
const LS_ROOM = 'oneagents.roundtable.activeRoomId';
const listViewListeners = new Set<() => void>();

export function readStoredView(): RoundtableView {
    try {
        const v = localStorage.getItem(LS_VIEW);
        if (v === 'room' || v === 'list') return v;
    } catch {
        /* ignore */
    }
    return 'list';
}

export function readStoredRoomId(): string | null {
    try {
        return localStorage.getItem(LS_ROOM);
    } catch {
        return null;
    }
}

export function persistListView(): void {
    try {
        localStorage.setItem(LS_VIEW, 'list');
    } catch {
        /* ignore */
    }
}

/**
 * User-facing app entries always mean "open the roundtable list".
 * Persist first so a not-yet-mounted view starts on the list, then notify an
 * already-mounted view so clicking the active app also leaves the room page.
 */
export function requestRoundtableListView(): void {
    persistListView();
    for (const listener of Array.from(listViewListeners)) listener();
}

export function subscribeRoundtableListView(listener: () => void): () => void {
    listViewListeners.add(listener);
    return () => listViewListeners.delete(listener);
}

export function persistRoomView(roomId: string): void {
    try {
        localStorage.setItem(LS_VIEW, 'room');
        localStorage.setItem(LS_ROOM, roomId);
    } catch {
        /* ignore */
    }
}

/** Initial shell: restore room only when we last left on a room page. */
export function resolveInitialNav(roomIdProp?: string): {
    view: RoundtableView | 'create';
    roomId?: string;
} {
    if (roomIdProp) {
        return { view: 'room', roomId: roomIdProp };
    }
    const view = readStoredView();
    const roomId = readStoredRoomId() || undefined;
    if (view === 'room' && roomId) {
        return { view: 'room', roomId };
    }
    return { view: 'list' };
}
