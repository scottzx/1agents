import assert from 'node:assert/strict';
import test from 'node:test';

import { roundtableBreadcrumbs } from './breadcrumbs';

const noop = () => undefined;

test('roundtable breadcrumb levels match list, room, and session hierarchy', () => {
    assert.deepEqual(
        roundtableBreadcrumbs({ view: 'list' }).map(c => c.label),
        ['圆桌讨论']
    );
    assert.deepEqual(
        roundtableBreadcrumbs({ view: 'room', roomTitle: '新品讨论', onList: noop }).map(c => c.label),
        ['圆桌讨论', '新品讨论']
    );
    assert.deepEqual(
        roundtableBreadcrumbs({
            view: 'session',
            roomTitle: '新品讨论',
            sessionTitle: '新品讨论·裁判',
            onList: noop,
            onRoom: noop,
        }).map(c => c.label),
        ['圆桌讨论', '新品讨论', '新品讨论·裁判']
    );
});

test('roundtable path crumbs navigate to their named levels without a text back crumb', () => {
    let target = '';
    const crumbs = roundtableBreadcrumbs({
        view: 'session',
        roomTitle: '圆桌 A',
        sessionTitle: '研发席位',
        onList: () => (target = 'list'),
        onRoom: () => (target = 'room-crumb'),
    });

    crumbs[0].onClick?.();
    assert.equal(target, 'list');
    crumbs[1].onClick?.();
    assert.equal(target, 'room-crumb');
    assert.equal(crumbs[2].onClick, undefined);
    assert.equal(
        crumbs.some(crumb => crumb.label === '返回'),
        false
    );
});
