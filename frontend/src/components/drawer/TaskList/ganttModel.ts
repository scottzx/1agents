import type { GanttData, GanttModule, GanttTaskEntry } from '@1agents/core/types/featureCatalog';

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

function aggregateFilteredModule(source: GanttModule, tasks: GanttTaskEntry[], children: GanttModule[]): GanttModule {
    const descendantTasks = [...tasks];
    const collectTasks = (modules: GanttModule[]) => {
        for (const module of modules) {
            descendantTasks.push(...module.tasks);
            collectTasks(module.children);
        }
    };
    collectTasks(children);

    const starts = descendantTasks.map(task => task.plannedStart).filter((value): value is string => !!value);
    const ends = descendantTasks.map(task => task.plannedEnd).filter((value): value is string => !!value);
    const progress =
        descendantTasks.length === 0
            ? 0
            : Math.round(descendantTasks.reduce((total, task) => total + task.progress, 0) / descendantTasks.length);

    return {
        ...source,
        tasks,
        children,
        aggStart: starts.length > 0 ? starts.sort()[0] : undefined,
        aggEnd: ends.length > 0 ? ends.sort().at(-1) : undefined,
        progress,
    };
}

/** Scope the derived schedule to one milestone while retaining module
 * hierarchy. Dates and progress are re-aggregated from the filtered tasks, so
 * the selected-version view never displays totals from other releases. */
export function filterGanttDataByMilestone(data: GanttData, milestoneId?: string): GanttData {
    if (!milestoneId) return data;
    const selectedMilestone = data.milestones.find(milestone => milestone.id === milestoneId);
    if (!selectedMilestone) return data;

    const visit = (module: GanttModule): GanttModule | null => {
        const tasks = module.tasks.filter(task => task.milestone === selectedMilestone.name);
        const children = module.children.map(visit).filter((child): child is GanttModule => !!child);
        if (tasks.length === 0 && children.length === 0) return null;
        return aggregateFilteredModule(module, tasks, children);
    };

    return {
        modules: data.modules.map(visit).filter((module): module is GanttModule => !!module),
        unscheduled: data.unscheduled.filter(task => task.milestone === selectedMilestone.name),
        milestones: [selectedMilestone],
    };
}

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
