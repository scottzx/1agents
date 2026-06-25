import type { TaskRecurrence } from '../TaskList/types';

// CalendarItem is the normalized agenda entry returned by GET /api/agent/agenda
// (#192). It unifies three sources — personal reminders, scheduled/recurring
// project tasks, and milestone target dates — across all workspaces. The `kind`
// drives both rendering (icon/color) and click routing (reminder → light
// popover; task/milestone → deep link to the project board).
export type CalendarItemKind = 'reminder' | 'task' | 'milestone';

export interface CalendarItem {
    id: string;
    kind: CalendarItemKind;
    title: string;
    date: string; // RFC3339 anchor (day, plus time when hasTime)
    hasTime: boolean;
    status?: string;
    recurrence?: TaskRecurrence | null;
    workspaceId: string;
    workspaceName?: string;
    number?: number;
    milestone?: string;
    taskType?: string;
}
