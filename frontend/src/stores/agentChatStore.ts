import { computed, signal } from '@preact/signals';

import { AGENT_TYPE_LABELS, type AgentType } from '../components/types';
import { pickableAgents } from './agentCatalogStore';
import { rememberChatRoom } from './chromeModeStore';
import {
    HUMAN_PARTICIPANT_ID,
    addParticipant,
    agentParticipant,
    createRoom,
    deserializeAgentChatState,
    emptyAgentChatState,
    humanParticipant,
    listMessages,
    listParticipants,
    postMessage,
    seedDefaultRoom,
    seedDemoThread,
    selectRoom,
    serializeAgentChatState,
    type AgentChatState,
    type RoomMessage,
    type RoomParticipant,
} from './agentChatRoom';

export const AGENT_CHAT_STORAGE_KEY = '1agents-agent-chat-state';

function browserStorage(): Storage | null {
    try {
        return typeof localStorage === 'undefined' ? null : localStorage;
    } catch {
        return null;
    }
}

function loadState(): AgentChatState {
    const storage = browserStorage();
    const loaded = storage ? deserializeAgentChatState(storage.getItem(AGENT_CHAT_STORAGE_KEY)) : emptyAgentChatState();
    return ensureSeeded(loaded);
}

function fallbackAgents(): Array<{ id: string; name: string; agentType?: string }> {
    const fromCatalog = pickableAgents.value.slice(0, 2).map(a => ({
        id: `agent-${a.type}`,
        name: AGENT_TYPE_LABELS[a.type as AgentType] || a.type,
        agentType: a.type,
    }));
    if (fromCatalog.length >= 2) return fromCatalog;
    const defaults = [
        { id: 'agent-codex', name: 'Codex', agentType: 'codex' },
        { id: 'agent-claudecode', name: 'Claude Code', agentType: 'claudecode' },
    ];
    const seen = new Set(fromCatalog.map(a => a.agentType as string));
    return [...fromCatalog, ...defaults.filter(d => !seen.has(d.agentType))].slice(0, 2);
}

function ensureSeeded(state: AgentChatState): AgentChatState {
    return seedDemoThread(seedDefaultRoom(state, fallbackAgents()));
}

const initial = loadState();
export const agentChatState = signal<AgentChatState>(initial);

export function replaceAgentChatState(next: AgentChatState): AgentChatState {
    return commit(next);
}

function commit(next: AgentChatState): AgentChatState {
    agentChatState.value = next;
    const storage = browserStorage();
    if (storage) {
        try {
            storage.setItem(AGENT_CHAT_STORAGE_KEY, serializeAgentChatState(next));
        } catch {
            /* quota — in-memory still works */
        }
    }
    rememberChatRoom(next.activeRoomId);
    return next;
}

export const chatRooms = computed(() => agentChatState.value.rooms);
export const activeChatRoomId = computed(() => agentChatState.value.activeRoomId);
export const activeChatRoom = computed(
    () => agentChatState.value.rooms.find(r => r.id === agentChatState.value.activeRoomId) ?? null
);
export const activeChatMessages = computed<RoomMessage[]>(() => {
    const id = agentChatState.value.activeRoomId;
    return id ? listMessages(agentChatState.value, id) : [];
});
export const activeChatParticipants = computed<RoomParticipant[]>(() => {
    const id = agentChatState.value.activeRoomId;
    return id ? listParticipants(agentChatState.value, id) : [];
});

export function openChatRoom(roomId: string): void {
    commit(selectRoom(agentChatState.value, roomId));
}

export function createChatRoom(title: string, extraAgents: RoomParticipant[] = []): string {
    const members = [
        humanParticipant(),
        ...fallbackAgents().map(a => agentParticipant(a.id, a.name, a.agentType)),
        ...extraAgents,
    ];
    const { state, room } = createRoom(agentChatState.value, title, members);
    commit(state);
    return room.id;
}

export function inviteAgent(roomId: string, participant: RoomParticipant): void {
    commit(addParticipant(agentChatState.value, roomId, participant));
}

export function postChatMessage(roomId: string, authorId: string, body: string): RoomMessage {
    const { state, message } = postMessage(agentChatState.value, roomId, authorId, body);
    commit(state);
    return message;
}

export function postHumanMessage(body: string): RoomMessage | null {
    const roomId = agentChatState.value.activeRoomId;
    if (!roomId) return null;
    return postChatMessage(roomId, HUMAN_PARTICIPANT_ID, body);
}

export function postAgentMessage(authorId: string, body: string): RoomMessage | null {
    const roomId = agentChatState.value.activeRoomId;
    if (!roomId) return null;
    return postChatMessage(roomId, authorId, body);
}

export { HUMAN_PARTICIPANT_ID };
export type { RoomMessage, RoomParticipant };
