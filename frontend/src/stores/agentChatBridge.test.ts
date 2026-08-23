import assert from 'node:assert/strict';
import test from 'node:test';

import type { RoundtableRoom, RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';

import { ingestRoundtableRoom, seatAuthorId, turnsFromChatResponse } from './agentChatBridge';
import { emptyAgentChatState, listMessages, listParticipants } from './agentChatRoom';

function seat(id: string, role: RoundtableSeat['role'], agentType: string): RoundtableSeat {
    return {
        id,
        room_id: 'rt-1',
        role,
        agent_type: agentType,
        workspace_id: 'ws',
        status: 'done',
        created_at: '2026-08-20T00:00:00Z',
    };
}

test('ingestRoundtableRoom maps two seats into distinct attributed posts', () => {
    const seats = [seat('s-codex', 'eng', 'codex'), seat('s-claude', 'product', 'claudecode')];
    const turns: RoundtableTurn[] = [
        {
            id: 't-user',
            room_id: 'rt-1',
            round: 1,
            kind: 'chat',
            content_text: '请一起看这个需求',
            created_at: '2026-08-20T00:00:01Z',
        },
        {
            id: 't-codex',
            room_id: 'rt-1',
            round: 1,
            seat_id: 's-codex',
            kind: 'speech',
            content_text: '我先列技术约束',
            created_at: '2026-08-20T00:00:02Z',
        },
        {
            id: 't-claude',
            room_id: 'rt-1',
            round: 1,
            seat_id: 's-claude',
            kind: 'speech',
            content_text: '产品侧要可验收',
            created_at: '2026-08-20T00:00:03Z',
        },
    ];
    const rt: RoundtableRoom = {
        id: 'rt-1',
        title: '需求讨论',
        state: 'drafting_brief',
        created_at: '2026-08-20T00:00:00Z',
        updated_at: '2026-08-20T00:00:03Z',
        phase: 'r1',
        phase_status: 'ready',
        next_action: 'none',
        progress: { completed: 2, total: 2, active_roles: ['eng', 'product'], failed_roles: [] },
        seats,
        turns,
    };

    const state = ingestRoundtableRoom(emptyAgentChatState(), rt);
    const room = state.rooms[0];
    assert.ok(room);
    assert.equal(room.roundtableRoomId, 'rt-1');
    const agents = listParticipants(state, room.id).filter(p => p.kind === 'agent');
    assert.equal(agents.length, 2);
    const msgs = listMessages(state, room.id);
    assert.equal(msgs.length, 3);
    assert.equal(msgs[1].authorId, seatAuthorId(seats[0]));
    assert.equal(msgs[1].body, '我先列技术约束');
    assert.equal(msgs[2].authorId, seatAuthorId(seats[1]));
    assert.equal(msgs[2].body, '产品侧要可验收');
    assert.notEqual(msgs[1].authorId, msgs[2].authorId);

    const again = ingestRoundtableRoom(state, rt);
    assert.equal(listMessages(again, room.id).length, 3);
});

test('turnsFromChatResponse keeps seat authors and drops empty bodies', () => {
    const seats = [seat('s1', 'eng', 'codex')];
    const mapped = turnsFromChatResponse(
        [
            { id: 'a', room_id: 'r', round: 1, kind: 'chat', content_text: '  ', created_at: '' },
            { id: 'b', room_id: 'r', round: 1, seat_id: 's1', kind: 'speech', content_text: 'hello', created_at: '' },
        ],
        seats
    );
    assert.equal(mapped.length, 1);
    assert.equal(mapped[0].authorId, seatAuthorId(seats[0]));
    assert.equal(mapped[0].body, 'hello');
});
