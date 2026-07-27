// Loaded-room presentation contract.
// Run: cd frontend && node --import tsx --test src/components/roundtable/content.test.ts

import assert from 'node:assert/strict';
import test from 'node:test';
import { h } from 'preact';
import renderToString from 'preact-render-to-string';

import type { RoundtableRoom, RoundtableTurn } from '@1agents/core/services/roundtableService';
import { BriefInspector, isBriefContentComplete, resolveBriefInspectorMode } from './BriefInspector';
import { mobilePaneForKey, RoundtableRoomContent } from './RoundtableRoomContent';
import { RoundtableSidebarView } from './RoundtableSidebarView';
import { TurnCard } from './TurnCard';
import { briefEventFromTurn, r1BriefEvents, timelineTurnsWithoutR1Chat } from './r1Timeline';

const brief = {
    title: '唯一标题正文',
    question: '唯一议题正文',
    constraints: '唯一约束正文',
    success_criteria: '唯一成功标准正文',
};

const legacyBriefTurn: RoundtableTurn = {
    id: 'brief-event',
    room_id: 'room-1',
    round: 1,
    seat_id: 'user',
    kind: 'system',
    content_text: [
        'Brief 已确认，进入 R2。',
        `title: ${brief.title}`,
        `question: ${brief.question}`,
        `constraints: ${brief.constraints}`,
        `success_criteria: ${brief.success_criteria}`,
    ].join('\n'),
    created_at: '2026-07-27T00:00:00Z',
};

const summaryTurn: RoundtableTurn = {
    id: 'summary-r3',
    room_id: 'room-1',
    round: 3,
    kind: 'summary',
    content_text: '唯一终稿正文',
    created_at: '2026-07-27T00:01:00Z',
};

const room: RoundtableRoom = {
    id: 'room-1',
    title: '去重测试房间',
    state: 'done',
    phase: 'done',
    phase_status: 'ready',
    next_action: 'none',
    progress: { completed: 1, total: 1, active_roles: [], failed_roles: [] },
    brief,
    summary_r3: summaryTurn.content_text,
    created_at: '2026-07-27T00:00:00Z',
    updated_at: '2026-07-27T00:01:00Z',
};

test('a loaded room renders one complete Brief and one copy of the Summary body', () => {
    const html = renderToString(
        h(RoundtableRoomContent, {
            room,
            seats: [],
            turns: [legacyBriefTurn, summaryTurn],
            header: h('header', { class: 'rt-room-header' }, h('h1', null, room.title)),
            sidebar: h(RoundtableSidebarView, {
                room,
                seats: [],
                turns: [legacyBriefTurn, summaryTurn],
            }),
        })
    );

    assert.equal(count(html, 'class="rt-brief"'), 1);
    assert.equal(html.includes('rt-brief-readonly'), false);
    for (const value of Object.values(brief)) {
        assert.equal(count(html, value), 1, `${value} should only render in the Inspector`);
    }

    assert.equal(count(html, summaryTurn.content_text), 1);
    assert.match(html, /终稿已生成/);
    assert.match(html, /href="#rt-summary-3-title"/);
});

test('legacy system and dedicated brief_confirmed turns render as compact events', () => {
    const dedicatedKinds = ['brief_confirmed', 'system/brief_confirmed'];
    const dedicated = dedicatedKinds.map(
        (kind, index): RoundtableTurn => ({
            ...legacyBriefTurn,
            id: `brief-event-new-${index}`,
            kind,
            content_text: '',
        })
    );

    for (const turn of [legacyBriefTurn, ...dedicated]) {
        const html = renderToString(h(TurnCard, { turn, seats: [] }));
        assert.match(html, /rt-turn-event kind-brief-confirmed/);
        assert.match(html, /Brief 已确认/);
        assert.equal(html.includes('title:'), false);
        assert.equal(html.includes(brief.question), false);
        assert.equal(html.includes('等待正文'), false);
    }
});

test('R1 workbench owns chat messages and ordinary timeline never duplicates them', () => {
    const r1Turns: RoundtableTurn[] = [
        {
            id: 'user-r1',
            room_id: 'room-r1',
            round: 1,
            seat_id: 'user',
            kind: 'chat',
            content_text: '第一轮用户澄清',
            created_at: '2026-07-27T00:00:00Z',
        },
        {
            id: 'referee-r1',
            room_id: 'room-r1',
            round: 1,
            seat_id: 'referee',
            kind: 'chat',
            content_text: '第一轮裁判追问',
            created_at: '2026-07-27T00:00:01Z',
        },
        {
            id: 'brief-proposed',
            room_id: 'room-r1',
            round: 1,
            kind: 'system',
            content_text: 'Brief 草案已更新至 v2，等待用户确认。',
            created_at: '2026-07-27T00:00:02Z',
        },
    ];
    const draftingRoom: RoundtableRoom = {
        ...room,
        id: 'room-r1',
        state: 'drafting_brief',
        brief: null,
        summary_r3: undefined,
    };
    const html = renderToString(
        h(RoundtableRoomContent, {
            room: draftingRoom,
            seats: [],
            turns: r1Turns,
            header: h('header', null, draftingRoom.title),
            primaryContent: h(
                'section',
                { class: 'fake-r1-chat' },
                h('p', null, r1Turns[0].content_text),
                h('p', null, r1Turns[1].content_text)
            ),
            sidebar: h('aside', null),
        })
    );

    assert.equal(count(html, r1Turns[0].content_text), 1);
    assert.equal(count(html, r1Turns[1].content_text), 1);
    assert.equal(html.includes('rt-turn-card'), false);
    assert.equal(html.includes(r1Turns[2].content_text), false);
});

