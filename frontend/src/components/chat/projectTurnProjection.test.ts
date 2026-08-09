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

test('uses explicit history turnId for repeated prompts without text matching', () => {
    const first = turn('turn-first', 'same', '2026-01-01T00:00:00Z');
    const second = turn('turn-second', 'same', '2026-01-02T00:00:00Z');
    const projected = projectChatTurns(
        [
            { ...user('u2', 'same'), turnId: 'turn-second' },
            { ...answer('a2'), turnId: 'turn-second' },
            { ...user('u1', 'same'), turnId: 'turn-first' },
            { ...answer('a1'), turnId: 'turn-first' },
        ],
        [first, second],
        []
    );

    assert.deepEqual(
        projected.filter(item => item.kind === 'user').map(item => item.turnId),
        ['turn-second', 'turn-first']
    );
});

test('surfaces failed and cancelled terminal states even without an assistant answer', () => {
    const failed = {
        ...turn('turn-failed', 'run', '2026-01-01T00:00:00Z'),
        status: 'failed' as const,
        errorText: '权限不足',
    };
    const projected = projectChatTurns([user('u1', 'run')], [failed], []);
    const receipt = projected.find(item => item.kind === 'turn_receipt');

    assert.ok(receipt && receipt.kind === 'turn_receipt');
    assert.equal(receipt.status, 'failed');
    assert.equal(receipt.content, '权限不足');
});

test('matches history turnIds that carry the runtime request id, not the canonical id', () => {
    // The bridge's history items carry the RUNTIME request id (the turn's
    // clientRequestId) as turnId, while /api/agent/turns returns the canonical
    // Turn id. The receipt must follow the failed turn's segment — between its
    // own content and the next user prompt — not drift to the timeline end.
    const failedTurn = {
        ...turn('turn-failed', 'merge branch', '2026-01-01T00:00:00Z'),
        clientRequestId: 'runtime-1',
        runtimeRequestId: 'runtime-1',
        status: 'failed' as const,
        errorCode: 'runtime_restarted',
        errorText: '1ACP restarted before the Turn reached a durable terminal state.',
        completedAt: '2026-01-01T00:00:10Z',
    };
    const nextTurn = {
        ...turn('turn-next', '继续', '2026-01-01T00:00:20Z'),
        clientRequestId: 'runtime-2',
        runtimeRequestId: 'runtime-2',
    };
    const items: ChatItem[] = [
        { ...user('u1', 'merge branch'), turnId: 'runtime-1' },
        { ...answer('a1'), turnId: 'runtime-1' },
        { ...user('u2', '继续'), turnId: 'runtime-2' },
        { ...answer('a2'), turnId: 'runtime-2' },
    ];

    const projected = projectChatTurns(items, [nextTurn, failedTurn], []);
    const kinds = projected.map(item => item.kind);
    const receiptIndex = projected.findIndex(item => item.kind === 'turn_receipt');

    assert.deepEqual(kinds, ['user', 'assistant_text', 'turn_receipt', 'user', 'assistant_text']);
    assert.equal(receiptIndex, 2, 'receipt lands inside the failed turn segment, not at the end');
    const receipt = projected[receiptIndex];
    assert.ok(receipt && receipt.kind === 'turn_receipt');
    assert.equal(receipt.turnId, 'turn-failed');
    assert.equal(receipt.content, '1ACP restarted before the Turn reached a durable terminal state.');
    assert.equal(projected[3].kind, 'user');
    assert.equal(projected[3].turnId, 'turn-next');
});
