import assert from 'node:assert/strict';
import test from 'node:test';

import { h } from 'preact';
import renderToString from 'preact-render-to-string';

import type { FeatureNode } from '@1agents/core/types/featureCatalog';
import type { Milestone } from './types';
import { FeatureNodeForm } from './FeatureNodeForm';

const root: FeatureNode = {
    id: 'root',
    kind: 'module',
    title: 'Root',
    position: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
};

const milestones: Milestone[] = [
    {
        id: 'version',
        name: '0.3.0',
        version: '0.3.0',
        position: 0,
        createdAt: '',
        updatedAt: '',
        total: 0,
        completed: 0,
    },
    {
        id: 'legacy',
        name: '旧阶段',
        isLegacy: true,
        position: 1,
        createdAt: '',
        updatedAt: '',
        total: 0,
        completed: 0,
    },
];

test('feature form offers only semantic target versions and explains non-silent updates', () => {
    const html = renderToString(
        h(FeatureNodeForm, {
            kind: 'feature',
            nodes: [root],
            milestones,
            initialParentId: root.id,
            busy: false,
            error: '',
            onCreate: async () => undefined,
        })
    );

    assert.match(html, /目标版本/);
    assert.match(html, /0\.3\.0/);
    assert.doesNotMatch(html, /旧阶段/);
    assert.match(html, /修改不会自动改写已有任务/);
});

test('module form shows derived coverage elsewhere instead of a target selector', () => {
    const html = renderToString(
        h(FeatureNodeForm, {
            kind: 'module',
            nodes: [root],
            milestones,
            busy: false,
            error: '',
            onCreate: async () => undefined,
        })
    );

    assert.doesNotMatch(html, /目标版本/);
    assert.doesNotMatch(html, /0\.3\.0/);
});
