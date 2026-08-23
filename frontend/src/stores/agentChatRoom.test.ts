import assert from 'node:assert/strict';
import test from 'node:test';

import {
    HUMAN_PARTICIPANT_ID,
    UnknownAuthorError,
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
    type Clock,
} from './agentChatRoom';

function seqClock(): Clock {
    let n = 0;
    return {
        now: () => `2026-08-20T00:00:0${n}Z`,
        id: () => `id-${++n}`,
    };
}

test('create room includes the human plus selected agents as first-class participants', () => {
    const clock = seqClock();
    const { state, room } = createRoom(
        emptyAgentChatState(),
        '设计讨论',
        [
            humanParticipant(),
            agentParticipant('agent-codex', 'Codex', 'codex'),
            agentParticipant('agent-claude', 'Claude', 'claudecode'),
        ],
        clock
    );
    const people = listParticipants(state, room.id);
    assert.equal(people.length, 3);
    assert.equal(people.filter(p => p.kind === 'agent').length, 2);
    assert.ok(people.some(p => p.id === HUMAN_PARTICIPANT_ID && p.kind === 'human'));
    assert.ok(people.some(p => p.id === 'agent-codex' && p.kind === 'agent'));
    assert.ok(people.some(p => p.id === 'agent-claude' && p.kind === 'agent'));
    assert.equal(state.activeRoomId, room.id);
});

test('two agents and the human can post into one room with distinct authors', () => {
    const clock = seqClock();
    const res = createRoom(
        emptyAgentChatState(),
        'Everyone',
        [
            humanParticipant(),
            agentParticipant('agent-codex', 'Codex', 'codex'),
            agentParticipant('agent-claude', 'Claude', 'claudecode'),
        ],
        clock
    );
    let state = res.state;
    const { room } = res;

    const human = postMessage(state, room.id, HUMAN_PARTICIPANT_ID, '先从目标说起', clock);
    state = human.state;
    const a1 = postMessage(state, room.id, 'agent-codex', '目标应可验收', clock);
    state = a1.state;
    const a2 = postMessage(state, room.id, 'agent-claude', '我来补交互约束', clock);
    state = a2.state;

    const msgs = listMessages(state, room.id);
    assert.equal(msgs.length, 3);
    assert.equal(msgs[0].authorId, HUMAN_PARTICIPANT_ID);
    assert.equal(msgs[0].body, '先从目标说起');
    assert.equal(msgs[1].authorId, 'agent-codex');
    assert.equal(msgs[1].body, '目标应可验收');
    assert.equal(msgs[2].authorId, 'agent-claude');
    assert.equal(msgs[2].body, '我来补交互约束');
    // Second agent post did not overwrite or re-attribute the first.
    assert.equal(msgs[1].id, a1.message.id);
    assert.notEqual(msgs[1].id, msgs[2].id);
    assert.notEqual(msgs[1].authorId, msgs[2].authorId);
    assert.equal(listMessages(state, room.id)[1].body, '目标应可验收');
});

test('a non-member cannot post; adding them later keeps prior posts intact', () => {
    const clock = seqClock();
    const created = createRoom(
        emptyAgentChatState(),
        'R',
        [humanParticipant(), agentParticipant('agent-codex', 'Codex', 'codex')],
        clock
    );
    let state = created.state;
    const { room } = created;
    state = postMessage(state, room.id, 'agent-codex', 'first', clock).state;
    assert.throws(() => postMessage(state, room.id, 'agent-claude', 'nope', clock), UnknownAuthorError);

    state = addParticipant(state, room.id, agentParticipant('agent-claude', 'Claude', 'claudecode'));
    state = postMessage(state, room.id, 'agent-claude', 'second', clock).state;
    const msgs = listMessages(state, room.id);
    assert.equal(msgs.length, 2);
    assert.equal(msgs[0].authorId, 'agent-codex');
    assert.equal(msgs[0].body, 'first');
    assert.equal(msgs[1].authorId, 'agent-claude');
});

test('serialize/deserialize restores rooms, members, and attributed messages', () => {
    const clock = seqClock();
    const created = createRoom(
        emptyAgentChatState(),
        'Persist me',
        [humanParticipant(), agentParticipant('a1', 'A1', 'codex'), agentParticipant('a2', 'A2', 'claudecode')],
        clock
    );
    let state = created.state;
    const { room } = created;
    state = postMessage(state, room.id, 'a1', 'alpha', clock).state;
    state = postMessage(state, room.id, 'a2', 'beta', clock).state;
    state = selectRoom(state, room.id);

    const roundtrip = deserializeAgentChatState(serializeAgentChatState(state));
    assert.equal(roundtrip.activeRoomId, room.id);
    const msgs = listMessages(roundtrip, room.id);
    assert.equal(msgs.map(m => `${m.authorId}:${m.body}`).join('|'), 'a1:alpha|a2:beta');
    assert.equal(listParticipants(roundtrip, room.id).filter(p => p.kind === 'agent').length, 2);
});

test('seedDefaultRoom only fills an empty store', () => {
    const clock = seqClock();
    const seeded = seedDefaultRoom(
        emptyAgentChatState(),
        [
            { id: 'agent-codex', name: 'Codex', agentType: 'codex' },
            { id: 'agent-claude', name: 'Claude', agentType: 'claudecode' },
        ],
        clock
    );
    assert.equal(seeded.rooms.length, 1);
    assert.equal(seeded.rooms[0].title, 'Everyone');
    const again = seedDefaultRoom(seeded, [{ id: 'x', name: 'X' }], clock);
    assert.equal(again.rooms.length, 1);
    assert.equal(again.rooms[0].id, seeded.rooms[0].id);
});

test('seedDemoThread attributes two agent posts without overwriting the first', () => {
    const clock = seqClock();
    const seeded = seedDefaultRoom(
        emptyAgentChatState(),
        [
            { id: 'agent-codex', name: 'Codex', agentType: 'codex' },
            { id: 'agent-claude', name: 'Claude', agentType: 'claudecode' },
        ],
        clock
    );
    const withThread = seedDemoThread(seeded, clock);
    const msgs = listMessages(withThread, seeded.rooms[0].id);
    assert.equal(msgs.length, 3);
    assert.equal(msgs[0].authorId, HUMAN_PARTICIPANT_ID);
    assert.equal(msgs[1].authorId, 'agent-codex');
    assert.equal(msgs[2].authorId, 'agent-claude');
    assert.notEqual(msgs[1].body, msgs[2].body);
    const again = seedDemoThread(withThread, clock);
    assert.equal(listMessages(again, seeded.rooms[0].id).length, 3);
});
