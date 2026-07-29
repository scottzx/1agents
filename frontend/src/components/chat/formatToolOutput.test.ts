import { test } from 'node:test';
import assert from 'node:assert/strict';
import { formatToolOutput } from './formatToolOutput';

test('formatToolOutput: parses JSON with output_for_prompt key (object value)', () => {
    const raw = JSON.stringify({
        output_for_prompt: { status: 'success', count: 42 },
        ignored_meta: 'meta',
    });
    const result = formatToolOutput(raw);
    const expected = JSON.stringify({ status: 'success', count: 42 }, null, 2);
    assert.equal(result, expected);
});

test('formatToolOutput: parses JSON with output_for_prompt key (stringified JSON value)', () => {
    const raw = JSON.stringify({
        output_for_prompt: JSON.stringify({ status: 'ok', data: [1, 2] }),
        ignored_meta: 'meta',
    });
    const result = formatToolOutput(raw);
    const expected = JSON.stringify({ status: 'ok', data: [1, 2] }, null, 2);
    assert.equal(result, expected);
});

test('formatToolOutput: parses JSON with output_for_prompt key (plain text string value)', () => {
    const raw = JSON.stringify({
        output_for_prompt: 'Execution completed.\nLine 2',
        ignored_meta: 'meta',
    });
    const result = formatToolOutput(raw);
    assert.equal(result, 'Execution completed.\nLine 2');
});

test('formatToolOutput: prioritizes output_for_prompt over formatted_output and output', () => {
    const raw = JSON.stringify({
        output_for_prompt: 'prompt output',
        formatted_output: 'formatted output',
        output: 'raw output',
    });
    const result = formatToolOutput(raw);
    assert.equal(result, 'prompt output');
});

test('formatToolOutput: falls back to formatted_output when output_for_prompt is absent', () => {
    const raw = JSON.stringify({
        formatted_output: { res: 'ok' },
        output: 'raw output',
    });
    const result = formatToolOutput(raw);
    const expected = JSON.stringify({ res: 'ok' }, null, 2);
    assert.equal(result, expected);
});

test('formatToolOutput: falls back to formatted_output stringified JSON', () => {
    const raw = JSON.stringify({
        formatted_output: '{"key": "val"}',
    });
    const result = formatToolOutput(raw);
    const expected = JSON.stringify({ key: 'val' }, null, 2);
    assert.equal(result, expected);
});

test('formatToolOutput: falls back to full pretty-printed JSON when no priority key matches', () => {
    const raw = JSON.stringify({
        custom_key: 123,
        name: 'test',
    });
    const result = formatToolOutput(raw);
    const expected = JSON.stringify({ custom_key: 123, name: 'test' }, null, 2);
    assert.equal(result, expected);
});

test('formatToolOutput: handles double-encoded JSON string containing priority key', () => {
    const inner = JSON.stringify({
        output_for_prompt: { ok: true },
    });
    const raw = JSON.stringify(inner);
    const result = formatToolOutput(raw);
    const expected = JSON.stringify({ ok: true }, null, 2);
    assert.equal(result, expected);
});

test('formatToolOutput: returns raw text unchanged when not JSON', () => {
    const raw = 'Simple text response from tool call';
    const result = formatToolOutput(raw);
    assert.equal(result, raw);
});

test('formatToolOutput: prioritizes file_matches when type is GrepSearch', () => {
    const raw = JSON.stringify({
        type: 'GrepSearch',
        file_matches: [{ Filename: '/path/to/file.ts', LineNumber: 10, LineContent: 'const a = 1;' }],
        output_for_prompt: 'some prompt output',
        formatted_output: 'some formatted output',
    });
    const result = formatToolOutput(raw);
    const expected = JSON.stringify(
        [{ Filename: '/path/to/file.ts', LineNumber: 10, LineContent: 'const a = 1;' }],
        null,
        2
    );
    assert.equal(result, expected);
});

test('formatToolOutput: falls back to output_for_prompt if type is GrepSearch but file_matches is missing', () => {
    const raw = JSON.stringify({
        type: 'GrepSearch',
        output_for_prompt: 'fallback prompt output',
    });
    const result = formatToolOutput(raw);
    assert.equal(result, 'fallback prompt output');
});

test('formatToolOutput: prioritizes FileContent when type is ReadFile', () => {
    const raw = JSON.stringify({
        type: 'ReadFile',
        FileContent: 'console.log("hello world");\n',
        output_for_prompt: 'ignored prompt output',
    });
    const result = formatToolOutput(raw);
    assert.equal(result, 'console.log("hello world");\n');
});

test('formatToolOutput: formats FileContent as JSON if FileContent is a JSON object or string', () => {
    const raw = JSON.stringify({
        type: 'ReadFile',
        FileContent: { key: 'value' },
        output_for_prompt: 'ignored',
    });
    const result = formatToolOutput(raw);
    const expected = JSON.stringify({ key: 'value' }, null, 2);
    assert.equal(result, expected);
});
