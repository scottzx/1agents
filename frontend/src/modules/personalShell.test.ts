import assert from 'node:assert/strict';
import test from 'node:test';

import { PERSONAL_SHELL_ID, PERSONAL_SHELL_ENTRIES, isPersonalShell, builtinEntriesForShell } from './personalShell';

// The registration of the current default workbench as the Personal Shell
// (#328). These entries are the acceptance list "保留项目、任务、Chat、终端、
// 文件、Inbox、日程、Agent、Function、浏览器和右侧多 Tab" — the test pins them
// so a rename/removal is deliberate, not accidental.
const REQUIRED_ENTRY_IDS = [
    'projects',
    'tasks',
    'chat',
    'terminal',
    'files',
    'inbox',
    'schedule',
    'agents',
    'functions',
    'browser',
    'side-panel',
];

test('personal shell is registered under the stable built-in id', () => {
    assert.equal(PERSONAL_SHELL_ID, 'personal');
    assert.equal(isPersonalShell('personal'), true);
    assert.equal(isPersonalShell('presales'), false);
    assert.equal(isPersonalShell(''), false);
});

test('personal shell retains every built-in workbench entry', () => {
    const ids = PERSONAL_SHELL_ENTRIES.map(e => e.id);
    for (const required of REQUIRED_ENTRY_IDS) {
        assert.ok(ids.includes(required), `missing personal shell entry: ${required}`);
    }
    // Entry ids are unique and stable deep-link targets.
    assert.equal(new Set(ids).size, ids.length);
    // Every entry carries an i18n label key.
    for (const e of PERSONAL_SHELL_ENTRIES) {
        assert.ok(e.labelKey.length > 0, `entry ${e.id} has no labelKey`);
    }
});

test('only the personal shell contributes built-in entries', () => {
    assert.equal(builtinEntriesForShell('personal').length, PERSONAL_SHELL_ENTRIES.length);
    // Other shells compose from declarative app mount points, not built-ins.
    assert.deepEqual(builtinEntriesForShell('presales'), []);
    assert.deepEqual(builtinEntriesForShell('commerce'), []);
    assert.deepEqual(builtinEntriesForShell(''), []);
});
