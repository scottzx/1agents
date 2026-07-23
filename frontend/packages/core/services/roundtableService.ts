import { apiFetch } from './apiClient';

/**
 * Agents 圆桌脑暴 — typed client for /api/roundtable/* (design.md §5–§6).
 * Main timeline binds content_text only; process_ref is for fold-out process.
 */

export type RoomState =
    | 'drafting_brief'
    | 'waiting_r2'
    | 'summarizing_r2'
    | 'waiting_r3'
    | 'summarizing_r3'
    | 'done'
    | 'failed';

export type SeatRole = 'referee' | 'market' | 'product' | 'eng' | 'ops' | 'finance';

/** Backend seat status; UI maps failed → error. */
export type SeatStatus = 'ready' | 'speaking' | 'done' | 'failed';

export type TurnKind = 'chat' | 'speech' | 'summary' | 'system';

export type ProductKind = 'software' | 'hardware' | 'hybrid';

export interface RoundtableBrief {
    title: string;
    question: string;
    constraints: string;
    success_criteria: string;
    product_kind?: ProductKind;
}

export interface RoundtableSeat {
    id: string;
    room_id: string;
    role: SeatRole;
    agent_type: string;
    workspace_id: string;
    session_id?: string;
    acp_session_id?: string;
    status: SeatStatus;
    created_at: string;
}

export interface RoundtableTurn {
    id: string;
    room_id: string;
    round: number;
    seat_id?: string;
    kind: TurnKind | string;
    content_text: string;
    process_ref?: string;
    created_at: string;
}

export interface RoundtableRoom {
    id: string;
    title: string;
    state: RoomState;
    brief?: RoundtableBrief | null;
    summary_r2?: string;
    summary_r3?: string;
    created_at: string;
    updated_at: string;
    seats?: RoundtableSeat[];
    turns?: RoundtableTurn[];
}

export interface CreateRoomRequest {
    title?: string;
}

export interface ChatRequest {
    text: string;
}

export interface ChatResponse {
    room?: RoundtableRoom;
    user_turn?: RoundtableTurn;
    referee_turn?: RoundtableTurn;
    turns?: RoundtableTurn[];
    [key: string]: unknown;
}

export interface ConfirmBriefRequest {
    title: string;
    question: string;
    constraints: string;
    success_criteria: string;
    product_kind?: ProductKind;
}

async function readError(res: Response): Promise<string> {
    const t = await res.text();
    return t || res.statusText || `HTTP ${res.status}`;
}

export const roundtableService = {
    /** GET /api/roundtable/rooms — list topic rooms (newest first) */
    async listRooms(limit = 100): Promise<RoundtableRoom[]> {
        const q = limit > 0 ? `?limit=${encodeURIComponent(String(limit))}` : '';
        const res = await apiFetch(`/roundtable/rooms${q}`, {
            cache: 'no-store',
        });
        if (!res.ok) throw new Error(await readError(res));
        const body = (await res.json()) as { rooms?: RoundtableRoom[] };
        return body.rooms || [];
    },

    /** POST /api/roundtable/rooms */
    async createRoom(req: CreateRoomRequest = {}): Promise<RoundtableRoom> {
        const res = await apiFetch('/roundtable/rooms', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as RoundtableRoom;
    },

    /** GET /api/roundtable/rooms/{id} — room + seats + turns */
    async getRoom(id: string): Promise<RoundtableRoom> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}`, {
            cache: 'no-store',
        });
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as RoundtableRoom;
    },

    /** GET /api/roundtable/rooms/{id}/turns */
    async listTurns(id: string): Promise<RoundtableTurn[]> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/turns`, {
            cache: 'no-store',
        });
        if (!res.ok) throw new Error(await readError(res));
        const body = (await res.json()) as { turns?: RoundtableTurn[] };
        return body.turns || [];
    },

    /** GET /api/roundtable/rooms/{id}/seats */
    async listSeats(id: string): Promise<RoundtableSeat[]> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/seats`, {
            cache: 'no-store',
        });
        if (!res.ok) throw new Error(await readError(res));
        const body = (await res.json()) as { seats?: RoundtableSeat[] };
        return body.seats || [];
    },

    /** POST /api/roundtable/rooms/{id}/chat — R1 user↔referee */
    async chat(id: string, req: ChatRequest): Promise<ChatResponse> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/chat`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as ChatResponse;
    },

    /** POST /api/roundtable/rooms/{id}/brief — confirm Brief → waiting_r2 */
    async confirmBrief(id: string, req: ConfirmBriefRequest): Promise<RoundtableRoom> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/brief`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as RoundtableRoom;
    },

    /** POST /api/roundtable/rooms/{id}/r2 */
    async runR2(id: string): Promise<RoundtableRoom | unknown> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/r2`, {
            method: 'POST',
        });
        if (!res.ok) throw new Error(await readError(res));
        return res.json();
    },

    /** POST /api/roundtable/rooms/{id}/r3 */
    async runR3(id: string): Promise<RoundtableRoom | unknown> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/r3`, {
            method: 'POST',
        });
        if (!res.ok) throw new Error(await readError(res));
        return res.json();
    },
};
