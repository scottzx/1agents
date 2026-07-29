import { h } from 'preact';
import renderToString from 'preact-render-to-string';
import test from 'node:test';
import assert from 'node:assert/strict';
import type { GanttData } from '@1agents/core/types/featureCatalog';
import { GanttUnscheduledTasks } from './GanttUnscheduledTasks';

test('renders the unscheduled task group independently of the time axis', () => {
    const tasks: GanttData['unscheduled'] = [
        {
            id: 'task-300',
            number: 300,
            title: '实现按功能模块分组的甘特图与蓝图导出',
            status: 'awaiting_human',
            progress: 0,
            dependsOn: [],
        },
    ];

    const html = renderToString(h(GanttUnscheduledTasks, { tasks, onTaskClick: () => undefined }));

    assert.match(html, /未排期任务 \(1\)/);
    assert.match(html, /#300 实现按功能模块分组的甘特图与蓝图导出/);
    assert.match(html, /awaiting_human/);
});
