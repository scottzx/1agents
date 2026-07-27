// Phase workbench acceptance coverage for #278.
// Run: cd frontend && yarn test:roundtable

import assert from 'node:assert/strict';
import test from 'node:test';
import { h } from 'preact';
import renderToString from 'preact-render-to-string';

import type {
    RoundtableRoom,
    RoundtableSeat,
    RoundtableTurn,
    SeatRole,
} from '@1agents/core/services/roundtableService';
import { LaunchWizard } from './LaunchWizard';
import { RoomList, roomCardStatus } from './RoomList';
import { RoundtableHeader } from './RoundtableHeader';
import { RoundtableSidebarView } from './RoundtableSidebarView';
import { StageWorkbench } from './StageWorkbench';
import { primaryActionForRoom } from './primaryAction';

const roles: SeatRole[] = ['referee', 'market', 'product', 'eng', 'ops', 'finance'];
const seats: RoundtableSeat[] = roles.map((role, index) => ({
    id: `seat-${role}`,
    room_id: 'room-workbench',
    role,
    agent_type: 'agent',
    workspace_id: `workspace-${index}`,
    session_id: `discussion-${index}`,
    status: 'ready',
    created_at: '2026-07-27T00:00:00Z',
}));

function room(patch: Partial<RoundtableRoom>): RoundtableRoom {
    return {
        id: 'room-workbench',
        title: '客服系统应该自研还是采购？',
        state: 'waiting_r2',
        phase: 'r2',
        phase_status: 'ready',
        next_action: 'start_r2',
        progress: { completed: 0, total: 5, active_roles: [], failed_roles: [] },
        created_at: '2026-07-27T00:00:00Z',
        updated_at: '2026-07-27T00:10:00Z',
        ...patch,
    };
}

function speech(role: SeatRole, round: 2 | 3, content: string, process = false): RoundtableTurn {
    return {
        id: `speech-${round}-${role}`,
        room_id: 'room-workbench',
        round,
        seat_id: `seat-${role}`,
        kind: 'speech',
        content_text: content,
        ...(process ? { process_ref: `process-${role}` } : {}),
        created_at: '2026-07-27T00:05:00Z',
    };
}

function summary(round: 2 | 3, content: string): RoundtableTurn {
    return {
        id: `summary-${round}`,
        room_id: 'room-workbench',
        round,
        seat_id: 'seat-referee',
        kind: 'summary',
        content_text: content,
        created_at: '2026-07-27T00:09:00Z',
    };
}

test('header is one compact surface with roundtable, phase, real progress, and primary action', () => {
    const r2 = room({
        state: 'summarizing_r2',
        phase_status: 'running',
        next_action: 'wait',
        progress: { completed: 2, total: 5, active_roles: ['eng'], failed_roles: [] },
        active_run: {
            id: 'run-r2',
            room_id: 'room-workbench',
            round: 2,
            status: 'running',
            idempotency_key: 'r2-key',
            created_at: '2026-07-27T00:00:00Z',
            updated_at: '2026-07-27T00:01:00Z',
        },
    });
    const html = renderToString(
        h(RoundtableHeader, {
            room: r2,
            action: h('button', { type: 'button' }, '主操作'),
        })
    );

    assert.equal(count(html, r2.title), 1);
    assert.match(html, /R2 进行中/);
    assert.match(html, /2\/5 已完成 · 研发进行中/);
    assert.match(html, /aria-live="polite"/);
    assert.match(html, /aria-atomic="true"/);
    assert.match(html, /rt-room-phase-icon/);
    assert.equal(count(html, '主操作'), 1);
    assert.equal(html.includes('刷新'), false);
    assert.equal(html.includes('rt-stage-steps'), false);
});

test('every server next_action maps to one correct user-facing status or action', () => {
    const cases = [
        ['confirm_brief', 'button', '完善并确认议题'],
        ['start_r2', 'button', '开始五席独立分析'],
        ['start_r3', 'button', '开始交叉回应'],
        ['wait', 'status', '正在讨论，无需操作'],
        ['inspect_failure', 'button', '重新同步状态'],
        ['retry_failed_seats', 'status', '请选择失败席位的恢复动作'],
        ['retry_summary', 'status', '席位结果已保留，等待重试总结'],
    ] as const;

    for (const [nextAction, kind, label] of cases) {
        const action = primaryActionForRoom(room({ next_action: nextAction }));
        assert.equal(action?.kind, kind);
        assert.equal(action?.label, label);
    }
    assert.equal(primaryActionForRoom(room({ next_action: 'none' })), null);
});

test('R2 shows real progress, five independent viewpoints, full bodies, and on-demand process', () => {
    const r2Turns = [
        speech('market', 2, '市场结论：先验证采购方案。', true),
        speech('product', 2, '产品结论：体验差异必须可量化。'),
        speech('eng', 2, '研发结论：自研核心集成层。'),
        speech('ops', 2, '[failed] 运营席暂未完成。'),
        speech('finance', 2, '财务结论：比较三年总成本。'),
        summary(2, 'Summary₂ 唯一正文标记'),
    ];
    const html = renderToString(
        h(StageWorkbench, {
            room: room({
                state: 'summarizing_r2',
                phase_status: 'running',
                next_action: 'wait',
                progress: {
                    completed: 2,
                    total: 5,
                    active_roles: ['product'],
                    failed_roles: ['ops'],
                },
                active_run: {
                    id: 'run-r2',
                    room_id: 'room-workbench',
                    round: 2,
                    status: 'running',
                    idempotency_key: 'r2-key',
                    created_at: '2026-07-27T00:00:00Z',
                    updated_at: '2026-07-27T00:01:00Z',
                },
                summary_r2: 'Summary₂ 唯一正文标记',
            }),
            seats,
            turns: r2Turns,
        })
    );

    assert.match(html, /比较五席的独立判断/);
    assert.match(html, /2\/5/);
    assert.equal(count(html, '<article class="rt-analysis-card'), 5);
    for (const label of ['市场', '产品', '研发', '运营', '财务']) assert.match(html, new RegExp(`>${label}<`, 'u'));
    assert.match(html, /结论预览/);
    assert.match(html, /查看完整正文/);
    assert.match(html, /查看分析过程/);
    assert.match(html, /需要处理/);
    assert.equal(count(html, 'Summary₂ 唯一正文标记'), 1);
});

