import assert from 'node:assert/strict';
import { test } from 'node:test';
import { turnsToChatItems } from '../src/adapter';
import { formatToolOutput } from '../src/formatToolOutput';
import { groupHistoricalTurns, inferFilesFromTurnItems, mergeChangeReport } from '../src/turns';
import type { GroupedChatItem, TurnChangeReport } from '../src/types';

test('session reader keeps dynamic tool inputs and false or zero results', () => {
    const items = turnsToChatItems([
        {
            turn: 1,
            startedAt: '2026-10-22T00:00:00Z',
            toolCalls: [
                { toolName: 'ReadFile', args: { path: 'src/app.ts' }, result: false },
                { toolName: 'Shell', args: 'pwd', result: 0 },
                { toolName: 'Other', args: null, result: 'plain output' },
            ],
        },
    ]);
    const group = items.find(item => item.kind === 'tool_use');
    assert.equal(group?.kind, 'tool_use');
    if (group?.kind !== 'tool_use') assert.fail('tool group missing');
    assert.equal(group.calls[0].input, '{\n  "path": "src/app.ts"\n}');
    assert.equal(group.calls[0].output, 'false');
    assert.equal(group.calls[1].input, 'pwd');
    assert.equal(group.calls[1].output, '0');
    assert.equal(group.calls[2].input, '');
    assert.equal(group.calls[2].output, 'plain output');
});

test('tool output keeps malformed JSON and unwraps nested ReadFile content', () => {
    const malformed = '  {"content": "unfinished  ';
    assert.equal(formatToolOutput(malformed), malformed);
    assert.equal(formatToolOutput("'not valid JSON'"), "'not valid JSON'");
    assert.equal(
        formatToolOutput(JSON.stringify({ type: 'ReadFile', FileContent: '{"content":"file body"}' })),
        'file body'
    );
    assert.equal(formatToolOutput(JSON.stringify({ output: false })), 'false');
    assert.equal(formatToolOutput(JSON.stringify({ output: 0 })), '0');
});

test('invalid tool JSON preserves explicit file locations and deletion precedence', () => {
    const inferred = inferFilesFromTurnItems([
        {
            kind: 'tool_group',
            calls: [{ id: 'edit-call', toolName: 'Edit', input: '{broken', locations: [{ path: 'src/app.ts' }] }],
        },
    ]);
    assert.deepEqual(inferred, [{ path: 'src/app.ts', op: 'modified' }]);
    const report: TurnChangeReport = {
        addedCount: 0,
        modifiedCount: 0,
        deletedCount: 1,
        files: [{ path: 'src/app.ts', op: 'deleted' }],
    };
    assert.deepEqual(mergeChangeReport(report, inferred)?.files, report.files);
});

test('historical turns fold and the latest turn retains one inferred change report', () => {
    const items: GroupedChatItem[] = [
        { id: 'u1', kind: 'user', content: 'first', createdAt: 1, turnId: '1' },
        { id: 'thinking1', kind: 'thinking', content: 'planning', createdAt: 2, turnId: '1' },
        { id: 'a1', kind: 'assistant_text', content: 'done', createdAt: 3, turnId: '1' },
        { id: 'u2', kind: 'user', content: 'second', createdAt: 4, turnId: '2' },
        {
            id: 'tool2',
            kind: 'tool_group',
            calls: [{ id: 'edit-call', toolName: 'Edit', input: '{broken', locations: [{ path: 'src/app.ts' }] }],
            thinkingBlocks: [],
            createdAt: 5,
            turnId: '2',
        },
        { id: 'a2', kind: 'assistant_text', content: 'updated', createdAt: 6, turnId: '2' },
    ];
    const grouped = groupHistoricalTurns(items);
    assert.deepEqual(
        grouped.map(item => item.kind),
        ['user', 'turn', 'user', 'tool_group', 'assistant_text']
    );
    const history = grouped[1];
    assert.equal(history.kind, 'turn');
    if (history.kind !== 'turn') assert.fail('historical turn missing');
    assert.equal(history.outcomeId, 'a1');
    const reports = grouped.filter(item => item.kind === 'assistant_text' && item.changeReport);
    assert.equal(reports.length, 1);
    const latest = reports[0];
    assert.equal(latest.kind, 'assistant_text');
    if (latest.kind !== 'assistant_text') assert.fail('latest reply missing');
    assert.deepEqual(latest.changeReport?.files, [{ path: 'src/app.ts', op: 'modified' }]);
});
