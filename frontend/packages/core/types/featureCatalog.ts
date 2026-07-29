/**
 * Feature Catalog shared wire contracts.
 *
 * Level, path, progress, and aggregated milestone data are derived by the
 * backend and intentionally do not belong to the persisted node contract.
 */

export type FeatureNodeKind = 'module' | 'feature';

export type FeatureProgressStatus = 'unplanned' | 'pending' | 'in_progress' | 'delivered' | 'replan';

export interface FeatureProgress {
    status: FeatureProgressStatus;
    /** Null means there is no non-cancelled delivery-task denominator. */
    progressPercent: number | null;
    completedTasks: number;
    totalTasks: number;
    coveredFeatures: number;
    totalFeatures: number;
    unplannedFeatures: number;
    replanFeatures: number;
}

export interface FeatureNode {
    id: string;
    parentId?: string;
    kind: FeatureNodeKind;
    title: string;
    description?: string;
    targetMilestoneId?: string;
    position: number;
    createdAt: string;
    updatedAt: string;
    progress?: FeatureProgress;
    /** Derived target-version distribution for descendant feature points. */
    versionCoverage?: FeatureVersionCoverage[];
}

export interface FeatureVersionCoverage {
    milestoneId: string;
    version: string;
    featureCount: number;
}

export type FeatureItemRelation = 'source' | 'delivery';

export interface FeatureItemLink {
    featureId: string;
    itemId: string;
    relation: FeatureItemRelation;
    createdAt: string;
}

export interface FeatureCatalog {
    nodes: FeatureNode[];
    links: FeatureItemLink[];
}

export interface CreateFeatureNodeInput {
    parentId?: string;
    kind: FeatureNodeKind;
    title: string;
    description?: string;
    targetMilestoneId?: string;
    position?: number;
}

export interface UpdateFeatureNodeInput {
    parentId?: string;
    title?: string;
    description?: string;
    targetMilestoneId?: string;
    position?: number;
}

export interface FeatureMilestoneTaskDiff {
    id: string;
    number?: number;
    title: string;
    currentMilestone?: string;
}

export interface FeatureMilestoneSyncPreview {
    featureId: string;
    targetMilestoneId?: string;
    targetMilestone?: string;
    targetVersion?: string;
    tasks: FeatureMilestoneTaskDiff[];
}

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
    version: string;
    targetDate?: string;
}

export interface GanttData {
    modules: GanttModule[];
    unscheduled: GanttTaskEntry[];
    milestones: GanttMilestone[];
}