test('R3 makes stance changes and response targets explicit and resets stale R2 progress', () => {
    const r3Turns = [
        speech('market', 3, '保留原判断，并回应产品席的体验担忧。'),
        speech('product', 3, '修正范围：先做最小闭环。'),
        speech('eng', 3, '反驳财务席的周期假设。'),
        speech('ops', 3, '新增证据：试点数据显示转化提升。'),
        speech('finance', 3, '保留预算上限，但修正回收期。'),
        summary(2, 'R2 基线正文只出现一次'),
    ];
    const html = renderToString(
        h(StageWorkbench, {
            room: room({
                state: 'waiting_r3',
                phase: 'r3',
                next_action: 'start_r3',
                progress: { completed: 5, total: 5, active_roles: [], failed_roles: [] },
                active_run: {
                    id: 'completed-r2',
                    room_id: 'room-workbench',
                    round: 2,
                    status: 'completed',
                    idempotency_key: 'r2-key',
                    created_at: '2026-07-27T00:00:00Z',
                    updated_at: '2026-07-27T00:01:00Z',
                },
                summary_r2: 'R2 基线正文只出现一次',
            }),
            seats,
            turns: r3Turns,
        })
    );

    assert.match(html, /查看观点如何被保留、修正或反驳/);
    assert.match(html, /0\/5/);
    for (const label of ['保留', '修正', '反驳', '新增证据', '回应对象']) {
        assert.match(html, new RegExp(label, 'u'));
    }
    assert.match(html, /产品席上一轮观点/);
    assert.equal(count(html, 'R2 基线正文只出现一次'), 1);
});

test('Done is final-first, renders each Summary section once, and keeps history collapsed', () => {
    const final = [
        '## 最终判断',
        '建议标记：采购标准能力，自研差异化集成。',
        '## 取舍与条件',
        '取舍标记：以更短上线时间换取有限定制。',
        '## 行动项',
        '行动标记：产品负责两周试点，财务负责核算。',
        '## 未决风险',
        '风险标记：供应商锁定和数据迁移。',
    ].join('\n');
    const historyTurns = [
        ...roles.slice(1).map(role => speech(role, 2, `${role} R2 evidence`)),
        summary(2, 'Summary₂ 历史唯一正文'),
        ...roles.slice(1).map(role => speech(role, 3, `${role} R3 evidence`)),
        summary(3, final),
    ];
    const html = renderToString(
        h(StageWorkbench, {
            room: room({
                state: 'done',
                phase: 'done',
                phase_status: 'completed',
                next_action: 'none',
                summary_r2: 'Summary₂ 历史唯一正文',
                summary_r3: final,
                progress: { completed: 5, total: 5, active_roles: [], failed_roles: [] },
            }),
            seats,
            turns: historyTurns,
        })
    );

    for (const label of ['最终建议', '关键取舍', '行动项与负责职能', '未决风险']) {
        assert.match(html, new RegExp(label, 'u'));
    }
    for (const marker of ['建议标记', '取舍标记', '行动标记', '风险标记', 'Summary₂ 历史唯一正文']) {
        assert.equal(count(html, marker), 1, `${marker} should have one canonical body instance`);
    }
    assert.match(html, /<details class="rt-history"><summary>查看 R2 \/ R3 历史轮次/);
    assert.equal(html.includes('<details class="rt-history" open'), false);
});

test('Inspector uses topic and participant tabs without copying Summary content', () => {
    const finalRoom = room({ summary_r3: '不要复制这段 Summary 正文' });
    const html = renderToString(
        h(RoundtableSidebarView, {
            room: finalRoom,
            seats,
            turns: [summary(3, '不要复制这段 Summary 正文')],
        })
    );

    assert.match(html, /role="tab"[^>]*>议题</);
    assert.match(html, /role="tab"[^>]*>参与者</);
    assert.match(html, /六席参与者/);
    assert.match(html, /终稿已生成/);
    assert.equal(html.includes('不要复制这段 Summary 正文'), false);
});

test('creation wizard is problem-first and exposes no session or harness terminology', () => {
    const html = renderToString(h(LaunchWizard, { onStart: () => undefined }));
    assert.match(html, /你希望圆桌解决什么问题/);
    assert.match(html, /带着问题开始/);
    assert.match(html, /六个角度/);
    assert.equal(/Grok Build|harness|session/iu.test(html), false);
});

test('roundtable list prioritizes the user question and waiting-for-me status', () => {
    const waitingRoom = room({ next_action: 'start_r3', state: 'waiting_r3', phase: 'r3' });
    assert.equal(roomCardStatus(waitingRoom), '等待我操作');

    const html = renderToString(
        h(RoomList, {
            onOpenRoom: () => undefined,
            onCreate: () => undefined,
        })
    );
    assert.match(html, /我的圆桌问题/);
    assert.match(html, /等待我操作/);
    assert.equal(/Grok Build|harness|session/iu.test(html), false);
});

function count(text: string, needle: string): number {
    return text.split(needle).length - 1;
}
