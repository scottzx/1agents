import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { ChatItem } from './hooks';
import type { AgentTurn, ProjectActivityEntry } from '@1agents/core/services/activityService';
import { projectChatTurns } from './projectTurnProjection';

const turn = (id: string, promptText: string, createdAt: string): AgentTurn => ({
    id,
    projectId: 'project-1',
    sessionId: 'session-1',
    status: 'completed',
    promptText,
    createdAt,
    updatedAt: createdAt,
});

const user = (id: string, content: string): ChatItem => ({
    id,
    kind: 'user',
    content,
    createdAt: 1,
});

const answer = (id: string): ChatItem => ({
    id,
    kind: 'assistant_text',
    content: id,
    createdAt: 2,
    streaming: false,
});

test('matches persisted turns by prompt before chronological fallback', () => {
    const projected = projectChatTurns(
        [user('u-new', 'second'), answer('a-new'), user('u-old', 'first'), answer('a-old')],
        [turn('turn-first', 'first', '2026-01-01T00:00:00Z'), turn('turn-second', 'second', '2026-01-02T00:00:00Z')],
        []
    );

    assert.deepEqual(
        projected.filter(item => item.kind === 'user').map(item => item.turnId),
        ['turn-second', 'turn-first']
    );
});

test('adds one accurate system receipt for a three-task mutation batch', () => {
    const activity: ProjectActivityEntry = {
        id: 'turn:turn-1',
        projectId: 'project-1',
        groupKind: 'turn',
        turnId: 'turn-1',
        sessionId: 'session-1',
        actorKind: 'agent',
        origin: 'mcp',
        status: 'succeeded',
        summary: '创建 3 个 Tasks',
        count: 3,
        eventIds: ['event-1', 'event-2', 'event-3'],
        targets: [
            { type: 'project_item', id: 'task-1', operation: 'create' },
            { type: 'project_item', id: 'task-2', operation: 'create' },
            { type: 'project_item', id: 'task-3', operation: 'create' },
        ],
        createdAt: '2026-01-01T00:00:01Z',
    };

    const projected = projectChatTurns(
        [user('u1', 'create tasks'), answer('a1')],
        [turn('turn-1', 'create tasks', '2026-01-01T00:00:00Z')],
        [activity]
    );
    const receipts = projected.filter(item => item.kind === 'turn_receipt');

    assert.equal(receipts.length, 1);
    assert.equal(receipts[0].content, '创建 3 个 Tasks');
    assert.equal(receipts[0].turnId, 'turn-1');
});

test('keeps legacy chat items untouched when no persisted turns exist', () => {
    const items = [user('u1', 'legacy'), answer('a1')];
    assert.equal(projectChatTurns(items, [], []), items);
});
