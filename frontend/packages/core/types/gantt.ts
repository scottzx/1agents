// ── Gantt chart read models (derived, never persisted) ──

export interface GanttTaskEntry {
    id: string;
    number: number;
    title: string;
    plannedStart?: string;
    plannedEnd?: string;
    status: string;
    milestone?: string;
    dependsOn: string[];
    progress: number;
}

export interface GanttModule {
    id: string;
    title: string;
    path: string[];
    depth: number;
    aggStart?: string;
    aggEnd?: string;
    progress: number;
    children: GanttModule[];
    tasks: GanttTaskEntry[];
}

export interface GanttMilestone {
    id: string;
    name: string;
    version: string;
    targetDate?: string;
}

export interface GanttData {
    modules: GanttModule[];
    unscheduled: GanttTaskEntry[];
    milestones: GanttMilestone[];
}
