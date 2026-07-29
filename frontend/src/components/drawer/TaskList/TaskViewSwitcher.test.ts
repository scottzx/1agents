import assert from 'node:assert/strict';
import test from 'node:test';
import { h } from 'preact';
import renderToString from 'preact-render-to-string';

import { resolveTaskListView, TaskViewSwitcher } from './TaskViewSwitcher';

test('feature catalog navigation is absent by default and appears in the specified order when enabled', () => {
    const disabled = renderToString(
        h(TaskViewSwitcher, {
            activeView: 'tasks',
            featureCatalogEnabled: false,
            onSelect: () => undefined,
        })
    );
    assert.equal(disabled.includes('功能蓝图'), false);
    assert.match(disabled, /需求.*任务/);

    const enabled = renderToString(
        h(TaskViewSwitcher, {
            activeView: 'features',
            featureCatalogEnabled: true,
            onSelect: () => undefined,
        })
    );
    assert.match(enabled, /需求.*功能蓝图.*任务/);
    assert.match(enabled, /class="active"[^>]*>功能蓝图/);
});

test('an unavailable persisted features view falls back only after config has loaded', () => {
    assert.equal(resolveTaskListView('features', false, false), 'features');
    assert.equal(resolveTaskListView('features', false, true), 'tasks');
    assert.equal(resolveTaskListView('features', true, true), 'features');
    assert.equal(resolveTaskListView('milestone', false, true), 'milestone');
});
