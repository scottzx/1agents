import assert from 'node:assert/strict';
import { test } from 'node:test';

import { applyTextDelta, applyToolCall, applyToolResult } from '@1agents/core/protocol/reducer';
import type { ChatItem } from '@1agents/core/protocol/types';

const MAIN_TURN = '910b3869-cb53-4254-8eb7-15cb67b95b9e'; // main agent (grok _meta.promptId)
const SUB_TURN = '019fe6e8-5cd7-7d91-b3de-a41e9521adbb'; // subagent (grok _meta.promptId)

function findSubagentCards(items: ChatItem[]): Extract<ChatItem, { kind: 'subagent_turn' }>[] {
    return items.filter((it): it is Extract<ChatItem, { kind: 'subagent_turn' }> => it.kind === 'subagent_turn');
}

test('subagent text_delta folds into its own card, never into the main thinking block', () => {
    let items: ChatItem[] = [];

    // Main agent's reasoning (thought, main turn id).
    items = applyTextDelta(items, 'The user wants ', 'thought', 'host-turn-1', MAIN_TURN, false);
    items = applyTextDelta(items, 'a summary.', 'thought', 'host-turn-1', MAIN_TURN, false);

    // Subagent starts: its thought chunks carry a DIFFERENT agentTurnId.
    items = applyTextDelta(items, 'I will ', 'thought', 'host-turn-1', SUB_TURN, true);
    items = applyTextDelta(items, 'inventory the dir.', 'thought', 'host-turn-1', SUB_TURN, true);
    // Subagent's spoken output also stays inside the card.
    items = applyTextDelta(items, '## Workspace summary', 'output', 'host-turn-1', SUB_TURN, true);

    // Main agent resumes: same agentTurnId as before → main thinking block.
    items = applyTextDelta(items, 'Done.', 'thought', 'host-turn-1', MAIN_TURN, false);
    // Main agent's final answer.
    items = applyTextDelta(items, 'One-line summary: flat dir.', 'output', 'host-turn-1', MAIN_TURN, false);

    const cards = findSubagentCards(items);
    assert.equal(cards.length, 1);
    assert.equal(cards[0].agentTurnId, SUB_TURN);
    assert.equal(cards[0].thinking, 'I will inventory the dir.');
    assert.equal(cards[0].output, '## Workspace summary');

    // Main thinking stayed in main-only blocks (pre/post subagent are
    // separate blocks — adjacency is required to merge, and the subagent
    // card sits between them). No subagent text leaked into either.
    const thinkings = items.filter(it => it.kind === 'thinking');
    assert.equal(thinkings.length, 2);
    assert.equal(thinkings[0].kind === 'thinking' && thinkings[0].content, 'The user wants a summary.');
    assert.equal(thinkings[1].kind === 'thinking' && thinkings[1].content, 'Done.');

    // Main output landed in the assistant_text block, untouched by subagent text.
    const assistant = items.filter(it => it.kind === 'assistant_text');
    assert.equal(assistant.length, 1);
    assert.equal(assistant[0].kind === 'assistant_text' && assistant[0].content, 'One-line summary: flat dir.');
});

test('parallel subagents each get their own card', () => {
    const subB = 'b1111111-0000-0000-0000-000000000000';
    let items: ChatItem[] = [];
    items = applyTextDelta(items, 'a1', 'thought', 'host-turn-1', SUB_TURN, true);
    items = applyTextDelta(items, 'b1', 'thought', 'host-turn-1', subB, true);
    items = applyTextDelta(items, 'a2', 'thought', 'host-turn-1', SUB_TURN, true);

    const cards = findSubagentCards(items);
    assert.equal(cards.length, 2);
    assert.equal(cards[0].agentTurnId, SUB_TURN);
    assert.equal(cards[0].thinking, 'a1a2');
    assert.equal(cards[1].agentTurnId, subB);
    assert.equal(cards[1].thinking, 'b1');
});

test('subagent tool_call and tool_result fold into the card, main tools stay in the main message', () => {
    let items: ChatItem[] = [];
    items = applyTextDelta(items, 'I will inventory.', 'thought', 'host-turn-1', SUB_TURN, true);

    // Subagent's own tool (list_dir) → card.calls.
    items = applyToolCall(
        { items, pendingResults: [], pendingPermissions: [] },
        { toolName: 'list_dir', toolCallId: 'call-sub-1', arguments: { directory: '/tmp/x' } },
        SUB_TURN,
        true
    ).items;
    // Its result attaches to the card's call.
    items = applyToolResult(
        { items, pendingResults: [], pendingPermissions: [] },
        { toolCallId: 'call-sub-1', text: 'sample.txt' },
        SUB_TURN,
        true
    ).items;

    // Main agent's tool (get_command_or_subagent_output) → main tool_use block.
    items = applyTextDelta(items, 'Waiting.', 'thought', 'host-turn-1', MAIN_TURN, false);
    items = applyToolCall(
        { items, pendingResults: [], pendingPermissions: [] },
        { toolName: 'get_command_or_subagent_output', toolCallId: 'call-main-1', arguments: { task_ids: ['t1'] } },
        MAIN_TURN,
        false
    ).items;

    const cards = findSubagentCards(items);
    assert.equal(cards.length, 1);
    assert.equal(cards[0].calls.length, 1);
    assert.equal(cards[0].calls[0].toolName, 'list_dir');
    assert.equal(cards[0].calls[0].output, 'sample.txt');
    assert.equal(cards[0].calls[0].status, 'completed');

    const mainTools = items.filter(it => it.kind === 'tool_use');
    assert.equal(mainTools.length, 1);
    assert.equal(mainTools[0].kind === 'tool_use' && mainTools[0].calls[0].toolName, 'get_command_or_subagent_output');
});

test('tool-first subagent creates the card so the call has a home', () => {
    let items: ChatItem[] = [];
    items = applyToolCall(
        { items, pendingResults: [], pendingPermissions: [] },
        { toolName: 'read_file', toolCallId: 'call-0', arguments: { path: '/tmp/x' } },
        SUB_TURN,
        true
    ).items;
    const cards = findSubagentCards(items);
    assert.equal(cards.length, 1);
    assert.equal(cards[0].calls.length, 1);
    assert.equal(cards[0].calls[0].toolName, 'read_file');

    // Later text still folds into the same card.
    items = applyTextDelta(items, 'Reading.', 'thought', 'host-turn-1', SUB_TURN, true);
    assert.equal(findSubagentCards(items)[0].thinking, 'Reading.');
});

test('without agentTurnId nothing changes (Claude Code path)', () => {
    let items: ChatItem[] = [];
    items = applyTextDelta(items, 'thought-1', 'thought', 'host-turn-1');
    items = applyTextDelta(items, 'thought-2', 'thought', 'host-turn-1');
    assert.equal(items.filter(it => it.kind === 'thinking').length, 1);
    assert.equal(findSubagentCards(items).length, 0);
});
