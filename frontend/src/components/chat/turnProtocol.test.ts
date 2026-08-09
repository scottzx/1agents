import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
    applyTextDelta,
    applyTurnState,
    normalizeHistory,
    selectOptimisticUsers,
} from '@1agents/core/protocol/reducer';
import { cancelQueuedAction, cancelTurnAction, promptAction } from '@1agents/core/protocol/wireProtocol';
import type { ChatItem } from '@1agents/core/protocol/types';

test('prompt submission keeps client request identity separate from Turn identity', () => {
    assert.deepEqual(promptAction('session-1', 'client-request-1', 'same prompt'), {
        action: 'prompt',
        sessionId: 'session-1',
        requestId: 'client-request-1',
        text: 'same prompt',
    });

    const optimistic: ChatItem[] = [
        {
            id: 'client-request-1',
            kind: 'user',
            content: 'same prompt',
            createdAt: 1,
            clientRequestId: 'client-request-1',
        },
    ];
    const queued = applyTurnState(optimistic, {
        turnId: 'turn-1',
        requestId: 'client-request-1',
        status: 'queued',
        queuePosition: 2,
    });

    assert.equal(queued[0].turnId, 'turn-1');
    assert.equal(queued[0].turnStatus, 'queued');
    assert.equal(queued[0].queuePosition, 2);
    assert.equal(queued[0].kind === 'user' && queued[0].queueRequestId, 'turn-1');

    const cancelled = applyTurnState(queued, {
        turnId: 'turn-1',
        status: 'cancelled',
    });
    assert.equal(cancelled[0].turnStatus, 'cancelled');
    assert.equal(cancelled[0].kind === 'user' && cancelled[0].queueStatus, undefined);
    assert.deepEqual(cancelQueuedAction('session-1', 'turn-1'), {
        action: 'cancel_turn',
        sessionId: 'session-1',
        turnId: 'turn-1',
    });
    assert.deepEqual(cancelTurnAction('session-1', 'turn-1'), {
        action: 'cancel_turn',
        sessionId: 'session-1',
        turnId: 'turn-1',
    });
});

test('history and late deltas remain fenced to their explicit Turn ids', () => {
    const history = normalizeHistory(
        [
            { kind: 'user', text: 'same', turnId: 'turn-1' },
            { kind: 'assistant_text', text: 'first', turnId: 'turn-1' },
            { kind: 'user', text: 'same', turnId: 'turn-2' },
        ],
        undefined
    );
    const withLateFirstDelta = applyTextDelta(history, ' late', 'output', 'turn-1');

    assert.equal(withLateFirstDelta.at(-1)?.turnId, 'turn-1');
    assert.equal(withLateFirstDelta.at(-1)?.kind, 'assistant_text');
    assert.equal(history[0].turnId, 'turn-1');
    assert.equal(history[2].turnId, 'turn-2');
});

test('history reload drops a stale in-flight user bubble when its runtime id is already persisted', () => {
    // History items carry the RUNTIME request id as turnId (the bridge's
    // `turn_results` key). A live bubble from before a restart carries the
    // CANONICAL Turn id but the same clientRequestId — it must be treated as
    // already persisted and dropped, not appended at the end of the timeline.
    const persistedTurnIds = new Set(['runtime-1', 'runtime-2']);
    const stale: ChatItem[] = [
        {
            id: 'stale-copy',
            kind: 'user',
            content: 'merge branch',
            createdAt: 1,
            clientRequestId: 'runtime-1',
            turnId: 'turn-1',
            turnStatus: 'running',
        },
    ];

    assert.deepEqual(selectOptimisticUsers(stale, persistedTurnIds), []);
});

test('history reload keeps optimistic user bubbles for turns not yet persisted', () => {
    const persistedTurnIds = new Set(['runtime-1']);
    const items: ChatItem[] = [
        {
            id: 'queued',
            kind: 'user',
            content: 'second prompt',
            createdAt: 2,
            clientRequestId: 'runtime-2',
            turnId: 'turn-2',
            turnStatus: 'queued',
        },
        {
            id: 'running',
            kind: 'user',
            content: 'third prompt',
            createdAt: 3,
            clientRequestId: 'runtime-3',
            turnId: 'turn-3',
            turnStatus: 'running',
        },
    ];

    const kept = selectOptimisticUsers(items, persistedTurnIds);
    assert.deepEqual(
        kept.map(item => item.id),
        ['queued', 'running']
    );
});

test('history reload drops a stale copy matched by canonical turn id too', () => {
    const persistedTurnIds = new Set(['turn-1']);
    const stale: ChatItem[] = [
        {
            id: 'stale',
            kind: 'user',
            content: 'merge branch',
            createdAt: 1,
            turnId: 'turn-1',
            turnStatus: 'running',
        },
    ];

    assert.deepEqual(selectOptimisticUsers(stale, persistedTurnIds), []);
});
