// Run: cd frontend && npx tsx --test src/stores/sidePanelModel.test.ts

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { isPinnedSidePanelTab, resolveSidePanelActiveId, withPinnedOverview } from './sidePanelModel';

test('isPinnedSidePanelTab treats session-status / overview as the pinned tab', () => {
    assert.equal(isPinnedSidePanelTab({ type: 'background' }), true);
    assert.equal(isPinnedSidePanelTab({ type: 'tasks' }), false);
});

test('withPinnedOverview keeps Overview first and unique', () => {
    const overview = { id: 'old-status', type: 'background' };
    const files = { id: 'files-1', type: 'files' };
    const extra = { id: 'status-2', type: 'background' };
    const pinned = { id: 'overview', type: 'background' };

    assert.deepEqual(withPinnedOverview([files, extra], pinned), [pinned, files]);
    assert.deepEqual(withPinnedOverview([overview, files], pinned), [pinned, files]);
    assert.deepEqual(withPinnedOverview([], pinned), [pinned]);
});

test('resolveSidePanelActiveId falls back to Overview when the selection is missing', () => {
    const tabs = [
        { id: 'overview', type: 'background' },
        { id: 'files-1', type: 'files' },
    ];
    assert.equal(resolveSidePanelActiveId(tabs, 'files-1'), 'files-1');
    assert.equal(resolveSidePanelActiveId(tabs, 'gone'), 'overview');
    assert.equal(resolveSidePanelActiveId(tabs, null), 'overview');
    assert.equal(resolveSidePanelActiveId([], null), null);
});
