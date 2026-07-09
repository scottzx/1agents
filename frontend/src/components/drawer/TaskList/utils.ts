import { PRIORITY_RANK } from './constants';
import type { ProjectItem, TaskRecurrence } from './types';

/** Order tasks for the table: top-level by priority then creation; each
 *  parent immediately followed by its (indented) subtasks. */
export function orderForTable(tasks: ProjectItem[]): Array<{ task: ProjectItem; isChild: boolean }> {
    const byParent = new Map<string, ProjectItem[]>();
    const tops: ProjectItem[] = [];
    for (const t of tasks) {
        if (t.parentId && tasks.some(p => p.id === t.parentId)) {
            const list = byParent.get(t.parentId) || [];
            list.push(t);
            byParent.set(t.parentId, list);
        } else {
            tops.push(t);
        }
    }
    const rank = (t: ProjectItem) => PRIORITY_RANK[t.priority || 'medium'] ?? 2;
    tops.sort((a, b) => rank(a) - rank(b) || a.createdAt.localeCompare(b.createdAt));
    const out: Array<{ task: ProjectItem; isChild: boolean }> = [];
    for (const t of tops) {
        out.push({ task: t, isChild: false });
        for (const c of byParent.get(t.id) || []) {
            out.push({ task: c, isChild: true });
        }
    }
    return out;
}

const WEEKDAY_CN = '日一二三四五六';
const WEEK_INDEX_CN: Record<number, string> = { 1: '第一个', 2: '第二个', 3: '第三个', 4: '第四个', [-1]: '最后一个' };

export function recurrenceLabel(r?: TaskRecurrence | null): string {
    if (!r) return '';
    const at = r.at ? ` ${r.at}` : '';
    const n = r.interval && r.interval > 1 ? r.interval : 0;
    const days = r.daysOfWeek && r.daysOfWeek.length ? r.daysOfWeek : undefined;

    let core: string;
    if (r.freq === 'daily') {
        core = n ? `每${n}天` : '每天';
    } else if (r.freq === 'weekly') {
        const unit = n ? `每${n}周` : '每周';
        const list = (days ?? [r.weekday ?? 0]).map(d => WEEKDAY_CN[d]).join('、');
        core = `${unit}${list}`;
    } else if (r.freq === 'yearly') {
        const m = r.month ?? 1;
        if (r.weekIndex && days) {
            core = `每年${m}月${WEEK_INDEX_CN[r.weekIndex] ?? ''}周${WEEKDAY_CN[days[0]]}`;
        } else {
            core = `每年${m}月${r.monthday ?? 1}号`;
        }
    } else {
        // monthly
        const unit = n ? `每${n}月` : '每月';
        if (r.weekIndex && days) {
            core = `${unit}${WEEK_INDEX_CN[r.weekIndex] ?? ''}周${WEEKDAY_CN[days[0]]}`;
        } else {
            core = `${unit}${r.monthday ?? 1}号`;
        }
    }

    let suffix = '';
    if (r.count && r.count > 0) suffix = ` · 共${r.count}次`;
    else if (r.until) suffix = ` · 至${r.until.slice(0, 10)}`;
    return `${core}${at}${suffix}`;
}

export function fmtDate(iso?: string): string {
    if (!iso) return '—';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '—';
    return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(
        d.getMinutes()
    ).padStart(2, '0')}`;
}

export function fmtDateOnly(iso?: string): string {
    if (!iso) return '—';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '—';
    return `${d.getMonth() + 1}/${d.getDate()}`;
}
