import assert from 'node:assert/strict';
import test from 'node:test';

test('persists feature catalog collapse ids independently for each project', async () => {
    const values = new Map<string, string>([
        [
            '1agents-project-view-prefs',
            JSON.stringify({
                'project-a': {
                    featureCatalogCollapsed: ['module-a'],
                },
            }),
        ],
    ]);
    const originalStorage = globalThis.localStorage;
    Object.defineProperty(globalThis, 'localStorage', {
        configurable: true,
        value: {
            getItem: (key: string) => values.get(key) ?? null,
            setItem: (key: string, value: string) => values.set(key, value),
            removeItem: (key: string) => values.delete(key),
            clear: () => values.clear(),
            key: (index: number) => [...values.keys()][index] ?? null,
            get length() {
                return values.size;
            },
        } satisfies Storage,
    });

    try {
        const prefs = await import('./projectViewPrefs');
        assert.deepEqual(prefs.getPrefs('project-a').featureCatalogCollapsed, ['module-a']);
        assert.deepEqual(prefs.getPrefs('project-b').featureCatalogCollapsed, []);

        prefs.updatePrefs('project-b', { featureCatalogCollapsed: ['module-b'] });

        const stored = JSON.parse(values.get('1agents-project-view-prefs') ?? '{}');
        assert.deepEqual(stored['project-a'].featureCatalogCollapsed, ['module-a']);
        assert.deepEqual(stored['project-b'].featureCatalogCollapsed, ['module-b']);
    } finally {
        Object.defineProperty(globalThis, 'localStorage', {
            configurable: true,
            value: originalStorage,
        });
    }
});
