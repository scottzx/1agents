import { h } from 'preact';

import type { Task } from './types';
import { fmtDateOnly } from './utils';

interface OverviewProps {
    tasks: Task[];
}

// The five buckets shown as stat cards; color maps to the semantic tokens.
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

export function Overview({ tasks }: OverviewProps) {
    const total = tasks.length;
    const completed = tasks.filter(t => t.status === 'completed').length;
    const pct = total === 0 ? 0 : Math.round((completed / total) * 100);

    // Group by milestone, preserving first-seen order.
    const order: string[] = [];
    const groups = new Map<string, { total: number; done: number }>();
    for (const t of tasks) {
        const m = t.milestone || '未分组';
        if (!groups.has(m)) {
            groups.set(m, { total: 0, done: 0 });
            order.push(m);
        }
        const g = groups.get(m)!;
        g.total += 1;
        if (t.status === 'completed') g.done += 1;
    }

    const deadlines = tasks
        .filter(t => t.plannedEnd && t.issueState !== 'closed' && t.status !== 'completed')
        .sort((a, b) => (a.plannedEnd || '').localeCompare(b.plannedEnd || ''))
        .slice(0, 5);

    // Donut geometry.
    const R = 52;
    const C = 2 * Math.PI * R;
    const dash = (pct / 100) * C;

    return (
        <div class="overview">
            <div class="overview-top bento-grid">
                <div class="bento-card overview-donut-card bento-span-2">
                    <svg class="overview-donut" viewBox="0 0 140 140" width="140" height="140">
                        <circle cx="70" cy="70" r={R} class="overview-donut-track" />
                        <circle
                            cx="70"
                            cy="70"
                            r={R}
                            class="overview-donut-value"
                            stroke-dasharray={`${dash} ${C - dash}`}
                            transform="rotate(-90 70 70)"
                        />
                        <text x="70" y="66" class="overview-donut-pct">{`${pct}%`}</text>
                        <text x="70" y="86" class="overview-donut-sub">{`${completed}/${total}`}</text>
                    </svg>
                    <div class="overview-donut-meta">
                        <div class="overview-donut-title">项目完成度</div>
                        <div class="overview-donut-desc">{`${total} 个任务 · ${completed} 个已完成`}</div>
                    </div>
                </div>
                {STATS.map(s => {
                    const n = tasks.filter(t => s.match(t.status)).length;
                    return (
                        <div key={s.key} class={`bento-card overview-stat stat-${s.cls}`}>
                            <div class="overview-stat-num">{n}</div>
                            <div class="overview-stat-label">{s.label}</div>
                        </div>
                    );
                })}
            </div>

            <div class="overview-section">
                <div class="overview-section-title">里程碑进度</div>
                <div class="overview-milestones">
                    {order.map(m => {
                        const g = groups.get(m)!;
                        const p = g.total === 0 ? 0 : Math.round((g.done / g.total) * 100);
                        return (
                            <div key={m} class="overview-ms-row">
                                <div class="overview-ms-name">{m}</div>
                                <div class="overview-ms-bar">
                                    <div class="overview-ms-fill" style={{ width: `${p}%` }} />
                                </div>
                                <div class="overview-ms-count">{`${g.done}/${g.total}`}</div>
                            </div>
                        );
                    })}
                </div>
            </div>

            <div class="overview-section">
                <div class="overview-section-title">临近截止</div>
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
    );
}
