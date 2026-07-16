// Tests for the pure column-state helpers behind DataGrid persist (#129):
//   - loadColState        — JSON → sanitized ColState[] (back-compat)
//   - reconcileColState   — merge saved state against live column defs
//
// Both helpers are intentionally pure (no DOM, no localStorage, no Preact),
// so they're cheap to exercise in node:test without a browser harness.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { loadColState, reconcileColState, type GridColumn } from './DataGrid';
import type { ColState } from './GridToolbar';

const cols = (...defs: Array<[string, number]>): GridColumn[] =>
    defs.map(([key, width]) => ({ key, label: key.toUpperCase(), width }));

test('loadColState: null / empty / whitespace returns []', () => {
    assert.deepEqual(loadColState(null), []);
    assert.deepEqual(loadColState(''), []);
});

test('loadColState: malformed JSON returns [] without throwing', () => {
    assert.deepEqual(loadColState('not json'), []);
    assert.deepEqual(loadColState('{'), []);
    assert.deepEqual(loadColState('{"key":'), []);
});

test('loadColState: non-array payload returns []', () => {
    assert.deepEqual(loadColState('{}'), []);
    assert.deepEqual(loadColState('null'), []);
    assert.deepEqual(loadColState('"hello"'), []);
    assert.deepEqual(loadColState('42'), []);
});

test('loadColState: legacy format `{key, visible}` (no width) reads cleanly', () => {
    const raw = JSON.stringify([
        { key: 'id', visible: true },
        { key: 'title', visible: false },
    ]);
    assert.deepEqual(loadColState(raw), [
        { key: 'id', visible: true },
        { key: 'title', visible: false },
    ]);
});

test('loadColState: skips entries missing key or with wrong key type', () => {
    const raw = JSON.stringify([
        { key: 'ok', visible: true },
        { visible: true },
        { key: '', visible: true },
        { key: 42, visible: true },
        null,
        'string',
    ]);
    assert.deepEqual(loadColState(raw), [{ key: 'ok', visible: true }]);
});

test('loadColState: width passes through only when a positive finite number', () => {
    const raw = JSON.stringify([
        { key: 'good', visible: true, width: 120 },
        { key: 'zero', visible: true, width: 0 },
        { key: 'neg', visible: true, width: -5 },
        { key: 'inf', visible: true, width: Number.POSITIVE_INFINITY },
        { key: 'nan', visible: true, width: Number.NaN },
        { key: 'str', visible: true, width: '120' },
        { key: 'obj', visible: true, width: { v: 120 } },
        { key: 'noW', visible: true },
    ]);
    assert.deepEqual(loadColState(raw), [
        { key: 'good', visible: true, width: 120 },
        { key: 'zero', visible: true },
        { key: 'neg', visible: true },
        { key: 'inf', visible: true },
        { key: 'nan', visible: true },
        { key: 'str', visible: true },
        { key: 'obj', visible: true },
        { key: 'noW', visible: true },
    ]);
});

test('loadColState: non-boolean visible defaults to true', () => {
    const raw = JSON.stringify([{ key: 'a', visible: 'yes' }, { key: 'b', visible: 0 }, { key: 'c' }]);
    assert.deepEqual(loadColState(raw), [
        { key: 'a', visible: true },
        { key: 'b', visible: true },
        { key: 'c', visible: true },
    ]);
});

test('loadColState: round-trips with JSON.stringify without losing width', () => {
    const original: ColState[] = [
        { key: 'id', visible: true, width: 64 },
        { key: 'title', visible: true, width: 260 },
        { key: 'assignee', visible: false },
    ];
    const parsed = loadColState(JSON.stringify(original));
    assert.deepEqual(parsed, original);
});

test('reconcileColState: empty prev → all-visible baseline in allColumns order', () => {
    const all = cols(['id', 64], ['title', 260], ['status', 112]);
    assert.deepEqual(reconcileColState([], all), [
        { key: 'id', visible: true },
        { key: 'title', visible: true },
        { key: 'status', visible: true },
    ]);
});

