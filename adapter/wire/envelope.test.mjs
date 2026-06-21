/**
 * envelope.mjs 的契约测试(node:test,零依赖,零构建)。
 *
 *   node --test adapter/wire/          # 或 npm test(在 adapter/)
 *
 * 两组断言:
 *   1) golden 对拍:每条 happy ACPMessageData → 精确 WsMessage(或 null)。
 *      golden 见 golden/acpMessageData-to-wsMessage.json,编码从两端类型定义推导的契约
 *      (非现网 acpx 抓包 —— 见 envelope.mjs 顶部「验收边界」)。
 *   2) 往返:可逆子集 toWsMessage → fromWsMessage 还原核心字段(ACPMessageData 子集)。
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { toWsMessage, fromWsMessage } from './envelope.mjs';

const goldenPath = fileURLToPath(new URL('./golden/acpMessageData-to-wsMessage.json', import.meta.url));
const { cases } = JSON.parse(readFileSync(goldenPath, 'utf8'));

for (const c of cases) {
    test(`toWsMessage: ${c.name}`, () => {
        assert.deepEqual(toWsMessage(c.in), c.out);
    });
}

// 防御:乱输入不抛、返回 null。
test('toWsMessage: 非对象 / 未知 type → null', () => {
    assert.equal(toWsMessage(null), null);
    assert.equal(toWsMessage(undefined), null);
    assert.equal(toWsMessage('nope'), null);
    assert.equal(toWsMessage({ type: 'does-not-exist' }), null);
});

// ⭐ 重点断言:thinking 与 reasoning 都产出 text_delta type:'thought'(对齐前端 ThinkingBubble)。
test('toWsMessage: thinking 与 reasoning 同走 thought 通道', () => {
    assert.deepEqual(toWsMessage({ type: 'thinking', text: 'X' }), {
        event: 'text_delta',
        text: 'X',
        type: 'thought',
    });
    assert.deepEqual(toWsMessage({ type: 'reasoning', message: 'X' }), {
        event: 'text_delta',
        text: 'X',
        type: 'thought',
    });
});

// 往返:agent 输出事件可逆还原核心字段(ACPMessageData 子集)。
test('fromWsMessage: message 往返', () => {
    const ws = toWsMessage({ type: 'message', message: 'Hi' });
    assert.deepEqual(fromWsMessage(ws), { type: 'message', message: 'Hi' });
});

test('fromWsMessage: thinking 往返(thought 逆向取 thinking)', () => {
    const ws = toWsMessage({ type: 'thinking', text: 'reason' });
    assert.deepEqual(fromWsMessage(ws), { type: 'thinking', text: 'reason' });
});

test('fromWsMessage: tool-call 往返', () => {
    const ws = toWsMessage({ type: 'tool-call', callId: 'c1', name: 'Read', input: { p: 1 }, id: 'm1' });
    assert.deepEqual(fromWsMessage(ws), { type: 'tool-call', name: 'Read', input: { p: 1 }, callId: 'c1' });
});

test('fromWsMessage: tool-result 往返', () => {
    const ws = toWsMessage({ type: 'tool-result', callId: 'c1', output: 'out', id: 'm2' });
    assert.deepEqual(fromWsMessage(ws), { type: 'tool-result', output: 'out', callId: 'c1', isError: false });
});

test('fromWsMessage: done → task_complete', () => {
    assert.deepEqual(fromWsMessage({ event: 'done' }), { type: 'task_complete' });
});

// 入站控制 / 未知事件不可逆 → null。
test('fromWsMessage: 不可逆输入 → null', () => {
    assert.equal(fromWsMessage({ action: 'prompt', text: 'hi' }), null);
    assert.equal(fromWsMessage({ event: 'session_ready' }), null);
    assert.equal(fromWsMessage(null), null);
});
