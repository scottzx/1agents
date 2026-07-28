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
export type SeatStatus = 'ready' | 'speaking' | 'done' | 'failed' | 'skipped';

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

export type RoundRunStatus =
    | 'queued'
    | 'running'
    | 'summarizing'
    | 'completed'
    | 'partial_failed'
    | 'failed'
    | 'canceled';

export type RoundtablePhase = 'r1' | 'r2' | 'r3' | 'done' | 'failed';
export type RoundRunErrorScope = 'room' | 'seat' | 'summary';
export type RoundRecoveryAction =
    | 'confirm_brief'
    | 'start_r2'
    | 'start_r3'
    | 'retry_failed_seats'
    | 'skip_and_summarize'
    | 'retry_summary'
    | 'reload_room';

export interface RoundRun {
    id: string;
    room_id: string;
    round: 2 | 3;
    status: RoundRunStatus;
    idempotency_key: string;
    created_at: string;
    updated_at: string;
    started_at?: string;
    finished_at?: string;
    error?: string;
    error_scope?: RoundRunErrorScope;
}

export interface RoundProgress {
    completed: number;
    total: number;
    active_roles: SeatRole[];
    failed_roles: SeatRole[];
    skipped_roles?: SeatRole[];
}

export interface RoundEvent {
    seq: number;
    room_id: string;
    run_id: string;
    round: 2 | 3;
    kind: 'run' | 'seat' | 'summary';
    status: string;
    role?: SeatRole;
    error?: string;
    created_at: string;
}

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
    phase: RoundtablePhase;
    phase_status: RoundRunStatus | 'ready';
    next_action:
        | 'confirm_brief'
        | 'start_r2'
        | 'start_r3'
        | 'wait'
        | 'retry_failed_seats'
        | 'skip_and_summarize'
        | 'retry_summary'
        | 'reload_room'
        | 'inspect_failure'
        | 'none';
    available_actions?: RoundRecoveryAction[];
    progress: RoundProgress;
    active_run?: RoundRun;
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

export interface StartRoundResponse {
    run_id: string;
    run: RoundRun;
    room: RoundtableRoom;
    reused: boolean;
}

export type RecoverRoundResponse = StartRoundResponse;

export interface RoundEventPage {
    events: RoundEvent[];
    last_seq: number;
}

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

    /** POST /api/roundtable/rooms/{id}/r2 — 202, execution continues asynchronously. */
    async runR2(id: string, idempotencyKey?: string): Promise<StartRoundResponse> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/r2`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ idempotency_key: idempotencyKey || '' }),
        });
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as StartRoundResponse;
    },

    /** POST /api/roundtable/rooms/{id}/r3 — 202, execution continues asynchronously. */
    async runR3(id: string, idempotencyKey?: string): Promise<StartRoundResponse> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/r3`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ idempotency_key: idempotencyKey || '' }),
        });
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as StartRoundResponse;
    },

    /** Retry exactly one failed panelist while preserving the same RoundRun. */
    async retrySeat(id: string, runId: string, role: SeatRole): Promise<RecoverRoundResponse> {
        const res = await apiFetch(
            `/roundtable/rooms/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/seats/${encodeURIComponent(
                role
            )}/retry`,
            { method: 'POST' }
        );
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as RecoverRoundResponse;
    },

    /** Mark every currently failed seat absent and continue with only the summary. */
    async skipFailedSeats(id: string, runId: string): Promise<RecoverRoundResponse> {
        const res = await apiFetch(
            `/roundtable/rooms/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/skip`,
            { method: 'POST' }
        );
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as RecoverRoundResponse;
    },

    /** Retry only the failed referee summary; panelist execution gates stay closed. */
    async retrySummary(id: string, runId: string): Promise<RecoverRoundResponse> {
        const res = await apiFetch(
            `/roundtable/rooms/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/summary/retry`,
            { method: 'POST' }
        );
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as RecoverRoundResponse;
    },

    /**
     * GET /events?after=<seq> — durable reconnect cursor. Call again with
     * last_seq after refresh/network recovery; no event at or before it repeats.
     */
    async listEvents(id: string, after = 0, limit = 200): Promise<RoundEventPage> {
        const query = new URLSearchParams({
            after: String(Math.max(0, after)),
            limit: String(Math.max(1, limit)),
        });
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/events?${query.toString()}`, {
            cache: 'no-store',
        });
        if (!res.ok) throw new Error(await readError(res));
        return (await res.json()) as RoundEventPage;
    },

    /** @deprecated Explicit synchronous compatibility path for pre-RoundRun clients. */
    async runR2Legacy(id: string): Promise<unknown> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/r2?wait=1`, {
            method: 'POST',
        });
        if (!res.ok) throw new Error(await readError(res));
        return res.json();
    },

    /** @deprecated Explicit synchronous compatibility path for pre-RoundRun clients. */
    async runR3Legacy(id: string): Promise<unknown> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}/r3?wait=1`, {
            method: 'POST',
        });
        if (!res.ok) throw new Error(await readError(res));
        return res.json();
    },

    /** DELETE /api/roundtable/rooms/{id} — permanent delete of room record and all seat content. */
    async deleteRoom(id: string): Promise<void> {
        const res = await apiFetch(`/roundtable/rooms/${encodeURIComponent(id)}`, {
            method: 'DELETE',
        });
        if (!res.ok) throw new Error(await readError(res));
    },
};
