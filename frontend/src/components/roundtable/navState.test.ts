import assert from 'node:assert/strict';
import test from 'node:test';
import {
    persistRoomView,
    readStoredRoomId,
    readStoredView,
    requestRoundtableListView,
    resolveInitialNav,
    subscribeRoundtableListView,
} from './navState';

class MemoryStorage implements Storage {
    private readonly values = new Map<string, string>();

    get length(): number {
        return this.values.size;
    }

    clear(): void {
        this.values.clear();
    }

    getItem(key: string): string | null {
        return this.values.get(key) ?? null;
    }

    key(index: number): string | null {
        return Array.from(this.values.keys())[index] ?? null;
    }

    removeItem(key: string): void {
        this.values.delete(key);
    }

    setItem(key: string, value: string): void {
        this.values.set(key, value);
    }
}

function resetStorage(): void {
    Object.defineProperty(globalThis, 'localStorage', {
        configurable: true,
        value: new MemoryStorage(),
    });
}

test('requesting the roundtable entry selects the list and notifies an already-mounted view', () => {
    resetStorage();
    persistRoomView('room-1');

    let requests = 0;
    const unsubscribe = subscribeRoundtableListView(() => {
        requests += 1;
    });

    requestRoundtableListView();

    assert.equal(readStoredView(), 'list');
    assert.equal(readStoredRoomId(), 'room-1');
    assert.equal(requests, 1);

    unsubscribe();
    requestRoundtableListView();
    assert.equal(requests, 1);
});

test('a normal app entry opens the list while an explicit room link still opens that room', () => {
    resetStorage();
    persistRoomView('room-1');

    requestRoundtableListView();

    assert.deepEqual(resolveInitialNav(), { view: 'list' });
    assert.deepEqual(resolveInitialNav('room-2'), { view: 'room', roomId: 'room-2' });
});
