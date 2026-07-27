// Pure helpers for roundtable timeline UI (slice 5 / design §6).
// Run: cd frontend && node --import tsx --test src/components/roundtable/stage.test.ts
// (or npx tsx --test … when tsx is available)

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { stageIndexFromState, isTerminalState, pollIntervalMs, speakingRoundFromState, STAGES } from './stage';
import { roleLabel, seatUiStatus, resolveTurnAuthor, ROLE_LABELS } from './roleLabels';
import { FIXED_ROSTER } from './LaunchWizard';
import type { RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';

test('stage strip maps state machine to R1–R3–终稿', () => {
    assert.deepEqual(
        STAGES.map(s => s.label),
        ['R1 命题', 'R2 首轮', 'R3 次轮', '终稿']
    );
    assert.equal(stageIndexFromState('drafting_brief'), 0);
    assert.equal(stageIndexFromState('waiting_r2'), 1);
    assert.equal(stageIndexFromState('summarizing_r2'), 1);
    assert.equal(stageIndexFromState('waiting_r3'), 2);
    assert.equal(stageIndexFromState('summarizing_r3'), 2);
    assert.equal(stageIndexFromState('done'), 3);
    assert.equal(stageIndexFromState('failed'), -1);
});

test('terminal states stop polling', () => {
    assert.equal(isTerminalState('done'), true);
    assert.equal(isTerminalState('failed'), true);
    assert.equal(isTerminalState('waiting_r2'), false);
    assert.equal(pollIntervalMs('done'), 0);
    assert.ok(pollIntervalMs('summarizing_r2') > 0);
});

test('speakingRoundFromState maps live seat badge to R1–R3', () => {
    assert.equal(speakingRoundFromState('drafting_brief'), 1);
    assert.equal(speakingRoundFromState('waiting_r2'), 2);
    assert.equal(speakingRoundFromState('summarizing_r2'), 2);
    assert.equal(speakingRoundFromState('waiting_r3'), 3);
    assert.equal(speakingRoundFromState('summarizing_r3'), 3);
    assert.equal(speakingRoundFromState('done'), 0);
});

test('seat labels are function names', () => {
    assert.equal(roleLabel('referee'), '裁判');
    assert.equal(roleLabel('market'), '市场');
    assert.equal(roleLabel('product'), '产品');
    assert.equal(roleLabel('eng'), '研发');
    assert.equal(roleLabel('ops'), '运营');
    assert.equal(roleLabel('finance'), '财务');
});

test('seat failed maps to error for UI chip; speaking → running', () => {
    assert.equal(seatUiStatus('speaking'), 'running');
    assert.equal(seatUiStatus('done'), 'done');
    assert.equal(seatUiStatus('failed'), 'error');
    assert.equal(seatUiStatus('ready'), 'ready');
});

test('turn author resolves seat role / user / system', () => {
    const seats: RoundtableSeat[] = [
        {
            id: 's-ref',
            room_id: 'r1',
            role: 'referee',
            agent_type: 'grok-build',
            workspace_id: 'tmp-1',
            status: 'ready',
            created_at: '',
        },
        {
            id: 's-eng',
            room_id: 'r1',
            role: 'eng',
            agent_type: 'grok-build',
            workspace_id: 'tmp-2',
            status: 'done',
            created_at: '',
        },
    ];
    const user: RoundtableTurn = {
        id: 't1',
        room_id: 'r1',
        round: 1,
        seat_id: 'user',
        kind: 'chat',
        content_text: 'hello',
        created_at: '',
    };
    const eng: RoundtableTurn = {
        id: 't2',
        room_id: 'r1',
        round: 2,
        seat_id: 's-eng',
        kind: 'speech',
        content_text: 'feasibility…',
        process_ref: 'sess-1',
        created_at: '',
    };
    const sys: RoundtableTurn = {
        id: 't3',
        room_id: 'r1',
        round: 1,
        kind: 'system',
        content_text: 'brief confirmed',
        created_at: '',
    };
    assert.equal(resolveTurnAuthor(user, seats), '用户');
    assert.equal(resolveTurnAuthor(eng, seats), '研发');
    assert.equal(resolveTurnAuthor(sys, seats), '系统');
});

test('launch wizard fixed roster is 6 seats with role labels', () => {
    assert.equal(FIXED_ROSTER.length, 6);
    assert.deepEqual(
        FIXED_ROSTER.map(s => s.role),
        ['referee', 'market', 'product', 'eng', 'ops', 'finance']
    );
    for (const s of FIXED_ROSTER) {
        assert.ok(ROLE_LABELS[s.role], `missing label for ${s.role}`);
        assert.ok(s.responsibility.trim(), `missing responsibility for ${s.role}`);
    }
});
