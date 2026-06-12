import { PRIORITY_RANK } from './constants';
import type { Task, TaskRecurrence } from './types';

/** Order tasks for the table: top-level by priority then creation; each
 *  parent immediately followed by its (indented) subtasks. */
export function orderForTable(tasks: Task[]): Array<{ task: Task; isChild: boolean }> {
    const byParent = new Map<string, Task[]>();
    const tops: Task[] = [];
    for (const t of tasks) {
        if (t.parentId && tasks.some(p => p.id === t.parentId)) {
            const list = byParent.get(t.parentId) || [];
            list.push(t);
            byParent.set(t.parentId, list);
        } else {
            tops.push(t);
        }
    }
    const rank = (t: Task) => PRIORITY_RANK[t.priority || 'medium'] ?? 2;
    tops.sort((a, b) => rank(a) - rank(b) || a.createdAt.localeCompare(b.createdAt));
    const out: Array<{ task: Task; isChild: boolean }> = [];
    for (const t of tops) {
        out.push({ task: t, isChild: false });
        for (const c of byParent.get(t.id) || []) {
            out.push({ task: c, isChild: true });
        }
    }
    return out;
}

export function recurrenceLabel(r?: TaskRecurrence | null): string {
    if (!r) return '';
    const at = r.at ? ` ${r.at}` : '';
    if (r.freq === 'daily') return `每天${at}`;
    if (r.freq === 'weekly') return `每周${'日一二三四五六'[r.weekday ?? 0]}${at}`;
    return `每月${r.monthday ?? 1}号${at}`;
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
