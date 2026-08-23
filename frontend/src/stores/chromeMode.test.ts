import assert from 'node:assert/strict';
import test from 'node:test';

import {
    CHAT_SURFACE_ID,
    CHROME_MODE_LABELS,
    CHROME_MODE_STORAGE_KEY,
    WORKBENCH_FALLBACK_ID,
    buildWorkbenchMenu,
    defaultChromeModeState,
    hydrateChromeModeState,
    parseChromeMode,
    readPersistedChromeMode,
    resolveActiveMenuId,
    restoreTarget,
    switchChromeMode,
    writePersistedChromeMode,
    type ChromeModeState,
    type StringStorage,
    type WorkbenchSurface,
} from './chromeMode';

function memoryStorage(seed?: Record<string, string>): StringStorage & { data: Map<string, string> } {
    const data = new Map<string, string>(Object.entries(seed ?? {}));
    return {
        data,
        getItem: (key: string) => (data.has(key) ? data.get(key)! : null),
        setItem: (key: string, value: string) => {
            data.set(key, value);
        },
    };
}

const workbenchA: WorkbenchSurface = {
    sidebarMode: 'project',
    stageView: 'project',
    activeDrawerTab: 'tasks',
    activeTab: 'terminal',
    activeSessionId: 'sess-a',
};

const workbenchB: WorkbenchSurface = {
    sidebarMode: 'assistant',
    stageView: 'conversation',
    activeDrawerTab: 'none',
    activeTab: 'new_chat',
    activeSessionId: 'sess-b',
};

test('default mode is 工作台 and dropdown labels are verbatim', () => {
    const state = defaultChromeModeState();
    assert.equal(state.mode, 'workbench');
    assert.equal(CHROME_MODE_LABELS.workbench, '工作台');
    assert.equal(CHROME_MODE_LABELS.chat, '聊天');
    assert.equal(parseChromeMode('chat'), 'chat');
    assert.equal(parseChromeMode('nope'), 'workbench');
    assert.equal(parseChromeMode(null), 'workbench');
});

test('one chrome menu lists product shells plus 聊天', () => {
    const items = buildWorkbenchMenu([
        { id: 'personal', name: '个人工作台' },
        { id: 'presales', name: '售前与交付' },
    ]);
    assert.deepEqual(
        items.map(i => `${i.kind}:${i.id}:${i.name}`),
        ['shell:personal:个人工作台', 'shell:presales:售前与交付', 'chat:chat:聊天']
    );
    assert.equal(resolveActiveMenuId('workbench', 'presales', items), 'presales');
    assert.equal(resolveActiveMenuId('chat', 'presales', items), CHAT_SURFACE_ID);
    const fallback = buildWorkbenchMenu([]);
    assert.equal(fallback[0].id, WORKBENCH_FALLBACK_ID);
    assert.equal(fallback[0].name, '工作台');
    assert.equal(fallback[1].id, CHAT_SURFACE_ID);
    assert.equal(fallback.length, 2);
});

test('switch 工作台→聊天 snapshots the workbench surface', () => {
    const start = defaultChromeModeState();
    const next = switchChromeMode(start, 'chat', { workbench: workbenchA });
    assert.equal(next.mode, 'chat');
    assert.deepEqual(next.lastWorkbench, workbenchA);
    assert.equal(next.lastChatRoomId, null);
});

test('switch 聊天→工作台 snapshots the last room and keeps the prior workbench', () => {
    const chatting: ChromeModeState = {
        mode: 'chat',
        lastWorkbench: workbenchA,
        lastChatRoomId: 'room-1',
    };
    const next = switchChromeMode(chatting, 'workbench', { chatRoomId: 'room-2' });
    assert.equal(next.mode, 'workbench');
    assert.deepEqual(next.lastWorkbench, workbenchA);
    assert.equal(next.lastChatRoomId, 'room-2');
});

test('switching back exposes the previously selected workbench or conversation identity', () => {
    let state = defaultChromeModeState();
    state = switchChromeMode(state, 'chat', { workbench: workbenchA });
    state = switchChromeMode(state, 'workbench', { chatRoomId: 'room-x' });
    const backToChat = switchChromeMode(state, 'chat', { workbench: workbenchB });
    assert.equal(backToChat.mode, 'chat');
    assert.equal(restoreTarget(backToChat).chatRoomId, 'room-x');

    const backToWorkbench = switchChromeMode(backToChat, 'workbench', { chatRoomId: 'room-x' });
    assert.equal(backToWorkbench.mode, 'workbench');
    assert.deepEqual(restoreTarget(backToWorkbench).workbench, workbenchB);
});

test('persisted mode is honored across write → read (reload/rehydrate)', () => {
    const storage = memoryStorage();
    let state = defaultChromeModeState();
    assert.equal(readPersistedChromeMode(storage).mode, 'workbench');

    state = switchChromeMode(state, 'chat', { workbench: workbenchA });
    writePersistedChromeMode(storage, state);
    assert.ok(storage.data.has(CHROME_MODE_STORAGE_KEY));

    const rehydrated = readPersistedChromeMode(storage);
    assert.equal(rehydrated.mode, 'chat');
    assert.deepEqual(rehydrated.lastWorkbench, workbenchA);

    const again = switchChromeMode(rehydrated, 'workbench', { chatRoomId: 'r-9' });
    writePersistedChromeMode(storage, again);
    const afterReload = readPersistedChromeMode(storage);
    assert.equal(afterReload.mode, 'workbench');
    assert.equal(afterReload.lastChatRoomId, 'r-9');
    assert.deepEqual(afterReload.lastWorkbench, workbenchA);
});

test('hydrate ignores corrupt payloads and keeps a usable default', () => {
    assert.equal(hydrateChromeModeState(null).mode, 'workbench');
    assert.equal(hydrateChromeModeState({ mode: 'chat', lastChatRoomId: 1 }).mode, 'chat');
    const storage = memoryStorage({ [CHROME_MODE_STORAGE_KEY]: '{not json' });
    assert.equal(readPersistedChromeMode(storage).mode, 'workbench');
});
