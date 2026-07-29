import assert from 'node:assert/strict';
import { test } from 'node:test';

import { applyTextDelta, applyTurnState, normalizeHistory } from '@1agents/core/protocol/reducer';
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
