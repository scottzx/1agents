/**
 * Persist roundtable app navigation so sidebar re-entry restores list or room.
 * - view=list  → topic card grid
 * - view=room  → last open room page (needs roomId)
 */

export type RoundtableView = 'list' | 'room';

const LS_VIEW = 'oneagents.roundtable.view';
const LS_ROOM = 'oneagents.roundtable.activeRoomId';

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
