import assert from 'node:assert/strict';
import test from 'node:test';

import type { GanttData, GanttModule, GanttTaskEntry } from '@1agents/core/types/featureCatalog';
import { filterGanttDataByMilestone } from './ganttModel';

function task(id: string, milestone: string, start: string, end: string, progress: number): GanttTaskEntry {
    return {
        id,
        number: Number(id.replace(/\D/g, '')),
        title: id,
        plannedStart: start,
        plannedEnd: end,
        status: 'pending',
        milestone,
        dependsOn: [],
        progress,
    };
}

function module(id: string, tasks: GanttTaskEntry[], children: GanttModule[] = []): GanttModule {
    return {
        id,
        title: id,
        path: [id],
        depth: 0,
        aggStart: '2026-01-01',
        aggEnd: '2026-04-01',
        progress: 99,
        children,
        tasks,
    };
}

test('filters milestone schedules while retaining ancestors and recalculating aggregates', () => {
    const current = task('task-1', 'Current release', '2026-02-01', '2026-02-10', 40);
    const later = task('task-2', 'Later release', '2026-03-01', '2026-03-20', 80);
    const childCurrent = task('task-3', 'Current release', '2026-01-28', '2026-02-05', 60);
    const data: GanttData = {
        modules: [module('root', [current, later], [module('child', [childCurrent])])],
        unscheduled: [
            { ...current, id: 'unscheduled-current', plannedStart: undefined, plannedEnd: undefined },
            { ...later, id: 'unscheduled-later', plannedStart: undefined, plannedEnd: undefined },
        ],
        milestones: [
            { id: 'm3', name: 'Current release', version: '0.3.0', targetDate: '2026-02-15' },
            { id: 'm4', name: 'Later release', version: '0.4.0', targetDate: '2026-03-30' },
        ],
    };
    const filtered = filterGanttDataByMilestone(data, 'm3');

    assert.deepEqual(
        filtered.modules[0].tasks.map(item => item.id),
        ['task-1']
    );
    assert.deepEqual(
        filtered.modules[0].children[0].tasks.map(item => item.id),
        ['task-3']
    );
    assert.equal(filtered.modules[0].aggStart, '2026-01-28');
    assert.equal(filtered.modules[0].aggEnd, '2026-02-10');
    assert.equal(filtered.modules[0].progress, 50);
    assert.deepEqual(
        filtered.unscheduled.map(item => item.id),
        ['unscheduled-current']
    );
    assert.deepEqual(
        filtered.milestones.map(item => item.id),
        ['m3']
    );
});
