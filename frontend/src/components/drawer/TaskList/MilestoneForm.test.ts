import assert from 'node:assert/strict';
import test from 'node:test';
import { h } from 'preact';
import renderToString from 'preact-render-to-string';

import type { Milestone } from './types';
import { MilestoneForm } from './MilestoneForm';

const base: Milestone = {
    id: 'm1',
    name: '0.1.0',
    version: '0.1.0',
    position: 0,
    predecessorId: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    total: 0,
    completed: 0,
};

const props = {
    milestones: [base],
    onSubmit: async () => undefined,
    onClose: () => undefined,
};

test('normal creation has no free-name or predecessor input and defaults to minor', () => {
    const html = renderToString(h(MilestoneForm, props));
    assert.equal(html.includes('milestone-form-input'), false);
    assert.equal(html.includes('前置里程碑'), false);
    assert.match(html, /value="minor" checked/);
    assert.match(html, /版本号和前序版本由服务端自动生成/);
});

test('version edit is immutable while legacy edit retains compatibility controls', () => {
    const versionHtml = renderToString(h(MilestoneForm, { ...props, initial: base }));
    assert.match(versionHtml, /版本号和前序版本由系统维护，不可修改/);
    assert.equal(versionHtml.includes('milestone-form-input'), false);
    assert.equal(versionHtml.includes('前置里程碑'), false);

    const legacy = { ...base, id: 'legacy', name: 'Workspace Inbox', version: undefined, isLegacy: true };
    const legacyHtml = renderToString(h(MilestoneForm, { ...props, initial: legacy }));
    assert.match(legacyHtml, /aria-label="历史里程碑名称"/);
    assert.match(legacyHtml, /前置里程碑/);
});
