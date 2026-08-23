/**
 * Map existing roundtable turns into the shared-room thread so 聊天 mode
 * reuses 1ACP/roundtable I/O instead of inventing a second protocol.
 */
import type { RoundtableRoom, RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';

import {
    HUMAN_PARTICIPANT_ID,
    addParticipant,
    agentParticipant,
    createRoom,
    humanParticipant,
    listMessages,
    postMessage,
    type AgentChatState,
} from './agentChatRoom';

export function seatAuthorId(seat: RoundtableSeat): string {
    return `agent-${seat.id}`;
}

function seatName(seat: RoundtableSeat): string {
    return seat.agent_type ? `${seat.role} · ${seat.agent_type}` : seat.role;
}

/**
 * Open or refresh a chat room backed by a roundtable room. Seats become
 * agent participants; turns become attributed messages (append-only).
 */
export function ingestRoundtableRoom(state: AgentChatState, rt: RoundtableRoom): AgentChatState {
    const seats = rt.seats ?? [];
    const turns = rt.turns ?? [];
    let next = state;
    let room = next.rooms.find(r => r.roundtableRoomId === rt.id);
    if (!room) {
        const members = [
            humanParticipant(),
            ...seats.map(s => agentParticipant(seatAuthorId(s), seatName(s), s.agent_type)),
        ];
        const created = createRoom(next, rt.title || '圆桌', members, undefined, rt.id);
        next = created.state;
        room = created.room;
    } else {
        for (const s of seats) {
            next = addParticipant(next, room.id, agentParticipant(seatAuthorId(s), seatName(s), s.agent_type));
        }
    }

    const existing = new Set(listMessages(next, room.id).map(m => m.id));
    const authorBySeat = new Map(seats.map(s => [s.id, seatAuthorId(s)]));

    for (const turn of turns) {
        if (existing.has(turn.id)) continue;
        const authorId = turn.seat_id ? authorBySeat.get(turn.seat_id) ?? HUMAN_PARTICIPANT_ID : HUMAN_PARTICIPANT_ID;
        const body = (turn.content_text || '').trim();
        if (!body) continue;
        try {
            const posted = postMessage(next, room.id, authorId, body, {
                now: () => turn.created_at || new Date().toISOString(),
                id: () => turn.id,
            });
            next = posted.state;
        } catch {
            // Skip turns whose author is not (yet) a participant.
        }
    }
    return next;
}

export function turnsFromChatResponse(
    turns: RoundtableTurn[] | undefined,
    seats: RoundtableSeat[]
): Array<{
    authorId: string;
    body: string;
    id: string;
    createdAt: string;
}> {
    const authorBySeat = new Map(seats.map(s => [s.id, seatAuthorId(s)]));
    return (turns ?? [])
        .map(turn => ({
            authorId: turn.seat_id ? authorBySeat.get(turn.seat_id) ?? HUMAN_PARTICIPANT_ID : HUMAN_PARTICIPANT_ID,
            body: (turn.content_text || '').trim(),
            id: turn.id,
            createdAt: turn.created_at || new Date().toISOString(),
        }))
        .filter(t => t.body);
}
