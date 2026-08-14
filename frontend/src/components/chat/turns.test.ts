// Turn grouping contract for MessageList.
// Run: cd frontend && npx tsx --test src/components/chat/turns.test.ts

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { GroupedChatItem, TurnContentItem } from './MessageBubble';
import { groupHistoricalTurns } from './turns';

const user = (id: string, queueStatus?: 'queued'): TurnContentItem => ({
    id,
    kind: 'user',
    content: id,
    createdAt: 1,
    ...(queueStatus ? { queueStatus } : {}),
});

const answer = (id: string): TurnContentItem => ({
    id,
    kind: 'assistant_text',
    content: id,
    createdAt: 2,
    streaming: false,
});

const thinking = (id: string): TurnContentItem => ({
    id,
    kind: 'thinking',
    content: id,
    createdAt: 2,
});

test('groups the second-latest and older turns but leaves the newest turn flat', () => {
    const items: GroupedChatItem[] = [
        user('u1'),
        thinking('think-1'),
        answer('a1'),
        user('u2'),
        thinking('think-2'),
        answer('a2'),
        user('u3'),
        thinking('think-3'),
        answer('a3'),
    ];

    const grouped = groupHistoricalTurns(items);

    assert.deepEqual(
        grouped.map(item => [item.kind, item.id]),
        [
            ['user', 'u1'],
            ['turn', 'turn-u1'],
            ['user', 'u2'],
            ['turn', 'turn-u2'],
            ['user', 'u3'],
            ['thinking', 'think-3'],
            ['assistant_text', 'a3'],
        ]
    );
    assert.equal(grouped[1].kind === 'turn' ? grouped[1].outcomeId : undefined, 'a1');
    assert.equal(grouped[3].kind === 'turn' ? grouped[3].outcomeId : undefined, 'a2');
});

test('keeps only the last assistant block as the collapsed turn outcome', () => {
    const grouped = groupHistoricalTurns([
        user('u1'),
        answer('progress'),
        thinking('think'),
        answer('final'),
        user('u2'),
    ]);

    const turn = grouped.find(item => item.kind === 'turn');
    assert.ok(turn && turn.kind === 'turn');
    assert.equal(turn.outcomeId, 'final');
    assert.deepEqual(
        turn.items.map(item => item.id),
        ['progress', 'think', 'final']
    );
});

test('does not treat queued prompts as turn boundaries', () => {
    const items: GroupedChatItem[] = [user('active'), thinking('live'), user('queued', 'queued')];
    assert.equal(groupHistoricalTurns(items), items);
});

test('uses a terminal error as the visible outcome when no final answer exists', () => {
    const error: TurnContentItem = { id: 'failed', kind: 'error', content: 'failed', createdAt: 3 };
    const grouped = groupHistoricalTurns([user('u1'), thinking('think'), error, user('u2')]);
    const turn = grouped[1];

    assert.equal(turn.kind, 'turn');
    assert.equal(turn.kind === 'turn' ? turn.outcomeId : undefined, 'failed');
});

test('does not add a redundant wrapper when a historical turn only has its answer', () => {
    const grouped = groupHistoricalTurns([user('u1'), answer('a1'), user('u2')]);
    assert.deepEqual(
        grouped.map(item => item.kind),
        ['user', 'assistant_text', 'user']
    );
});

test('moves changeReport onto the folded turn and keeps it on the latest user', () => {
    const report = {
        turnId: 'turn-1',
        recipeVersion: 1,
        addedCount: 1,
        deletedCount: 0,
        modifiedCount: 0,
        files: [{ path: 'a.ts', op: 'added' as const }],
        source: 'live' as const,
        computedAt: '2026-08-13T00:00:00Z',
    };
    const firstUser = { ...user('u1'), turnId: 'turn-1', changeReport: report };
    const latestUser = { ...user('u2'), turnId: 'turn-2', changeReport: { ...report, turnId: 'turn-2' } };
    const grouped = groupHistoricalTurns([firstUser, thinking('think'), answer('a1'), latestUser, answer('a2')]);

    const foldedUser = grouped[0];
    const foldedTurn = grouped[1];
    assert.equal(foldedUser.kind, 'user');
    assert.equal(foldedUser.kind === 'user' ? foldedUser.changeReport : 'missing', undefined);
    assert.equal(foldedTurn.kind, 'turn');
    assert.equal(foldedTurn.kind === 'turn' ? foldedTurn.changeReport?.addedCount : 0, 1);

    const latest = grouped.find(item => item.kind === 'user' && item.id === 'u2');
    assert.equal(latest?.kind === 'user' ? latest.changeReport?.turnId : undefined, 'turn-2');
});

test('uses the persisted turn id and status when explicit attribution is available', () => {
    const firstUser = { ...user('u1'), turnId: 'turn-123', turnStatus: 'failed' as const };
    const receipt: TurnContentItem = {
        id: 'receipt',
        kind: 'turn_receipt',
        content: 'Turn 执行失败',
        count: 0,
        status: 'failed',
        createdAt: 3,
        turnId: 'turn-123',
        turnStatus: 'failed',
    };
    const grouped = groupHistoricalTurns([firstUser, thinking('think'), receipt, user('u2')]);
    const turn = grouped[1];

    assert.equal(turn.kind, 'turn');
    assert.equal(turn.id, 'turn-turn-123');
    assert.equal(turn.kind === 'turn' ? turn.turnStatus : undefined, 'failed');
    assert.equal(turn.kind === 'turn' ? turn.outcomeId : undefined, 'receipt');
});
