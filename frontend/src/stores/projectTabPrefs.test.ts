import assert from 'node:assert/strict';
import test from 'node:test';

import {
    ensureLoaded,
    getFeatureCatalogEnabled,
    getProjectConfigStatus,
    setFeatureCatalogEnabled,
} from './projectTabPrefs';

test('missing featureCatalogEnabled defaults to disabled after loading legacy config', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(JSON.stringify({ hiddenTabs: ['plan'] }));
    try {
        await ensureLoaded('legacy-project', '/tmp/legacy-project');
        assert.equal(getFeatureCatalogEnabled('legacy-project'), false);
        assert.deepEqual(getProjectConfigStatus('legacy-project'), {
            loading: false,
            loaded: true,
            loadError: '',
            saving: false,
            saveError: '',
        });
    } finally {
        globalThis.fetch = originalFetch;
    }
});

test('project config exposes loading state while the request is in flight', async () => {
    const originalFetch = globalThis.fetch;
    let resolveLoad: ((response: Response) => void) | undefined;
    globalThis.fetch = async () =>
        new Promise<Response>(resolve => {
            resolveLoad = resolve;
        });
    try {
        const loading = ensureLoaded('loading-project', '/tmp/loading-project');
        assert.equal(getProjectConfigStatus('loading-project').loading, true);
        assert.equal(getProjectConfigStatus('loading-project').loaded, false);

        resolveLoad?.(new Response('{}'));
        await loading;
        assert.equal(getProjectConfigStatus('loading-project').loading, false);
        assert.equal(getProjectConfigStatus('loading-project').loaded, true);
    } finally {
        globalThis.fetch = originalFetch;
    }
});

test('feature catalog setting persists as its own shallow-merge patch', async () => {
    const originalFetch = globalThis.fetch;
    const requests: Array<{ input: string; init: RequestInit }> = [];
    let persisted: Record<string, unknown> = {};
    globalThis.fetch = async (input, init) => {
        if (!init?.method) return new Response(JSON.stringify(persisted));
        requests.push({ input: String(input), init });
        persisted = { ...persisted, ...JSON.parse(String(init.body)) };
        return new Response('{"ok":true}');
    };
    try {
        await ensureLoaded('enabled-project', '/tmp/enabled-project');
        const saved = await setFeatureCatalogEnabled('enabled-project', '/tmp/enabled-project', true);
        assert.equal(saved, true);
        assert.equal(getFeatureCatalogEnabled('enabled-project'), true);
        assert.equal(requests.length, 1);
        assert.match(requests[0].input, /^\/api\/project\/local-config\?/);
        assert.deepEqual(JSON.parse(String(requests[0].init.body)), { featureCatalogEnabled: true });

        // A fresh store slot reads the persisted project-local value, matching a
        // page refresh without relying on this process's optimistic cache.
        await ensureLoaded('reopened-project', '/tmp/enabled-project');
        assert.equal(getFeatureCatalogEnabled('reopened-project'), true);

        await setFeatureCatalogEnabled('enabled-project', '/tmp/enabled-project', false);
        assert.deepEqual(JSON.parse(String(requests[1].init.body)), { featureCatalogEnabled: false });
        assert.equal(
            requests.every(request => request.input.startsWith('/api/project/local-config?')),
            true
        );
    } finally {
        globalThis.fetch = originalFetch;
    }
});

test('failed feature catalog save rolls back the optimistic value and exposes the error', async () => {
    const originalFetch = globalThis.fetch;
    let resolveSave: ((response: Response) => void) | undefined;
    globalThis.fetch = async (_input, init) => {
        if (!init?.method) return new Response('{}');
        return new Promise<Response>(resolve => {
            resolveSave = resolve;
        });
    };
    try {
        await ensureLoaded('rollback-project', '/tmp/rollback-project');
        const saving = setFeatureCatalogEnabled('rollback-project', '/tmp/rollback-project', true);
        assert.equal(getFeatureCatalogEnabled('rollback-project'), true);
        assert.equal(getProjectConfigStatus('rollback-project').saving, true);

        resolveSave?.(new Response('no', { status: 500 }));
        assert.equal(await saving, false);
        assert.equal(getFeatureCatalogEnabled('rollback-project'), false);
        assert.equal(getProjectConfigStatus('rollback-project').saving, false);
        assert.match(getProjectConfigStatus('rollback-project').saveError, /HTTP 500/);
    } finally {
        globalThis.fetch = originalFetch;
    }
});
