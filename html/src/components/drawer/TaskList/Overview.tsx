import { h } from 'preact';
import { useMemo } from 'preact/hooks';
import type { EChartsOption } from 'echarts';

import { theme } from '../../../stores/uiStore';
import { EChart } from './EChart';
import { PRIORITY_LABELS } from './constants';
import type { Task } from './types';
import { fmtDateOnly } from './utils';

interface OverviewProps {
    tasks: Task[];
}

// Status buckets shown as KPI cards; color maps to the semantic tokens.
const STATS = [
    { key: 'todo', label: '待办', cls: 'todo', match: (s: Task['status']) => s === 'pending' || s === 'queued' },
    { key: 'running', label: '进行中', cls: 'running', match: (s: Task['status']) => s === 'running' },
    { key: 'completed', label: '已完成', cls: 'completed', match: (s: Task['status']) => s === 'completed' },
    { key: 'blocked', label: '阻塞', cls: 'blocked', match: (s: Task['status']) => s === 'blocked' },
    {
        key: 'failed',
        label: '失败/取消',
        cls: 'failed',
        match: (s: Task['status']) => s === 'failed' || s === 'cancelled',
    },
];

// ── KPI card helpers ────────────────────────────────────────────────────────
// 未完成 = 未开始(pending/queued) + 未完成(running/blocked); terminal states excluded.
const isTaskType = (t: Task) => (t.type ?? 'task') === 'task';
const isUncompleted = (t: Task) =>
    t.status === 'pending' || t.status === 'queued' || t.status === 'running' || t.status === 'blocked';

