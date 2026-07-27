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

export type BriefStatus = 'draft' | 'proposed' | 'confirmed' | 'superseded';

export type BriefProposer = 'user' | 'referee';

export interface RoundtableBriefVersion {
    room_id: string;
    version: number;
    status: BriefStatus;
    content: RoundtableBrief;
    proposed_by: BriefProposer;
    source_turn_id?: string;
    created_at: string;
    updated_at: string;
    confirmed_at?: string;
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
    /** Compatibility projection of current_brief.content. */
    brief?: RoundtableBrief | null;
    current_brief_version?: number;
    confirmed_brief_version?: number;
    r2_brief_version?: number;
    current_brief?: RoundtableBriefVersion | null;
    confirmed_brief?: RoundtableBriefVersion | null;
    r2_brief?: RoundtableBriefVersion | null;
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

export interface BriefContentRequest {
    title: string;
    question: string;
    constraints: string;
    success_criteria: string;
    product_kind?: ProductKind;
}

export interface SaveBriefDraftRequest extends BriefContentRequest {
    expected_version: number;
}

export interface ProposeBriefRequest extends BriefContentRequest {
    expected_version: number;
    source_turn_id?: string;
}

export interface ConfirmBriefRequest {
    version: number;
    expected_version: number;
}

/** Deprecated one-shot management contract; agents must use proposeBrief. */
export type LegacySetBriefRequest = BriefContentRequest;

export class BriefVersionConflictError extends Error {
    readonly status = 409;

    constructor(
        message: string,
        readonly expectedVersion: number,
        readonly currentVersion: number
    ) {
        super(message);
        this.name = 'BriefVersionConflictError';
    }
}

async function readError(res: Response): Promise<string> {
    const t = await res.text();
    return t || res.statusText || `HTTP ${res.status}`;
}

async function readBriefMutationError(res: Response): Promise<Error> {
    const text = await res.text();
    if (res.status === 409) {
        try {
            const payload = JSON.parse(text) as {
                message?: string;
                expected_version?: number;
                current_version?: number;
            };
            return new BriefVersionConflictError(
                payload.message || 'Brief has been updated; reload the current version.',
                payload.expected_version ?? -1,
                payload.current_version ?? -1
            );
        } catch {
            return new BriefVersionConflictError(text || 'Brief version conflict.', -1, -1);
        }
    }
    return new Error(text || res.statusText || `HTTP ${res.status}`);
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

    /** Save a user-authored draft; stale expected_version returns BriefVersionConflictError. */
    async saveBriefDraft(id: string, req: SaveBriefDraftRequest): Promise<RoundtableRoom> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/brief/draft`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw await readBriefMutationError(res);
        return (await res.json()) as RoundtableRoom;
    },

    /** Agent/referee proposal path. This endpoint cannot confirm a Brief. */
    async proposeBrief(id: string, req: ProposeBriefRequest): Promise<RoundtableRoom> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/brief/propose`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw await readBriefMutationError(res);
        return (await res.json()) as RoundtableRoom;
    },

    /** User-only confirmation of an existing current Brief version. */
    async confirmBrief(id: string, req: ConfirmBriefRequest): Promise<RoundtableRoom> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/brief/confirm`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw await readBriefMutationError(res);
        return (await res.json()) as RoundtableRoom;
    },

    /** @deprecated Compatibility/admin one-shot set+confirm path. */
    async setBriefLegacy(id: string, req: LegacySetBriefRequest): Promise<RoundtableRoom> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/brief`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
        });
        if (!res.ok) throw await readBriefMutationError(res);
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
