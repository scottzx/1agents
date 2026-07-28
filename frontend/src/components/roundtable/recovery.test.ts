import { strict as assert } from 'node:assert';
import test from 'node:test';
import { h } from 'preact';
import renderToString from 'preact-render-to-string';
import type {
    RoundtableRoom,
    RoundtableSeat,
    RoundtableTurn,
    SeatRole,
} from '@1agents/core/services/roundtableService';
import { recoveryKind, RoundRecoveryNotice } from './RoundRecoveryNotice';
import { speechForSeat } from './workbench';

const roles: SeatRole[] = ['market', 'product', 'eng', 'ops', 'finance'];

function room(scope: 'room' | 'seat' | 'summary', failedRoles: SeatRole[] = []): RoundtableRoom {
    return {
        id: 'room-recovery',
        title: '恢复测试',
        state: scope === 'room' || scope === 'summary' ? 'failed' : 'summarizing_r2',
        created_at: '2026-07-27T00:00:00Z',
        updated_at: '2026-07-27T00:01:00Z',
        phase: 'r2',
        phase_status: scope === 'seat' ? 'partial_failed' : 'failed',
        next_action: scope === 'seat' ? 'retry_failed_seats' : scope === 'summary' ? 'retry_summary' : 'reload_room',
        available_actions:
            scope === 'seat'
                ? ['retry_failed_seats', 'skip_and_summarize']
                : scope === 'summary'
                  ? ['retry_summary']
                  : ['reload_room'],
        progress: {
            completed: 5 - failedRoles.length,
            total: 5,
            active_roles: [],
            failed_roles: failedRoles,
            skipped_roles: [],
        },
        active_run: {
            id: 'run-same',
            room_id: 'room-recovery',
            round: 2,
            status: scope === 'seat' ? 'partial_failed' : 'failed',
            idempotency_key: 'same-run',
            created_at: '2026-07-27T00:00:00Z',
            updated_at: '2026-07-27T00:01:00Z',
            error_scope: scope,
            error: `${scope} failed`,
        },
    };
}

interface TestVNode {
    type?: unknown;
    props?: Record<string, unknown>;
}

function buttonNodes(value: unknown): TestVNode[] {
    if (Array.isArray(value)) return value.flatMap(buttonNodes);
    if (!value || typeof value !== 'object') return [];
    const node = value as TestVNode;
    const nested = buttonNodes(node.props?.children);
    return node.type === 'button' ? [node, ...nested] : nested;
}

function nodeText(value: unknown): string {
    if (Array.isArray(value)) return value.map(nodeText).join('');
    if (typeof value === 'string') return value;
    if (!value || typeof value !== 'object') return '';
    return nodeText((value as TestVNode).props?.children);
}

test('seat recovery restored from server state exposes only per-seat retry and skip actions', () => {
    const restored = JSON.parse(JSON.stringify(room('seat', ['ops']))) as RoundtableRoom;
    const calls: string[] = [];
    const html = renderToString(
        h(RoundRecoveryNotice, {
            room: restored,
            onRetrySeat: role => {
                calls.push(`retry:${role}`);
            },
            onSkip: () => {
                calls.push('skip');
            },
            onRetrySummary: () => {
                calls.push('summary');
            },
            onReload: () => {
                calls.push('reload');
            },
        })
    );

    assert.equal(recoveryKind(restored), 'seat');
    assert.match(html, /已完成席位及其结果已保留/);
    assert.match(html, /仅重试运营席/);
    assert.match(html, /跳过缺席席位并继续总结/);
    assert.doesNotMatch(html, /仅重试总结/);
    assert.doesNotMatch(html, /仅重试市场席|仅重试产品席|仅重试研发席|仅重试财务席/);

    const tree = RoundRecoveryNotice({
        room: restored,
        onRetrySeat: role => {
            calls.push(`retry:${role}`);
        },
        onSkip: () => {
            calls.push('skip');
        },
        onRetrySummary: () => {
            calls.push('summary');
        },
        onReload: () => {
            calls.push('reload');
        },
    });
    const buttons = buttonNodes(tree);
    assert.deepEqual(
        buttons.map(button => nodeText(button.props?.children)),
        ['仅重试运营席', '跳过缺席席位并继续总结']
    );
    for (const button of buttons) {
        const click = button.props?.onClick;
        assert.equal(typeof click, 'function');
        (click as () => void)();
    }
    assert.deepEqual(calls, ['retry:ops', 'skip']);
});

test('summary and room failures restore distinct recovery controls', () => {
    const noop = () => undefined;
    const summaryHTML = renderToString(
        h(RoundRecoveryNotice, {
            room: room('summary'),
            onRetrySeat: noop,
            onSkip: noop,
            onRetrySummary: noop,
            onReload: noop,
        })
    );
    assert.match(summaryHTML, /总结生成失败/);
    assert.match(summaryHTML, /不会重新运行任何 panelist/);
    assert.match(summaryHTML, /仅重试总结/);
    assert.doesNotMatch(summaryHTML, /跳过缺席席位|仅重试运营席/);

    const roomHTML = renderToString(
        h(RoundRecoveryNotice, {
            room: room('room'),
            onRetrySeat: noop,
            onSkip: noop,
            onRetrySummary: noop,
            onReload: noop,
        })
    );
    assert.match(roomHTML, /房间状态同步失败/);
    assert.match(roomHTML, /重新同步房间/);
    assert.doesNotMatch(roomHTML, /仅重试总结|跳过缺席席位/);
});

test('retry result wins over the earlier failed turn without removing preserved history', () => {
    const seat: RoundtableSeat = {
        id: 'seat-ops',
        room_id: 'room-recovery',
        role: 'ops',
        agent_type: 'grok-build',
        workspace_id: 'workspace-ops',
        status: 'done',
        created_at: '2026-07-27T00:00:00Z',
    };
    const turns: RoundtableTurn[] = [
        {
            id: 'failed-attempt',
            room_id: 'room-recovery',
            round: 2,
            seat_id: seat.id,
            kind: 'speech',
            content_text: '[failed] 运营席首次失败',
            created_at: '2026-07-27T00:01:00Z',
        },
        {
            id: 'successful-retry',
            room_id: 'room-recovery',
            round: 2,
            seat_id: seat.id,
            kind: 'speech',
            content_text: '运营席重试后的有效结论',
            created_at: '2026-07-27T00:02:00Z',
        },
    ];

    assert.equal(turns.length, 2, 'failed-attempt history remains available');
    assert.equal(speechForSeat(turns, seat, 2)?.id, 'successful-retry');
    assert.deepEqual(roles, ['market', 'product', 'eng', 'ops', 'finance']);
});