// Read a CSS custom property off :root — lets ECharts borrow the active
// theme's semantic tokens so charts re-color on light/dark switch.
function cssVar(name: string): string {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

const PRIORITY_ORDER: Array<Task['priority']> = ['urgent', 'high', 'medium', 'low'];

export function Overview({ tasks }: OverviewProps) {
    // Subscribe to theme so options recompute (with fresh token reads) on swap.
    const activeTheme = theme.value;

    const total = tasks.length;
    const completed = tasks.filter(t => t.status === 'completed').length;
    const pct = total === 0 ? 0 : Math.round((completed / total) * 100);

    const deadlines = tasks
        .filter(t => t.plannedEnd && t.issueState !== 'closed' && t.status !== 'completed')
        .sort((a, b) => (a.plannedEnd || '').localeCompare(b.plannedEnd || ''))
        .slice(0, 6);

    const options = useMemo(() => {
        const tokens = {
            text: cssVar('--text-main'),
            sub: cssVar('--text-secondary'),
            muted: cssVar('--text-muted'),
            border: cssVar('--border-color'),
            card: cssVar('--bg-card'),
            accent: cssVar('--accent-color'),
            success: cssVar('--success-fg'),
            warning: cssVar('--warning-fg'),
            danger: cssVar('--danger-fg'),
            orange: cssVar('--orange-fg'),
            purple: cssVar('--purple-fg'),
        };

        const tooltip = {
            backgroundColor: cssVar('--bg-tooltip'),
            borderWidth: 0,
            textStyle: { color: cssVar('--text-tooltip'), fontSize: 12 },
        };
        const axisLabel = { color: tokens.sub, fontSize: 11 };
        const splitLine = { lineStyle: { color: tokens.border, type: 'dashed' as const } };

        // ── Status distribution (doughnut) ──────────────────────────────
        const statusColors: Record<string, string> = {
            todo: tokens.muted,
            running: tokens.success,
            completed: tokens.accent,
            blocked: tokens.warning,
            failed: tokens.danger,
        };
        const statusData = STATS.map(s => ({
            name: s.label,
            value: tasks.filter(t => s.match(t.status)).length,
            itemStyle: { color: statusColors[s.key] },
        })).filter(d => d.value > 0);

        const statusOption: EChartsOption = {
            tooltip: { trigger: 'item', ...tooltip },
            legend: {
                bottom: 0,
                icon: 'circle',
                itemWidth: 9,
                itemHeight: 9,
                textStyle: { color: tokens.sub, fontSize: 11 },
            },
            title: {
                text: `${pct}%`,
                subtext: `${completed}/${total}`,
                left: 'center',
                top: '36%',
                textStyle: { color: tokens.text, fontSize: 26, fontWeight: 800 },
                subtextStyle: { color: tokens.muted, fontSize: 12 },
            },
            series: [
                {
                    type: 'pie',
                    radius: ['52%', '74%'],
                    center: ['50%', '46%'],
                    avoidLabelOverlap: false,
                    itemStyle: { borderColor: tokens.card, borderWidth: 2 },
                    label: { show: false },
                    emphasis: { scale: true, scaleSize: 4 },
                    data: statusData,
                },
            ],
        };

        // ── Priority distribution (bar) ─────────────────────────────────
        const priorityColors: Record<string, string> = {
            urgent: tokens.danger,
            high: tokens.orange,
            medium: tokens.accent,
            low: tokens.muted,
        };
        const priorityOption: EChartsOption = {
            tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, ...tooltip },
            grid: { left: 4, right: 16, top: 16, bottom: 4, containLabel: true },
            xAxis: {
                type: 'category',
                data: PRIORITY_ORDER.map(p => PRIORITY_LABELS[p!]),
                axisLabel,
                axisLine: { lineStyle: { color: tokens.border } },
                axisTick: { show: false },
            },
            yAxis: { type: 'value', minInterval: 1, axisLabel, splitLine },
            series: [
                {
                    type: 'bar',
                    barWidth: '46%',
                    itemStyle: { borderRadius: [4, 4, 0, 0] },
                    data: PRIORITY_ORDER.map(p => ({
                        value: tasks.filter(t => (t.priority || 'medium') === p).length,
                        itemStyle: { color: priorityColors[p!] },
                    })),
                },
            ],
        };

        // ── Milestone progress (stacked horizontal bar) ─────────────────
        const msOrder: string[] = [];
        const msGroups = new Map<string, { total: number; done: number }>();
        for (const t of tasks) {
            const m = t.milestone || '未分组';
            if (!msGroups.has(m)) {
                msGroups.set(m, { total: 0, done: 0 });
                msOrder.push(m);
            }
            const g = msGroups.get(m)!;
            g.total += 1;
            if (t.status === 'completed') g.done += 1;
        }
        const msTop = msOrder
            .map(m => ({ name: m, ...msGroups.get(m)! }))
            .sort((a, b) => b.total - a.total)
            .slice(0, 8)
            .reverse(); // ECharts y-category renders bottom→top
        const milestoneOption: EChartsOption = {
            tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, ...tooltip },
            legend: { top: 0, right: 0, textStyle: { color: tokens.sub, fontSize: 11 }, itemWidth: 12, itemHeight: 12 },
            grid: { left: 4, right: 12, top: 28, bottom: 4, containLabel: true },
            xAxis: { type: 'value', minInterval: 1, axisLabel, splitLine },
            yAxis: {
                type: 'category',
                data: msTop.map(m => m.name),
                axisLabel: { ...axisLabel, width: 96, overflow: 'truncate' },
                axisLine: { lineStyle: { color: tokens.border } },
                axisTick: { show: false },
            },
            series: [
                {
                    name: '已完成',
                    type: 'bar',
                    stack: 'ms',
                    itemStyle: { color: tokens.accent, borderRadius: [4, 0, 0, 4] },
                    data: msTop.map(m => m.done),
                },
                {
                    name: '未完成',
                    type: 'bar',
                    stack: 'ms',
                    itemStyle: { color: tokens.border, borderRadius: [0, 4, 4, 0] },
                    data: msTop.map(m => m.total - m.done),
                },
            ],
        };

        // ── Assignee load (stacked horizontal bar by status group) ──────
        const aOrder: string[] = [];
        const aGroups = new Map<string, { running: number; done: number; other: number; total: number }>();
        for (const t of tasks) {
            const a = t.assignee || '未分配';
            if (!aGroups.has(a)) {
                aGroups.set(a, { running: 0, done: 0, other: 0, total: 0 });
                aOrder.push(a);
            }
            const g = aGroups.get(a)!;
            g.total += 1;
            if (t.status === 'running') g.running += 1;
            else if (t.status === 'completed') g.done += 1;
            else g.other += 1;
        }
        const aTop = aOrder
            .map(a => ({ name: a, ...aGroups.get(a)! }))
            .sort((a, b) => b.total - a.total)
            .slice(0, 8)
            .reverse();
        const assigneeOption: EChartsOption = {
            tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, ...tooltip },
            legend: { top: 0, right: 0, textStyle: { color: tokens.sub, fontSize: 11 }, itemWidth: 12, itemHeight: 12 },
            grid: { left: 4, right: 12, top: 28, bottom: 4, containLabel: true },
            xAxis: { type: 'value', minInterval: 1, axisLabel, splitLine },
            yAxis: {
                type: 'category',
                data: aTop.map(a => a.name),
                axisLabel: { ...axisLabel, width: 96, overflow: 'truncate' },
                axisLine: { lineStyle: { color: tokens.border } },
                axisTick: { show: false },
            },
            series: [
                {
                    name: '进行中',
                    type: 'bar',
                    stack: 'a',
                    itemStyle: { color: tokens.success },
                    data: aTop.map(a => a.running),
                },
                {
                    name: '已完成',
                    type: 'bar',
                    stack: 'a',
                    itemStyle: { color: tokens.accent },
                    data: aTop.map(a => a.done),
                },
                {
                    name: '其他',
                    type: 'bar',
                    stack: 'a',
                    itemStyle: { color: tokens.muted },
                    data: aTop.map(a => a.other),
                },
            ],
        };

        // activeTheme is a dep so the token reads above re-run on theme switch.
        return { statusOption, priorityOption, milestoneOption, assigneeOption };
    }, [tasks, activeTheme]);

    const hasData = total > 0;

    // KPI cards: plain counts of four categories.
    const kpis = [
        {
            key: 'task',
            label: '任务',
            cls: 'completed',
            value: tasks.filter(t => isTaskType(t) && isUncompleted(t)).length, // 所有未完成的任务
        },
        { key: 'blocked', label: '阻塞', cls: 'blocked', value: tasks.filter(t => t.status === 'blocked').length },
        { key: 'req', label: '需求', cls: 'running', value: tasks.filter(t => t.type === 'requirement').length },
        { key: 'bug', label: '缺陷', cls: 'failed', value: tasks.filter(t => t.type === 'bug').length },
    ];

    return (
        <div class="overview">
            <div class="overview-kpis">
                {kpis.map(s => (
                    <div key={s.key} class={`bento-card overview-stat stat-${s.cls}`}>
                        <div class="overview-stat-num">{s.value}</div>
                        <div class="overview-stat-label">{s.label}</div>
                    </div>
                ))}
            </div>

            {!hasData ? (
                <div class="overview-empty-board">暂无任务数据</div>
            ) : (
                <div class="overview-widgets bento-grid">
                    <div class="bento-card overview-widget">
                        <div class="overview-widget-title">状态分布</div>
                        <EChart option={options.statusOption} height={260} />
                    </div>

                    <div class="bento-card overview-widget">
                        <div class="overview-widget-title">优先级分布</div>
                        <EChart option={options.priorityOption} height={260} />
                    </div>

                    <div class="bento-card overview-widget bento-span-2">
                        <div class="overview-widget-title">里程碑进度</div>
                        <EChart option={options.milestoneOption} height={260} />
                    </div>

                    <div class="bento-card overview-widget bento-span-2">
                        <div class="overview-widget-title">负责人负载</div>
                        <EChart option={options.assigneeOption} height={260} />
                    </div>

                    <div class="bento-card overview-widget bento-span-2">
                        <div class="overview-widget-title">临近截止</div>
                        {deadlines.length === 0 ? (
                            <div class="overview-empty">暂无设定计划截止的进行项</div>
                        ) : (
                            <div class="overview-deadlines">
                                {deadlines.map(t => (
                                    <div key={t.id} class="overview-dl-row">
                                        <span class="overview-dl-date">{fmtDateOnly(t.plannedEnd)}</span>
                                        <span class="overview-dl-title">{t.title}</span>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
