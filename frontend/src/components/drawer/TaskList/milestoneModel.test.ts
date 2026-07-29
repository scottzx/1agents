import assert from 'node:assert/strict';
import test from 'node:test';

import type { Milestone } from './types';
import { parseSemVer, splitMilestones } from './milestoneModel';

function milestone(id: string, version: string, position: number): Milestone {
    return {
        id,
        name: version,
        version,
        position,
        predecessorId: '',
        createdAt: '2026-01-01T00:00:00Z',
        updatedAt: '2026-01-01T00:00:00Z',
        total: 0,
        completed: 0,
    };
}

test('sorts valid versions numerically newest-first instead of lexically or by position', () => {
    const input = [milestone('ten', '0.10.0', 99), milestone('one', '1.0.0', 20), milestone('nine', '0.9.0', 0)];
    const { versions } = splitMilestones(input);
    assert.deepEqual(
        versions.map(item => item.version),
        ['1.0.0', '0.10.0', '0.9.0']
    );
});

test('keeps free-form and invalid historical values outside the SemVer tree', () => {
    const named = { ...milestone('named', '', 2), name: 'Workspace Inbox', version: undefined, isLegacy: true };
    const invalid = { ...milestone('invalid', 'v1.0.0', 1), name: 'v1.0.0' };
    const { versions, legacy } = splitMilestones([named, milestone('valid', '0.1.0', 3), invalid]);

    assert.deepEqual(
        versions.map(item => item.id),
        ['valid']
    );
    assert.deepEqual(
        legacy.map(item => item.id),
        ['invalid', 'named']
    );
    assert.equal(parseSemVer('01.0.0'), null);
    assert.deepEqual(parseSemVer('10.20.30'), { major: 10, minor: 20, patch: 30 });
});
