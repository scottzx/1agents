import assert from 'node:assert/strict';
import test from 'node:test';

import { backendTarget, setActiveDevice } from './apiClient';
import { featureCatalogService } from './featureCatalogService';

test('feature catalog history service maps all five version endpoints and restore request id', async () => {
    const originalFetch = globalThis.fetch;
    const requests: Array<{ url: string; method: string; body?: Record<string, unknown> }> = [];
    const responses = [
        { items: [], hasMore: false },
        { id: 'v1', alias: '', kind: 'manual' },
        { id: 'v1', alias: 'renamed', kind: 'manual' },
        { ok: true },
        {
            requestId: 'restore-request',
            targetVersion: { id: 'v1' },
            safetyVersion: { id: 's1' },
            restoredNodeCount: 2,
            restoredLinkCount: 1,
            skippedLinkCount: 0,
            clearedTargetMilestoneCount: 0,
            warnings: [],
            warningsTruncated: false,
        },
    ];
    backendTarget.value = { mode: 'direct' };
    setActiveDevice(null);
    globalThis.fetch = async (input, init) => {
        requests.push({
            url: String(input),
            method: init?.method ?? 'GET',
            body: init?.body ? JSON.parse(String(init.body)) : undefined,
        });
        return new Response(JSON.stringify(responses.shift()), {
            status: 200,
            headers: { 'content-type': 'application/json' },
        });
    };
    try {
        await featureCatalogService.listVersions('project id', 'next/cursor');
        await featureCatalogService.createVersion('project id', 'alias');
        await featureCatalogService.renameVersion('project id', 'v1', 'renamed');
        await featureCatalogService.deleteVersion('project id', 'v1');
        const restored = await featureCatalogService.restoreVersion('project id', 'v1', 'restore-request');

        assert.deepEqual(
            requests.map(request => [request.method, request.url]),
            [
                ['GET', '/api/agent/feature-catalog/versions?workspace_id=project%20id&cursor=next%2Fcursor'],
                ['POST', '/api/agent/feature-catalog/versions'],
                ['PATCH', '/api/agent/feature-catalog/versions/v1'],
                ['DELETE', '/api/agent/feature-catalog/versions/v1?workspace_id=project%20id'],
                ['POST', '/api/agent/feature-catalog/versions/v1/restore'],
            ]
        );
        assert.deepEqual(requests[1].body, { workspace_id: 'project id', alias: 'alias' });
        assert.deepEqual(requests[2].body, { workspace_id: 'project id', alias: 'renamed' });
        assert.deepEqual(requests[4].body, {
            workspace_id: 'project id',
            requestId: 'restore-request',
        });
        assert.equal(restored.safetyVersion.id, 's1');
    } finally {
        globalThis.fetch = originalFetch;
    }
});

test('feature catalog item move uses one atomic batch request', async () => {
    const originalFetch = globalThis.fetch;
    let request: { url: string; method: string; body: Record<string, unknown> } | undefined;
    backendTarget.value = { mode: 'direct' };
    setActiveDevice(null);
    globalThis.fetch = async (input, init) => {
        request = {
            url: String(input),
            method: init?.method ?? 'GET',
            body: JSON.parse(String(init?.body)),
        };
        return new Response(JSON.stringify({ results: [] }), {
            status: 200,
            headers: { 'content-type': 'application/json' },
        });
    };
    try {
        await featureCatalogService.moveItem('project-1', 'feature-a', 'feature-b', 'task-1', 'delivery');
        assert.deepEqual(request, {
            url: '/api/agent/feature-catalog/batch',
            method: 'POST',
            body: {
                workspace_id: 'project-1',
                operations: [
                    {
                        op: 'link',
                        featureId: 'feature-b',
                        itemId: 'task-1',
                        relation: 'delivery',
                    },
                    {
                        op: 'unlink',
                        featureId: 'feature-a',
                        itemId: 'task-1',
                        relation: 'delivery',
                    },
                ],
            },
        });
    } finally {
        globalThis.fetch = originalFetch;
    }
});
