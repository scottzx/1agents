/**
 * WorkCase / CaseRef / DomainRef read models for the Personal Shell
 * cross-shell work aggregation (task #329, design §4.2/§4.3/§8).
 *
 * These mirror the backend `internal/agent` PersonalAggregate JSON shapes. The
 * frontend never reads domain tables — domain object state arrives as a
 * read-only SubjectSummary resolved through the owning domain's Query provider,
 * or as a restricted placeholder when that summary is not visible.
 */

/** Task status values the kernel can return (see meta.TaskStatus). */
export type AggTaskStatus =
    | 'pending'
    | 'queued'
    | 'running'
    | 'completed'
    | 'failed'
    | 'cancelled'
    | 'blocked'
    | 'not_ready'
    | 'pending_review'
    | 'awaiting_human';

/** Attention buckets an aggregated item can belong to. */
export type AggBucket = 'running' | 'awaiting' | 'failed' | 'blocked' | 'due_soon' | 'open';

/** Bucket filter values accepted by the aggregate endpoint. */
export type AggBucketFilter = AggBucket | 'all';

/** Sort fields accepted by the aggregate endpoint. */
export type AggSortField = '' | 'updated' | 'created' | 'due' | 'priority' | 'title' | 'status';

/**
 * Read-only summary of a domain object (DomainRef), or a restricted
 * placeholder when the owning domain will not reveal it.
 */
export interface SubjectSummary {
    ref: string;
    namespace: string;
    type: string;
    id: string;
    /** False when this is a restricted placeholder. */
    available: boolean;
    title?: string;
    status?: string;
    link?: string;
    /** Present when !available: unknown_provider / permission_denied / not_found / version_unsupported / invalid_ref / error. */
    restrictedReason?: string;
}

/** The latest execution/verification run driving an item's classification. */
export interface AggRunSummary {
    id: string;
    kind: 'execution' | 'verification';
    status: 'running' | 'completed' | 'failed' | 'cancelled';
    attempt: number;
    needsHuman?: boolean;
    errorText?: string;
    startedAt: string;
}

/** Navigation coordinates back to the owning shell / case / subject / task. */
export interface AggDeepLink {
    /** Shell the item itself opens in. */
    shell: string;
    taskWorkspaceId?: string;
    taskId?: string;
    taskNumber?: number;
    /** CaseRef string ("case:<ws>:<caseId>") of the bound WorkCase, if any. */
    caseRef?: string;
    /** Subject DomainRef string + the shell owning the subject's domain, if any. */
    subjectRef?: string;
    subjectShell?: string;
}

/** One aggregated cross-shell work item. */
export interface AggregateWorkItem {
    id: string;
    number?: number;
    title: string;
    status: AggTaskStatus;
    priority?: 'urgent' | 'high' | 'medium' | 'low';
    type?: 'task' | 'requirement' | 'bug' | 'discussion';
    executor?: 'agent' | 'function' | 'human';
    assignee?: string;
    workspaceId: string;
    workspaceName?: string;
    buckets: AggBucket[];
    dueAt?: string;
    updatedAt: string;
    createdAt: string;
    caseRef?: string;
    caseTitle?: string;
    caseStatus?: string;
    subject?: SubjectSummary;
    run?: AggRunSummary;
    deepLink: AggDeepLink;
}

/** Paginated Personal Shell aggregate response. */
export interface PersonalAggregateResponse {
    shell: string;
    actor: string;
    generatedAt: string;
    total: number;
    limit: number;
    offset: number;
    hasMore: boolean;
    counts: Record<string, number>;
    items: AggregateWorkItem[];
}

/** Query parameters for the aggregate endpoint. */
export interface PersonalAggregateQuery {
    bucket?: AggBucketFilter;
    workspace?: string;
    case?: string;
    status?: string;
    sort?: AggSortField;
    dir?: 'asc' | 'desc';
    limit?: number;
    offset?: number;
    actor?: string;
}
