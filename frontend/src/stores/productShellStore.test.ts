import assert from 'node:assert/strict';
import test from 'node:test';

// localStorage must exist before the store module evaluates (the active
// shell signal seeds itself from it at import time).
const values = new Map<string, string>();
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

const shells = [
    { id: 'personal', name: '个人工作台', version: '1.0.0', enabled: true },
    { id: 'presales', name: '售前与交付', version: '1.0.0', enabled: true },
    { id: 'commerce', name: '电商运营', version: '1.0.0', enabled: false },
];

test('productShellStore resolves the active shell like the backend', async () => {
    const store = await import('./productShellStore');

    // Persisted active shell still enabled → kept.
    assert.equal(store.resolveActiveShell('presales', shells, 'personal'), 'presales');
    // Persisted shell disabled → backend effective default wins.
    assert.equal(store.resolveActiveShell('commerce', shells, 'personal'), 'personal');
    // Effective default also disabled → first enabled shell.
    assert.equal(store.resolveActiveShell('commerce', shells, 'commerce'), 'personal');
    // Nothing enabled → '' (legacy mode, no filtering).
    assert.equal(
        store.resolveActiveShell(
            'x',
            shells.map(s => ({ ...s, enabled: false })),
            ''
        ),
        ''
    );
});

test('mount visibility keeps legacy mounts in every shell', async () => {
    const store = await import('./productShellStore');

    // Legacy mode (no active shell): everything renders as before.
    store.activeShellId.value = '';
    assert.equal(store.mountVisibleInActiveShell(undefined), true);
    assert.equal(store.mountVisibleInActiveShell(['presales']), true);

    // Active shell: legacy mounts (no shells list) stay visible everywhere;
    // targeted mounts only in their shells.
    store.activeShellId.value = 'personal';
    assert.equal(store.mountVisibleInActiveShell(undefined), true);
    assert.equal(store.mountVisibleInActiveShell([]), true);
    assert.equal(store.mountVisibleInActiveShell(['personal']), true);
    assert.equal(store.mountVisibleInActiveShell(['presales', 'commerce']), false);

    store.activeShellId.value = '';
});

test('compareMountPlacement sorts by slot then order (legacy keeps declaration order)', async () => {
    const store = await import('./productShellStore');

    // Legacy mounts: slot falls back to type; within one type, missing
    // order = 0 keeps declaration order.
    const legacy = [
        { type: 'l1-page', id: 'b' },
        { type: 'l1-page', id: 'a' },
        { type: 'l1-page', id: 'c' },
    ];
    assert.deepEqual(
        [...legacy].sort(store.compareMountPlacement).map(m => m.id),
        ['b', 'a', 'c']
    );

    // Explicit slot groups ahead of/behind by name; order sorts within.
    const mixed = [
        { type: 'l1-page', slot: 'nav', order: 20, id: 'nav-late' },
        { type: 'l1-page', id: 'legacy-2' },
        { type: 'l1-page', slot: 'nav', order: 10, id: 'nav-early' },
        { type: 'l1-page', id: 'legacy-1' },
    ];
    assert.deepEqual(
        [...mixed].sort(store.compareMountPlacement).map(m => m.id),
        ['legacy-2', 'legacy-1', 'nav-early', 'nav-late']
    );

    // Slot fallback mirrors the backend: declared slot wins over type.
    assert.equal(store.resolvedMountSlot({ type: 'l1-page', slot: 'home' }), 'home');
    assert.equal(store.resolvedMountSlot({ type: 'project-tab' }), 'project-tab');
});

test('shell deep-link hash parsing and building are inverse and url-safe', async () => {
    const store = await import('./productShellStore');

    assert.equal(store.parseShellHash('#shell=personal'), 'personal');
    assert.equal(store.parseShellHash('#shell=presales'), 'presales');
    // No fragment / unrelated fragment → ''.
    assert.equal(store.parseShellHash(''), '');
    assert.equal(store.parseShellHash('#/m/settings/general'), '');
    // Encoded ids round-trip.
    assert.equal(store.parseShellHash(store.buildShellHash('personal')), 'personal');
    assert.equal(store.buildShellHash(''), '');
    assert.equal(store.buildShellHash('commerce'), '#shell=commerce');
});

test('boot resolution honors a valid shell deep link over the persisted shell', async () => {
    const store = await import('./productShellStore');

    // A valid, enabled deep link wins over the persisted active shell.
    assert.equal(store.resolveBootShell('personal', 'presales', shells, 'personal'), 'presales');
    // A deep link to a disabled shell is ignored → falls back to persisted.
    assert.equal(store.resolveBootShell('personal', 'commerce', shells, 'personal'), 'personal');
    // An unknown deep link is ignored → backend effective default.
    assert.equal(store.resolveBootShell('', 'nope', shells, 'personal'), 'personal');
    // No deep link → same as resolveActiveShell.
    assert.equal(store.resolveBootShell('presales', '', shells, 'personal'), 'presales');
});
