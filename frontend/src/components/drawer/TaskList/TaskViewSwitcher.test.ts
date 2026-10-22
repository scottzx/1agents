import assert from 'node:assert/strict';
import test from 'node:test';
import { h } from 'preact';
import renderToString from 'preact-render-to-string';

import { resolveTaskListView, TaskViewSwitcher } from './TaskViewSwitcher';

test('project navigation retains task, requirements and milestone views', () => {
    const rendered = renderToString(h(TaskViewSwitcher, { activeView: 'tasks', onSelect: () => undefined }));
    assert.match(rendered, /总览.*讨论.*需求.*任务.*里程碑/);
    assert.match(rendered, /class="active"[^>]*>任务/);
});

test('a retired or unknown saved view falls back to tasks', () => {
    assert.equal(resolveTaskListView('features'), 'tasks');
    assert.equal(resolveTaskListView('feature-catalog'), 'tasks');
    assert.equal(resolveTaskListView('unknown'), 'tasks');
    assert.equal(resolveTaskListView(undefined), 'tasks');
    assert.equal(resolveTaskListView('milestone'), 'milestone');
});