test('reconcileColState: language switch — labels come from allColumns, user visibility preserved', () => {
    // Simulates the runtime reconcile triggered when the parent re-renders
    // with a translated column list. Keys are unchanged so order/visibility/
    // width carry over; labels render fresh from allColumns.
    const prev: ColState[] = [
        { key: 'id', visible: true, width: 64 },
        { key: 'title', visible: false }, // user hid this one
        { key: 'status', visible: true, width: 200 },
    ];
    const allCN: GridColumn[] = [
        { key: 'id', label: 'ID', width: 64 },
        { key: 'title', label: '任务', width: 260 },
        { key: 'status', label: '状态', width: 112 },
    ];
    const reconciled = reconcileColState(prev, allCN);
    assert.deepEqual(reconciled, [
        { key: 'id', visible: true, width: 64 },
        { key: 'title', visible: false },
        { key: 'status', visible: true, width: 200 },
    ]);
});

test('reconcileColState: dynamic column added — existing config is preserved, new column appended visible', () => {
    const prev: ColState[] = [
        { key: 'id', visible: true, width: 64 },
        { key: 'title', visible: false },
    ];
    const all = cols(['id', 64], ['title', 260], ['source', 140]);
    const reconciled = reconcileColState(prev, all);
    assert.deepEqual(reconciled, [
        { key: 'id', visible: true, width: 64 },
        { key: 'title', visible: false },
        { key: 'source', visible: true },
    ]);
});

test('reconcileColState: column removed from data source — stale entry dropped', () => {
    const prev: ColState[] = [
        { key: 'id', visible: true, width: 64 },
        { key: 'legacy', visible: false }, // gone from allColumns
        { key: 'title', visible: true, width: 300 },
    ];
    const all = cols(['id', 64], ['title', 260]);
    const reconciled = reconcileColState(prev, all);
    assert.deepEqual(reconciled, [
        { key: 'id', visible: true, width: 64 },
        { key: 'title', visible: true, width: 300 },
    ]);
});

test('reconcileColState: simultaneous add + remove preserves order, drops removed, appends new', () => {
    const prev: ColState[] = [
        { key: 'a', visible: true },
        { key: 'b', visible: true, width: 100 },
        { key: 'c', visible: false },
    ];
    const all = cols(['a', 60], ['c', 90], ['d', 120]);
    const reconciled = reconcileColState(prev, all);
    assert.deepEqual(reconciled, [
        { key: 'a', visible: true },
        { key: 'c', visible: false },
        { key: 'd', visible: true },
    ]);
});

test('reconcileColState: is pure — does not mutate prev or allColumns', () => {
    const prev: ColState[] = [{ key: 'a', visible: true, width: 80 }];
    const all = cols(['a', 64], ['b', 100]);
    const prevSnapshot = JSON.stringify(prev);
    const allSnapshot = JSON.stringify(all);
    const result = reconcileColState(prev, all);
    assert.equal(JSON.stringify(prev), prevSnapshot);
    assert.equal(JSON.stringify(all), allSnapshot);
    // Result must be a fresh array (signal consumers rely on identity change)
    assert.notEqual(result, prev);
    assert.equal(result[0] !== prev[0], true);
});

test('reconcileColState: is idempotent — applying twice yields structurally equal output', () => {
    const prev: ColState[] = [
        { key: 'a', visible: true, width: 80 },
        { key: 'b', visible: false },
    ];
    const all = cols(['a', 64], ['b', 100], ['c', 120]);
    const once = reconcileColState(prev, all);
    const twice = reconcileColState(once, all);
    assert.deepEqual(twice, once);
});

test('reconcileColState: end-to-end back-compat — old JSON (no width) round-trips through reconcile', () => {
    // Mirrors acceptance criterion 3/4: pre-width localStorage must read
    // without throwing, and reconciled state must keep visibility + the
    // existing width-less entry (renderer falls back to GridColumn.width).
    const legacyJson = JSON.stringify([
        { key: 'id', visible: true },
        { key: 'title', visible: false },
    ]);
    const loaded = loadColState(legacyJson);
    const all = cols(['id', 64], ['title', 260], ['status', 112]);
    const reconciled = reconcileColState(loaded, all);
    assert.deepEqual(reconciled, [
        { key: 'id', visible: true },
        { key: 'title', visible: false },
        { key: 'status', visible: true },
    ]);
    // Renderer path: missing width → use column default
    assert.equal(reconciled[0].width ?? all[0].width, 64);
    assert.equal(reconciled[2].width ?? all[2].width, 112);
});