test('post-R1 timeline filters chat and Brief events while retaining discussion turns', () => {
    const turns: RoundtableTurn[] = [
        {
            id: 'chat',
            room_id: 'room-1',
            round: 1,
            kind: 'chat',
            content_text: 'R1 对话',
            created_at: '2026-07-27T00:00:00Z',
        },
        {
            id: 'confirmed',
            room_id: 'room-1',
            round: 1,
            kind: 'system',
            content_text: '你已确认 Brief v3，进入 R2。',
            created_at: '2026-07-27T00:00:01Z',
        },
        {
            id: 'speech',
            room_id: 'room-1',
            round: 2,
            kind: 'speech',
            content_text: '市场席发言',
            created_at: '2026-07-27T00:00:02Z',
        },
    ];

    assert.deepEqual(
        timelineTurnsWithoutR1Chat(turns).map(turn => turn.id),
        ['speech']
    );
});

test('brief proposal and confirmation become compact Chat references targeting Inspector', () => {
    const proposed: RoundtableTurn = {
        id: 'proposed',
        room_id: 'room-1',
        round: 1,
        kind: 'system',
        content_text: 'Brief 草案已更新至 v2，等待用户确认。',
        created_at: '2026-07-27T00:00:00Z',
    };
    const confirmed: RoundtableTurn = {
        ...proposed,
        id: 'confirmed',
        content_text: '你已确认 Brief v2，进入 R2。',
    };

    assert.deepEqual(
        r1BriefEvents([proposed, confirmed]).map(event => event.label),
        ['Brief v2 已提案', 'Brief v2 已确认']
    );
    assert.equal(briefEventFromTurn({ ...proposed, kind: 'speech' }), null);
});

test('Brief Inspector state model covers loading, empty, saving, conflict, error, and confirmed', () => {
    assert.equal(resolveBriefInspectorMode({ loading: true }), 'loading');
    assert.equal(resolveBriefInspectorMode({}), 'empty');
    assert.equal(resolveBriefInspectorMode({ saving: true, hasVersion: true }), 'saving');
    assert.equal(resolveBriefInspectorMode({ conflict: true, hasVersion: true }), 'conflict');
    assert.equal(resolveBriefInspectorMode({ error: true, hasVersion: true }), 'error');
    assert.equal(resolveBriefInspectorMode({ confirmed: true, hasVersion: true }), 'confirmed');
    assert.equal(resolveBriefInspectorMode({ hasVersion: true }), 'ready');
});

test('Brief Inspector binds current version and renders confirmed state as the only full body', () => {
    const confirmedRoom: RoundtableRoom = {
        ...room,
        state: 'waiting_r2',
        current_brief_version: 4,
        confirmed_brief_version: 4,
        current_brief: {
            room_id: room.id,
            version: 4,
            status: 'confirmed',
            content: brief,
            proposed_by: 'user',
            created_at: room.created_at,
            updated_at: room.updated_at,
            confirmed_at: room.updated_at,
        },
    };
    const html = renderToString(h(BriefInspector, { room: confirmedRoom }));

    assert.match(html, /v4 · 已确认/);
    assert.match(html, /已锁定为后续讨论快照/);
    assert.equal(count(html, brief.title), 1);
    assert.equal(count(html, brief.question), 1);
    assert.equal(html.includes('确认并进入 R2'), false);
    assert.equal(isBriefContentComplete(brief), true);
    assert.equal(isBriefContentComplete({ ...brief, constraints: '—' }), false);
});

test('mobile room exposes discussion, Brief, and participant tabs without duplicating Inspector content', () => {
    const selected: string[] = [];
    const props = {
        room,
        seats: [],
        turns: [summaryTurn],
        header: h('header', null, room.title),
        primaryContent: h('section', null, '讨论主区'),
        sidebar: h(RoundtableSidebarView, {
            room,
            seats: [],
            turns: [summaryTurn],
            activeTab: 'topic' as const,
        }),
        mobilePane: 'brief' as const,
        onMobilePaneChange: (pane: string) => selected.push(pane),
    };
    const html = renderToString(h(RoundtableRoomContent, props));

    assert.match(html, /data-mobile-pane="brief"/);
    assert.match(html, /role="tab"[^>]*aria-selected="true"[^>]*>Brief</);
    assert.equal(count(html, '>讨论<'), 1);
    assert.equal(count(html, '>Brief<'), 1);
    assert.equal(count(html, '>参与者<'), 2, 'mobile tab and desktop Inspector tab share one canonical participant panel');
    assert.equal(count(html, 'class="rt-brief"'), 1);

    const tree = RoundtableRoomContent(props);
    const participantTab = buttonNodes(tree).find(node => nodeText(node.props?.children) === '参与者');
    assert.equal(typeof participantTab?.props?.onClick, 'function');
    (participantTab?.props?.onClick as () => void)();
    assert.deepEqual(selected, ['participants']);

    assert.equal(mobilePaneForKey('discussion', 'ArrowRight'), 'brief');
    assert.equal(mobilePaneForKey('brief', 'ArrowRight'), 'participants');
    assert.equal(mobilePaneForKey('discussion', 'ArrowLeft'), 'participants');
    assert.equal(mobilePaneForKey('participants', 'Home'), 'discussion');
    assert.equal(mobilePaneForKey('discussion', 'End'), 'participants');
});

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

function count(text: string, needle: string): number {
    return text.split(needle).length - 1;
}
