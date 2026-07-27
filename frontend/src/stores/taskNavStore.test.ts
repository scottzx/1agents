import assert from 'node:assert/strict';
import test from 'node:test';

import { clearHeaderBackActions, headerBackAction, registerHeaderBackAction } from './headerBackStore';

test('global header back restores the parent action after a nested detail closes', () => {
    clearHeaderBackActions();
    let target = '';

    const closeProject = registerHeaderBackAction('project', () => (target = 'overview'), 10);
    headerBackAction.value?.();
    assert.equal(target, 'overview');

    const closeTask = registerHeaderBackAction('task', () => (target = 'task-list'), 100);
    headerBackAction.value?.();
    assert.equal(target, 'task-list');

    closeTask();
    headerBackAction.value?.();
    assert.equal(target, 'overview');

    closeProject();
    assert.equal(headerBackAction.value, null);
});

test('disposing an old registration cannot remove a newer action from the same owner', () => {
    clearHeaderBackActions();
    let target = '';

    const disposeOld = registerHeaderBackAction('detail', () => (target = 'old'), 10);
    const disposeNew = registerHeaderBackAction('detail', () => (target = 'new'), 10);
    disposeOld();
    headerBackAction.value?.();
    assert.equal(target, 'new');

    disposeNew();
    assert.equal(headerBackAction.value, null);
});
