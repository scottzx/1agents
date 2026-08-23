/**
 * Shared-room model for 聊天 mode: one thread, many first-class authors.
 *
 * Pure reducers — no DOM, no network. Posts append; a second agent's
 * message never overwrites or re-attributes the first.
 */

export const HUMAN_PARTICIPANT_ID = 'human';

export type ParticipantKind = 'human' | 'agent';

export interface RoomParticipant {
    id: string;
    kind: ParticipantKind;
    name: string;
    /** Engine / catalog type when kind === 'agent' (codex, claudecode, …). */
    agentType?: string;
}

export interface RoomMessage {
    id: string;
    roomId: string;
    authorId: string;
    body: string;
    createdAt: string;
}

export interface ChatRoom {
    id: string;
    title: string;
    participantIds: string[];
    createdAt: string;
    updatedAt: string;
    /** Optional link to an existing roundtable room (reuse, not a second protocol). */
    roundtableRoomId?: string;
}

export interface AgentChatState {
    rooms: ChatRoom[];
    participants: Record<string, RoomParticipant>;
    messages: Record<string, RoomMessage[]>;
    activeRoomId: string | null;
}

export interface Clock {
    now(): string;
    id(): string;
}

const defaultClock = (): Clock => {
    let n = 0;
    return {
        now: () => new Date().toISOString(),
        id: () => `acr-${Date.now().toString(36)}-${(++n).toString(36)}`,
    };
};

export function emptyAgentChatState(): AgentChatState {
    return { rooms: [], participants: {}, messages: {}, activeRoomId: null };
}

export function humanParticipant(name = '我'): RoomParticipant {
    return { id: HUMAN_PARTICIPANT_ID, kind: 'human', name };
}

export function agentParticipant(id: string, name: string, agentType?: string): RoomParticipant {
    return { id, kind: 'agent', name, agentType };
}

function ensureParticipant(state: AgentChatState, p: RoomParticipant): AgentChatState {
    if (state.participants[p.id]) {
        return {
            ...state,
            participants: { ...state.participants, [p.id]: { ...state.participants[p.id], ...p } },
        };
    }
    return { ...state, participants: { ...state.participants, [p.id]: p } };
}

export function createRoom(
    state: AgentChatState,
    title: string,
    members: RoomParticipant[],
    clock: Clock = defaultClock(),
    roundtableRoomId?: string
): { state: AgentChatState; room: ChatRoom } {
    let next = state;
    const ids: string[] = [];
    for (const p of members) {
        next = ensureParticipant(next, p);
        if (!ids.includes(p.id)) ids.push(p.id);
    }
    if (!ids.includes(HUMAN_PARTICIPANT_ID)) {
        next = ensureParticipant(next, humanParticipant());
        ids.unshift(HUMAN_PARTICIPANT_ID);
    }
    const ts = clock.now();
    const room: ChatRoom = {
        id: clock.id(),
        title: title.trim() || '未命名房间',
        participantIds: ids,
        createdAt: ts,
        updatedAt: ts,
        roundtableRoomId,
    };
    next = {
        ...next,
        rooms: [room, ...next.rooms],
        messages: { ...next.messages, [room.id]: [] },
        activeRoomId: room.id,
    };
    return { state: next, room };
}

export function addParticipant(state: AgentChatState, roomId: string, participant: RoomParticipant): AgentChatState {
    const room = state.rooms.find(r => r.id === roomId);
    if (!room) return state;
    const next = ensureParticipant(state, participant);
    if (room.participantIds.includes(participant.id)) return next;
    return {
        ...next,
        rooms: next.rooms.map(r =>
            r.id === roomId ? { ...r, participantIds: [...r.participantIds, participant.id] } : r
        ),
    };
}

export function selectRoom(state: AgentChatState, roomId: string | null): AgentChatState {
    if (roomId && !state.rooms.some(r => r.id === roomId)) return state;
    return { ...state, activeRoomId: roomId };
}

export function listParticipants(state: AgentChatState, roomId: string): RoomParticipant[] {
    const room = state.rooms.find(r => r.id === roomId);
    if (!room) return [];
    return room.participantIds.map(id => state.participants[id]).filter((p): p is RoomParticipant => !!p);
}

