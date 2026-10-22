import assert from 'node:assert/strict';
import test from 'node:test';

import { ensureLoaded, getHiddenTabs, getProjectConfigStatus, setTabHidden } from './projectTabPrefs';

test('project config exposes loading state while the request is in flight', async () => {
    const originalFetch = globalThis.fetch;
    let resolveLoad: ((response: Response) => void) | undefined;
    globalThis.fetch = async () => new Promise<Response>(resolve => { resolveLoad = resolve; });
    try {
        const loading = ensureLoaded('loading-project', '/tmp/loading-project');
        assert.equal(getProjectConfigStatus('loading-project').loading, true);
        assert.equal(getProjectConfigStatus('loading-project').loaded, false);
        resolveLoad?.(new Response('{}'));
        await loading;
        assert.equal(getProjectConfigStatus('loading-project').loading, false);
        assert.equal(getProjectConfigStatus('loading-project').loaded, true);
    } finally { globalThis.fetch = originalFetch; }
});

test('tab visibility persists only its own shallow-merge patch', async () => {
    const originalFetch = globalThis.fetch;
    const requests: RequestInit[] = [];
    let persisted: Record<string, unknown> = { otherSetting: 'keep' };
    globalThis.fetch = async (_input, init) => {
        if (!init?.method) return new Response(JSON.stringify(persisted));
        requests.push(init);
        persisted = { ...persisted, ...JSON.parse(String(init.body)) };
        return new Response('{"ok":true}');
    };
    try {
        await ensureLoaded('tabs-project', '/tmp/tabs-project');
        assert.equal(await setTabHidden('tabs-project', '/tmp/tabs-project', 'plan', true), true);
        assert.equal(getHiddenTabs('tabs-project').has('plan'), true);
        assert.deepEqual(JSON.parse(String(requests[0].body)), { hiddenTabs: ['plan'] });
        assert.equal(persisted.otherSetting, 'keep');
        await ensureLoaded('tabs-reopened-project', '/tmp/tabs-project');
        assert.equal(getHiddenTabs('tabs-reopened-project').has('plan'), true);
        assert.equal(await setTabHidden('tabs-project', '/tmp/tabs-project', 'plan', false), true);
        assert.deepEqual(JSON.parse(String(requests[1].body)), { hiddenTabs: [] });
    } finally { globalThis.fetch = originalFetch; }
});

test('failed tab save restores the optimistic visibility and exposes the error', async () => {
    const originalFetch = globalThis.fetch;
    let resolveSave: ((response: Response) => void) | undefined;
    globalThis.fetch = async (_input, init) => {
        if (!init?.method) return new Response('{}');
        return new Promise<Response>(resolve => { resolveSave = resolve; });
    };
    try {
        await ensureLoaded('rollback-project', '/tmp/rollback-project');
        const saving = setTabHidden('rollback-project', '/tmp/rollback-project', 'plan', true);
        assert.equal(getHiddenTabs('rollback-project').has('plan'), true);
        assert.equal(getProjectConfigStatus('rollback-project').saving, true);
        resolveSave?.(new Response('no', { status: 500 }));
        assert.equal(await saving, false);
        assert.equal(getHiddenTabs('rollback-project').has('plan'), false);
        assert.equal(getProjectConfigStatus('rollback-project').saving, false);
        assert.match(getProjectConfigStatus('rollback-project').saveError, /HTTP 500/);
    } finally { globalThis.fetch = originalFetch; }
});
