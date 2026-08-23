import { h } from 'preact';
import { useEffect, useState } from 'preact/hooks';

import { AGENT_TYPE_LABELS, type AgentType } from '../types';
import { t, type Lang } from '../../i18n';
import { AgentAvatar } from '../chat/AgentAvatar';
import { pickableAgents } from '../../stores/agentCatalogStore';
import {
    HUMAN_PARTICIPANT_ID,
    activeChatMessages,
    activeChatParticipants,
    activeChatRoom,
    agentChatState,
    chatRooms,
    createChatRoom,
    inviteAgent,
    openChatRoom,
    postChatMessage,
    replaceAgentChatState,
    type RoomParticipant,
} from '../../stores/agentChatStore';
import { agentParticipant } from '../../stores/agentChatRoom';
import { roundtableService } from '@1agents/core/services/roundtableService';
import { ingestRoundtableRoom, seatAuthorId } from '../../stores/agentChatBridge';
import { showToast } from '../../stores/uiStore';

interface ChatModeShellProps {
    language: Lang;
}

function authorName(p: RoomParticipant | undefined, fallback: string): string {
    return p?.name || fallback;
}

export function ChatModeShell({ language }: ChatModeShellProps) {
    const rooms = chatRooms.value;
    const active = activeChatRoom.value;
    const messages = activeChatMessages.value;
    const people = activeChatParticipants.value;
    const [draft, setDraft] = useState('');
    const [busy, setBusy] = useState(false);
    const [authorId, setAuthorId] = useState(HUMAN_PARTICIPANT_ID);

    useEffect(() => {
        if (!people.some(p => p.id === authorId)) setAuthorId(HUMAN_PARTICIPANT_ID);
    }, [active?.id, people, authorId]);

    const send = async (alsoDiscuss: boolean) => {
        const text = draft.trim();
        if (!text || !active) return;
        postChatMessage(active.id, authorId, text);
        setDraft('');
        if (!alsoDiscuss) return;
        setBusy(true);
        try {
            const rt = await roundtableService.createRoom({ title: active.title });
            const chat = await roundtableService.chat(rt.id, { text });
            const detailed = chat.room ?? (await roundtableService.getRoom(rt.id));
            if (detailed) {
                const merged = ingestRoundtableRoom(agentChatState.value, {
                    ...detailed,
                    turns: chat.turns ?? detailed.turns,
                });
                replaceAgentChatState(merged);
                for (const seat of detailed.seats ?? []) {
                    inviteAgent(
                        active.id,
                        agentParticipant(seatAuthorId(seat), seat.agent_type || seat.role, seat.agent_type)
                    );
                }
            }
        } catch (err) {
            // Human post already landed. Roundtable is reuse, not required.
            showToast(t('chatmode.roundtableUnavailable', language));
            void err;
        } finally {
            setBusy(false);
        }
    };

    const addFromCatalog = (type: string, name: string) => {
        if (!active) return;
        inviteAgent(active.id, agentParticipant(`agent-${type}`, name, type));
    };

    return (
        <div class="chat-mode-shell" data-testid="chat-mode-shell">
            <aside class="chat-mode-rooms" aria-label={t('chatmode.rooms', language)}>
                <div class="chat-mode-rooms-head">
                    <h2>{t('chatmode.rooms', language)}</h2>
                    <button
                        type="button"
                        class="chat-mode-new-room"
                        onClick={() => createChatRoom(t('chatmode.newRoom', language))}
                    >
                        {t('chatmode.newRoom', language)}
                    </button>
                </div>
                <ul class="chat-mode-room-list">
                    {rooms.map(room => (
                        <li key={room.id}>
                            <button
                                type="button"
                                class={`chat-mode-room-item${room.id === active?.id ? ' active' : ''}`}
                                onClick={() => openChatRoom(room.id)}
                            >
                                <span class="chat-mode-room-title">{room.title}</span>
                                <span class="chat-mode-room-meta">{room.participantIds.length}</span>
                            </button>
                        </li>
                    ))}
                </ul>
            </aside>

            <section class="chat-mode-thread" aria-label={t('chatmode.thread', language)}>
                <header class="chat-mode-thread-head">
                    <h2>{active?.title || t('chatmode.noRoom', language)}</h2>
                </header>
                <div class="chat-mode-messages">
                    {messages.length === 0 && <div class="chat-mode-empty">{t('chatmode.emptyThread', language)}</div>}
                    {messages.map(m => {
                        const who = people.find(p => p.id === m.authorId);
                        const isHuman = m.authorId === HUMAN_PARTICIPANT_ID;
                        const agentType = (who?.agentType || 'codex') as AgentType;
                        return (
                            <article
                                key={m.id}
                                class={`chat-mode-msg${isHuman ? ' human' : ' agent'}`}
                                data-author-id={m.authorId}
                                data-author-kind={who?.kind || 'agent'}
                            >
                                {!isHuman && <AgentAvatar agentType={agentType} class="chat-mode-msg-avatar" />}
                                <div class="chat-mode-msg-body">
                                    <div class="chat-mode-msg-meta">
                                        <span class="chat-mode-msg-name">{authorName(who, m.authorId)}</span>
                                        <time>{m.createdAt.replace('T', ' ').slice(0, 16)}</time>
                                    </div>
                                    <p>{m.body}</p>
                                </div>
                            </article>
                        );
                    })}
                </div>
                <form
                    class="chat-mode-composer"
                    onSubmit={e => {
                        e.preventDefault();
                        void send(false);
                    }}
                >
                    <label class="chat-mode-author">
                        <span>{t('chatmode.speakAs', language)}</span>
                        <select
                            value={authorId}
                            aria-label={t('chatmode.speakAs', language)}
                            onChange={e => setAuthorId((e.target as HTMLSelectElement).value)}
                        >
                            {people.map(p => (
                                <option key={p.id} value={p.id}>
                                    {p.name}
                                </option>
                            ))}
                        </select>
                    </label>
                    <textarea
                        value={draft}
                        rows={2}
                        placeholder={t('chatmode.placeholder', language)}
                        onInput={e => setDraft((e.target as HTMLTextAreaElement).value)}
                        onKeyDown={e => {
                            if (e.key === 'Enter' && !e.shiftKey) {
                                e.preventDefault();
                                void send(false);
                            }
                        }}
                    />
                    <div class="chat-mode-composer-actions">
                        <button type="submit" disabled={!draft.trim() || busy}>
                            {t('chatmode.send', language)}
                        </button>
                        <button
                            type="button"
                            class="chat-mode-discuss"
                            disabled={!draft.trim() || busy}
                            onClick={() => void send(true)}
                        >
                            {t('chatmode.discuss', language)}
                        </button>
                    </div>
                </form>
            </section>

            <aside class="chat-mode-roster" aria-label={t('chatmode.roster', language)}>
                <h2>{t('chatmode.roster', language)}</h2>
                <ul>
                    {people.map(p => (
                        <li key={p.id} class="chat-mode-roster-item" data-participant-kind={p.kind}>
                            {p.kind === 'agent' ? (
                                <AgentAvatar
                                    agentType={(p.agentType || 'codex') as AgentType}
                                    class="chat-mode-roster-avatar"
                                />
                            ) : (
                                <span class="chat-mode-roster-human" aria-hidden="true">
                                    {p.name.slice(0, 1)}
                                </span>
                            )}
                            <span>
                                <strong>{p.name}</strong>
                                <em>
                                    {p.kind === 'human' ? t('chatmode.kindHuman', language) : p.agentType || 'agent'}
                                </em>
                            </span>
                            <button
                                type="button"
                                class={`chat-mode-speak${authorId === p.id ? ' active' : ''}`}
                                onClick={() => setAuthorId(p.id)}
                            >
                                {t('chatmode.speakAs', language)}
                            </button>
                        </li>
                    ))}
                </ul>
                <div class="chat-mode-invite">
                    <div class="chat-mode-invite-label">{t('chatmode.invite', language)}</div>
                    {pickableAgents.value.slice(0, 8).map(a => (
                        <button
                            type="button"
                            key={a.type}
                            class="chat-mode-invite-btn"
                            onClick={() => addFromCatalog(a.type, AGENT_TYPE_LABELS[a.type as AgentType] || a.type)}
                        >
                            {AGENT_TYPE_LABELS[a.type as AgentType] || a.type}
                        </button>
                    ))}
                </div>
            </aside>
        </div>
    );
}