export function listMessages(state: AgentChatState, roomId: string): RoomMessage[] {
    return state.messages[roomId] ? state.messages[roomId].slice() : [];
}

export class UnknownRoomError extends Error {
    constructor(roomId: string) {
        super(`unknown room: ${roomId}`);
        this.name = 'UnknownRoomError';
    }
}

export class UnknownAuthorError extends Error {
    constructor(authorId: string, roomId: string) {
        super(`author ${authorId} is not a participant of ${roomId}`);
        this.name = 'UnknownAuthorError';
    }
}

/**
 * Append a message from `authorId`. Previous messages are copied, never
 * mutated — a second agent post cannot overwrite or re-attribute the first.
 */
export function postMessage(
    state: AgentChatState,
    roomId: string,
    authorId: string,
    body: string,
    clock: Clock = defaultClock()
): { state: AgentChatState; message: RoomMessage } {
    const room = state.rooms.find(r => r.id === roomId);
    if (!room) throw new UnknownRoomError(roomId);
    if (!room.participantIds.includes(authorId)) throw new UnknownAuthorError(authorId, roomId);
    const text = body.trim();
    if (!text) throw new Error('empty message');
    const prev = state.messages[roomId] ?? [];
    const message: RoomMessage = {
        id: clock.id(),
        roomId,
        authorId,
        body: text,
        createdAt: clock.now(),
    };
    const nextMessages = [...prev, message];
    return {
        state: {
            ...state,
            messages: { ...state.messages, [roomId]: nextMessages },
            rooms: state.rooms.map(r => (r.id === roomId ? { ...r, updatedAt: message.createdAt } : r)),
            activeRoomId: roomId,
        },
        message,
    };
}

export function seedDefaultRoom(
    state: AgentChatState,
    agents: Array<{ id: string; name: string; agentType?: string }>,
    clock: Clock = defaultClock()
): AgentChatState {
    if (state.rooms.length > 0) return state;
    const members: RoomParticipant[] = [
        humanParticipant(),
        ...agents.map(a => agentParticipant(a.id, a.name, a.agentType)),
    ];
    return createRoom(state, 'Everyone', members, clock).state;
}

/** First-run thread so 聊天 opens on a multi-author conversation, not a blank pane. */
export function seedDemoThread(state: AgentChatState, clock: Clock = defaultClock()): AgentChatState {
    const roomId = state.activeRoomId || state.rooms[0]?.id;
    if (!roomId) return state;
    if ((state.messages[roomId] ?? []).length > 0) return state;
    const agents = listParticipants(state, roomId).filter(p => p.kind === 'agent');
    if (agents.length < 2) return state;
    let next = state;
    next = postMessage(next, roomId, HUMAN_PARTICIPANT_ID, '请两位一起过一下方案边界。', clock).state;
    next = postMessage(next, roomId, agents[0].id, '我先列可验收的目标。', clock).state;
    next = postMessage(next, roomId, agents[1].id, '我补交互与约束。', clock).state;
    return next;
}

export function serializeAgentChatState(state: AgentChatState): string {
    return JSON.stringify(state);
}

export function deserializeAgentChatState(raw: string | null): AgentChatState {
    if (!raw) return emptyAgentChatState();
    try {
        const parsed = JSON.parse(raw) as Partial<AgentChatState>;
        if (!parsed || typeof parsed !== 'object') return emptyAgentChatState();
        const rooms = Array.isArray(parsed.rooms) ? parsed.rooms : [];
        const participants = parsed.participants && typeof parsed.participants === 'object' ? parsed.participants : {};
        const messages = parsed.messages && typeof parsed.messages === 'object' ? parsed.messages : {};
        const activeRoomId =
            typeof parsed.activeRoomId === 'string' && rooms.some(r => r.id === parsed.activeRoomId)
                ? parsed.activeRoomId
                : rooms[0]?.id ?? null;
        return { rooms, participants, messages, activeRoomId };
    } catch {
        return emptyAgentChatState();
    }
}
