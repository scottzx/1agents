import type { GanttModule } from '@1agents/core/types/featureCatalog';

export type TimeGranularity = 'day' | 'week' | 'month';

export interface GanttRow {
    type: 'module' | 'task';
    id: string;
    title: string;
    path: string[];
    depth: number;
    start?: Date;
    end?: Date;
    progress: number;
    status?: string;
    milestone?: string;
    dependsOn?: string[];
    collapsed?: boolean;
    moduleId?: string;
}

export interface TimeAxis {
    start: Date;
    end: Date;
    totalDays: number;
    labels: { date: Date; label: string }[];
}

const DAY_MS = 86400000;

export function parseDate(s?: string): Date | undefined {
    if (!s) return undefined;
    const d = new Date(s);
    return isNaN(d.getTime()) ? undefined : d;
}

export function flattenGanttRows(modules: GanttModule[], collapsed: Set<string>): GanttRow[] {
    const rows: GanttRow[] = [];
    const visit = (mod: GanttModule) => {
        rows.push({
            type: 'module',
            id: mod.id,
            title: mod.title,
            path: mod.path,
            depth: mod.depth,
            start: parseDate(mod.aggStart),
            end: parseDate(mod.aggEnd),
            progress: mod.progress,
            collapsed: collapsed.has(mod.id),
        });
        if (collapsed.has(mod.id)) return;
        for (const task of mod.tasks) {
            rows.push({
                type: 'task',
                id: task.id,
                title: `#${task.number} ${task.title}`,
                path: mod.path,
                depth: mod.depth + 1,
                start: parseDate(task.plannedStart),
                end: parseDate(task.plannedEnd),
                progress: task.progress,
                status: task.status,
                milestone: task.milestone,
                dependsOn: task.dependsOn,
                moduleId: mod.id,
            });
        }
        for (const child of mod.children) visit(child);
    };
    for (const mod of modules) visit(mod);
    return rows;
}

export function computeTimeAxis(rows: GanttRow[], granularity: TimeGranularity): TimeAxis | null {
    let minDate: Date | undefined;
    let maxDate: Date | undefined;
    for (const row of rows) {
        if (row.start && (!minDate || row.start < minDate)) minDate = row.start;
        if (row.end && (!maxDate || row.end > maxDate)) maxDate = row.end;
    }
    if (!minDate || !maxDate) return null;
    // Add padding
    const padDays = granularity === 'month' ? 14 : granularity === 'week' ? 7 : 3;
    const start = new Date(minDate.getTime() - padDays * DAY_MS);
    const end = new Date(maxDate.getTime() + padDays * DAY_MS);
    const totalDays = Math.ceil((end.getTime() - start.getTime()) / DAY_MS);
    const labels = generateLabels(start, end, granularity);
    return { start, end, totalDays, labels };
}

function generateLabels(start: Date, end: Date, granularity: TimeGranularity): { date: Date; label: string }[] {
    const labels: { date: Date; label: string }[] = [];
    const cursor = new Date(start);
    while (cursor <= end) {
        if (granularity === 'day') {
            labels.push({ date: new Date(cursor), label: `${cursor.getMonth() + 1}/${cursor.getDate()}` });
            cursor.setDate(cursor.getDate() + 1);
        } else if (granularity === 'week') {
            labels.push({ date: new Date(cursor), label: `${cursor.getMonth() + 1}/${cursor.getDate()}` });
            cursor.setDate(cursor.getDate() + 7);
        } else {
            labels.push({
                date: new Date(cursor),
                label: `${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, '0')}`,
            });
            cursor.setMonth(cursor.getMonth() + 1);
        }
    }
    return labels;
}

export function dateToX(date: Date, axis: TimeAxis, chartWidth: number): number {
    const offset = (date.getTime() - axis.start.getTime()) / DAY_MS;
    return (offset / axis.totalDays) * chartWidth;
}
