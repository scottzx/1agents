import assert from 'node:assert/strict';
import test from 'node:test';

test('persists task filters independently for each project', async () => {
    const values = new Map<string, string>([
        [
            '1agents-project-view-prefs',
            JSON.stringify({
                'project-a': {
                    search: 'task-a',
                    groupBy: 'feature',
                    grids: { tasks: { groupBy: 'feature', collapsed: ['old-module'] } },
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
        assert.equal(prefs.getPrefs('project-a').search, 'task-a');
        assert.equal(prefs.getPrefs('project-b').search, '');
        assert.equal(prefs.getGridPrefs('project-a', 'tasks').groupBy, 'none');

        prefs.updatePrefs('project-b', { search: 'task-b' });

        const stored = JSON.parse(values.get('1agents-project-view-prefs') ?? '{}');
        assert.equal(stored['project-a'].search, 'task-a');
        assert.equal(stored['project-b'].search, 'task-b');
    } finally {
        Object.defineProperty(globalThis, 'localStorage', {
            configurable: true,
            value: originalStorage,
        });
    }
});
